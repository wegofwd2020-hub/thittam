-- 021_seed_document_permissions.up.sql
-- #139 slice E: grant the three document permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change so NEW
-- tenants receive these at seedSystemRoles time; the two seed fixtures are
-- updated too. All three halves are required (see #168's review).
--
-- Idempotent by necessity: migrations/iam runs against the public schema via
-- `make migrate-all` AND against every new tenant_<uuid> at CreateTenant.
--
-- is_system = true only: custom roles are a tenant's own business.
-- document:read is granted to ALL seven roles: billing.DownloadInvoice
-- forwards the caller's token to document.GetDownloadURL, so narrowing reads
-- would break invoice download (#139 slice E spec §4).

UPDATE roles SET permissions = array_append(permissions, 'document:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'member', 'inventory_manager', 'project_supervisor')
  AND NOT ('document:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'document:write')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('document:write' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'document:delete')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('document:delete' = ANY (permissions));
