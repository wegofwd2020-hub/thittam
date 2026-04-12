-- 010_add_impersonation_ended_at.down.sql
DROP INDEX IF EXISTS idx_impersonation_active_expired;
ALTER TABLE platform_impersonation_log DROP COLUMN IF EXISTS ended_at;
