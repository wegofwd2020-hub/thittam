-- 001_create_tenants.up.sql
-- Core tenant table for multi-tenancy. Each tenant gets a schema `tenant_<uuid>`.

CREATE TABLE tenants (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    slug       TEXT        NOT NULL UNIQUE,
    plan       TEXT        NOT NULL DEFAULT 'starter'
                           CHECK (plan IN ('starter', 'professional', 'enterprise')),
    status     TEXT        NOT NULL DEFAULT 'active'
                           CHECK (status IN ('active', 'suspended', 'deactivated')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tenants_slug ON tenants (slug);
CREATE INDEX idx_tenants_status ON tenants (status) WHERE status = 'active';
