-- Reporting analytics service queries.
-- Read-only projections. No writes except watermark updates.

-- name: GetBudgetSummary :one
SELECT
    b.id,
    b.label,
    b.status,
    b.total_amount,
    COALESCE(SUM(li.actual_amount),    0) AS actual_spend,
    COALESCE(SUM(li.committed_amount), 0) AS committed_spend
FROM budgets b
LEFT JOIN budget_line_items li ON li.budget_id = b.id
WHERE b.id = $1 AND b.tenant_id = $2
GROUP BY b.id, b.label, b.status, b.total_amount;

-- name: GetExpenseSummaryByCategory :many
SELECT
    category_id,
    status,
    COUNT(*)                AS count,
    COALESCE(SUM(amount),0) AS total_amount
FROM expenses
WHERE tenant_id = $1 AND ($2::uuid IS NULL OR production_id = $2)
GROUP BY category_id, status
ORDER BY total_amount DESC;

-- name: GetProductionFinancialSummary :one
SELECT
    p.id AS production_id,
    p.title,
    COALESCE(b.total_amount, 0)                                    AS budget_total,
    COALESCE(SUM(CASE WHEN e.status='paid' THEN e.amount ELSE 0 END), 0) AS actual_spend,
    COALESCE(SUM(CASE WHEN e.status IN ('submitted','approved') THEN e.amount ELSE 0 END), 0) AS committed_spend
FROM productions p
LEFT JOIN budgets b     ON b.production_id = p.id AND b.status = 'approved'
LEFT JOIN expenses e    ON e.production_id = p.id
WHERE p.id = $1 AND p.tenant_id = $2
GROUP BY p.id, p.title, b.total_amount;

-- name: GetTenantDashboardStats :one
SELECT
    COUNT(DISTINCT p.id)                             AS production_count,
    COUNT(DISTINCT e.id) FILTER (WHERE e.status = 'submitted') AS pending_approvals,
    COALESCE(SUM(e.amount) FILTER (WHERE e.status = 'paid'), 0) AS total_spent
FROM productions p
LEFT JOIN expenses e ON e.tenant_id = p.tenant_id
WHERE p.tenant_id = $1;

-- name: UpsertProjectionWatermark :exec
INSERT INTO projection_watermarks (subject, last_event_at)
VALUES ($1, $2)
ON CONFLICT (subject) DO UPDATE
    SET last_event_at = EXCLUDED.last_event_at,
        updated_at    = now();

-- name: GetProjectionWatermark :one
SELECT * FROM projection_watermarks WHERE subject = $1;
