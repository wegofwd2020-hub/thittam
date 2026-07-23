-- Project management service queries.
-- All tables live in the tenant-scoped schema (tenant_<uuid>).

-- name: CreateProduction :one
INSERT INTO productions (id, tenant_id, title, slug, description, genre, language, status, start_date, end_date, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetProduction :one
SELECT * FROM productions
WHERE id = $1 AND tenant_id = $2;

-- name: ListProductions :many
SELECT * FROM productions
WHERE tenant_id = $1
  AND ($2 = '' OR status = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdateProduction :one
UPDATE productions
SET title       = COALESCE(NULLIF($3, ''), title),
    description = $4,
    genre       = COALESCE(NULLIF($5, ''), genre),
    status      = COALESCE(NULLIF($6, ''), status),
    start_date  = $7,
    end_date    = $8,
    updated_at  = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: ArchiveProduction :one
UPDATE productions SET status = 'archived', updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: CreatePhase :one
INSERT INTO phases (id, production_id, tenant_id, phase_type, status, start_date, end_date)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPhases :many
SELECT * FROM phases WHERE production_id = $1 AND tenant_id = $2 ORDER BY created_at ASC LIMIT $3 OFFSET $4;

-- name: GetPhase :one
SELECT * FROM phases WHERE id = $1 AND tenant_id = $2;

-- name: UpdatePhaseStatus :one
UPDATE phases SET status = $2, updated_at = now()
WHERE id = $1 AND tenant_id = $3
RETURNING *;

-- name: AddCrewMember :one
INSERT INTO crew_members (id, production_id, tenant_id, user_id, name, role, department, day_rate, currency, start_date, end_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: ListCrewMembers :many
SELECT * FROM crew_members WHERE production_id = $1 AND tenant_id = $2 ORDER BY name ASC LIMIT $3 OFFSET $4;

-- name: RemoveCrewMember :exec
DELETE FROM crew_members WHERE id = $1 AND tenant_id = $2;
