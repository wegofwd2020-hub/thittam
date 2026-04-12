package document

import (
	"context"
	"errors"

	"github.com/google/uuid"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements documentv1.DocumentServiceServer by delegating to Service.
type Handler struct {
	documentv1.UnimplementedDocumentServiceServer
	svc *Service
}

// NewHandler creates a document handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// --- Upload lifecycle ---

func (h *Handler) InitiateUpload(ctx context.Context, req *documentv1.InitiateUploadRequest) (*documentv1.UploadURL, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	uploadedBy, err := uuid.Parse(req.GetUploadedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid uploaded_by")
	}

	ir := &InitiateUploadRequest{
		TenantID:   tenantID,
		Name:       req.GetName(),
		MimeType:   req.GetMimeType(),
		UploadedBy: uploadedBy,
	}
	if s := req.GetProductionId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production_id")
		}
		ir.ProductionID = &id
	}
	if s := req.GetFolderId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid folder_id")
		}
		ir.FolderID = &id
	}

	u, err := h.svc.InitiateUpload(ctx, ir)
	if err != nil {
		return nil, grpcErr(err)
	}
	return &documentv1.UploadURL{
		DocumentId: u.DocumentID.String(),
		Url:        u.URL,
		ExpiresAt:  timestamppb.New(u.ExpiresAt),
	}, nil
}

func (h *Handler) ConfirmUpload(ctx context.Context, req *documentv1.ConfirmUploadRequest) (*documentv1.Document, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid document_id")
	}

	doc, err := h.svc.ConfirmUpload(ctx, tenantID, docID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return docToProto(doc), nil
}

// --- Document management ---

func (h *Handler) GetDocument(ctx context.Context, req *documentv1.GetDocumentRequest) (*documentv1.Document, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	doc, err := h.svc.GetDocument(ctx, tenantID, docID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return docToProto(doc), nil
}

func (h *Handler) ListDocuments(ctx context.Context, req *documentv1.ListDocumentsRequest) (*documentv1.ListDocumentsResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	var productionID, folderID *uuid.UUID
	if s := req.GetProductionId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production_id")
		}
		productionID = &id
	}
	if s := req.GetFolderId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid folder_id")
		}
		folderID = &id
	}

	docs, err := h.svc.ListDocuments(ctx, tenantID, productionID, folderID, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*documentv1.Document, len(docs))
	for i := range docs {
		out[i] = docToProto(&docs[i])
	}
	return &documentv1.ListDocumentsResponse{Documents: out}, nil
}

func (h *Handler) DeleteDocument(ctx context.Context, req *documentv1.DeleteDocumentRequest) (*documentv1.DeleteDocumentResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	if err := h.svc.DeleteDocument(ctx, tenantID, docID); err != nil {
		return nil, grpcErr(err)
	}
	return &documentv1.DeleteDocumentResponse{}, nil
}

func (h *Handler) GetDownloadURL(ctx context.Context, req *documentv1.GetDownloadURLRequest) (*documentv1.DownloadURL, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	u, err := h.svc.GetDownloadURL(ctx, tenantID, docID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return &documentv1.DownloadURL{
		Url:       u.URL,
		ExpiresAt: timestamppb.New(u.ExpiresAt),
	}, nil
}

func (h *Handler) MoveDocument(ctx context.Context, req *documentv1.MoveDocumentRequest) (*documentv1.Document, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid document_id")
	}
	folderID, err := uuid.Parse(req.GetFolderId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid folder_id")
	}

	doc, err := h.svc.MoveDocument(ctx, tenantID, docID, folderID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return docToProto(doc), nil
}

// --- Versioning ---

func (h *Handler) CreateVersion(ctx context.Context, req *documentv1.CreateVersionRequest) (*documentv1.UploadURL, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid document_id")
	}
	uploadedBy, err := uuid.Parse(req.GetUploadedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid uploaded_by")
	}

	u, err := h.svc.CreateVersion(ctx, tenantID, docID, uploadedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return &documentv1.UploadURL{
		DocumentId: u.DocumentID.String(),
		Url:        u.URL,
		ExpiresAt:  timestamppb.New(u.ExpiresAt),
	}, nil
}

func (h *Handler) ConfirmVersion(ctx context.Context, req *documentv1.ConfirmVersionRequest) (*documentv1.DocumentVersion, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid document_id")
	}
	uploadedBy, err := uuid.Parse(req.GetUploadedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid uploaded_by")
	}

	dv, err := h.svc.ConfirmVersion(ctx, tenantID, docID, int(req.GetVersion()), uploadedBy)
	if err != nil {
		return nil, grpcErr(err)
	}
	return versionToProto(dv), nil
}

func (h *Handler) ListVersions(ctx context.Context, req *documentv1.ListVersionsRequest) (*documentv1.ListVersionsResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid document_id")
	}

	// Proto does not carry limit/offset yet — apply a server-side cap.
	const defaultLimit = 50
	versions, err := h.svc.ListVersions(ctx, tenantID, docID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*documentv1.DocumentVersion, len(versions))
	for i := range versions {
		out[i] = versionToProto(&versions[i])
	}
	return &documentv1.ListVersionsResponse{Versions: out}, nil
}

func (h *Handler) RestoreVersion(ctx context.Context, req *documentv1.RestoreVersionRequest) (*documentv1.Document, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	docID, err := uuid.Parse(req.GetDocumentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid document_id")
	}

	doc, err := h.svc.RestoreVersion(ctx, tenantID, docID, int(req.GetVersion()))
	if err != nil {
		return nil, grpcErr(err)
	}
	return docToProto(doc), nil
}

// --- Folders ---

func (h *Handler) CreateFolder(ctx context.Context, req *documentv1.CreateFolderRequest) (*documentv1.Folder, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	createdBy, err := uuid.Parse(req.GetCreatedBy())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid created_by")
	}

	folder := &Folder{
		TenantID:  tenantID,
		Name:      req.GetName(),
		CreatedBy: createdBy,
	}
	if s := req.GetProductionId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production_id")
		}
		folder.ProductionID = &id
	}
	if s := req.GetParentId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid parent_id")
		}
		folder.ParentID = &id
	}

	created, err := h.svc.CreateFolder(ctx, folder)
	if err != nil {
		return nil, grpcErr(err)
	}
	return folderToProto(created), nil
}

func (h *Handler) ListFolders(ctx context.Context, req *documentv1.ListFoldersRequest) (*documentv1.ListFoldersResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}

	var productionID *uuid.UUID
	if s := req.GetProductionId(); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid production_id")
		}
		productionID = &id
	}

	// Proto does not carry limit/offset yet — apply a server-side cap.
	const defaultLimit = 100
	folders, err := h.svc.ListFolders(ctx, tenantID, productionID, defaultLimit, 0)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*documentv1.Folder, len(folders))
	for i := range folders {
		out[i] = folderToProto(&folders[i])
	}
	return &documentv1.ListFoldersResponse{Folders: out}, nil
}

// --- Proto converters ---

func docToProto(d *Document) *documentv1.Document {
	p := &documentv1.Document{
		Id:             d.ID.String(),
		TenantId:       d.TenantID.String(),
		Name:           d.Name,
		MimeType:       d.MimeType,
		SizeBytes:      d.SizeBytes,
		StorageKey:     d.StorageKey,
		CurrentVersion: int32(d.CurrentVersion),
		UploadedBy:     d.UploadedBy.String(),
		CreatedAt:      timestamppb.New(d.CreatedAt),
	}
	if d.ProductionID != nil {
		p.ProductionId = d.ProductionID.String()
	}
	if d.FolderID != nil {
		p.FolderId = d.FolderID.String()
	}
	if d.DeletedAt != nil {
		p.DeletedAt = timestamppb.New(*d.DeletedAt)
	}
	return p
}

func versionToProto(v *DocumentVersion) *documentv1.DocumentVersion {
	return &documentv1.DocumentVersion{
		Id:         v.ID.String(),
		DocumentId: v.DocumentID.String(),
		Version:    int32(v.Version),
		StorageKey: v.StorageKey,
		SizeBytes:  v.SizeBytes,
		UploadedBy: v.UploadedBy.String(),
		CreatedAt:  timestamppb.New(v.CreatedAt),
	}
}

func folderToProto(f *Folder) *documentv1.Folder {
	p := &documentv1.Folder{
		Id:        f.ID.String(),
		TenantId:  f.TenantID.String(),
		Name:      f.Name,
		CreatedBy: f.CreatedBy.String(),
		CreatedAt: timestamppb.New(f.CreatedAt),
	}
	if f.ProductionID != nil {
		p.ProductionId = f.ProductionID.String()
	}
	if f.ParentID != nil {
		p.ParentId = f.ParentID.String()
	}
	return p
}

// grpcErr maps document domain errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrDocumentNotFound),
		errors.Is(err, ErrFolderNotFound),
		errors.Is(err, ErrVersionNotFound):
		return status.Error(codes.NotFound, err.Error())

	case errors.Is(err, ErrDocumentDeleted):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, ErrUploadNotConfirmed):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, ErrStorageKeyConflict):
		return status.Error(codes.AlreadyExists, err.Error())

	case errors.Is(err, ErrPresignFailed):
		return status.Error(codes.Internal, err.Error())

	default:
		return status.Error(codes.Internal, "internal error")
	}
}
