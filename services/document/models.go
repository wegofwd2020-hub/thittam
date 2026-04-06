// Package document implements the document service.
// It is a universal (non-vertical-aware) service that acts as a thin
// orchestration layer over an S3-compatible object store (MinIO / AWS S3).
// Large files are never routed through the service — clients upload and
// download directly via presigned URLs.
package document

import (
	"time"

	"github.com/google/uuid"
)

// Folder is a named container for organising documents within a tenant
// or production. Folders are hierarchical via the optional ParentID.
type Folder struct {
	ID           uuid.UUID  `json:"id"`
	TenantID     uuid.UUID  `json:"tenant_id"`
	ProductionID *uuid.UUID `json:"production_id,omitempty"`
	Name         string     `json:"name"`
	ParentID     *uuid.UUID `json:"parent_id,omitempty"`
	CreatedBy    uuid.UUID  `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Document is the metadata record for an uploaded file.
// SizeBytes = 0 means the upload has been initiated but not yet confirmed.
// DeletedAt non-nil means the document is soft-deleted; the object is purged
// from storage 30 days after deletion.
type Document struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenant_id"`
	ProductionID   *uuid.UUID `json:"production_id,omitempty"`
	FolderID       *uuid.UUID `json:"folder_id,omitempty"`
	Name           string     `json:"name"`
	MimeType       string     `json:"mime_type"`
	SizeBytes      int64      `json:"size_bytes"`
	StorageKey     string     `json:"storage_key"`
	CurrentVersion int        `json:"current_version"`
	UploadedBy     uuid.UUID  `json:"uploaded_by"`
	CreatedAt      time.Time  `json:"created_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// DocumentVersion is one revision of a document's content.
type DocumentVersion struct {
	ID         uuid.UUID `json:"id"`
	DocumentID uuid.UUID `json:"document_id"`
	Version    int       `json:"version"`
	StorageKey string    `json:"storage_key"`
	SizeBytes  int64     `json:"size_bytes"`
	UploadedBy uuid.UUID `json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// InitiateUploadRequest carries the metadata needed to reserve a document
// record and generate a presigned PUT URL.
type InitiateUploadRequest struct {
	TenantID     uuid.UUID
	ProductionID *uuid.UUID
	FolderID     *uuid.UUID
	Name         string
	MimeType     string
	UploadedBy   uuid.UUID
}

// UploadURL is returned by InitiateUpload and CreateVersion.
// The client PUTs the file bytes directly to URL, then calls ConfirmUpload.
type UploadURL struct {
	DocumentID uuid.UUID `json:"document_id"`
	URL        string    `json:"url"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// DownloadURL is a short-lived presigned GET URL (15-minute TTL).
type DownloadURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
