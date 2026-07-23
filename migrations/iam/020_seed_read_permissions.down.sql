-- 020_seed_read_permissions.down.sql
-- Reverse of 020. expense:read is new to every role, so removing it everywhere
-- is correct.

UPDATE roles
SET permissions = array_remove(permissions, 'expense:read')
WHERE is_system = true;

-- inventory_manager held inventory:read BEFORE this migration. A blind
-- array_remove across all system roles would strip a grant this migration
-- never added, so that role is excluded.
UPDATE roles
SET permissions = array_remove(permissions, 'inventory:read')
WHERE is_system = true
  AND name <> 'inventory_manager';
