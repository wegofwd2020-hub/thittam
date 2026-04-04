-- 005_add_is_demo_to_tenants.up.sql
-- Flags demo tenants for easy identification and bulk cleanup.

ALTER TABLE tenants ADD COLUMN is_demo BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX idx_tenants_demo ON tenants (is_demo) WHERE is_demo = true;
