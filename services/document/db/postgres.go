package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wegofwd2020/thittam/services/document"
)

// Postgres implements document.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed document repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ document.Repository = (*Postgres)(nil)

// --- Folders ---

func (p *Postgres) CreateFolder(ctx context.Context, f *document.Folder) error {
	row, err := p.q.CreateFolder(ctx, CreateFolderParams{
		ID:           f.ID,
		TenantID:     f.TenantID,
		ProductionID: uuidToPgUUID(f.ProductionID),
		Name:         f.Name,
		ParentID:     uuidToPgUUID(f.ParentID),
		CreatedBy:    f.CreatedBy,
	})
	if err != nil {
		return fmt.Errorf("document: create folder: %w", err)
	}
	*f = folderFromDB(row)
	return nil
}

func (p *Postgres) GetFolder(ctx context.Context, tenantID, id uuid.UUID) (*document.Folder, error) {
	row, err := p.q.GetFolder(ctx, GetFolderParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, document.ErrFolderNotFound
		}
		return nil, fmt.Errorf("document: get folder %s: %w", id, err)
	}
	f := folderFromDB(row)
	return &f, nil
}

func (p *Postgres) ListFolders(ctx context.Context, tenantID uuid.UUID, productionID *uuid.UUID, limit, offset int) ([]document.Folder, error) {
	const rawSQL = `SELECT id, tenant_id, production_id, name, parent_id, created_by, created_at
		FROM folders
		WHERE tenant_id = $1
		  AND ($2::uuid IS NULL OR production_id = $2)
		ORDER BY name ASC
		LIMIT $3 OFFSET $4`
	rows, err := p.db.Query(ctx, rawSQL, tenantID, productionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("document: list folders: %w", err)
	}
	defer rows.Close()

	var out []document.Folder
	for rows.Next() {
		var r Folder
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ProductionID, &r.Name, &r.ParentID, &r.CreatedBy, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("document: list folders scan: %w", err)
		}
		out = append(out, folderFromDB(r))
	}
	return out, rows.Err()
}

// --- Documents ---

func (p *Postgres) CreateDocument(ctx context.Context, doc *document.Document) error {
	row, err := p.q.CreateDocument(ctx, CreateDocumentParams{
		ID:           doc.ID,
		TenantID:     doc.TenantID,
		ProductionID: uuidToPgUUID(doc.ProductionID),
		FolderID:     uuidToPgUUID(doc.FolderID),
		Name:         doc.Name,
		MimeType:     doc.MimeType,
		SizeBytes:    doc.SizeBytes,
		StorageKey:   doc.StorageKey,
		UploadedBy:   doc.UploadedBy,
	})
	if err != nil {
		return fmt.Errorf("document: create document: %w", err)
	}
	*doc = docFromDB(row)
	return nil
}

func (p *Postgres) GetDocument(ctx context.Context, tenantID, id uuid.UUID) (*document.Document, error) {
	row, err := p.q.GetDocument(ctx, GetDocumentParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, document.ErrDocumentNotFound
		}
		return nil, fmt.Errorf("document: get document %s: %w", id, err)
	}
	d := docFromDB(row)
	return &d, nil
}

func (p *Postgres) ListDocuments(ctx context.Context, tenantID uuid.UUID, productionID, folderID *uuid.UUID, limit, offset int) ([]document.Document, error) {
	const sql = `SELECT id, tenant_id, production_id, folder_id, name, mime_type, size_bytes,
		storage_key, current_version, uploaded_by, created_at, deleted_at
		FROM documents
		WHERE tenant_id = $1
		  AND ($2::uuid IS NULL OR production_id = $2)
		  AND ($3::uuid IS NULL OR folder_id = $3)
		  AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`
	rows, err := p.db.Query(ctx, sql, tenantID, productionID, folderID, int32(limit), int32(offset))
	if err != nil {
		return nil, fmt.Errorf("document: list documents: %w", err)
	}
	defer rows.Close()

	var out []document.Document
	for rows.Next() {
		var r Document
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.ProductionID, &r.FolderID,
			&r.Name, &r.MimeType, &r.SizeBytes, &r.StorageKey,
			&r.CurrentVersion, &r.UploadedBy, &r.CreatedAt, &r.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("document: list documents scan: %w", err)
		}
		out = append(out, docFromDB(r))
	}
	return out, rows.Err()
}

// UpdateDocument applies all mutable fields: size_bytes, folder_id,
// storage_key, and current_version. Used after ConfirmUpload, MoveDocument,
// ConfirmVersion, and RestoreVersion.
func (p *Postgres) UpdateDocument(ctx context.Context, doc *document.Document) error {
	const sql = `UPDATE documents
		SET size_bytes = $3, folder_id = $4, storage_key = $5, current_version = $6
		WHERE id = $1 AND tenant_id = $2`
	tag, err := p.db.Exec(ctx, sql,
		doc.ID,
		doc.TenantID,
		doc.SizeBytes,
		uuidToPgUUID(doc.FolderID),
		doc.StorageKey,
		doc.CurrentVersion,
	)
	if err != nil {
		return fmt.Errorf("document: update document %s: %w", doc.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return document.ErrDocumentNotFound
	}
	return nil
}

func (p *Postgres) SoftDeleteDocument(ctx context.Context, tenantID, id uuid.UUID, deletedAt time.Time) error {
	return p.q.SoftDeleteDocument(ctx, SoftDeleteDocumentParams{ID: id, TenantID: tenantID})
}

// --- Versions ---

func (p *Postgres) CreateVersion(ctx context.Context, v *document.DocumentVersion) error {
	row, err := p.q.CreateDocumentVersion(ctx, CreateDocumentVersionParams{
		ID:         v.ID,
		DocumentID: v.DocumentID,
		Version:    int32(v.Version),
		StorageKey: v.StorageKey,
		SizeBytes:  v.SizeBytes,
		UploadedBy: v.UploadedBy,
	})
	if err != nil {
		return fmt.Errorf("document: create version: %w", err)
	}
	*v = versionFromDB(row)
	return nil
}

func (p *Postgres) GetVersion(ctx context.Context, documentID uuid.UUID, version int) (*document.DocumentVersion, error) {
	row, err := p.q.GetDocumentVersion(ctx, GetDocumentVersionParams{
		DocumentID: documentID,
		Version:    int32(version),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, document.ErrVersionNotFound
		}
		return nil, fmt.Errorf("document: get version %d: %w", version, err)
	}
	v := versionFromDB(row)
	return &v, nil
}

func (p *Postgres) ListVersions(ctx context.Context, documentID uuid.UUID, limit, offset int) ([]document.DocumentVersion, error) {
	rows, err := p.q.ListDocumentVersions(ctx, ListDocumentVersionsParams{
		DocumentID: documentID,
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("document: list versions: %w", err)
	}
	out := make([]document.DocumentVersion, len(rows))
	for i, r := range rows {
		out[i] = versionFromDB(r)
	}
	return out, nil
}

// --- DB → domain converters ---

func docFromDB(r Document) document.Document {
	d := document.Document{
		ID:             r.ID,
		TenantID:       r.TenantID,
		Name:           r.Name,
		MimeType:       r.MimeType,
		SizeBytes:      r.SizeBytes,
		StorageKey:     r.StorageKey,
		CurrentVersion: int(r.CurrentVersion),
		UploadedBy:     r.UploadedBy,
		CreatedAt:      r.CreatedAt,
	}
	if r.ProductionID.Valid {
		id := r.ProductionID.Bytes
		uid := uuid.UUID(id)
		d.ProductionID = &uid
	}
	if r.FolderID.Valid {
		id := r.FolderID.Bytes
		uid := uuid.UUID(id)
		d.FolderID = &uid
	}
	if r.DeletedAt.Valid {
		t := r.DeletedAt.Time
		d.DeletedAt = &t
	}
	return d
}

func versionFromDB(r DocumentVersion) document.DocumentVersion {
	return document.DocumentVersion{
		ID:         r.ID,
		DocumentID: r.DocumentID,
		Version:    int(r.Version),
		StorageKey: r.StorageKey,
		SizeBytes:  r.SizeBytes,
		UploadedBy: r.UploadedBy,
		CreatedAt:  r.CreatedAt,
	}
}

func folderFromDB(r Folder) document.Folder {
	f := document.Folder{
		ID:        r.ID,
		TenantID:  r.TenantID,
		Name:      r.Name,
		CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt,
	}
	if r.ProductionID.Valid {
		id := uuid.UUID(r.ProductionID.Bytes)
		f.ProductionID = &id
	}
	if r.ParentID.Valid {
		id := uuid.UUID(r.ParentID.Bytes)
		f.ParentID = &id
	}
	return f
}

// --- Helpers ---

func uuidToPgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

