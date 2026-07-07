-- name: CreateTenant :one
INSERT INTO tenants (name, slug, plan) VALUES ($1, $2, $3) RETURNING *;

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
