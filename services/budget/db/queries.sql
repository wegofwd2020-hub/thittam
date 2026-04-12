-- Budget planning service queries.

-- name: CreateBudget :one
INSERT INTO budgets (id, production_id, tenant_id, label, status, currency, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetBudget :one
SELECT * FROM budgets WHERE id = $1 AND tenant_id = $2;

-- name: ListBudgets :many
SELECT * FROM budgets
WHERE tenant_id = $1 AND ($2::uuid IS NULL OR production_id = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdateBudgetStatus :one
UPDATE budgets
SET status       = $3,
    submitted_by = CASE WHEN $3 = 'submitted' THEN $4 ELSE submitted_by END,
    approved_by  = CASE WHEN $3 = 'approved'  THEN $4 ELSE approved_by  END,
    submitted_at = CASE WHEN $3 = 'submitted' THEN now() ELSE submitted_at END,
    approved_at  = CASE WHEN $3 = 'approved'  THEN now() ELSE approved_at  END,
    updated_at   = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateBudgetTotals :exec
UPDATE budgets
SET total_amount = (SELECT COALESCE(SUM(budgeted_amount), 0) FROM budget_line_items WHERE budget_id = $1),
    updated_at  = now()
WHERE id = $1;

-- name: CreateLineItem :one
INSERT INTO budget_line_items (id, budget_id, tenant_id, category_id, description, account_code, budgeted_amount)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetLineItem :one
SELECT * FROM budget_line_items WHERE id = $1;

-- name: ListLineItems :many
SELECT * FROM budget_line_items WHERE budget_id = $1 ORDER BY category_id, created_at ASC LIMIT $2 OFFSET $3;

-- name: UpdateLineItemAmounts :one
UPDATE budget_line_items
SET budgeted_amount  = COALESCE($3, budgeted_amount),
    actual_amount    = COALESCE($4, actual_amount),
    committed_amount = COALESCE($5, committed_amount),
    updated_at       = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: LockLineItem :exec
UPDATE budget_line_items SET is_locked = true, updated_at = now()
WHERE id = $1;
