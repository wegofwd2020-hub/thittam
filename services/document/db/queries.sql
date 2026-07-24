-- Document service queries.

-- name: CreateFolder :one
INSERT INTO folders (id, tenant_id, production_id, name, parent_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetFolder :one
SELECT * FROM folders WHERE id = $1 AND tenant_id = $2;

-- name: CreateDocument :one
INSERT INTO documents (id, tenant_id, production_id, folder_id, name, mime_type, size_bytes, storage_key, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetDocument :one
SELECT * FROM documents WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: SoftDeleteDocument :exec
UPDATE documents SET deleted_at = now() WHERE id = $1 AND tenant_id = $2;

-- name: CreateDocumentVersion :one
INSERT INTO document_versions (id, document_id, version, storage_key, size_bytes, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListDocumentVersions :many
SELECT * FROM document_versions
WHERE document_id = $1
ORDER BY version ASC
LIMIT $2 OFFSET $3;

-- name: GetDocumentVersion :one
SELECT * FROM document_versions WHERE document_id = $1 AND version = $2;
