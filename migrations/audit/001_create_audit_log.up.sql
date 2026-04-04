-- 001_create_audit_log.up.sql
-- Append-only audit log for all security-sensitive and financial operations.
-- The application role has NO UPDATE or DELETE privileges on this table.

CREATE TABLE audit_log (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL,
    actor_id      UUID        NOT NULL,
    actor_email   TEXT        NOT NULL,
    actor_ip      TEXT,
    action        TEXT        NOT NULL,
    resource_type TEXT        NOT NULL,
    resource_id   UUID        NOT NULL,
    production_id UUID,
    old_state     JSONB,
    new_state     JSONB,
    metadata      JSONB,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Query patterns: by tenant+resource, by actor, by time range
CREATE INDEX idx_audit_tenant_resource ON audit_log (tenant_id, resource_type, resource_id);
CREATE INDEX idx_audit_actor ON audit_log (tenant_id, actor_id, occurred_at DESC);
CREATE INDEX idx_audit_time ON audit_log (tenant_id, occurred_at DESC);
CREATE INDEX idx_audit_action ON audit_log (tenant_id, action, occurred_at DESC);

-- Append-only enforcement: revoke UPDATE and DELETE from the application role.
-- Run this AFTER creating the role. Uncomment and adjust role name for your environment.
-- REVOKE UPDATE, DELETE ON audit_log FROM thittam_app;
