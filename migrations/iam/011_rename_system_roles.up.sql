-- 011_rename_system_roles.up.sql
-- ADR-014 Phase 1: rename movie-production-specific system role names to generic
-- industry-agnostic names. Display labels are resolved per-tenant via the
-- vertical YAML role_labels map (see pkg/vertical).
--
-- Idempotent: each UPDATE matches only the old name. Re-running after success
-- is a no-op because the WHERE no longer matches anything.
--
-- Rollback: see 011_rename_system_roles.down.sql (reverse UPDATEs).

UPDATE roles SET name = 'manager'
  WHERE name = 'executive_producer' AND is_system = true;

UPDATE roles SET name = 'coordinator'
  WHERE name = 'line_producer' AND is_system = true;

UPDATE roles SET name = 'accountant'
  WHERE name = 'production_accountant' AND is_system = true;

UPDATE roles SET name = 'member'
  WHERE name = 'crew_member' AND is_system = true;

-- Remove department_head only if no users still reference it. Users that held
-- this role must be reassigned to project_supervisor (or another role) by an
-- application-level migration before this row can be safely deleted.
-- The pre-flight check below aborts the migration with a clear error if
-- assignments remain — the partial migration above is still safe and
-- idempotent.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM user_roles ur
    JOIN roles r ON r.id = ur.role_id
    WHERE r.name = 'department_head' AND r.is_system = true
  ) THEN
    RAISE EXCEPTION 'department_head still has user assignments — reassign users to project_supervisor before running this migration';
  END IF;

  DELETE FROM roles WHERE name = 'department_head' AND is_system = true;
END $$;
