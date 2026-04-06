-- 001_create_folders.up.sql
-- Hierarchical folder tree per tenant. Folders may be scoped to a production
-- (production_id IS NOT NULL) or tenant-global (production_id IS NULL).

CREATE TABLE folders (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL,
    production_id UUID,
    name          TEXT        NOT NULL,
    parent_id     UUID        REFERENCES folders(id) ON DELETE CASCADE,
    created_by    UUID        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_folders_tenant     ON folders (tenant_id);
CREATE INDEX idx_folders_production ON folders (tenant_id, production_id) WHERE production_id IS NOT NULL;
CREATE INDEX idx_folders_parent     ON folders (parent_id) WHERE parent_id IS NOT NULL;
