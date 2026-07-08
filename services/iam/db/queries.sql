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

-- name: FindTenantByNormalizedName :one
-- Look up a tenant by name under the migration-018 normalisation
-- (case-insensitive, trimmed, internal whitespace collapsed). Uses the
-- tenants_name_ci_unique index. Powers the pre-flight duplicate check.
SELECT * FROM tenants
WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
    = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')
LIMIT 1;

-- name: UpdateTenantStatus :one
-- Sets the tenant's status and stamps the appropriate lifecycle timestamp
-- on first entry (#92). Repeat calls preserve the original timestamp so
-- the grace / deactivation clocks keep ticking from the real transition.
--
-- Legal-hold columns (#92 Stage 4) use COALESCE so SuspendTenant callers
-- that don't supply hold fields do not clobber an active hold. Clearing
-- a hold is a separate admin operation (not in this RPC).
UPDATE tenants SET
    status         = @status::text,
    suspended_at   = CASE WHEN @status::text = 'suspended'   AND suspended_at   IS NULL THEN now() ELSE suspended_at   END,
    deactivated_at = CASE WHEN @status::text = 'deactivated' AND deactivated_at IS NULL THEN now() ELSE deactivated_at END,
    hold_until     = COALESCE(sqlc.narg('hold_until'),    hold_until),
    freeze_reason  = COALESCE(sqlc.narg('freeze_reason'), freeze_reason)
WHERE id = @id
RETURNING *;

-- name: CountTenantsOnHold :one
-- Counts tenants currently on legal hold (#92 Stage 5). Used by the
-- retention sweeper to export a thittam_retention_tenants_on_hold
-- gauge per run. Counts ALL tenants with freeze_reason set,
-- regardless of status — an 'active' tenant under a precautionary
-- hold is still a held tenant.
SELECT COUNT(*) FROM tenants WHERE freeze_reason IS NOT NULL;

-- name: ClearTenantLegalHold :one
-- Unconditionally clears the two legal-hold columns on a tenant,
-- releasing the retention-sweeper pause applied via SuspendTenant
-- with hold_until/freeze_reason (#92 Stage 4 pair). The tenant's
-- status is NOT touched — whatever state it was in (typically
-- 'suspended') is preserved and the sweeper's normal lifecycle
-- clock resumes on the next run.
--
-- Idempotent: running on a tenant with no hold is a no-op UPDATE
-- that still returns the row. Callers decide whether to treat the
-- no-op as "already cleared" (no audit event) vs. "cleared now".
UPDATE tenants SET
    hold_until    = NULL,
    freeze_reason = NULL
WHERE id = $1
RETURNING *;

-- name: SetTenantLegalHold :one
-- Sets the two legal-hold columns on a tenant WITHOUT touching status or the
-- suspended_at/deactivated_at anchors — the operator override that pauses or
-- extends the retention sweeper for an already-suspended tenant (#119).
-- Unlike ClearTenantLegalHold this writes the columns: a NULL hold_until is an
-- indefinite hold, a future hold_until is a dated extension. Collision and
-- status-eligibility are enforced in the service layer, not here.
UPDATE tenants SET
    hold_until    = $2,
    freeze_reason = $3
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
-- before @now (the sweeper's "now"). Each WHERE branch is backed by a
-- partial index created in migration 016, so the query is cheap even
-- with many active tenants.
--
-- Legal-hold filter (#92 Stage 4): a tenant with freeze_reason set is on
-- legal hold. An indefinite hold (hold_until NULL) pins the tenant in its
-- current state forever; a time-bounded hold releases once hold_until has
-- passed. Active holds are filtered out here so the sweeper never
-- advances a tenant under litigation / regulatory freeze.
--
-- @now is cast to timestamptz explicitly. Without the cast, the parser
-- saw @now used both as `@now - INTERVAL` (where it could be interval)
-- and as `hold_until <= @now` (where it must be timestamptz) and failed
-- to unify — sqlc then emits interface{} for the param and Postgres
-- rejects the timestamp-compare at runtime.
SELECT * FROM tenants
 WHERE ((status = 'suspended'   AND suspended_at   <= $1::timestamptz - INTERVAL '30 days')
     OR (status = 'grace'       AND suspended_at   <= $1::timestamptz - INTERVAL '90 days')
     OR (status = 'deactivated' AND deactivated_at <= $1::timestamptz - INTERVAL '180 days'))
   AND (freeze_reason IS NULL
        OR (hold_until IS NOT NULL AND hold_until <= $1::timestamptz))
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

-- name: CreateTenantPurgeRequest :one
INSERT INTO tenant_purge_requests (
    id, tenant_id, status, requested_by, request_reason, tenant_name, tenant_slug
) VALUES ($1, $2, 'pending', $3, $4, $5, $6)
RETURNING *;

-- name: GetOpenTenantPurgeRequest :one
SELECT * FROM tenant_purge_requests
WHERE tenant_id = $1 AND status IN ('pending', 'approved')
LIMIT 1;

-- name: ApproveTenantPurgeRequest :one
UPDATE tenant_purge_requests
   SET status = 'approved', approved_by = $2, approved_at = now()
 WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: CancelTenantPurgeRequest :one
UPDATE tenant_purge_requests
   SET status = 'cancelled', cancelled_by = $2, cancelled_at = now()
 WHERE id = $1 AND status IN ('pending', 'approved')
RETURNING *;

-- name: ListApprovedTenantPurgeRequests :many
SELECT * FROM tenant_purge_requests
 WHERE status = 'approved'
 ORDER BY approved_at
 LIMIT $1;

-- name: MarkTenantPurgeRequestFailed :one
-- Status-guarded so an ambiguous commit (server commits the purge tx,
-- client sees a network error and retries the mark-failed call) can never
-- clobber a request that actually reached 'executed'. Zero rows means the
-- request already left 'approved' — either it executed, or it was
-- cancelled mid-flight — and the caller (PurgeApprovedTenant) treats that
-- as a benign reconcile, not a failure.
UPDATE tenant_purge_requests
   SET status = 'failed', failure_reason = $2
 WHERE id = $1 AND status = 'approved'
RETURNING *;
