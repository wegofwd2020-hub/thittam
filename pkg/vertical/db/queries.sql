-- name: GetVerticalConfigForTenant :one
-- Returns the base vertical config merged with any tenant-specific overrides.
-- Uses PostgreSQL shallow merge for backward compatibility.
SELECT
    vd.config || COALESCE(tv.config_override, '{}'::jsonb) AS merged_config
FROM tenant_verticals tv
JOIN vertical_definitions vd ON vd.id = tv.vertical_id
WHERE tv.tenant_id = $1
  AND vd.is_active = true;

-- name: GetVerticalConfigAndOverride :one
-- Returns base config and override separately for Go-side deep-merge.
SELECT
    vd.config AS base_config,
    tv.config_override AS config_override
FROM tenant_verticals tv
JOIN vertical_definitions vd ON vd.id = tv.vertical_id
WHERE tv.tenant_id = $1
  AND vd.is_active = true;

-- name: GetVerticalDefinition :one
SELECT * FROM vertical_definitions WHERE id = $1 AND is_active = true;

-- name: ListVerticalDefinitions :many
SELECT id, name, version, description
FROM vertical_definitions
WHERE is_active = true
ORDER BY name;

-- name: UpsertVerticalDefinition :one
INSERT INTO vertical_definitions (id, name, version, description, config)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO UPDATE
    SET name        = EXCLUDED.name,
        version     = EXCLUDED.version,
        description = EXCLUDED.description,
        config      = EXCLUDED.config,
        updated_at  = now()
RETURNING *;

-- name: DeactivateVerticalDefinition :exec
UPDATE vertical_definitions SET is_active = false, updated_at = now() WHERE id = $1;

-- name: BindTenantVertical :exec
INSERT INTO tenant_verticals (tenant_id, vertical_id, config_override, registered_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (tenant_id) DO NOTHING;

-- name: UpdateTenantVerticalOverride :exec
UPDATE tenant_verticals
SET config_override = $2
WHERE tenant_id = $1;

-- name: GetTenantVertical :one
SELECT * FROM tenant_verticals WHERE tenant_id = $1;

-- name: UnbindTenantVertical :exec
DELETE FROM tenant_verticals WHERE tenant_id = $1;
