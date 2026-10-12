//go:build integration

// Real-Postgres proof that ListJournalEntries treats a nil period as "no filter"
// (zero-UUID trap, same class as #185 and #220), that a period filter does not
// widen, and that status composes with period. SKIPs without THITTAM_TEST_DSN.
package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/ledger"
	ledgerdb "github.com/wegofwd2020/thittam/services/ledger/db"
)

func TestLedger_ListJournalEntries_Filters(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := ledgerdb.NewPostgres(pool)

	tenantID := seedLedgerListTenant(t, pool)
	periodA := seedPeriod(t, pool, tenantID, 2026, 1)
	periodB := seedPeriod(t, pool, tenantID, 2026, 2)

	jeA1 := seedJournalEntry(t, pool, tenantID, periodA, "draft")
	jeA2 := seedJournalEntry(t, pool, tenantID, periodA, "posted")
	jeB1 := seedJournalEntry(t, pool, tenantID, periodB, "draft")

	t.Run("nil period returns every tenant entry", func(t *testing.T) {
		all, err := repo.ListJournalEntries(ctx, tenantID, nil, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{jeA1, jeA2, jeB1}, journalIDs(all),
			"nil period must not filter (zero-UUID trap)")
	})

	t.Run("period filter returns only that period and does not widen", func(t *testing.T) {
		got, err := repo.ListJournalEntries(ctx, tenantID, &periodA, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{jeA1, jeA2}, journalIDs(got))
	})

	t.Run("period AND status compose", func(t *testing.T) {
		got, err := repo.ListJournalEntries(ctx, tenantID, &periodA, "posted", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{jeA2}, journalIDs(got))
	})
}

func journalIDs(rows []ledger.JournalEntry) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// seedLedgerListTenant inserts a tenant and removes its rows on cleanup. Children
// go first: journal entries reference accounting periods.
func seedLedgerListTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Ledger List Tenant "+tenantID.String(), "ledger-list-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM journal_entries WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete journal entries")
		_, err = pool.Exec(ctx, `DELETE FROM accounting_periods WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete periods")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

func seedPeriod(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, year, month int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO accounting_periods (id, tenant_id, year, month) VALUES ($1, $2, $3, $4)`,
		id, tenantID, year, month)
	require.NoError(t, err, "insert accounting period")
	return id
}

// seedJournalEntry inserts one entry. entry_number is UNIQUE, so it is suffixed with a fresh UUID.
func seedJournalEntry(t *testing.T, pool *pgxpool.Pool, tenantID, periodID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO journal_entries (id, tenant_id, period_id, entry_number, narration, status)
		 VALUES ($1, $2, $3, $4, 'test entry', $5)`,
		id, tenantID, periodID, "JE-"+id.String(), status)
	require.NoError(t, err, "insert journal entry")
	return id
}
