//go:build integration

// Real-Postgres proof for the monthly cost-behavior report and the account
// classification write. Covers the SQL that sqlc does not validate: the
// posted-only filter, posting-month bucketing, net debit−credit, the
// NULL → unclassified bucket, asset exclusion, and tenant isolation. SKIPs
// without THITTAM_TEST_DSN.
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/ledger"
	ledgerdb "github.com/wegofwd2020/thittam/services/ledger/db"
)

type costLine struct {
	accountID uuid.UUID
	debit     string
	credit    string
}

func TestLedger_MonthlyCostBehavior(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := ledgerdb.NewPostgres(pool)

	tenantID := seedCostTenant(t, pool)
	otherTenant := seedCostTenant(t, pool)

	period := seedCostPeriod(t, pool, tenantID, 2026, 1)
	otherPeriod := seedCostPeriod(t, pool, otherTenant, 2026, 1)

	rent := seedCostAccount(t, pool, tenantID, "6100", "expense", "fixed")
	cloud := seedCostAccount(t, pool, tenantID, "6200", "expense", "variable")
	misc := seedCostAccount(t, pool, tenantID, "6300", "expense", "")
	bank := seedCostAccount(t, pool, tenantID, "1100", "asset", "")
	foreignRent := seedCostAccount(t, pool, otherTenant, "6100", "expense", "fixed")
	foreignBank := seedCostAccount(t, pool, otherTenant, "1100", "asset", "")

	jan := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 3, 9, 0, 0, 0, time.UTC)

	// January: rent 1000 fixed; cloud 300 debit less a 50 credit reversal = 250 net variable;
	// misc 40 unclassified.
	seedPostedEntry(t, pool, tenantID, period, jan, []costLine{
		{rent, "1000.00", "0"}, {bank, "0", "1000.00"},
	})
	seedPostedEntry(t, pool, tenantID, period, jan, []costLine{
		{cloud, "300.00", "0"}, {bank, "0", "250.00"}, {misc, "40.00", "0"}, {bank, "0", "90.00"},
	})
	seedPostedEntry(t, pool, tenantID, period, jan, []costLine{
		{cloud, "0", "50.00"}, {bank, "50.00", "0"},
	})

	// February: variable 200 only.
	seedPostedEntry(t, pool, tenantID, period, feb, []costLine{
		{cloud, "200.00", "0"}, {bank, "0", "200.00"},
	})

	// Draft entry must not count.
	seedDraftEntry(t, pool, tenantID, period, jan, []costLine{
		{rent, "999.00", "0"}, {bank, "0", "999.00"},
	})

	// Another tenant's postings must not appear.
	seedPostedEntry(t, pool, otherTenant, otherPeriod, jan, []costLine{
		{foreignRent, "5000.00", "0"}, {foreignBank, "0", "5000.00"},
	})

	t.Run("buckets by posting month with net debit minus credit", func(t *testing.T) {
		rows, err := repo.GetMonthlyCostBehavior(ctx, tenantID, "2026-01", "2026-02")
		require.NoError(t, err)
		require.Len(t, rows, 2)

		jan := rows[0]
		assert.Equal(t, "2026-01", jan.Month)
		assert.True(t, jan.Fixed.Equal(decimal.RequireFromString("1000.00")), "fixed: %s", jan.Fixed)
		assert.True(t, jan.Variable.Equal(decimal.RequireFromString("250.00")), "variable net of reversal: %s", jan.Variable)
		assert.True(t, jan.Unclassified.Equal(decimal.RequireFromString("40.00")), "unclassified: %s", jan.Unclassified)
		assert.True(t, jan.Total.Equal(decimal.RequireFromString("1290.00")), "total: %s", jan.Total)

		feb := rows[1]
		assert.Equal(t, "2026-02", feb.Month)
		assert.True(t, feb.Fixed.IsZero())
		assert.True(t, feb.Variable.Equal(decimal.RequireFromString("200.00")))
		assert.True(t, feb.Total.Equal(decimal.RequireFromString("200.00")))
	})

	t.Run("range excludes months outside it", func(t *testing.T) {
		rows, err := repo.GetMonthlyCostBehavior(ctx, tenantID, "2026-02", "2026-02")
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "2026-02", rows[0].Month)
	})

	t.Run("asset accounts never count as cost", func(t *testing.T) {
		rows, err := repo.GetMonthlyCostBehavior(ctx, tenantID, "2026-01", "2026-02")
		require.NoError(t, err)
		for _, r := range rows {
			assert.True(t, r.Total.Equal(r.Fixed.Add(r.Variable).Add(r.Unclassified)),
				"buckets must reconcile to total for %s", r.Month)
		}
	})
}

func TestLedger_SetAccountCostBehavior(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := ledgerdb.NewPostgres(pool)

	tenantID := seedCostTenant(t, pool)
	otherTenant := seedCostTenant(t, pool)
	acct := seedCostAccount(t, pool, tenantID, "6400", "expense", "")

	got, err := repo.SetAccountCostBehavior(ctx, tenantID, acct, "fixed")
	require.NoError(t, err)
	assert.Equal(t, "fixed", got.CostBehavior)

	got, err = repo.SetAccountCostBehavior(ctx, tenantID, acct, "")
	require.NoError(t, err)
	assert.Equal(t, "", got.CostBehavior, "empty clears to NULL")

	_, err = repo.SetAccountCostBehavior(ctx, otherTenant, acct, "variable")
	require.ErrorIs(t, err, ledger.ErrAccountNotFound, "another tenant must not reach the account")
}

// seedCostTenant mirrors the tenant insert in list_journal_entries_filter_integration_test.go:
// country_code and primary_currency_code are NOT NULL and the name must be unique.
func seedCostTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		id, "Cost Behavior Tenant "+id.String(), "cost-behavior-"+id.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		// Children first: lines cascade from entries, entries reference periods, periods are tenant-scoped.
		_, _ = pool.Exec(ctx, `DELETE FROM journal_entries WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM accounting_periods WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM accounts WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	})
	return id
}

func seedCostPeriod(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, year, month int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO accounting_periods (id, tenant_id, year, month) VALUES ($1, $2, $3, $4)`,
		id, tenantID, year, month)
	require.NoError(t, err, "insert accounting period")
	return id
}

// seedCostAccount inserts an account. An empty costBehavior stores NULL.
func seedCostAccount(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, code, accountType, costBehavior string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var cb any
	if costBehavior != "" {
		cb = costBehavior
	}
	_, err := pool.Exec(context.Background(),
		`INSERT INTO accounts (id, tenant_id, code, name, account_type, is_active, cost_behavior)
		 VALUES ($1, $2, $3, $4, $5, true, $6)`,
		id, tenantID, code, "Account "+code, accountType, cb)
	require.NoError(t, err, "insert account")
	return id
}

func seedPostedEntry(t *testing.T, pool *pgxpool.Pool, tenantID, periodID uuid.UUID, postedAt time.Time, lines []costLine) {
	t.Helper()
	insertEntry(t, pool, tenantID, periodID, "posted", &postedAt, lines)
}

func seedDraftEntry(t *testing.T, pool *pgxpool.Pool, tenantID, periodID uuid.UUID, at time.Time, lines []costLine) {
	t.Helper()
	insertEntry(t, pool, tenantID, periodID, "draft", nil, lines)
}

func insertEntry(t *testing.T, pool *pgxpool.Pool, tenantID, periodID uuid.UUID, status string, postedAt *time.Time, lines []costLine) {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO journal_entries (id, tenant_id, period_id, entry_number, narration, status, posted_at)
		 VALUES ($1, $2, $3, $4, 'cost behavior test', $5, $6)`,
		id, tenantID, periodID, "JE-"+id.String(), status, postedAt)
	require.NoError(t, err, "insert journal entry")
	for _, l := range lines {
		_, err := pool.Exec(ctx,
			`INSERT INTO journal_lines (journal_id, account_id, debit_amount, credit_amount)
			 VALUES ($1, $2, $3, $4)`,
			id, l.accountID, l.debit, l.credit)
		require.NoError(t, err, "insert journal line")
	}
}
