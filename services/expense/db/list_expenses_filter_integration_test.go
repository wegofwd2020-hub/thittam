//go:build integration

// Integration test for the ListExpenses submitted_by / production_id filter
// fix (#165 task 2). Task 1 added a nullable submitted_by param to the sqlc
// query; task 2 threads a real submittedBy uuid.UUID through
// expense.Repository -> expense.Service -> Postgres. The unit suite
// (service_test.go) uses a mock repo and cannot see whether the actual SQL
// predicate works, so this test exercises the real Postgres repository
// (services/expense/db.Postgres) directly.
//
// Uses pkg/testdb (SKIPs without THITTAM_TEST_DSN); seeds rows directly via
// the pool and cleans them up in t.Cleanup, following the pattern in
// tests/integration/document_tenant_isolation_test.go.
package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/expense"
	expensedb "github.com/wegofwd2020/thittam/services/expense/db"
)

// TestExpense_ListExpenses_SubmittedByFilter seeds one tenant, two submitters
// (A, B), two productions (P1, P2), and three expenses (A/P1, A/P2, B/P1),
// then proves:
//   - filtering by submittedBy=A with no production filter returns both of
//     A's expenses across both productions (proves the submitted_by
//     predicate works, and that a nil productionID does not filter).
//   - no submittedBy / no production filter returns all three (proves the
//     nil production_id filter still returns everything).
//   - filtering by a specific production with no submittedBy returns both
//     expenses in that production regardless of submitter (proves
//     production_id filtering still works on its own).
func TestExpense_ListExpenses_SubmittedByFilter(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := expensedb.NewPostgres(pool)

	tenantID := seedExpenseFilterTenant(t, pool)

	submitterA := uuid.New()
	submitterB := uuid.New()
	prodP1 := uuid.New()
	prodP2 := uuid.New()

	expAP1 := seedFilterExpense(t, pool, tenantID, prodP1, submitterA)
	expAP2 := seedFilterExpense(t, pool, tenantID, prodP2, submitterA)
	expBP1 := seedFilterExpense(t, pool, tenantID, prodP1, submitterB)

	// submittedBy=A, no production filter -> A's 2 expenses across both
	// productions, none of B's.
	byA, err := repo.ListExpenses(ctx, tenantID, uuid.Nil, "", 20, 0, submitterA)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{expAP1, expAP2}, expenseIDs(byA),
		"submitted_by filter must return only A's expenses across all productions")

	// No filters at all -> all 3.
	all, err := repo.ListExpenses(ctx, tenantID, uuid.Nil, "", 20, 0, uuid.Nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{expAP1, expAP2, expBP1}, expenseIDs(all),
		"a nil production_id and nil submitted_by must return every tenant expense")

	// productionID=P1, no submittedBy -> A/P1 + B/P1, not A/P2.
	byP1, err := repo.ListExpenses(ctx, tenantID, prodP1, "", 20, 0, uuid.Nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{expAP1, expBP1}, expenseIDs(byP1),
		"production_id filter must still work independently of submitted_by")
}

func expenseIDs(rows []expense.Expense) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// seedExpenseFilterTenant inserts a tenant for this test and registers
// cleanup. country_code and primary_currency_code are NOT NULL since
// migration 014; the name must be unique under tenants_name_ci_unique
// (lower(trim(name))), so it is suffixed with a fresh UUID.
func seedExpenseFilterTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Expense Filter Tenant "+tenantID.String(), "expense-filter-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM expenses WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete expenses")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

// seedFilterExpense inserts a minimal submitted expense for the given
// tenant/production/submitter and returns its id. Cleanup is handled by
// seedExpenseFilterTenant's tenant-scoped DELETE.
func seedFilterExpense(t *testing.T, pool *pgxpool.Pool, tenantID, productionID, submittedBy uuid.UUID) uuid.UUID {
	t.Helper()
	expenseID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO expenses (id, production_id, tenant_id, category_id, amount, submitted_by, status)
		 VALUES ($1, $2, $3, 'catering', 100.00, $4, 'submitted')`,
		expenseID, productionID, tenantID, submittedBy)
	require.NoError(t, err, "insert expense")
	return expenseID
}
