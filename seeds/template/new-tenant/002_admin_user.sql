-- 002_admin_user.sql — first admin user for <TENANT_NAME>
--
-- Creates one user and grants them the super_admin role. The role UUID
-- is resolved dynamically against the roles seeded in 001_tenant.sql —
-- this seed works against any tenant UUID without editing a hardcoded
-- role ID.
--
-- Password hash: generate ONCE outside this file. See the README for
-- the bcrypt invocation. Never commit plaintext passwords.

-- ──────────────────────────────────────────────────────────────────
-- 1. Admin user
-- ──────────────────────────────────────────────────────────────────

INSERT INTO users (id, tenant_id, email, display_name, password_hash, status)
VALUES (
    '<ADMIN_USER_UUID>',
    '<TENANT_UUID>',
    '<ADMIN_EMAIL>',
    '<ADMIN_DISPLAY_NAME>',
    '<BCRYPT_PASSWORD_HASH>',
    'active'
)
ON CONFLICT (id) DO NOTHING;

-- ──────────────────────────────────────────────────────────────────
-- 2. Grant super_admin (tenant-wide; project_id = NULL)
-- ──────────────────────────────────────────────────────────────────
-- assigned_by = the admin themselves. During platform-led provisioning
-- there is no other user to credit the assignment to; self-assignment
-- is the convention used by the xyz-cba seed as well.

INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
SELECT
    '<ADMIN_USER_UUID>',
    r.id,
    NULL,
    '<ADMIN_USER_UUID>'
  FROM roles r
 WHERE r.tenant_id = '<TENANT_UUID>'
   AND r.name      = 'super_admin'
ON CONFLICT (user_id, role_id) DO NOTHING;
