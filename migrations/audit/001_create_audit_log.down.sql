-- 001_create_audit_log.down.sql
-- WARNING: Dropping audit_log destroys compliance records. Use only in dev/test.

DROP INDEX IF EXISTS idx_audit_action;
DROP INDEX IF EXISTS idx_audit_time;
DROP INDEX IF EXISTS idx_audit_actor;
DROP INDEX IF EXISTS idx_audit_tenant_resource;
DROP TABLE IF EXISTS audit_log;
