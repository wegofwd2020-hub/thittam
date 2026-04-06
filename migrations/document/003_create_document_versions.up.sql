-- 003_create_document_versions.up.sql
-- One row per uploaded version of a document. Version 1 is created when
-- ConfirmUpload is called. Subsequent versions are added via CreateVersion +
-- ConfirmVersion. RestoreVersion updates documents.current_version to point
-- to a previous version's storage_key without deleting newer versions.

CREATE TABLE document_versions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID        NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version     INT         NOT NULL,
    storage_key TEXT        NOT NULL,
    size_bytes  BIGINT      NOT NULL,
    uploaded_by UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (document_id, version)
);

CREATE INDEX idx_doc_versions_document ON document_versions (document_id);
