-- 021_seed_document_permissions.down.sql
-- Reverse of 021. All three strings are new to every role this migration
-- touches (unlike slice D's inventory:read, which pre-existed on
-- inventory_manager), so removing each unconditionally across is_system roles
-- is correct — there is no pre-existing grant to preserve.

UPDATE roles SET permissions = array_remove(permissions, 'document:read')   WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'document:write')  WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'document:delete') WHERE is_system = true;
