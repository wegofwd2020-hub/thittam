package document

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fixed test IDs ---

var (
	fixedTenantID = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixedProdID   = uuid.MustParse("b2000000-0000-0000-0000-000000000002")
	fixedDocID    = uuid.MustParse("c3000000-0000-0000-0000-000000000003")
	fixedFolderID = uuid.MustParse("d4000000-0000-0000-0000-000000000004")
	fixedUserID   = uuid.MustParse("e5000000-0000-0000-0000-000000000005")
)

// --- Mock Repository ---

type mockRepo struct {
	createFolderFn      func(ctx context.Context, folder *Folder) error
	getFolderFn         func(ctx context.Context, tenantID, id uuid.UUID) (*Folder, error)
	listFoldersFn       func(ctx context.Context, tenantID uuid.UUID, productionID *uuid.UUID) ([]Folder, error)
	createDocumentFn    func(ctx context.Context, doc *Document) error
	getDocumentFn       func(ctx context.Context, tenantID, id uuid.UUID) (*Document, error)
	listDocumentsFn     func(ctx context.Context, tenantID uuid.UUID, productionID, folderID *uuid.UUID, limit, offset int) ([]Document, error)
	updateDocumentFn    func(ctx context.Context, doc *Document) error
	softDeleteFn        func(ctx context.Context, tenantID, id uuid.UUID, deletedAt time.Time) error
	createVersionFn     func(ctx context.Context, v *DocumentVersion) error
	getVersionFn        func(ctx context.Context, documentID uuid.UUID, version int) (*DocumentVersion, error)
	listVersionsFn      func(ctx context.Context, documentID uuid.UUID) ([]DocumentVersion, error)
}

func (m *mockRepo) CreateFolder(ctx context.Context, f *Folder) error {
	if m.createFolderFn != nil { return m.createFolderFn(ctx, f) }
	return nil
}
func (m *mockRepo) GetFolder(ctx context.Context, tenantID, id uuid.UUID) (*Folder, error) {
	if m.getFolderFn != nil { return m.getFolderFn(ctx, tenantID, id) }
	return &Folder{ID: id, TenantID: tenantID}, nil
}
func (m *mockRepo) ListFolders(ctx context.Context, tenantID uuid.UUID, productionID *uuid.UUID) ([]Folder, error) {
	if m.listFoldersFn != nil { return m.listFoldersFn(ctx, tenantID, productionID) }
	return nil, nil
}
func (m *mockRepo) CreateDocument(ctx context.Context, doc *Document) error {
	if m.createDocumentFn != nil { return m.createDocumentFn(ctx, doc) }
	return nil
}
func (m *mockRepo) GetDocument(ctx context.Context, tenantID, id uuid.UUID) (*Document, error) {
	if m.getDocumentFn != nil { return m.getDocumentFn(ctx, tenantID, id) }
	return &Document{
		ID: id, TenantID: tenantID,
		Name: "receipt.pdf", MimeType: "application/pdf",
		StorageKey: "key/v1/receipt.pdf", SizeBytes: 1024,
		CurrentVersion: 1, UploadedBy: fixedUserID,
	}, nil
}
func (m *mockRepo) ListDocuments(ctx context.Context, tenantID uuid.UUID, productionID, folderID *uuid.UUID, limit, offset int) ([]Document, error) {
	if m.listDocumentsFn != nil { return m.listDocumentsFn(ctx, tenantID, productionID, folderID, limit, offset) }
	return nil, nil
}
func (m *mockRepo) UpdateDocument(ctx context.Context, doc *Document) error {
	if m.updateDocumentFn != nil { return m.updateDocumentFn(ctx, doc) }
	return nil
}
func (m *mockRepo) SoftDeleteDocument(ctx context.Context, tenantID, id uuid.UUID, deletedAt time.Time) error {
	if m.softDeleteFn != nil { return m.softDeleteFn(ctx, tenantID, id, deletedAt) }
	return nil
}
func (m *mockRepo) CreateVersion(ctx context.Context, v *DocumentVersion) error {
	if m.createVersionFn != nil { return m.createVersionFn(ctx, v) }
	return nil
}
func (m *mockRepo) GetVersion(ctx context.Context, documentID uuid.UUID, version int) (*DocumentVersion, error) {
	if m.getVersionFn != nil { return m.getVersionFn(ctx, documentID, version) }
	return &DocumentVersion{ID: uuid.New(), DocumentID: documentID, Version: version, SizeBytes: 512, StorageKey: "key/v1/file.pdf"}, nil
}
func (m *mockRepo) ListVersions(ctx context.Context, documentID uuid.UUID) ([]DocumentVersion, error) {
	if m.listVersionsFn != nil { return m.listVersionsFn(ctx, documentID) }
	return nil, nil
}

// --- Mock ObjectStore ---

type mockStore struct {
	presignPutFn func(ctx context.Context, key string, ttl time.Duration) (string, error)
	presignGetFn func(ctx context.Context, key string, ttl time.Duration) (string, error)
	statFn       func(ctx context.Context, key string) (int64, error)
	deleteFn     func(ctx context.Context, key string) error
}

func (m *mockStore) PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if m.presignPutFn != nil { return m.presignPutFn(ctx, key, ttl) }
	return "https://storage.example.com/put/" + key, nil
}
func (m *mockStore) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if m.presignGetFn != nil { return m.presignGetFn(ctx, key, ttl) }
	return "https://storage.example.com/get/" + key, nil
}
func (m *mockStore) Stat(ctx context.Context, key string) (int64, error) {
	if m.statFn != nil { return m.statFn(ctx, key) }
	return 2048, nil
}
func (m *mockStore) Delete(ctx context.Context, key string) error {
	if m.deleteFn != nil { return m.deleteFn(ctx, key) }
	return nil
}

// --- Mock EventPublisher ---

type mockPublisher struct {
	uploadedFn func(ctx context.Context, tenantID, documentID uuid.UUID) error
	deletedFn  func(ctx context.Context, tenantID, documentID uuid.UUID) error
}

func (m *mockPublisher) PublishDocumentUploaded(ctx context.Context, t, d uuid.UUID) error {
	if m.uploadedFn != nil { return m.uploadedFn(ctx, t, d) }
	return nil
}
func (m *mockPublisher) PublishDocumentDeleted(ctx context.Context, t, d uuid.UUID) error {
	if m.deletedFn != nil { return m.deletedFn(ctx, t, d) }
	return nil
}

func newTestService(repo Repository) *Service {
	return NewService(repo, &mockStore{}, &mockPublisher{})
}

// --- Tests: storageKey helper ---

func TestStorageKey_WithProduction(t *testing.T) {
	t.Parallel()
	prodID := fixedProdID
	key := storageKey(fixedTenantID, fixedDocID, &prodID, 1, "receipt.pdf")
	expected := fixedTenantID.String() + "/" + fixedProdID.String() + "/" + fixedDocID.String() + "/v1/receipt.pdf"
	assert.Equal(t, expected, key)
}

func TestStorageKey_WithoutProduction(t *testing.T) {
	t.Parallel()
	key := storageKey(fixedTenantID, fixedDocID, nil, 2, "contract.pdf")
	expected := fixedTenantID.String() + "/shared/" + fixedDocID.String() + "/v2/contract.pdf"
	assert.Equal(t, expected, key)
}

// --- Tests: InitiateUpload ---

func TestInitiateUpload_CreatesRecordAndReturnsURL(t *testing.T) {
	t.Parallel()
	var savedDoc *Document
	svc := NewService(&mockRepo{
		createDocumentFn: func(_ context.Context, doc *Document) error {
			savedDoc = doc
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	result, err := svc.InitiateUpload(context.Background(), &InitiateUploadRequest{
		TenantID:   fixedTenantID,
		Name:       "script.pdf",
		MimeType:   "application/pdf",
		UploadedBy: fixedUserID,
	})

	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.DocumentID)
	assert.Contains(t, result.URL, "put/")
	assert.True(t, result.ExpiresAt.After(time.Now()))
	assert.Equal(t, int64(0), savedDoc.SizeBytes)
	assert.Equal(t, 1, savedDoc.CurrentVersion)
}

func TestInitiateUpload_StorageKeyContainsTenantAndDocID(t *testing.T) {
	t.Parallel()
	var capturedKey string
	svc := NewService(&mockRepo{}, &mockStore{
		presignPutFn: func(_ context.Context, key string, _ time.Duration) (string, error) {
			capturedKey = key
			return "https://put/" + key, nil
		},
	}, &mockPublisher{})

	result, err := svc.InitiateUpload(context.Background(), &InitiateUploadRequest{
		TenantID:   fixedTenantID,
		Name:       "budget.xlsx",
		MimeType:   "application/vnd.ms-excel",
		UploadedBy: fixedUserID,
	})
	require.NoError(t, err)
	assert.Contains(t, capturedKey, fixedTenantID.String())
	assert.Contains(t, capturedKey, result.DocumentID.String())
	assert.Contains(t, capturedKey, "v1/budget.xlsx")
}

// --- Tests: ConfirmUpload ---

func TestConfirmUpload_StatsSizeAndCreatesVersion(t *testing.T) {
	t.Parallel()
	var createdVersion *DocumentVersion
	var updatedDoc *Document

	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{
				ID: id, TenantID: fixedTenantID,
				StorageKey: "key/v1/file.pdf", SizeBytes: 0,
				CurrentVersion: 1, UploadedBy: fixedUserID,
			}, nil
		},
		updateDocumentFn: func(_ context.Context, doc *Document) error {
			updatedDoc = doc
			return nil
		},
		createVersionFn: func(_ context.Context, v *DocumentVersion) error {
			createdVersion = v
			return nil
		},
	}, &mockStore{
		statFn: func(_ context.Context, _ string) (int64, error) {
			return 98304, nil // 96 KB
		},
	}, &mockPublisher{})

	doc, err := svc.ConfirmUpload(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.Equal(t, int64(98304), doc.SizeBytes)
	assert.Equal(t, int64(98304), updatedDoc.SizeBytes)
	require.NotNil(t, createdVersion)
	assert.Equal(t, 1, createdVersion.Version)
	assert.Equal(t, int64(98304), createdVersion.SizeBytes)
}

func TestConfirmUpload_PublishesEvent(t *testing.T) {
	t.Parallel()
	published := false
	svc := NewService(&mockRepo{}, &mockStore{}, &mockPublisher{
		uploadedFn: func(_ context.Context, _, _ uuid.UUID) error {
			published = true
			return nil
		},
	})

	_, err := svc.ConfirmUpload(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.True(t, published)
}

func TestConfirmUpload_DeletedDocument(t *testing.T) {
	t.Parallel()
	now := time.Now()
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, DeletedAt: &now}, nil
		},
	}, &mockStore{}, &mockPublisher{})

	_, err := svc.ConfirmUpload(context.Background(), fixedTenantID, fixedDocID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDocumentDeleted)
}

// --- Tests: DeleteDocument ---

func TestDeleteDocument_SoftDeletes(t *testing.T) {
	t.Parallel()
	var capturedDeletedAt time.Time
	svc := NewService(&mockRepo{
		softDeleteFn: func(_ context.Context, _, _ uuid.UUID, deletedAt time.Time) error {
			capturedDeletedAt = deletedAt
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	err := svc.DeleteDocument(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.False(t, capturedDeletedAt.IsZero())
}

func TestDeleteDocument_Idempotent(t *testing.T) {
	t.Parallel()
	softDeleteCalled := false
	now := time.Now()
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, DeletedAt: &now}, nil
		},
		softDeleteFn: func(_ context.Context, _, _ uuid.UUID, _ time.Time) error {
			softDeleteCalled = true
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	err := svc.DeleteDocument(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.False(t, softDeleteCalled) // already deleted — no-op
}

func TestDeleteDocument_PublishesEvent(t *testing.T) {
	t.Parallel()
	published := false
	svc := NewService(&mockRepo{}, &mockStore{}, &mockPublisher{
		deletedFn: func(_ context.Context, _, _ uuid.UUID) error {
			published = true
			return nil
		},
	})

	err := svc.DeleteDocument(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.True(t, published)
}

// --- Tests: GetDownloadURL ---

func TestGetDownloadURL_ReturnsPresignedURL(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})

	dl, err := svc.GetDownloadURL(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.Contains(t, dl.URL, "get/")
	assert.True(t, dl.ExpiresAt.After(time.Now()))
	assert.True(t, dl.ExpiresAt.Before(time.Now().Add(downloadURLTTL+time.Minute)))
}

func TestGetDownloadURL_PendingUpload(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, SizeBytes: 0}, nil // not yet confirmed
		},
	}, &mockStore{}, &mockPublisher{})

	_, err := svc.GetDownloadURL(context.Background(), fixedTenantID, fixedDocID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUploadNotConfirmed)
}

// --- Tests: Versioning ---

func TestCreateVersion_IncrementsVersion(t *testing.T) {
	t.Parallel()
	var capturedKey string
	svc := NewService(&mockRepo{}, &mockStore{
		presignPutFn: func(_ context.Context, key string, _ time.Duration) (string, error) {
			capturedKey = key
			return "https://put/" + key, nil
		},
	}, &mockPublisher{})

	result, err := svc.CreateVersion(context.Background(), fixedTenantID, fixedDocID, fixedUserID)
	require.NoError(t, err)
	assert.Contains(t, capturedKey, "v2/") // current is v1, next is v2
	assert.Equal(t, fixedDocID, result.DocumentID)
}

func TestCreateVersion_PendingDocumentBlocked(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, SizeBytes: 0, CurrentVersion: 1}, nil
		},
	}, &mockStore{}, &mockPublisher{})

	_, err := svc.CreateVersion(context.Background(), fixedTenantID, fixedDocID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUploadNotConfirmed)
}

func TestRestoreVersion_UpdatesCurrentVersion(t *testing.T) {
	t.Parallel()
	var updatedDoc *Document
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: fixedTenantID, SizeBytes: 4096, CurrentVersion: 3}, nil
		},
		getVersionFn: func(_ context.Context, _ uuid.UUID, version int) (*DocumentVersion, error) {
			return &DocumentVersion{Version: version, StorageKey: "key/v1/file.pdf", SizeBytes: 1024}, nil
		},
		updateDocumentFn: func(_ context.Context, doc *Document) error {
			updatedDoc = doc
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	doc, err := svc.RestoreVersion(context.Background(), fixedTenantID, fixedDocID, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, doc.CurrentVersion)
	assert.Equal(t, "key/v1/file.pdf", doc.StorageKey)
	assert.Equal(t, 1, updatedDoc.CurrentVersion)
}

func TestRestoreVersion_AlreadyCurrent_Idempotent(t *testing.T) {
	t.Parallel()
	updateCalled := false
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, SizeBytes: 1024, CurrentVersion: 2}, nil
		},
		updateDocumentFn: func(_ context.Context, _ *Document) error {
			updateCalled = true
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	_, err := svc.RestoreVersion(context.Background(), fixedTenantID, fixedDocID, 2)
	require.NoError(t, err)
	assert.False(t, updateCalled)
}

// --- Tests: Folders ---

func TestCreateFolder_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})

	folder, err := svc.CreateFolder(context.Background(), &Folder{
		TenantID:  fixedTenantID,
		Name:      "Contracts",
		CreatedBy: fixedUserID,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, folder.ID)
}

// --- Tests: ListDocuments ---

func TestListDocuments_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listDocumentsFn: func(_ context.Context, _ uuid.UUID, _, _ *uuid.UUID, limit, _ int) ([]Document, error) {
			capturedLimit = limit
			return nil, nil
		},
	}, &mockStore{}, &mockPublisher{})

	_, err := svc.ListDocuments(context.Background(), fixedTenantID, nil, nil, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

// --- Tests: MoveDocument ---

func TestMoveDocument_UpdatesFolderID(t *testing.T) {
	t.Parallel()
	var updatedDoc *Document
	svc := NewService(&mockRepo{
		updateDocumentFn: func(_ context.Context, doc *Document) error {
			updatedDoc = doc
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	doc, err := svc.MoveDocument(context.Background(), fixedTenantID, fixedDocID, fixedFolderID)
	require.NoError(t, err)
	require.NotNil(t, doc.FolderID)
	assert.Equal(t, fixedFolderID, *doc.FolderID)
	assert.Equal(t, fixedFolderID, *updatedDoc.FolderID)
}

// --- Additional coverage tests ---

func TestConfirmVersion_Success(t *testing.T) {
	t.Parallel()
	var createdVersion *DocumentVersion
	svc := NewService(&mockRepo{
		getDocumentFn: func(_ context.Context, tenantID, id uuid.UUID) (*Document, error) {
			return &Document{
				ID:             id,
				TenantID:       tenantID,
				Name:           "contract.pdf",
				StorageKey:     "key/v1/contract.pdf",
				SizeBytes:      1024,
				CurrentVersion: 1,
				UploadedBy:     fixedUserID,
			}, nil
		},
		createVersionFn: func(_ context.Context, v *DocumentVersion) error {
			createdVersion = v
			return nil
		},
	}, &mockStore{}, &mockPublisher{})

	dv, err := svc.ConfirmVersion(context.Background(), fixedTenantID, fixedDocID, 2, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, 2, dv.Version)
	assert.Equal(t, 2, createdVersion.Version)
}

func TestConfirmVersion_WrongVersion(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tenantID, id uuid.UUID) (*Document, error) {
			return &Document{
				ID: id, TenantID: tenantID, SizeBytes: 1024, CurrentVersion: 1,
			}, nil
		},
	})

	// Trying to confirm version 5, but document is on version 1 (next should be 2).
	_, err := svc.ConfirmVersion(context.Background(), fixedTenantID, fixedDocID, 5, fixedUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 2")
}

func TestListVersions_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		listVersionsFn: func(_ context.Context, docID uuid.UUID) ([]DocumentVersion, error) {
			return []DocumentVersion{
				{ID: uuid.New(), DocumentID: docID, Version: 1},
				{ID: uuid.New(), DocumentID: docID, Version: 2},
			}, nil
		},
	}, &mockStore{}, &mockPublisher{})

	versions, err := svc.ListVersions(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err)
	assert.Len(t, versions, 2)
}

func TestListFolders_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		listFoldersFn: func(_ context.Context, tenantID uuid.UUID, _ *uuid.UUID) ([]Folder, error) {
			return []Folder{{ID: fixedFolderID, TenantID: tenantID, Name: "Contracts"}}, nil
		},
	}, &mockStore{}, &mockPublisher{})

	folders, err := svc.ListFolders(context.Background(), fixedTenantID, nil)
	require.NoError(t, err)
	assert.Len(t, folders, 1)
	assert.Equal(t, "Contracts", folders[0].Name)
}

func TestGetDocument_Deleted(t *testing.T) {
	t.Parallel()
	now := time.Now()
	svc := newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tenantID, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tenantID, DeletedAt: &now}, nil
		},
	})

	_, err := svc.GetDocument(context.Background(), fixedTenantID, fixedDocID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDocumentDeleted)
}

func TestGetDocument_RepoError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			return nil, ErrDocumentNotFound
		},
	})

	_, err := svc.GetDocument(context.Background(), fixedTenantID, fixedDocID)
	require.Error(t, err)
}

func TestDeleteDocument_AlreadyDeleted_Idempotent(t *testing.T) {
	t.Parallel()
	now := time.Now()
	svc := newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tenantID, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tenantID, DeletedAt: &now}, nil
		},
	})

	err := svc.DeleteDocument(context.Background(), fixedTenantID, fixedDocID)
	require.NoError(t, err) // idempotent — no error
}

func TestGetDownloadURL_UploadNotConfirmed(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tenantID, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tenantID, SizeBytes: 0}, nil // no bytes yet
		},
	})

	_, err := svc.GetDownloadURL(context.Background(), fixedTenantID, fixedDocID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUploadNotConfirmed)
}

func TestCreateVersion_UploadNotConfirmed(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tenantID, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tenantID, SizeBytes: 0}, nil
		},
	})

	_, err := svc.CreateVersion(context.Background(), fixedTenantID, fixedDocID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUploadNotConfirmed)
}
