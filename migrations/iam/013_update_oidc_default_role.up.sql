-- 013_update_oidc_default_role.up.sql
-- ADR-014 Phase 1 follow-up (#45): OIDC default_role was 'crew_member'; the
-- role was renamed to 'member' in migration 011. Existing tenant_auth_config
-- rows still carry the old value, and the column DEFAULT clause was wrong
-- until migration 003 was edited in place (only affects fresh installs).
--
-- This migration brings existing rows into line and locks the new DEFAULT.
-- Idempotent — safe to re-run.

UPDATE tenant_auth_config
   SET default_role = 'member'
 WHERE default_role = 'crew_member';

ALTER TABLE tenant_auth_config
    ALTER COLUMN default_role SET DEFAULT 'member';
