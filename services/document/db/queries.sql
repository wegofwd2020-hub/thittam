-- Document service queries.

-- name: CreateFolder :one
INSERT INTO folders (id, tenant_id, production_id, name, parent_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetFolder :one
SELECT * FROM folders WHERE id = $1 AND tenant_id = $2;

-- name: ListFolders :many
SELECT * FROM folders
WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR production_id = $2)
  AND ($3::uuid IS NULL OR parent_id = $3)
ORDER BY name ASC;

-- name: CreateDocument :one
INSERT INTO documents (id, tenant_id, production_id, folder_id, name, mime_type, size_bytes, storage_key, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: ListDocuments :many
SELECT * FROM documents
WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR production_id = $2)
  AND ($3::uuid IS NULL OR folder_id = $3)
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: UpdateDocumentSize :exec
UPDATE documents SET size_bytes = $2 WHERE id = $1;

-- name: IncrementDocumentVersion :one
UPDATE documents SET current_version = current_version + 1 WHERE id = $1 RETURNING *;

-- name: SoftDeleteDocument :exec
UPDATE documents SET deleted_at = now() WHERE id = $1 AND tenant_id = $2;

-- name: CreateDocumentVersion :one
INSERT INTO document_versions (id, document_id, version, storage_key, size_bytes, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListDocumentVersions :many
SELECT * FROM document_versions
WHERE document_id = $1
ORDER BY version ASC;

-- name: GetDocumentVersion :one
SELECT * FROM document_versions WHERE document_id = $1 AND version = $2;
