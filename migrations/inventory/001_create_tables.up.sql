-- 001_create_tables.up.sql — inventory-management service tables
-- These tables live in the tenant-scoped schema (tenant_<uuid>).

CREATE TABLE assets (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL,
    asset_code      TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    category_id     TEXT        NOT NULL,
    description     TEXT,
    ownership_type  TEXT        NOT NULL DEFAULT 'owned'
                                CHECK (ownership_type IN ('owned','rented','borrowed')),
    status          TEXT        NOT NULL DEFAULT 'available'
                                CHECK (status IN ('available','checked_out','under_repair','retired')),
    purchase_date   DATE,
    purchase_cost   NUMERIC(14,2),
    serial_number   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, asset_code)
);

CREATE INDEX idx_assets_tenant ON assets (tenant_id);
CREATE INDEX idx_assets_status ON assets (tenant_id, status);
CREATE INDEX idx_assets_category ON assets (tenant_id, category_id);

CREATE TABLE asset_checkouts (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id         UUID        NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    production_id    UUID        NOT NULL,
    tenant_id        UUID        NOT NULL,
    checked_out_to   UUID        NOT NULL,
    checked_out_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expected_return  DATE,
    checked_in_at    TIMESTAMPTZ,
    condition_out    TEXT,
    condition_in     TEXT
);

CREATE INDEX idx_checkouts_asset ON asset_checkouts (asset_id);
CREATE INDEX idx_checkouts_production ON asset_checkouts (production_id);
CREATE INDEX idx_checkouts_tenant ON asset_checkouts (tenant_id);
