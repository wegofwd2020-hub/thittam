-- 025_seed_ledger_permissions.up.sql
-- Grant the ledger permissions to existing tenants' system roles.
--
-- systemRoles (services/iam/service.go) already grants these to new tenants.
-- Tenants created before ledger:* existed have roles rows without them, so the
-- Ledger REST API rejects their users with "missing required permission".
--
-- Mirrors systemRoles exactly:
--   super_admin: ledger:read, ledger:write, ledger:post, ledger:admin
--   manager, coordinator: ledger:read
--   accountant: ledger:read, ledger:write, ledger:post
--
-- Idempotent: each grant is applied only when the permission is missing.
-- is_system = true only.

UPDATE roles SET permissions = array_append(permissions, 'ledger:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant')
  AND NOT ('ledger:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'ledger:write')
WHERE is_system = true
  AND name IN ('super_admin', 'accountant')
  AND NOT ('ledger:write' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'ledger:post')
WHERE is_system = true
  AND name IN ('super_admin', 'accountant')
  AND NOT ('ledger:post' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'ledger:admin')
WHERE is_system = true
  AND name = 'super_admin'
  AND NOT ('ledger:admin' = ANY (permissions));
