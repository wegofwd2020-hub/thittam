-- 002_create_documents.up.sql
-- Document records. size_bytes = 0 means the upload has been initiated but not
-- yet confirmed. The storage_key follows the pattern:
--   {tenant_id}/{production_id_or_shared}/{document_id}/v{version}/{filename}
--
-- deleted_at is a soft-delete timestamp. A background job purges the object from
-- storage 30 days after deletion.

CREATE TABLE documents (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL,
    production_id   UUID,
    folder_id       UUID        REFERENCES folders(id) ON DELETE SET NULL,
    name            TEXT        NOT NULL,
    mime_type       TEXT        NOT NULL,
    size_bytes      BIGINT      NOT NULL DEFAULT 0,
    storage_key     TEXT        NOT NULL UNIQUE,
    current_version INT         NOT NULL DEFAULT 1,
    uploaded_by     UUID        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_docs_tenant     ON documents (tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_docs_production ON documents (tenant_id, production_id) WHERE production_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_docs_folder     ON documents (folder_id) WHERE folder_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_docs_deleted    ON documents (deleted_at) WHERE deleted_at IS NOT NULL;
