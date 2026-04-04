-- 002_create_users.up.sql
-- Users table. Each user belongs to exactly one tenant.
-- The first user created during registration is granted super_admin role.

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email         TEXT        NOT NULL,
    display_name  TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'active'
                              CHECK (status IN ('active', 'invited', 'deactivated')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, email)
);

CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_tenant ON users (tenant_id);
