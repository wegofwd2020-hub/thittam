-- 013_update_oidc_default_role.down.sql
-- Restore the pre-ADR-014 default role string.

UPDATE tenant_auth_config
   SET default_role = 'crew_member'
 WHERE default_role = 'member';

ALTER TABLE tenant_auth_config
    ALTER COLUMN default_role SET DEFAULT 'crew_member';
