-- name: CreateTenant :one
-- country_code/primary_currency_code are NOT NULL as of migration 014 with no
-- DB default; registration doesn't collect these at signup yet, so default to
-- the same IN/INR values migration 014 backfilled onto pre-existing rows.
-- STOPGAP — TODO(#115): collect country at signup + derive currency (like iam)
-- and drop these literals before registration is wired to a live signup flow.
INSERT INTO tenants (name, slug, plan, country_code, primary_currency_code)
VALUES ($1, $2, $3, 'IN', 'INR') RETURNING *;

-- name: TenantExistsBySlug :one
SELECT EXISTS(SELECT 1 FROM tenants WHERE slug = $1);

-- name: TenantExistsByNormalizedName :one
SELECT EXISTS (
    SELECT 1 FROM tenants
    WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
        = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')
);

-- name: CreateUser :one
INSERT INTO users (tenant_id, email, display_name, password_hash)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UserExistsByEmail :one
SELECT EXISTS(SELECT 1 FROM users WHERE email = $1);

-- name: GetTenantByID :one
SELECT * FROM tenants WHERE id = $1;
