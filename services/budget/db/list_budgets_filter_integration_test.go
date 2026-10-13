//go:build integration

// Real-Postgres proof that ListBudgets treats a nil production_id as "no filter"
// (zero-UUID trap, same class as #185 and #220). SKIPs without THITTAM_TEST_DSN.
package db_test

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

func TestBudget_ListBudgets_NilProductionIsNoFilter(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := budgetdb.NewPostgres(pool)

	tenantID := seedBudgetListTenant(t, pool)
	prodA := uuid.New()
	prodB := uuid.New()
	bA := seedBudget(t, pool, tenantID, prodA)
	bB := seedBudget(t, pool, tenantID, prodB)

	t.Run("nil production returns every tenant budget", func(t *testing.T) {
		all, err := repo.ListBudgets(ctx, tenantID, uuid.Nil, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{bA, bB}, budgetIDs(all),
			"nil production_id must not filter (zero-UUID trap)")
	})

	t.Run("production filter returns only that production", func(t *testing.T) {
		onlyA, err := repo.ListBudgets(ctx, tenantID, prodA, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{bA}, budgetIDs(onlyA))
	})
}

// #233: the status filter must narrow results, and compose with the production
// filter. Before the fix, status was accepted and silently dropped.
func TestBudget_ListBudgets_StatusFilter(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := budgetdb.NewPostgres(pool)

	tenantID := seedBudgetListTenant(t, pool)
	prodA := uuid.New()
	prodB := uuid.New()
	draftA := seedBudget(t, pool, tenantID, prodA)
	approvedA := seedBudgetInStatus(t, pool, tenantID, prodA, "approved")
	approvedB := seedBudgetInStatus(t, pool, tenantID, prodB, "approved")

	t.Run("empty status returns every status", func(t *testing.T) {
		all, err := repo.ListBudgets(ctx, tenantID, uuid.Nil, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{draftA, approvedA, approvedB}, budgetIDs(all))
	})

	t.Run("status filter returns only that status", func(t *testing.T) {
		approved, err := repo.ListBudgets(ctx, tenantID, uuid.Nil, "approved", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{approvedA, approvedB}, budgetIDs(approved))

		drafts, err := repo.ListBudgets(ctx, tenantID, uuid.Nil, "draft", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{draftA}, budgetIDs(drafts))
	})

	t.Run("status composes with production filter", func(t *testing.T) {
		got, err := repo.ListBudgets(ctx, tenantID, prodA, "approved", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{approvedA}, budgetIDs(got))
	})
}

func budgetIDs(rows []budget.Budget) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// seedBudgetListTenant inserts a tenant and removes its budgets on cleanup.
// country_code and primary_currency_code are NOT NULL since migration 014; the
// name must be unique under tenants_name_ci_unique.
func seedBudgetListTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Budget List Tenant "+tenantID.String(), "budget-list-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM budgets WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete budgets")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

func seedBudget(t *testing.T, pool *pgxpool.Pool, tenantID, productionID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO budgets (id, production_id, tenant_id, label, created_by)
		 VALUES ($1, $2, $3, 'test budget', $4)`,
		id, productionID, tenantID, uuid.New())
	require.NoError(t, err, "insert budget")
	return id
}

// seedBudgetInStatus inserts a budget and moves it to the given status.
func seedBudgetInStatus(t *testing.T, pool *pgxpool.Pool, tenantID, productionID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := seedBudget(t, pool, tenantID, productionID)
	_, err := pool.Exec(context.Background(),
		`UPDATE budgets SET status = $1 WHERE id = $2 AND tenant_id = $3`,
		status, id, tenantID)
	require.NoError(t, err, "set budget status")
	return id
}
