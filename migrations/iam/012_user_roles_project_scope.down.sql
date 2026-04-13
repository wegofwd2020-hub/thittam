-- 012_user_roles_project_scope.down.sql
-- Reverses 012_user_roles_project_scope.up.sql.
-- Project-scoped assignments (project_id IS NOT NULL) are dropped because the
-- restored composite PK cannot represent them.

DROP TRIGGER IF EXISTS trg_user_roles_enforce_scope ON user_roles;
DROP FUNCTION IF EXISTS enforce_user_role_scope();

DELETE FROM user_roles WHERE project_id IS NOT NULL;

DROP INDEX IF EXISTS user_roles_unique;
DROP INDEX IF EXISTS idx_user_roles_user_project;

ALTER TABLE user_roles DROP COLUMN IF EXISTS project_id;

ALTER TABLE user_roles ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);
