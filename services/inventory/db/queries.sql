-- Inventory management service queries.

-- name: CreateAsset :one
INSERT INTO assets (id, tenant_id, asset_code, name, category_id, description, ownership_type, status, purchase_date, purchase_cost, serial_number)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAsset :one
SELECT * FROM assets WHERE id = $1 AND tenant_id = $2;

-- name: GetAssetByCode :one
SELECT * FROM assets WHERE asset_code = $1 AND tenant_id = $2;

-- name: ListAssets :many
SELECT * FROM assets
WHERE tenant_id = $1
  AND ($2 = '' OR status = $2)
  AND ($3 = '' OR category_id = $3)
ORDER BY name ASC
LIMIT $4 OFFSET $5;

-- name: UpdateAssetStatus :one
UPDATE assets SET status = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateAsset :one
UPDATE assets
SET name         = COALESCE(NULLIF($3, ''), name),
    description  = $4,
    category_id  = COALESCE(NULLIF($5, ''), category_id),
    updated_at   = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: CheckoutAsset :one
INSERT INTO asset_checkouts (id, asset_id, production_id, tenant_id, checked_out_to, expected_return, condition_out)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CheckinAsset :one
UPDATE asset_checkouts
SET checked_in_at = now(), condition_in = $3
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: ListCheckouts :many
SELECT * FROM asset_checkouts
WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR asset_id = $2)
  AND ($3::uuid IS NULL OR production_id = $3)
ORDER BY checked_out_at DESC
LIMIT $4 OFFSET $5;

-- name: GetActiveCheckout :one
SELECT * FROM asset_checkouts
WHERE asset_id = $1 AND checked_in_at IS NULL
LIMIT 1;
