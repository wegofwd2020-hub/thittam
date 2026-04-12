package document

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	// assumedBandwidthBytesPerSec is a conservative 10 Mbps baseline used to
	// estimate file transfer time when sizing presigned URL windows.
	assumedBandwidthBytesPerSec int64 = 1_250_000

	urlWindowBuffer = 5 * time.Minute  // headroom added on top of transfer time
	urlWindowMin    = 15 * time.Minute // floor — never issue a URL shorter than this
	urlWindowMax    = 4 * time.Hour    // cap — never issue a URL longer than this

	// largeURLWindowThreshold is the TTL above which a security warn log is emitted
	// so that operators can review unusually large file transfers.
	largeURLWindowThreshold = time.Hour
)

// Logger defines structured logging for the document service.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}

type noopLogger struct{}

func (noopLogger) Info(_ string, _ ...interface{}) {}
func (noopLogger) Warn(_ string, _ ...interface{}) {}

// ObjectStore is the interface for S3-compatible object storage.
// Implementations wrap MinIO (self-hosted) or AWS S3 (cloud).
// Provider credentials come exclusively from environment variables.
type ObjectStore interface {
	// PresignedPutURL returns a URL the client uses to PUT file bytes directly.
	PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// PresignedGetURL returns a short-lived URL for downloading an object.
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// Stat returns the size in bytes of an existing object.
	// Used to confirm a completed upload without reading the file.
	Stat(ctx context.Context, key string) (int64, error)

	// Delete removes an object permanently from storage.
	Delete(ctx context.Context, key string) error
}

// EventPublisher publishes NATS domain events from the document service.
type EventPublisher interface {
	PublishDocumentUploaded(ctx context.Context, tenantID, documentID uuid.UUID) error
	PublishDocumentDeleted(ctx context.Context, tenantID, documentID uuid.UUID) error
}

// Service implements document business logic.
type Service struct {
	repo      Repository
	store     ObjectStore
	publisher EventPublisher
	logger    Logger
}

// NewService creates a document service.
func NewService(repo Repository, store ObjectStore, publisher EventPublisher) *Service {
	return &Service{
		repo:      repo,
		store:     store,
		publisher: publisher,
		logger:    noopLogger{},
	}
}

// WithLogger attaches a structured logger to the service.
// If not set, log output is silently discarded.
func (s *Service) WithLogger(l Logger) *Service {
	s.logger = l
	return s
}

// presignedURLWindow returns the presigned URL validity window for a file of
// sizeBytes. Formula: max(urlWindowMin, ceil(sizeBytes/bandwidth) + urlWindowBuffer),
// capped at urlWindowMax. Zero or negative sizes return urlWindowMin.
func presignedURLWindow(sizeBytes int64) time.Duration {
	if sizeBytes <= 0 {
		return urlWindowMin
	}
	// Integer ceiling division: (a + b - 1) / b
	transferSecs := (sizeBytes + assumedBandwidthBytesPerSec - 1) / assumedBandwidthBytesPerSec
	window := time.Duration(transferSecs)*time.Second + urlWindowBuffer
	if window < urlWindowMin {
		window = urlWindowMin
	}
	if window > urlWindowMax {
		window = urlWindowMax
	}
	return window
}

// --- Upload lifecycle ---

// InitiateUpload reserves a document record and returns a presigned PUT URL
// for direct client-to-storage upload. The document is in a pending state
// (SizeBytes = 0) until ConfirmUpload is called.
func (s *Service) InitiateUpload(ctx context.Context, req *InitiateUploadRequest) (*UploadURL, error) {
	docID := uuid.New()
	key := storageKey(req.TenantID, docID, req.ProductionID, 1, req.Name)

	ttl := presignedURLWindow(req.SizeHintBytes)
	if ttl > largeURLWindowThreshold {
		s.logger.Warn("large presigned upload URL window issued",
			"tenant_id", req.TenantID.String(),
			"document_id", docID.String(),
			"size_hint_bytes", req.SizeHintBytes,
			"ttl", ttl.String(),
		)
	}

	url, err := s.store.PresignedPutURL(ctx, key, ttl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPresignFailed, err)
	}

	doc := &Document{
		ID:             docID,
		TenantID:       req.TenantID,
		ProductionID:   req.ProductionID,
		FolderID:       req.FolderID,
		Name:           req.Name,
		MimeType:       req.MimeType,
		SizeBytes:      0, // confirmed on ConfirmUpload
		StorageKey:     key,
		CurrentVersion: 1,
		UploadedBy:     req.UploadedBy,
	}
	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("document: initiate upload — create record: %w", err)
	}

	return &UploadURL{
		DocumentID: docID,
		URL:        url,
		ExpiresAt:  time.Now().UTC().Add(ttl),
	}, nil
}

// ConfirmUpload is called after the client has successfully PUT the file.
// It stats the object to read the real size, creates the first version record,
// and publishes the uploaded event.
func (s *Service) ConfirmUpload(ctx context.Context, tenantID, documentID uuid.UUID) (*Document, error) {
	doc, err := s.repo.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, fmt.Errorf("document: confirm upload — get record: %w", err)
	}
	if doc.DeletedAt != nil {
		return nil, ErrDocumentDeleted
	}

	sizeBytes, err := s.store.Stat(ctx, doc.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("document: confirm upload — stat object: %w", err)
	}

	doc.SizeBytes = sizeBytes
	if err := s.repo.UpdateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("document: confirm upload — update size: %w", err)
	}

	version := &DocumentVersion{
		ID:         uuid.New(),
		DocumentID: documentID,
		Version:    1,
		StorageKey: doc.StorageKey,
		SizeBytes:  sizeBytes,
		UploadedBy: doc.UploadedBy,
	}
	if err := s.repo.CreateVersion(ctx, version); err != nil {
		return nil, fmt.Errorf("document: confirm upload — create version: %w", err)
	}

	// Best-effort event publish — do not fail the confirm on event errors.
	_ = s.publisher.PublishDocumentUploaded(ctx, tenantID, documentID)

	return doc, nil
}

// --- Document management ---

// GetDocument retrieves a document by ID. Returns ErrDocumentDeleted for
// soft-deleted documents so callers can distinguish "never existed" from
// "was deleted".
func (s *Service) GetDocument(ctx context.Context, tenantID, id uuid.UUID) (*Document, error) {
	doc, err := s.repo.GetDocument(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("document: get %s: %w", id, err)
	}
	if doc.DeletedAt != nil {
		return nil, ErrDocumentDeleted
	}
	return doc, nil
}

// ListDocuments returns non-deleted documents for a tenant with optional
// production and folder filters.
func (s *Service) ListDocuments(ctx context.Context, tenantID uuid.UUID, productionID, folderID *uuid.UUID, limit, offset int) ([]Document, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	docs, err := s.repo.ListDocuments(ctx, tenantID, productionID, folderID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("document: list: %w", err)
	}
	return docs, nil
}

// DeleteDocument soft-deletes a document. The object is retained in storage
// for 30 days before a background job permanently purges it.
func (s *Service) DeleteDocument(ctx context.Context, tenantID, id uuid.UUID) error {
	doc, err := s.repo.GetDocument(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("document: delete — get record: %w", err)
	}
	if doc.DeletedAt != nil {
		return nil // idempotent
	}

	now := time.Now().UTC()
	if err := s.repo.SoftDeleteDocument(ctx, tenantID, id, now); err != nil {
		return fmt.Errorf("document: soft delete %s: %w", id, err)
	}

	_ = s.publisher.PublishDocumentDeleted(ctx, tenantID, id)
	return nil
}

// GetDownloadURL returns a presigned GET URL for the current version. The URL
// validity window scales with file size (min 15 min, max 4 h).
func (s *Service) GetDownloadURL(ctx context.Context, tenantID, id uuid.UUID) (*DownloadURL, error) {
	doc, err := s.GetDocument(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if doc.SizeBytes == 0 {
		return nil, ErrUploadNotConfirmed
	}

	ttl := presignedURLWindow(doc.SizeBytes)
	if ttl > largeURLWindowThreshold {
		s.logger.Warn("large presigned download URL window issued",
			"tenant_id", tenantID.String(),
			"document_id", id.String(),
			"size_bytes", doc.SizeBytes,
			"ttl", ttl.String(),
		)
	}

	url, err := s.store.PresignedGetURL(ctx, doc.StorageKey, ttl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPresignFailed, err)
	}

	return &DownloadURL{
		URL:       url,
		ExpiresAt: time.Now().UTC().Add(ttl),
	}, nil
}

// MoveDocument moves a document to a different folder.
func (s *Service) MoveDocument(ctx context.Context, tenantID, documentID, folderID uuid.UUID) (*Document, error) {
	doc, err := s.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, err
	}
	// Verify the target folder belongs to the same tenant.
	if _, err := s.repo.GetFolder(ctx, tenantID, folderID); err != nil {
		return nil, fmt.Errorf("document: move — %w", ErrFolderNotFound)
	}
	doc.FolderID = &folderID
	if err := s.repo.UpdateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("document: move %s to folder %s: %w", documentID, folderID, err)
	}
	return doc, nil
}

// --- Versioning ---

// CreateVersion initiates a new version upload. Returns a presigned PUT URL
// for the new version; the client calls ConfirmVersion after uploading.
func (s *Service) CreateVersion(ctx context.Context, tenantID, documentID, uploadedBy uuid.UUID) (*UploadURL, error) {
	doc, err := s.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, err
	}
	if doc.SizeBytes == 0 {
		return nil, ErrUploadNotConfirmed
	}

	nextVersion := doc.CurrentVersion + 1
	key := storageKey(doc.TenantID, doc.ID, doc.ProductionID, nextVersion, doc.Name)

	// Use the current version's size as a hint — new versions are typically similar.
	ttl := presignedURLWindow(doc.SizeBytes)
	if ttl > largeURLWindowThreshold {
		s.logger.Warn("large presigned upload URL window issued",
			"tenant_id", tenantID.String(),
			"document_id", documentID.String(),
			"size_bytes", doc.SizeBytes,
			"ttl", ttl.String(),
		)
	}

	url, err := s.store.PresignedPutURL(ctx, key, ttl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPresignFailed, err)
	}

	return &UploadURL{
		DocumentID: documentID,
		URL:        url,
		ExpiresAt:  time.Now().UTC().Add(ttl),
	}, nil
}

// ConfirmVersion is called after the client uploads a new version.
// It stats the object, records the new version, and updates the document's
// current version pointer.
func (s *Service) ConfirmVersion(ctx context.Context, tenantID, documentID uuid.UUID, version int, uploadedBy uuid.UUID) (*DocumentVersion, error) {
	doc, err := s.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, err
	}

	expectedVersion := doc.CurrentVersion + 1
	if version != expectedVersion {
		return nil, fmt.Errorf("document: confirm version — expected %d got %d", expectedVersion, version)
	}

	key := storageKey(doc.TenantID, doc.ID, doc.ProductionID, version, doc.Name)
	sizeBytes, err := s.store.Stat(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("document: confirm version — stat object: %w", err)
	}

	dv := &DocumentVersion{
		ID:         uuid.New(),
		DocumentID: documentID,
		Version:    version,
		StorageKey: key,
		SizeBytes:  sizeBytes,
		UploadedBy: uploadedBy,
	}
	if err := s.repo.CreateVersion(ctx, dv); err != nil {
		return nil, fmt.Errorf("document: create version record: %w", err)
	}

	doc.CurrentVersion = version
	doc.StorageKey = key
	doc.SizeBytes = sizeBytes
	if err := s.repo.UpdateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("document: update current version pointer: %w", err)
	}

	return dv, nil
}

// ListVersions returns all versions for a document in ascending order.
func (s *Service) ListVersions(ctx context.Context, tenantID, documentID uuid.UUID) ([]DocumentVersion, error) {
	// Verify document exists and belongs to tenant.
	if _, err := s.GetDocument(ctx, tenantID, documentID); err != nil {
		return nil, err
	}
	versions, err := s.repo.ListVersions(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("document: list versions: %w", err)
	}
	return versions, nil
}

// RestoreVersion makes a previous version the current one by updating the
// document's storage key and current version pointer. No storage objects are
// copied or deleted — all versions are preserved.
func (s *Service) RestoreVersion(ctx context.Context, tenantID, documentID uuid.UUID, version int) (*Document, error) {
	doc, err := s.GetDocument(ctx, tenantID, documentID)
	if err != nil {
		return nil, err
	}
	if version == doc.CurrentVersion {
		return doc, nil // already current — idempotent
	}

	dv, err := s.repo.GetVersion(ctx, documentID, version)
	if err != nil {
		return nil, fmt.Errorf("document: restore — %w", ErrVersionNotFound)
	}

	doc.CurrentVersion = dv.Version
	doc.StorageKey = dv.StorageKey
	doc.SizeBytes = dv.SizeBytes
	if err := s.repo.UpdateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("document: restore version %d: %w", version, err)
	}
	return doc, nil
}

// --- Folders ---

// CreateFolder creates a new folder for organising documents.
func (s *Service) CreateFolder(ctx context.Context, folder *Folder) (*Folder, error) {
	if folder.ID == uuid.Nil {
		folder.ID = uuid.New()
	}
	if err := s.repo.CreateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("document: create folder: %w", err)
	}
	return folder, nil
}

// ListFolders returns all folders for a tenant with an optional production filter.
func (s *Service) ListFolders(ctx context.Context, tenantID uuid.UUID, productionID *uuid.UUID) ([]Folder, error) {
	folders, err := s.repo.ListFolders(ctx, tenantID, productionID)
	if err != nil {
		return nil, fmt.Errorf("document: list folders: %w", err)
	}
	return folders, nil
}

// --- Helpers ---

// storageKey builds the canonical object key for a document version.
// Pattern: {tenantID}/{productionID|shared}/{documentID}/v{version}/{filename}
func storageKey(tenantID, documentID uuid.UUID, productionID *uuid.UUID, version int, filename string) string {
	prodSegment := "shared"
	if productionID != nil {
		prodSegment = productionID.String()
	}
	return fmt.Sprintf("%s/%s/%s/v%d/%s",
		tenantID.String(),
		prodSegment,
		documentID.String(),
		version,
		filename,
	)
}
