-- 003_create_tenant_auth_config.up.sql
-- Stores per-tenant OIDC configuration. Tenants without a row use local (email/password) auth.

CREATE TABLE tenant_auth_config (
    tenant_id          UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    auth_method        TEXT NOT NULL DEFAULT 'local'
                       CHECK (auth_method IN ('local', 'oidc')),
    issuer_url         TEXT,          -- OIDC issuer URL (e.g., https://accounts.google.com)
    client_id          TEXT,          -- OAuth2 client ID
    client_secret_enc  TEXT,          -- Encrypted client secret (AES-256-GCM)
    scopes             TEXT[] NOT NULL DEFAULT ARRAY['openid', 'email', 'profile'],
    email_claim        TEXT NOT NULL DEFAULT 'email',
    display_name_claim TEXT NOT NULL DEFAULT 'name',
    groups_claim       TEXT,          -- Optional: IdP groups claim for role mapping
    auto_provision     BOOLEAN NOT NULL DEFAULT true,
    default_role       TEXT NOT NULL DEFAULT 'crew_member',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- OIDC fields are required when auth_method = 'oidc'
    CONSTRAINT oidc_fields_required CHECK (
        auth_method = 'local' OR (
            issuer_url IS NOT NULL AND
            client_id IS NOT NULL AND
            client_secret_enc IS NOT NULL
        )
    )
);

CREATE TRIGGER trg_tenant_auth_config_updated_at
    BEFORE UPDATE ON tenant_auth_config
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
