-- 004_create_platform_users.up.sql
-- Platform-level administration accounts (separate from tenant users).
-- These accounts manage tenants, verticals, and platform configuration.

CREATE TABLE platform_users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    display_name  TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL
                              CHECK (role IN ('platform_owner', 'platform_admin', 'platform_support')),
    mfa_secret    TEXT,       -- TOTP secret (encrypted); NULL = MFA not yet set up
    mfa_enabled   BOOLEAN     NOT NULL DEFAULT false,
    status        TEXT        NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'deactivated')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    UUID        REFERENCES platform_users(id),
    last_login_at TIMESTAMPTZ
);

CREATE INDEX idx_platform_users_email ON platform_users (email);
CREATE INDEX idx_platform_users_status ON platform_users (status) WHERE status = 'active';

-- Impersonation audit log
CREATE TABLE platform_impersonation_log (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_user_id  UUID        NOT NULL REFERENCES platform_users(id),
    tenant_id         UUID        NOT NULL REFERENCES tenants(id),
    impersonated_user UUID        NOT NULL REFERENCES users(id),
    reason            TEXT        NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at        TIMESTAMPTZ NOT NULL,
    ip_address        TEXT
);

CREATE INDEX idx_impersonation_platform_user ON platform_impersonation_log (platform_user_id);
CREATE INDEX idx_impersonation_tenant ON platform_impersonation_log (tenant_id);
