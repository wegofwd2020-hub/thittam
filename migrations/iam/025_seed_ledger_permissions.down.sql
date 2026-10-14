-- 025_seed_ledger_permissions.down.sql
UPDATE roles SET permissions = array_remove(permissions, 'ledger:read')
WHERE is_system = true;

UPDATE roles SET permissions = array_remove(permissions, 'ledger:write')
WHERE is_system = true;

UPDATE roles SET permissions = array_remove(permissions, 'ledger:post')
WHERE is_system = true;

UPDATE roles SET permissions = array_remove(permissions, 'ledger:admin')
WHERE is_system = true;
