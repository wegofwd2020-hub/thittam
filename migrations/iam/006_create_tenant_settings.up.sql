-- 006_create_tenant_settings.up.sql
-- Stores per-tenant UI theme overrides (colors, logo, branding).
-- Tenants without a row use the vertical's default theme.

CREATE TABLE tenant_settings (
    tenant_id       UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    theme_override  JSONB,       -- color overrides, logo URLs, typography
    logo_url        TEXT,        -- uploaded tenant logo
    favicon_url     TEXT,        -- custom favicon
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID
);
