-- 022_seed_billing_permissions.down.sql
-- Reverse of 022. Both strings are new to every role this migration touches,
-- so removing each unconditionally across is_system roles is correct -- no
-- pre-existing grant to preserve (unlike slice D's inventory:read).

UPDATE roles SET permissions = array_remove(permissions, 'billing:read')   WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'billing:manage') WHERE is_system = true;
