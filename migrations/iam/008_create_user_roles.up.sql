-- 008_create_user_roles.up.sql
-- Maps users to roles. A user can hold multiple roles within their tenant.
-- The composite PK prevents duplicate assignments; ON CONFLICT is safe to use
-- for idempotent role grants.

CREATE TABLE user_roles (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     UUID        NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_by UUID        NOT NULL REFERENCES users(id),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_user   ON user_roles (user_id);
CREATE INDEX idx_user_roles_role   ON user_roles (role_id);
