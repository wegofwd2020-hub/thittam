-- IAM service queries.
-- tenants, users, roles, user_roles, invitations live in the shared (public) schema.
-- platform_users is the internal admin user table.

-- name: CreateTenant :one
INSERT INTO tenants (
    id, name, slug, plan, status,
    address_line1, address_line2, city, country_code, postal_code, primary_currency_code
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (slug) DO NOTHING
RETURNING *;

-- name: UpdateTenantAddress :one
UPDATE tenants SET
    address_line1 = $2,
    address_line2 = $3,
    city = $4,
    country_code = $5,
    postal_code = $6,
    primary_currency_code = $7
WHERE id = $1
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: GetTenantBySlug :one
SELECT * FROM tenants WHERE slug = $1;

-- name: UpdateTenantStatus :one
-- Sets the tenant's status and stamps the appropriate lifecycle timestamp
-- on first entry (#92). Repeat calls preserve the original timestamp so
-- the grace / deactivation clocks keep ticking from the real transition.
UPDATE tenants SET
    status         = $2,
    suspended_at   = CASE WHEN $2 = 'suspended'   AND suspended_at   IS NULL THEN now() ELSE suspended_at   END,
    deactivated_at = CASE WHEN $2 = 'deactivated' AND deactivated_at IS NULL THEN now() ELSE deactivated_at END
WHERE id = $1
RETURNING *;

-- name: TransitionTenantStatus :one
-- Idempotent lifecycle transition for the retention sweeper. The WHERE
-- clause guards against concurrent sweepers double-advancing a tenant;
-- callers that get ID = uuid.Nil in the returned row know someone else
-- moved the tenant first (#92).
UPDATE tenants SET
    status         = @to_status,
    suspended_at   = CASE WHEN @to_status::text = 'suspended'   AND suspended_at   IS NULL THEN now() ELSE suspended_at   END,
    deactivated_at = CASE WHEN @to_status::text = 'deactivated' AND deactivated_at IS NULL THEN now() ELSE deactivated_at END
WHERE id = @id AND status = @from_status
RETURNING *;

-- name: ListTenantsDueForLifecycle :many
-- Surfaces tenants whose next automatic lifecycle transition is due at or
-- before $1 (the sweeper's "now"). Each WHERE branch is backed by a
-- partial index created in migration 016, so the query is cheap even
-- with many active tenants.
SELECT * FROM tenants
 WHERE (status = 'suspended'   AND suspended_at   <= $1 - INTERVAL '30 days')
    OR (status = 'grace'       AND suspended_at   <= $1 - INTERVAL '90 days')
    OR (status = 'deactivated' AND deactivated_at <= $1 - INTERVAL '180 days')
 ORDER BY COALESCE(deactivated_at, suspended_at) NULLS LAST
 LIMIT $2;

-- name: CreateUser :one
INSERT INTO users (id, tenant_id, email, display_name, password_hash, status)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, email) DO NOTHING
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE tenant_id = $1 AND email = $2;

-- name: ListUsers :many
SELECT * FROM users
WHERE tenant_id = $1 AND ($2 = '' OR status = $2)
ORDER BY display_name ASC
LIMIT $3 OFFSET $4;

-- name: UpdateUserStatus :one
UPDATE users SET status = $2 WHERE id = $1 AND tenant_id = $3 RETURNING *;

-- name: UpdateUserPasswordHash :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: CreateRole :one
INSERT INTO roles (id, tenant_id, name, permissions, is_system)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, name) DO NOTHING
RETURNING *;

-- name: ListRoles :many
SELECT * FROM roles WHERE tenant_id = $1 ORDER BY name ASC;

-- name: AssignRole :exec
INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, role_id, COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid))
DO NOTHING;

-- name: RevokeRole :exec
DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2;

-- name: ListUserRoles :many
SELECT r.*
FROM roles r
JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1;

-- name: CreateInvitation :one
INSERT INTO invitations (id, tenant_id, email, invited_by, token, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, email) DO UPDATE
    SET token = EXCLUDED.token, expires_at = EXCLUDED.expires_at, accepted_at = NULL
RETURNING *;

-- name: GetInvitationByToken :one
SELECT * FROM invitations
WHERE token = $1 AND accepted_at IS NULL AND expires_at > now();

-- name: AcceptInvitation :exec
UPDATE invitations SET accepted_at = now() WHERE id = $1;
