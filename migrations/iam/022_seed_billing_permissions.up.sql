-- 022_seed_billing_permissions.up.sql
-- #139 slice F: grant the two billing permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change for new
-- tenants; both seed fixtures too. All three halves required (see #168).
--
-- Idempotent by necessity: migrations/iam runs against the public schema via
-- `make migrate-all` AND against every new tenant_<uuid> at CreateTenant.
-- is_system = true only.

UPDATE roles SET permissions = array_append(permissions, 'billing:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'accountant')
  AND NOT ('billing:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'billing:manage')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('billing:manage' = ANY (permissions));
