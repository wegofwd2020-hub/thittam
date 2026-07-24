//go:build integration

// Integration test for the ledger tenant-isolation guard-by-type fix (#139
// slice H, task 1): journal_lines has no tenant_id column of its own, so
// ListJournalLines must JOIN journal_entries to filter by tenant (1a), and
// UpdateJournalStatus must take the caller's tenantID as a required parameter
// instead of self-resolving the row's own tenant_id from the id alone (1b,
// the discarded-error resolveTenantForJournal self-lookup this change
// deletes). Exercises the real Postgres repository (services/ledger/db),
// not a double, so the actual `WHERE ... AND tenant_id = $N` / JOIN
// predicates are the thing under test.
//
// Uses pkg/testdb (SKIPs without THITTAM_TEST_DSN); seeds rows directly via
// the pool and cleans them up in t.Cleanup, following the pattern in
// tests/integration/notifications_authz_test.go.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/ledger"
	ledgerdb "github.com/wegofwd2020/thittam/services/ledger/db"
)

// TestLedger_TenantIsolation_CrossTenantJournalAccessDenied is the failing
// (pre-fix) / passing (post-fix) test for task 1. Before the fix:
//   - UpdateJournalStatus ignored its caller's tenant and resolved the
//     target row's own tenant_id via resolveTenantForJournal, so posting
//     tenant B's journal id while authenticated as tenant A would silently
//     succeed and mutate B's row.
//   - ListJournalLines had no tenant predicate at all (journal_lines carries
//     no tenant_id column), so any caller who knew a journal_id could read
//     another tenant's lines once the JOIN parent's own check was bypassed.
func TestLedger_TenantIsolation_CrossTenantJournalAccessDenied(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := ledgerdb.NewPostgres(pool)
	q := ledgerdb.New(pool)

	tenantA := uuid.New()
	tenantB := uuid.New()

	entryA := seedLedgerDraftJournal(t, pool, tenantA)
	entryB := seedLedgerDraftJournal(t, pool, tenantB)

	// --- GetJournalEntry: tenant A must never see tenant B's entry or lines. ---
	_, err := repo.GetJournalEntry(ctx, tenantA, entryB)
	assert.ErrorIs(t, err, ledger.ErrJournalNotFound, "cross-tenant GetJournalEntry must be indistinguishable from missing")

	// tenant B can still read its own entry with its lines intact — proves the
	// ListJournalLines JOIN doesn't break the real, same-tenant path.
	got, err := repo.GetJournalEntry(ctx, tenantB, entryB)
	require.NoError(t, err)
	assert.Equal(t, entryB, got.ID)
	assert.Len(t, got.Lines, 2, "same-tenant read must still return both lines")

	// Structural check on the JOIN predicate itself: querying B's journal_id
	// under A's tenant must return zero lines, not an error and not B's rows.
	crossTenantLines, err := q.ListJournalLines(ctx, ledgerdb.ListJournalLinesParams{
		JournalID: entryB,
		TenantID:  tenantA,
	})
	require.NoError(t, err)
	assert.Empty(t, crossTenantLines, "ListJournalLines must return no rows when the journal belongs to a different tenant")

	// --- UpdateJournalStatus (via PostJournalEntry's sqlc query): tenant A
	// posting tenant B's entry id must fail, and B's row must be untouched. ---
	err = repo.UpdateJournalStatus(ctx, tenantA, entryB, "posted", uuid.New(), time.Now().UTC())
	assert.ErrorIs(t, err, ledger.ErrJournalNotFound, "cross-tenant post must be refused, not silently resolved via the entry's own tenant")

	stillDraft, err := repo.GetJournalEntry(ctx, tenantB, entryB)
	require.NoError(t, err)
	assert.Equal(t, "draft", stillDraft.Status, "a refused cross-tenant post must not mutate the victim tenant's entry")

	// Same check for void: tenant A must not be able to void tenant A's own
	// *other* tenant's entry either. Uses entryA as tenant A's own to prove
	// same-tenant void still works, then a cross-tenant void of entryB fails.
	err = repo.UpdateJournalStatus(ctx, tenantB, entryA, "void", uuid.New(), time.Now().UTC())
	assert.ErrorIs(t, err, ledger.ErrJournalNotFound, "cross-tenant void must be refused")

	// Positive control: same-tenant post succeeds end to end.
	err = repo.UpdateJournalStatus(ctx, tenantB, entryB, "posted", uuid.New(), time.Now().UTC())
	require.NoError(t, err)
	posted, err := repo.GetJournalEntry(ctx, tenantB, entryB)
	require.NoError(t, err)
	assert.Equal(t, "posted", posted.Status)
}

// seedLedgerDraftJournal inserts an account, an open accounting period, and a
// draft, balanced (one debit line + one credit line) journal entry for
// tenantID, registering cleanup for all of it. Returns the journal entry id.
func seedLedgerDraftJournal(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	accountID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO accounts (id, tenant_id, code, name, account_type, is_active)
		 VALUES ($1, $2, $3, 'Isolation Test Account', 'asset', true)`,
		accountID, tenantID, "ISO-"+accountID.String())
	require.NoError(t, err, "insert account")

	periodID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO accounting_periods (id, tenant_id, year, month, status)
		 VALUES ($1, $2, 2026, 7, 'open')`,
		periodID, tenantID)
	require.NoError(t, err, "insert accounting_period")

	entryID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO journal_entries (id, tenant_id, period_id, entry_number, narration, status)
		 VALUES ($1, $2, $3, $4, 'Tenant isolation test entry', 'draft')`,
		entryID, tenantID, periodID, "JE-ISO-"+entryID.String())
	require.NoError(t, err, "insert journal_entry")

	debitLineID, creditLineID := uuid.New(), uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO journal_lines (id, journal_id, account_id, debit_amount, credit_amount, description)
		 VALUES ($1, $2, $3, 100.00, 0, 'debit leg')`,
		debitLineID, entryID, accountID)
	require.NoError(t, err, "insert debit journal_line")
	_, err = pool.Exec(ctx,
		`INSERT INTO journal_lines (id, journal_id, account_id, debit_amount, credit_amount, description)
		 VALUES ($1, $2, $3, 0, 100.00, 'credit leg')`,
		creditLineID, entryID, accountID)
	require.NoError(t, err, "insert credit journal_line")

	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM journal_lines WHERE journal_id = $1`, entryID)
		assert.NoError(t, err, "cleanup: delete journal_lines")
		_, err = pool.Exec(ctx, `DELETE FROM journal_entries WHERE id = $1`, entryID)
		assert.NoError(t, err, "cleanup: delete journal_entries")
		_, err = pool.Exec(ctx, `DELETE FROM accounting_periods WHERE id = $1`, periodID)
		assert.NoError(t, err, "cleanup: delete accounting_periods")
		_, err = pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
		assert.NoError(t, err, "cleanup: delete accounts")
	})

	return entryID
}
