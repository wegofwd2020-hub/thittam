-- 023_seed_notifications_permissions.up.sql
-- #139 slice G: grant the two notifications permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change for new
-- tenants; both seed fixtures too. All three halves required (see #168).
--
-- Idempotent by necessity: migrations/iam runs against the public schema via
-- `make migrate-all` AND against every new tenant_<uuid> at CreateTenant.
-- is_system = true only.

UPDATE roles SET permissions = array_append(permissions, 'notifications:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('notifications:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'notifications:manage')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('notifications:manage' = ANY (permissions));
