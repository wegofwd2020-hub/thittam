-- 011_rename_system_roles.down.sql
-- Reverses 011_rename_system_roles.up.sql by restoring the old role names.
-- Note: department_head row is not recreated — if it was deleted in the up
-- migration, the seed (or iam.Service.seedSystemRoles) must re-insert it.

UPDATE roles SET name = 'executive_producer'
  WHERE name = 'manager' AND is_system = true;

UPDATE roles SET name = 'line_producer'
  WHERE name = 'coordinator' AND is_system = true;

UPDATE roles SET name = 'production_accountant'
  WHERE name = 'accountant' AND is_system = true;

UPDATE roles SET name = 'crew_member'
  WHERE name = 'member' AND is_system = true;
