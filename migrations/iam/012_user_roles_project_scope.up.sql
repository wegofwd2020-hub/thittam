-- 012_user_roles_project_scope.up.sql
-- ADR-014 Phase 2: project-scoped role assignments.
--
-- A NULL project_id means tenant-wide assignment (existing behaviour).
-- A non-NULL project_id scopes the role to a single production.
--
-- Schema decisions:
--   * No FK on project_id — productions live in per-tenant schemas (tenant_<uuid>),
--     user_roles lives in the shared schema. Cross-schema FK is not possible.
--     Referential integrity is enforced at the application layer.
--   * The (user_id, role_id) primary key is replaced with a unique index that
--     treats NULL project_id as a sentinel value, so a user can hold the same
--     role both tenant-wide and on multiple distinct projects without collision.
--   * Role-scope rules (project_supervisor must be scoped, super_admin must not
--     be scoped, etc.) are enforced via a trigger function — PostgreSQL CHECK
--     constraints cannot reference other tables.

ALTER TABLE user_roles
    ADD COLUMN IF NOT EXISTS project_id UUID;

-- Replace composite PK so the same role can be granted to a user on multiple
-- projects (and tenant-wide simultaneously). NULL project_id is folded into a
-- sentinel UUID so the unique index treats "tenant-wide" as a single bucket.
ALTER TABLE user_roles DROP CONSTRAINT IF EXISTS user_roles_pkey;

CREATE UNIQUE INDEX IF NOT EXISTS user_roles_unique
    ON user_roles (user_id, role_id, COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE INDEX IF NOT EXISTS idx_user_roles_user_project
    ON user_roles (user_id, project_id)
    WHERE project_id IS NOT NULL;

-- Trigger enforces the role-scope contract:
--   * project_supervisor MUST have a project_id
--   * super_admin, manager, coordinator, accountant, inventory_manager, member
--     MUST NOT have a project_id
-- The list mirrors services/iam.systemRoles. Custom roles (is_system = false)
-- are unconstrained — tenants may scope them however they wish.
CREATE OR REPLACE FUNCTION enforce_user_role_scope()
RETURNS TRIGGER AS $$
DECLARE
    role_name TEXT;
    role_is_system BOOLEAN;
BEGIN
    SELECT name, is_system INTO role_name, role_is_system
    FROM roles WHERE id = NEW.role_id;

    IF NOT role_is_system THEN
        RETURN NEW;
    END IF;

    IF role_name = 'project_supervisor' AND NEW.project_id IS NULL THEN
        RAISE EXCEPTION 'role % must be project-scoped (project_id required)', role_name
            USING ERRCODE = 'check_violation';
    END IF;

    IF role_name IN ('super_admin','manager','coordinator','accountant','inventory_manager','member')
       AND NEW.project_id IS NOT NULL THEN
        RAISE EXCEPTION 'role % is tenant-wide and cannot carry a project_id', role_name
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_user_roles_enforce_scope ON user_roles;
CREATE TRIGGER trg_user_roles_enforce_scope
    BEFORE INSERT OR UPDATE ON user_roles
    FOR EACH ROW EXECUTE FUNCTION enforce_user_role_scope();
