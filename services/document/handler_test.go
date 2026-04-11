package document

import (
	"context"
	"testing"

	"github.com/google/uuid"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newHandler() *Handler {
	return NewHandler(newTestService(&mockRepo{}))
}

// --- InitiateUpload ---

func TestHandler_InitiateUpload_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	uploadedBy := uuid.New()

	resp, err := newHandler().InitiateUpload(context.Background(), &documentv1.InitiateUploadRequest{
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
	resp, err := newHandler().InitiateUpload(context.Background(), &documentv1.InitiateUploadRequest{
		TenantId:     uuid.New().String(),
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
	_, err := newHandler().InitiateUpload(context.Background(), &documentv1.InitiateUploadRequest{
		TenantId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InitiateUpload_InvalidUploadedBy(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InitiateUpload(context.Background(), &documentv1.InitiateUploadRequest{
		TenantId: uuid.New().String(), UploadedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InitiateUpload_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InitiateUpload(context.Background(), &documentv1.InitiateUploadRequest{
		TenantId: uuid.New().String(), ProductionId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InitiateUpload_InvalidFolderID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InitiateUpload(context.Background(), &documentv1.InitiateUploadRequest{
		TenantId: uuid.New().String(), FolderId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ConfirmUpload ---

func TestHandler_ConfirmUpload_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	docID := uuid.New()

	resp, err := newHandler().ConfirmUpload(context.Background(), &documentv1.ConfirmUploadRequest{
		TenantId:   tenantID.String(),
		DocumentId: docID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
	assert.Equal(t, int64(2048), resp.GetSizeBytes())
}

func TestHandler_ConfirmUpload_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmUpload(context.Background(), &documentv1.ConfirmUploadRequest{
		TenantId: "bad", DocumentId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ConfirmUpload_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmUpload(context.Background(), &documentv1.ConfirmUploadRequest{
		TenantId: uuid.New().String(), DocumentId: "bad",
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
	}))

	resp, err := h.GetDocument(context.Background(), &documentv1.GetDocumentRequest{
		TenantId: tenantID.String(),
		Id:       docID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
	assert.Equal(t, "budget.pdf", resp.GetName())
}

func TestHandler_GetDocument_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDocument(context.Background(), &documentv1.GetDocumentRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetDocument_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDocument(context.Background(), &documentv1.GetDocumentRequest{
		TenantId: uuid.New().String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetDocument_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, _, _ uuid.UUID) (*Document, error) {
			return nil, ErrDocumentNotFound
		},
	}))
	_, err := h.GetDocument(context.Background(), &documentv1.GetDocumentRequest{
		TenantId: uuid.New().String(), Id: uuid.New().String(),
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
	}))

	resp, err := h.ListDocuments(context.Background(), &documentv1.ListDocumentsRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetDocuments(), 1)
}

func TestHandler_ListDocuments_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListDocuments(context.Background(), &documentv1.ListDocumentsRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListDocuments_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListDocuments(context.Background(), &documentv1.ListDocumentsRequest{
		TenantId: uuid.New().String(), ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListDocuments_InvalidFolderID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListDocuments(context.Background(), &documentv1.ListDocumentsRequest{
		TenantId: uuid.New().String(), FolderId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- DeleteDocument ---

func TestHandler_DeleteDocument_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().DeleteDocument(context.Background(), &documentv1.DeleteDocumentRequest{
		TenantId: uuid.New().String(),
		Id:       uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_DeleteDocument_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().DeleteDocument(context.Background(), &documentv1.DeleteDocumentRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_DeleteDocument_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().DeleteDocument(context.Background(), &documentv1.DeleteDocumentRequest{
		TenantId: uuid.New().String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetDownloadURL ---

func TestHandler_GetDownloadURL_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetDownloadURL(context.Background(), &documentv1.GetDownloadURLRequest{
		TenantId: uuid.New().String(),
		Id:       uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Contains(t, resp.GetUrl(), "https://")
}

func TestHandler_GetDownloadURL_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDownloadURL(context.Background(), &documentv1.GetDownloadURLRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetDownloadURL_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDownloadURL(context.Background(), &documentv1.GetDownloadURLRequest{
		TenantId: uuid.New().String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- MoveDocument ---

func TestHandler_MoveDocument_Success(t *testing.T) {
	t.Parallel()
	docID := uuid.New()
	folderID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getDocumentFn: func(_ context.Context, tid, id uuid.UUID) (*Document, error) {
			return &Document{ID: id, TenantID: tid, Name: "file.pdf", SizeBytes: 100, CurrentVersion: 1, UploadedBy: fixedUserID}, nil
		},
	}))

	resp, err := h.MoveDocument(context.Background(), &documentv1.MoveDocumentRequest{
		TenantId:   uuid.New().String(),
		DocumentId: docID.String(),
		FolderId:   folderID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
}

func TestHandler_MoveDocument_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().MoveDocument(context.Background(), &documentv1.MoveDocumentRequest{
		TenantId: "bad", DocumentId: uuid.New().String(), FolderId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_MoveDocument_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().MoveDocument(context.Background(), &documentv1.MoveDocumentRequest{
		TenantId: uuid.New().String(), DocumentId: "bad", FolderId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_MoveDocument_InvalidFolderID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().MoveDocument(context.Background(), &documentv1.MoveDocumentRequest{
		TenantId: uuid.New().String(), DocumentId: uuid.New().String(), FolderId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateVersion ---

func TestHandler_CreateVersion_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().CreateVersion(context.Background(), &documentv1.CreateVersionRequest{
		TenantId:   uuid.New().String(),
		DocumentId: uuid.New().String(),
		UploadedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Contains(t, resp.GetUrl(), "https://")
}

func TestHandler_CreateVersion_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateVersion(context.Background(), &documentv1.CreateVersionRequest{
		TenantId: "bad", DocumentId: uuid.New().String(), UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateVersion_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateVersion(context.Background(), &documentv1.CreateVersionRequest{
		TenantId: uuid.New().String(), DocumentId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateVersion_InvalidUploadedBy(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateVersion(context.Background(), &documentv1.CreateVersionRequest{
		TenantId: uuid.New().String(), DocumentId: uuid.New().String(), UploadedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ConfirmVersion ---

func TestHandler_ConfirmVersion_Success(t *testing.T) {
	t.Parallel()
	// Default doc has CurrentVersion=1; confirm version 2.
	resp, err := newHandler().ConfirmVersion(context.Background(), &documentv1.ConfirmVersionRequest{
		TenantId:   uuid.New().String(),
		DocumentId: uuid.New().String(),
		Version:    2,
		UploadedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.GetVersion())
}

func TestHandler_ConfirmVersion_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmVersion(context.Background(), &documentv1.ConfirmVersionRequest{
		TenantId: "bad", DocumentId: uuid.New().String(), UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ConfirmVersion_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmVersion(context.Background(), &documentv1.ConfirmVersionRequest{
		TenantId: uuid.New().String(), DocumentId: "bad", UploadedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ConfirmVersion_InvalidUploadedBy(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ConfirmVersion(context.Background(), &documentv1.ConfirmVersionRequest{
		TenantId: uuid.New().String(), DocumentId: uuid.New().String(), UploadedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListVersions ---

func TestHandler_ListVersions_Success(t *testing.T) {
	t.Parallel()
	docID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listVersionsFn: func(_ context.Context, id uuid.UUID) ([]DocumentVersion, error) {
			return []DocumentVersion{{ID: uuid.New(), DocumentID: id, Version: 1}}, nil
		},
	}))

	resp, err := h.ListVersions(context.Background(), &documentv1.ListVersionsRequest{
		TenantId:   uuid.New().String(),
		DocumentId: docID.String(),
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetVersions(), 1)
}

func TestHandler_ListVersions_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListVersions(context.Background(), &documentv1.ListVersionsRequest{
		TenantId: "bad", DocumentId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListVersions_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListVersions(context.Background(), &documentv1.ListVersionsRequest{
		TenantId: uuid.New().String(), DocumentId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- RestoreVersion ---

func TestHandler_RestoreVersion_Success(t *testing.T) {
	t.Parallel()
	// Default doc has CurrentVersion=1; restoring version 2 calls GetVersion.
	docID := uuid.New()
	resp, err := newHandler().RestoreVersion(context.Background(), &documentv1.RestoreVersionRequest{
		TenantId:   uuid.New().String(),
		DocumentId: docID.String(),
		Version:    2,
	})
	require.NoError(t, err)
	assert.Equal(t, docID.String(), resp.GetId())
}

func TestHandler_RestoreVersion_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RestoreVersion(context.Background(), &documentv1.RestoreVersionRequest{
		TenantId: "bad", DocumentId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_RestoreVersion_InvalidDocumentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RestoreVersion(context.Background(), &documentv1.RestoreVersionRequest{
		TenantId: uuid.New().String(), DocumentId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateFolder ---

func TestHandler_CreateFolder_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().CreateFolder(context.Background(), &documentv1.CreateFolderRequest{
		TenantId:  uuid.New().String(),
		Name:      "Scripts",
		CreatedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, "Scripts", resp.GetName())
}

func TestHandler_CreateFolder_WithProductionAndParent(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().CreateFolder(context.Background(), &documentv1.CreateFolderRequest{
		TenantId:     uuid.New().String(),
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
	_, err := newHandler().CreateFolder(context.Background(), &documentv1.CreateFolderRequest{
		TenantId: "bad", CreatedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateFolder_InvalidCreatedBy(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateFolder(context.Background(), &documentv1.CreateFolderRequest{
		TenantId: uuid.New().String(), CreatedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateFolder_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateFolder(context.Background(), &documentv1.CreateFolderRequest{
		TenantId: uuid.New().String(), ProductionId: "bad", CreatedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateFolder_InvalidParentID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateFolder(context.Background(), &documentv1.CreateFolderRequest{
		TenantId: uuid.New().String(), ParentId: "bad", CreatedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListFolders ---

func TestHandler_ListFolders_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listFoldersFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]Folder, error) {
			return []Folder{{ID: uuid.New(), TenantID: tenantID, Name: "Scripts"}}, nil
		},
	}))

	resp, err := h.ListFolders(context.Background(), &documentv1.ListFoldersRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetFolders(), 1)
	assert.Equal(t, "Scripts", resp.GetFolders()[0].GetName())
}

func TestHandler_ListFolders_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListFolders(context.Background(), &documentv1.ListFoldersRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListFolders_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListFolders(context.Background(), &documentv1.ListFoldersRequest{
		TenantId: uuid.New().String(), ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
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
