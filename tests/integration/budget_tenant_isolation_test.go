//go:build integration

// Integration test for the budget tenant-isolation guard-by-type fix (#139
// slice H, task 2): UpdateBudgetStatus previously resolved the target row's
// own tenant_id via a self-lookup (`SELECT tenant_id FROM budgets WHERE
// id = $1`) instead of taking the caller's tenantID as a parameter, so the
// `WHERE id=$1 AND tenant_id=$2` predicate in the sqlc query was a
// tautology that could never reject a cross-tenant id. This test exercises
// the real Postgres repository (services/budget/db) via services/budget's
// Service, so the actual predicate is the thing under test — not a double.
//
// Uses pkg/testdb (SKIPs without THITTAM_TEST_DSN); seeds rows directly via
// the pool and cleans them up in t.Cleanup, following the pattern in
// tests/integration/ledger_tenant_isolation_test.go.
package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/budget"
	budgetdb "github.com/wegofwd2020/thittam/services/budget/db"
)

// TestBudget_TenantIsolation_CrossTenantStatusUpdateDenied is the failing
// (pre-fix) / passing (post-fix) test for task 2. Before the fix,
// UpdateBudgetStatus ignored its caller's tenant entirely and resolved the
// target row's own tenant_id by id alone, so tenant A calling
// SubmitBudget/ApproveBudget on tenant B's budget id would silently succeed
// and mutate B's row.
func TestBudget_TenantIsolation_CrossTenantStatusUpdateDenied(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := budgetdb.NewPostgres(pool)
	svc := budget.NewService(repo)

	tenantA := uuid.New()
	tenantB := uuid.New()

	budgetA := seedDraftBudget(t, pool, tenantA)
	budgetB := seedDraftBudget(t, pool, tenantB)

	// --- SubmitBudget: tenant A submitting tenant B's budget id must fail,
	// and B's row must stay in draft. ---
	err := svc.SubmitBudget(ctx, tenantA, budgetB, uuid.New())
	assert.ErrorIs(t, err, budget.ErrBudgetNotFound, "cross-tenant SubmitBudget must be refused, not silently resolved via the budget's own tenant")

	stillDraft, err := repo.GetBudget(ctx, tenantB, budgetB)
	require.NoError(t, err)
	assert.Equal(t, "draft", stillDraft.Status, "a refused cross-tenant submit must not mutate the victim tenant's budget")

	// --- ApproveBudget: same check, using tenant A's own budget as the
	// victim this time to prove the isolation isn't accidentally symmetric
	// with the first case. ---
	err = svc.ApproveBudget(ctx, tenantB, budgetA, uuid.New())
	assert.ErrorIs(t, err, budget.ErrBudgetNotFound, "cross-tenant ApproveBudget must be refused")

	stillDraftA, err := repo.GetBudget(ctx, tenantA, budgetA)
	require.NoError(t, err)
	assert.Equal(t, "draft", stillDraftA.Status, "a refused cross-tenant approve must not mutate the victim tenant's budget")

	// Positive control: same-tenant submit then approve succeeds end to end,
	// proving the fix didn't just break the predicate for everyone.
	require.NoError(t, svc.SubmitBudget(ctx, tenantB, budgetB, uuid.New()))
	require.NoError(t, svc.ApproveBudget(ctx, tenantB, budgetB, uuid.New()))
	approved, err := repo.GetBudget(ctx, tenantB, budgetB)
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
}

// seedDraftBudget inserts a minimal draft budget for tenantID, registering
// cleanup. Returns the budget id.
func seedDraftBudget(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	budgetID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO budgets (id, production_id, tenant_id, label, status, currency, created_by)
		 VALUES ($1, $2, $3, 'Tenant isolation test budget', 'draft', 'INR', $4)`,
		budgetID, uuid.New(), tenantID, uuid.New())
	require.NoError(t, err, "insert budget")

	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM budgets WHERE id = $1`, budgetID)
		assert.NoError(t, err, "cleanup: delete budgets")
	})

	return budgetID
}
