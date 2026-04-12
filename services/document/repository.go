package document

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines all data access required by the document service.
type Repository interface {
	// Folders
	CreateFolder(ctx context.Context, folder *Folder) error
	GetFolder(ctx context.Context, tenantID, id uuid.UUID) (*Folder, error)
	ListFolders(ctx context.Context, tenantID uuid.UUID, productionID *uuid.UUID, limit, offset int) ([]Folder, error)

	// Documents
	CreateDocument(ctx context.Context, doc *Document) error
	GetDocument(ctx context.Context, tenantID, id uuid.UUID) (*Document, error)
	ListDocuments(ctx context.Context, tenantID uuid.UUID, productionID, folderID *uuid.UUID, limit, offset int) ([]Document, error)
	UpdateDocument(ctx context.Context, doc *Document) error
	SoftDeleteDocument(ctx context.Context, tenantID, id uuid.UUID, deletedAt time.Time) error

	// Versions
	CreateVersion(ctx context.Context, v *DocumentVersion) error
	GetVersion(ctx context.Context, documentID uuid.UUID, version int) (*DocumentVersion, error)
	ListVersions(ctx context.Context, documentID uuid.UUID, limit, offset int) ([]DocumentVersion, error)
}
