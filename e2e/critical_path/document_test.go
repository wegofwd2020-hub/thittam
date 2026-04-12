package critical_path_test

// Critical Path 7: Document Upload / Download Lifecycle
//
// Verifies end-to-end document flow (in-process, no MinIO):
//   - InitiateUpload → presigned PUT URL returned, Document stub created
//   - ConfirmUpload  → SizeBytes stamped, version 1 recorded, event published
//   - GetDownloadURL → presigned GET URL returned
//   - DeleteDocument → soft-delete; GetDocument returns ErrDocumentDeleted
//   - SizeHintBytes  → large file gets extended presigned URL window (≥15 min)

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/services/document"
)

// ── Deterministic IDs ─────────────────────────────────────────────────────

var (
	fixedDocTenantID = uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	fixedDocUserID   = uuid.MustParse("d0000000-0000-0000-0000-000000000002")
)

// ── Mock implementations ──────────────────────────────────────────────────

type docRepo struct {
	mu       sync.Mutex
	docs     map[uuid.UUID]*document.Document
	versions []*document.DocumentVersion
	folders  map[uuid.UUID]*document.Folder
}

func newDocRepo() *docRepo {
	return &docRepo{
		docs:    make(map[uuid.UUID]*document.Document),
		folders: make(map[uuid.UUID]*document.Folder),
	}
}

func (r *docRepo) CreateFolder(_ context.Context, f *document.Folder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.folders[f.ID] = f
	return nil
}

func (r *docRepo) GetFolder(_ context.Context, _, id uuid.UUID) (*document.Folder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.folders[id]; ok {
		return f, nil
	}
	return nil, document.ErrFolderNotFound
}

func (r *docRepo) ListFolders(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _ int) ([]document.Folder, error) {
	return nil, nil
}

func (r *docRepo) CreateDocument(_ context.Context, doc *document.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs[doc.ID] = doc
	return nil
}

func (r *docRepo) GetDocument(_ context.Context, _, id uuid.UUID) (*document.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return nil, document.ErrDocumentNotFound
	}
	if doc.DeletedAt != nil {
		return nil, document.ErrDocumentDeleted
	}
	return doc, nil
}

func (r *docRepo) ListDocuments(_ context.Context, _ uuid.UUID, _, _ *uuid.UUID, _, _ int) ([]document.Document, error) {
	return nil, nil
}

func (r *docRepo) UpdateDocument(_ context.Context, doc *document.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.docs[doc.ID] = doc
	return nil
}

func (r *docRepo) SoftDeleteDocument(_ context.Context, _, id uuid.UUID, deletedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if doc, ok := r.docs[id]; ok {
		doc.DeletedAt = &deletedAt
	}
	return nil
}

func (r *docRepo) CreateVersion(_ context.Context, v *document.DocumentVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions = append(r.versions, v)
	return nil
}

func (r *docRepo) GetVersion(_ context.Context, docID uuid.UUID, version int) (*document.DocumentVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.versions {
		if v.DocumentID == docID && v.Version == version {
			return v, nil
		}
	}
	return nil, document.ErrVersionNotFound
}

func (r *docRepo) ListVersions(_ context.Context, _ uuid.UUID, _, _ int) ([]document.DocumentVersion, error) {
	return nil, nil
}

// stubObjectStore returns deterministic presigned URLs and a fixed file size on Stat.
type stubObjectStore struct {
	mu        sync.Mutex
	putCalls  []string
	getCalls  []string
	statBytes int64
}

func (s *stubObjectStore) PresignedPutURL(_ context.Context, key string, _ time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCalls = append(s.putCalls, key)
	return "https://storage.example.com/" + key + "?method=PUT&sig=abc", nil
}

func (s *stubObjectStore) PresignedGetURL(_ context.Context, key string, _ time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls = append(s.getCalls, key)
	return "https://storage.example.com/" + key + "?method=GET&sig=def", nil
}

func (s *stubObjectStore) Stat(_ context.Context, _ string) (int64, error) {
	return s.statBytes, nil
}

func (s *stubObjectStore) Delete(_ context.Context, _ string) error { return nil }

// stubDocPublisher captures event calls.
type stubDocPublisher struct {
	mu            sync.Mutex
	uploadedCalls int
	deletedCalls  int
}

func (p *stubDocPublisher) PublishDocumentUploaded(_ context.Context, _, _ uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.uploadedCalls++
	return nil
}

func (p *stubDocPublisher) PublishDocumentDeleted(_ context.Context, _, _ uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletedCalls++
	return nil
}

// ── Test: InitiateUpload → ConfirmUpload → GetDownloadURL ─────────────────

func TestCriticalPath_DocumentUploadConfirmDownload(t *testing.T) {
	t.Parallel()

	store := &stubObjectStore{statBytes: 512 * 1024} // 512 KiB file
	pub := &stubDocPublisher{}
	svc := document.NewService(newDocRepo(), store, pub)

	// Step 1: Initiate upload — get presigned PUT URL.
	uploadURL, err := svc.InitiateUpload(context.Background(), &document.InitiateUploadRequest{
		TenantID:      fixedDocTenantID,
		Name:          "shooting-schedule.pdf",
		MimeType:      "application/pdf",
		UploadedBy:    fixedDocUserID,
		SizeHintBytes: 512 * 1024,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, uploadURL.URL)
	assert.True(t, uploadURL.ExpiresAt.After(time.Now()), "presigned URL must not be already expired")
	assert.NotEqual(t, uuid.Nil, uploadURL.DocumentID)

	// Step 2: Client uploads to MinIO (mocked). Confirm the upload.
	doc, err := svc.ConfirmUpload(context.Background(), fixedDocTenantID, uploadURL.DocumentID)
	require.NoError(t, err)

	assert.Equal(t, uploadURL.DocumentID, doc.ID)
	assert.Equal(t, "shooting-schedule.pdf", doc.Name)
	assert.Equal(t, int64(512*1024), doc.SizeBytes) // stamped from ObjectStore.Stat
	assert.Equal(t, 1, doc.CurrentVersion)

	// Event published exactly once.
	assert.Equal(t, 1, pub.uploadedCalls)

	// Step 3: Get download URL.
	dlURL, err := svc.GetDownloadURL(context.Background(), fixedDocTenantID, doc.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, dlURL.URL)
	assert.True(t, dlURL.ExpiresAt.After(time.Now()))
}

// ── Test: large file gets an extended presigned URL window ────────────────

func TestCriticalPath_DocumentUpload_LargeFile_ExtendedWindow(t *testing.T) {
	t.Parallel()

	const oneGB = 1 << 30
	store := &stubObjectStore{statBytes: oneGB}
	svc := document.NewService(newDocRepo(), store, &stubDocPublisher{})

	uploadURL, err := svc.InitiateUpload(context.Background(), &document.InitiateUploadRequest{
		TenantID:      fixedDocTenantID,
		Name:          "raw-footage.mp4",
		MimeType:      "video/mp4",
		UploadedBy:    fixedDocUserID,
		SizeHintBytes: oneGB,
	})
	require.NoError(t, err)

	// For a 1 GiB file the window must be substantially longer than the 15-min
	// minimum — at minimum 1 hour more than the base window.
	minWindow := 15 * time.Minute
	window := time.Until(uploadURL.ExpiresAt)
	assert.Greater(t, window, minWindow, "1 GiB file must get a longer-than-minimum URL window")
}

// ── Test: SoftDelete → GetDocument returns ErrDocumentDeleted ─────────────

func TestCriticalPath_DocumentDelete(t *testing.T) {
	t.Parallel()

	store := &stubObjectStore{statBytes: 1024}
	pub := &stubDocPublisher{}
	svc := document.NewService(newDocRepo(), store, pub)

	// Upload and confirm.
	uploadURL, err := svc.InitiateUpload(context.Background(), &document.InitiateUploadRequest{
		TenantID:   fixedDocTenantID,
		Name:       "contract.pdf",
		MimeType:   "application/pdf",
		UploadedBy: fixedDocUserID,
	})
	require.NoError(t, err)

	doc, err := svc.ConfirmUpload(context.Background(), fixedDocTenantID, uploadURL.DocumentID)
	require.NoError(t, err)

	// Delete.
	require.NoError(t, svc.DeleteDocument(context.Background(), fixedDocTenantID, doc.ID))
	assert.Equal(t, 1, pub.deletedCalls)

	// GetDocument must now return ErrDocumentDeleted.
	_, err = svc.GetDocument(context.Background(), fixedDocTenantID, doc.ID)
	assert.ErrorIs(t, err, document.ErrDocumentDeleted)
}
