-- 023_seed_notifications_permissions.down.sql
-- Reverse of 023. Both strings are new to every role this migration touches,
-- so removing each unconditionally across is_system roles is correct.

UPDATE roles SET permissions = array_remove(permissions, 'notifications:read')   WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'notifications:manage') WHERE is_system = true;
