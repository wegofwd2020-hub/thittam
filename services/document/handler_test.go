package document

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// allowAllPerm grants every permission; authz semantics are covered by
// pkg/interceptor's own tests.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm denies every permission, so a denial test proves the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}

func newHandler() *Handler {
	return NewHandler(newTestService(&mockRepo{}), allowAllPerm{})
}

// newHandlerWithRepo builds a Handler backed by the given mock repo — used by
// tests that need to assert the repository is (or is not) reached.
func newHandlerWithRepo(r *mockRepo) *Handler {
	return NewHandler(newTestService(r), allowAllPerm{})
}

// newHandlerWithRepoDeny builds a Handler backed by the given mock repo whose
// permission checker denies every check — used by the denial tests to prove
// the gate fires before the repository is reached.
func newHandlerWithRepoDeny(r *mockRepo) *Handler {
	return NewHandler(newTestService(r), denyPerm{})
}

// callerCtx returns a context carrying a verified caller in tenant tid, as
// UnaryAuthInterceptor would have produced from a valid token (#138).
// Handler tests bypass the interceptor, so they must inject the caller themselves.
func callerCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}

// --- InitiateUpload ---

func TestHandler_InitiateUpload_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	uploadedBy := uuid.New()

	resp, err := newHandler().InitiateUpload(callerCtx(tenantID), &documentv1.InitiateUploadRequest{
		TenantId:   tenantID.String(),
		Name:       "script.pdf",
		MimeType:   "application/pdf",
		UploadedBy: uploadedBy.String(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetDocumentId())
	assert.Contains(t, resp.GetUrl(), "https://")
}

func TestHandler_InitiateUpload_WithProductionAndFolder(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().InitiateUpload(callerCtx(tid), &documentv1.InitiateUploadRequest{
		TenantId:     tid.String(),
		ProductionId: uuid.New().String(),
		FolderId:     uuid.New().String(),
		Name:         "budget.xlsx",
		MimeType:     "application/vnd.ms-excel",
		UploadedBy:   uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetDocumentId())
}

func TestHandler_InitiateUpload_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InitiateUpload(callerCtx(uuid.New()), &documentv1.InitiateUploadRequest{
		TenantId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InitiateUpload_InvalidUploadedBy(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().InitiateUpload(callerCtx(tid), &documentv1.InitiateUploadRequest{
		TenantId: tid.String(), UploadedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InitiateUpload_InvalidProductionID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().InitiateUpload(callerCtx(tid), &documentv1.InitiateUploadRequest{
		TenantId: tid.String(), ProductionId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InitiateUpload_InvalidFolderID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().InitiateUpload(callerCtx(tid), &documentv1.InitiateUploadRequest{
		TenantId: tid.String(), FolderId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ConfirmUpload ---

func TestHandler_ConfirmUpload_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	docID := uuid.New()

	resp, err := newHandler().ConfirmUpload(callerCtx(tenantID), &documentv1.ConfirmUploadRequest{
		TenantId:   tenantID.String(),
		DocumentId: docID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
	assert.Equal(t, int64(2048), resp.GetSizeBytes())
}

func TestHandler_ConfirmUpload_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmUpload(callerCtx(uuid.New()), &documentv1.ConfirmUploadRequest{
		TenantId: "bad", DocumentId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ConfirmUpload_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ConfirmUpload(callerCtx(tid), &documentv1.ConfirmUploadRequest{
		TenantId: tid.String(), DocumentId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetDocument ---

func TestHandler_GetDocument_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	docID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tid, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tid, Name: "budget.pdf", SizeBytes: 5000, CurrentVersion: 1, UploadedBy: fixedUserID}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetDocument(callerCtx(tenantID), &documentv1.GetDocumentRequest{
		TenantId: tenantID.String(),
		Id:       docID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
	assert.Equal(t, "budget.pdf", resp.GetName())
}

func TestHandler_GetDocument_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDocument(callerCtx(uuid.New()), &documentv1.GetDocumentRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetDocument_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetDocument(callerCtx(tid), &documentv1.GetDocumentRequest{
		TenantId: tid.String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetDocument_NotFound(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			return nil, ErrDocumentNotFound
		},
	}), allowAllPerm{})
	_, err := h.GetDocument(callerCtx(tid), &documentv1.GetDocumentRequest{
		TenantId: tid.String(), Id: uuid.New().String(),
	})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- ListDocuments ---

func TestHandler_ListDocuments_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listDocumentsFn: func(_ context.Context, _ uuid.UUID, _, _ *uuid.UUID, _, _ int) ([]Document, error) {
			return []Document{{ID: uuid.New(), TenantID: tenantID, Name: "script.pdf", CurrentVersion: 1, UploadedBy: fixedUserID}}, nil
		},
	}), allowAllPerm{})

	resp, err := h.ListDocuments(callerCtx(tenantID), &documentv1.ListDocumentsRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetDocuments(), 1)
}

func TestHandler_ListDocuments_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListDocuments(callerCtx(uuid.New()), &documentv1.ListDocumentsRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListDocuments_InvalidProductionID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListDocuments(callerCtx(tid), &documentv1.ListDocumentsRequest{
		TenantId: tid.String(), ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListDocuments_InvalidFolderID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListDocuments(callerCtx(tid), &documentv1.ListDocumentsRequest{
		TenantId: tid.String(), FolderId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- DeleteDocument ---

func TestHandler_DeleteDocument_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().DeleteDocument(callerCtx(tid), &documentv1.DeleteDocumentRequest{
		TenantId: tid.String(),
		Id:       uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_DeleteDocument_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().DeleteDocument(callerCtx(uuid.New()), &documentv1.DeleteDocumentRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_DeleteDocument_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().DeleteDocument(callerCtx(tid), &documentv1.DeleteDocumentRequest{
		TenantId: tid.String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetDownloadURL ---

func TestHandler_GetDownloadURL_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().GetDownloadURL(callerCtx(tid), &documentv1.GetDownloadURLRequest{
		TenantId: tid.String(),
		Id:       uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Contains(t, resp.GetUrl(), "https://")
}

func TestHandler_GetDownloadURL_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDownloadURL(callerCtx(uuid.New()), &documentv1.GetDownloadURLRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetDownloadURL_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetDownloadURL(callerCtx(tid), &documentv1.GetDownloadURLRequest{
		TenantId: tid.String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- MoveDocument ---

func TestHandler_MoveDocument_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	docID := uuid.New()
	folderID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tid, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tid, Name: "file.pdf", SizeBytes: 100, CurrentVersion: 1, UploadedBy: fixedUserID}, nil
		},
	}), allowAllPerm{})

	resp, err := h.MoveDocument(callerCtx(tid), &documentv1.MoveDocumentRequest{
		TenantId:   tid.String(),
		DocumentId: docID.String(),
		FolderId:   folderID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
}

func TestHandler_MoveDocument_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().MoveDocument(callerCtx(uuid.New()), &documentv1.MoveDocumentRequest{
		TenantId: "bad", DocumentId: uuid.New().String(), FolderId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_MoveDocument_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().MoveDocument(callerCtx(tid), &documentv1.MoveDocumentRequest{
		TenantId: tid.String(), DocumentId: "bad", FolderId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_MoveDocument_InvalidFolderID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().MoveDocument(callerCtx(tid), &documentv1.MoveDocumentRequest{
		TenantId: tid.String(), DocumentId: uuid.New().String(), FolderId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateVersion ---

func TestHandler_CreateVersion_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().CreateVersion(callerCtx(tid), &documentv1.CreateVersionRequest{
		TenantId:   tid.String(),
		DocumentId: uuid.New().String(),
		UploadedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Contains(t, resp.GetUrl(), "https://")
}

func TestHandler_CreateVersion_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateVersion(callerCtx(uuid.New()), &documentv1.CreateVersionRequest{
		TenantId: "bad", DocumentId: uuid.New().String(), UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateVersion_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateVersion(callerCtx(tid), &documentv1.CreateVersionRequest{
		TenantId: tid.String(), DocumentId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateVersion_InvalidUploadedBy(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateVersion(callerCtx(tid), &documentv1.CreateVersionRequest{
		TenantId: tid.String(), DocumentId: uuid.New().String(), UploadedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ConfirmVersion ---

func TestHandler_ConfirmVersion_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	// Default doc has CurrentVersion=1; confirm version 2.
	resp, err := newHandler().ConfirmVersion(callerCtx(tid), &documentv1.ConfirmVersionRequest{
		TenantId:   tid.String(),
		DocumentId: uuid.New().String(),
		Version:    2,
		UploadedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.GetVersion())
}

func TestHandler_ConfirmVersion_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmVersion(callerCtx(uuid.New()), &documentv1.ConfirmVersionRequest{
		TenantId: "bad", DocumentId: uuid.New().String(), UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ConfirmVersion_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ConfirmVersion(callerCtx(tid), &documentv1.ConfirmVersionRequest{
		TenantId: tid.String(), DocumentId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ConfirmVersion_InvalidUploadedBy(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ConfirmVersion(callerCtx(tid), &documentv1.ConfirmVersionRequest{
		TenantId: tid.String(), DocumentId: uuid.New().String(), UploadedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListVersions ---

func TestHandler_ListVersions_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	docID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listVersionsFn: func(_ context.Context, id uuid.UUID, _, _ int) ([]DocumentVersion, error) {
			return []DocumentVersion{{ID: uuid.New(), DocumentID: id, Version: 1}}, nil
		},
	}), allowAllPerm{})

	resp, err := h.ListVersions(callerCtx(tid), &documentv1.ListVersionsRequest{
		TenantId:   tid.String(),
		DocumentId: docID.String(),
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetVersions(), 1)
}

func TestHandler_ListVersions_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListVersions(callerCtx(uuid.New()), &documentv1.ListVersionsRequest{
		TenantId: "bad", DocumentId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListVersions_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListVersions(callerCtx(tid), &documentv1.ListVersionsRequest{
		TenantId: tid.String(), DocumentId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- RestoreVersion ---

func TestHandler_RestoreVersion_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	// Default doc has CurrentVersion=1; restoring version 2 calls GetVersion.
	docID := uuid.New()
	resp, err := newHandler().RestoreVersion(callerCtx(tid), &documentv1.RestoreVersionRequest{
		TenantId:   tid.String(),
		DocumentId: docID.String(),
		Version:    2,
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
}

func TestHandler_RestoreVersion_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RestoreVersion(callerCtx(uuid.New()), &documentv1.RestoreVersionRequest{
		TenantId: "bad", DocumentId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_RestoreVersion_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().RestoreVersion(callerCtx(tid), &documentv1.RestoreVersionRequest{
		TenantId: tid.String(), DocumentId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateFolder ---

func TestHandler_CreateFolder_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().CreateFolder(callerCtx(tid), &documentv1.CreateFolderRequest{
		TenantId:  tid.String(),
		Name:      "Scripts",
		CreatedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, "Scripts", resp.GetName())
}

func TestHandler_CreateFolder_WithProductionAndParent(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().CreateFolder(callerCtx(tid), &documentv1.CreateFolderRequest{
		TenantId:     tid.String(),
		ProductionId: uuid.New().String(),
		ParentId:     uuid.New().String(),
		Name:         "Contracts",
		CreatedBy:    uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, "Contracts", resp.GetName())
}

func TestHandler_CreateFolder_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateFolder(callerCtx(uuid.New()), &documentv1.CreateFolderRequest{
		TenantId: "bad", CreatedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateFolder_InvalidCreatedBy(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateFolder(callerCtx(tid), &documentv1.CreateFolderRequest{
		TenantId: tid.String(), CreatedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateFolder_InvalidProductionID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateFolder(callerCtx(tid), &documentv1.CreateFolderRequest{
		TenantId: tid.String(), ProductionId: "bad", CreatedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateFolder_InvalidParentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateFolder(callerCtx(tid), &documentv1.CreateFolderRequest{
		TenantId: tid.String(), ParentId: "bad", CreatedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListFolders ---

func TestHandler_ListFolders_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listFoldersFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _ int) ([]Folder, error) {
			return []Folder{{ID: uuid.New(), TenantID: tenantID, Name: "Scripts"}}, nil
		},
	}), allowAllPerm{})

	resp, err := h.ListFolders(callerCtx(tenantID), &documentv1.ListFoldersRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetFolders(), 1)
	assert.Equal(t, "Scripts", resp.GetFolders()[0].GetName())
}

func TestHandler_ListFolders_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListFolders(callerCtx(uuid.New()), &documentv1.ListFoldersRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListFolders_InvalidProductionID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListFolders(callerCtx(tid), &documentv1.ListFoldersRequest{
		TenantId: tid.String(), ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- Tenant boundary (#144) ---

func TestHandler_CrossTenantRead_Denied(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	victimTenant := uuid.New()
	require.NotEqual(t, callerTenant, victimTenant)

	h := newHandlerWithRepo(&mockRepo{
		getDocumentFn: func(context.Context, uuid.UUID, uuid.UUID) (*Document, error) {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil, nil
		},
	})

	_, err := h.GetDocument(callerCtx(callerTenant), &documentv1.GetDocumentRequest{
		TenantId: victimTenant.String(),
		Id:       uuid.New().String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListDocuments_UsesTokenTenant(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenant uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		listDocumentsFn: func(_ context.Context, tenantID uuid.UUID, _, _ *uuid.UUID, _, _ int) ([]Document, error) {
			gotTenant = tenantID
			return nil, nil
		},
	})

	// Request carries NO tenant at all: the token supplies it.
	_, err := h.ListDocuments(callerCtx(tid), &documentv1.ListDocumentsRequest{})
	require.NoError(t, err)
	assert.Equal(t, tid, gotTenant, "the repository must receive the token's tenant")
}

// --- Authorization denial (#139) ---
//
// One test per RPC. Each installs t.Fatal on the first repository fn its
// happy path reaches when only that field is overridden (every other field
// on mockRepo falls back to its zero-error default). A PASS here before the
// gates are inserted (Step 6) would mean the tripwire is unreachable and the
// test proves nothing; a PASS after the gates are inserted (Step 8) is the
// intended outcome — the permission check must short-circuit before the
// repository is ever touched.

func TestHandler_GetDocument_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: GetDocument must deny before querying")
			return nil, nil
		},
	})

	_, err := h.GetDocument(callerCtx(uuid.New()), &documentv1.GetDocumentRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListDocuments_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		listDocumentsFn: func(_ context.Context, _ uuid.UUID, _, _ *uuid.UUID, _, _ int) ([]Document, error) {
			t.Fatal("repository reached: ListDocuments must deny before querying")
			return nil, nil
		},
	})

	_, err := h.ListDocuments(callerCtx(uuid.New()), &documentv1.ListDocumentsRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetDownloadURL_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: GetDownloadURL must deny before querying (calls GetDocument first)")
			return nil, nil
		},
	})

	_, err := h.GetDownloadURL(callerCtx(uuid.New()), &documentv1.GetDownloadURLRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListVersions_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		listVersionsFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]DocumentVersion, error) {
			t.Fatal("repository reached: ListVersions must deny before querying")
			return nil, nil
		},
	})

	_, err := h.ListVersions(callerCtx(uuid.New()), &documentv1.ListVersionsRequest{
		DocumentId: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListFolders_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		listFoldersFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _ int) ([]Folder, error) {
			t.Fatal("repository reached: ListFolders must deny before querying")
			return nil, nil
		},
	})

	_, err := h.ListFolders(callerCtx(uuid.New()), &documentv1.ListFoldersRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_InitiateUpload_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		createDocumentFn: func(_ context.Context, _ *Document) error {
			t.Fatal("repository reached: InitiateUpload must deny before creating a record")
			return nil
		},
	})

	_, err := h.InitiateUpload(callerCtx(uuid.New()), &documentv1.InitiateUploadRequest{
		Name:       "script.pdf",
		MimeType:   "application/pdf",
		UploadedBy: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ConfirmUpload_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: ConfirmUpload must deny before querying")
			return nil, nil
		},
	})

	_, err := h.ConfirmUpload(callerCtx(uuid.New()), &documentv1.ConfirmUploadRequest{
		DocumentId: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_MoveDocument_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getFolderFn: func(_ context.Context, _, _ uuid.UUID) (*Folder, error) {
			t.Fatal("repository reached: MoveDocument must deny before querying the target folder")
			return nil, nil
		},
	})

	_, err := h.MoveDocument(callerCtx(uuid.New()), &documentv1.MoveDocumentRequest{
		DocumentId: uuid.New().String(),
		FolderId:   uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CreateVersion_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: CreateVersion must deny before querying (calls GetDocument first)")
			return nil, nil
		},
	})

	_, err := h.CreateVersion(callerCtx(uuid.New()), &documentv1.CreateVersionRequest{
		DocumentId: uuid.New().String(),
		UploadedBy: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ConfirmVersion_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		createVersionFn: func(_ context.Context, _ *DocumentVersion) error {
			t.Fatal("repository reached: ConfirmVersion must deny before creating a version record")
			return nil
		},
	})

	_, err := h.ConfirmVersion(callerCtx(uuid.New()), &documentv1.ConfirmVersionRequest{
		DocumentId: uuid.New().String(),
		Version:    2,
		UploadedBy: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CreateFolder_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		createFolderFn: func(_ context.Context, _ *Folder) error {
			t.Fatal("repository reached: CreateFolder must deny before creating a record")
			return nil
		},
	})

	_, err := h.CreateFolder(callerCtx(uuid.New()), &documentv1.CreateFolderRequest{
		Name:      "Scripts",
		CreatedBy: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_DeleteDocument_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: DeleteDocument must deny before querying")
			return nil, nil
		},
	})

	_, err := h.DeleteDocument(callerCtx(uuid.New()), &documentv1.DeleteDocumentRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_RestoreVersion_Denied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepoDeny(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			t.Fatal("repository reached: RestoreVersion must deny before querying (calls GetDocument first)")
			return nil, nil
		},
	})

	_, err := h.RestoreVersion(callerCtx(uuid.New()), &documentv1.RestoreVersionRequest{
		DocumentId: uuid.New().String(),
		Version:    2,
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- grpcErr ---

func TestGrpcErr_Document_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrDocumentNotFound, codes.NotFound},
		{ErrFolderNotFound, codes.NotFound},
		{ErrVersionNotFound, codes.NotFound},
		{ErrDocumentDeleted, codes.FailedPrecondition},
		{ErrUploadNotConfirmed, codes.FailedPrecondition},
		{ErrStorageKeyConflict, codes.AlreadyExists},
		{ErrPresignFailed, codes.Internal},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
