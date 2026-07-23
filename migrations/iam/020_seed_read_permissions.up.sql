-- 020_seed_read_permissions.up.sql
-- #139 slice D: grant the two read permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change so NEW
-- tenants receive these at seedSystemRoles time. This migration covers the
-- tenants that already exist. Both halves are required.
--
-- Idempotent by necessity, not politeness: migrations/iam runs against the
-- public schema via `make migrate-all` AND against every new tenant_<uuid>
-- schema at CreateTenant, so these statements execute in more than one
-- context. The NOT (... = ANY (permissions)) guard makes a re-run a no-op.
--
-- is_system = true only: custom roles are a tenant's own business.

UPDATE roles
SET permissions = array_append(permissions, 'expense:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('expense:read' = ANY (permissions));

-- inventory:read already existed but was granted to inventory_manager alone —
-- not even super_admin — so gating the inventory reads would have locked out
-- every role that can check an asset out. Widen it to exactly the roles that
-- already hold inventory:checkout.
UPDATE roles
SET permissions = array_append(permissions, 'inventory:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'project_supervisor')
  AND NOT ('inventory:read' = ANY (permissions));
