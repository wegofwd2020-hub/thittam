//go:build integration

// Integration test for the retention-sweeper legal-hold filter introduced
// in #92 Stage 4. Verifies that ListTenantsDueForLifecycle:
//
//   - Returns tenants with no hold (the baseline)
//   - Skips tenants on indefinite hold (freeze_reason set, hold_until NULL)
//   - Skips tenants on an active time-bounded hold (hold_until in the future)
//   - Returns tenants whose time-bounded hold has expired
//
// Each test wraps its work in testdb.NewTx so rollback is automatic — no
// cross-test interference.

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

// insertSuspendedTenant creates a tenants row in 'suspended' state with
// suspended_at set to 45 days before now — well past the 30-day sweeper
// threshold so the row is always a candidate without the hold filter.
func insertSuspendedTenant(
	t *testing.T,
	tx pgx.Tx,
	name string,
	holdUntil *time.Time,
	freezeReason *string,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := tx.Exec(context.Background(),
		`INSERT INTO tenants (
            id, name, slug, country_code, primary_currency_code,
            status, suspended_at, hold_until, freeze_reason
         )
         VALUES ($1, $2, $3, 'US', 'USD',
                 'suspended', now() - INTERVAL '45 days',
                 $4, $5)`,
		id, name, "slug-"+id.String()[:8], holdUntil, freezeReason)
	require.NoError(t, err, "insert tenant %q", name)
	return id
}

func TestListTenantsDueForLifecycle_NoHold_Listed(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	id := insertSuspendedTenant(t, tx, "Unheld Studios", nil, nil)

	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(),
		Limit:   100,
	})
	require.NoError(t, err)

	assert.True(t, containsTenant(rows, id), "tenant with no hold must be listed")
}

func TestListTenantsDueForLifecycle_IndefiniteHold_Skipped(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	reason := "SEC litigation — indefinite preservation"
	id := insertSuspendedTenant(t, tx, "Frozen Studios", nil, &reason)

	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(),
		Limit:   100,
	})
	require.NoError(t, err)

	assert.False(t, containsTenant(rows, id),
		"tenant on indefinite hold (freeze_reason set, hold_until NULL) must never be listed")
}

func TestListTenantsDueForLifecycle_ActiveHold_Skipped(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	future := time.Now().UTC().Add(30 * 24 * time.Hour)
	reason := "30-day discovery window"
	id := insertSuspendedTenant(t, tx, "Discovery Studios", &future, &reason)

	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(),
		Limit:   100,
	})
	require.NoError(t, err)

	assert.False(t, containsTenant(rows, id),
		"tenant with hold_until in the future must be skipped")
}

func TestListTenantsDueForLifecycle_ExpiredHold_Listed(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	// Hold expired yesterday — lifecycle transitions should resume.
	past := time.Now().UTC().Add(-24 * time.Hour)
	reason := "discovery window (now elapsed)"
	id := insertSuspendedTenant(t, tx, "Released Studios", &past, &reason)

	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(),
		Limit:   100,
	})
	require.NoError(t, err)

	assert.True(t, containsTenant(rows, id),
		"tenant whose hold_until is in the past must be listed again")
}

func TestClearTenantLegalHold_RestoresSweeperListing(t *testing.T) {
	// End-to-end of the hold lifecycle: a tenant on indefinite hold is
	// skipped by ListTenantsDueForLifecycle; after ClearTenantLegalHold
	// it becomes a candidate again.
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	reason := "SEC litigation — indefinite"
	id := insertSuspendedTenant(t, tx, "Pre-Clear Studios", nil, &reason)

	// Baseline: tenant is on hold, sweeper must skip it.
	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(),
		Limit:   100,
	})
	require.NoError(t, err)
	require.False(t, containsTenant(rows, id), "sanity: held tenant must be skipped before clearing")

	// Clear the hold. Returned row must have both columns NULL.
	cleared, err := q.ClearTenantLegalHold(context.Background(), id)
	require.NoError(t, err)
	assert.False(t, cleared.HoldUntil.Valid, "hold_until must be NULL after clear")
	assert.False(t, cleared.FreezeReason.Valid, "freeze_reason must be NULL after clear")

	// Post-clear: sweeper must now list the tenant.
	rows, err = q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(),
		Limit:   100,
	})
	require.NoError(t, err)
	assert.True(t, containsTenant(rows, id), "cleared tenant must be listable for lifecycle again")
}

func TestCountTenantsOnHold(t *testing.T) {
	// Baseline 0, then one held tenant bumps the count to 1.
	// Exercises the sweeper's per-run gauge query (#92 Stage 5).
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	// Baseline: transaction is fresh + rollback-isolated, so any
	// pre-existing held tenants in the DB are invisible here.
	n, err := q.CountTenantsOnHold(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "baseline should be zero inside a fresh tx")

	reason := "SEC litigation"
	insertSuspendedTenant(t, tx, "Held Studios", nil, &reason)

	n, err = q.CountTenantsOnHold(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	// A second tenant with no hold does not bump the count.
	insertSuspendedTenant(t, tx, "Free Studios", nil, nil)
	n, err = q.CountTenantsOnHold(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestClearTenantLegalHold_NoHold_Idempotent(t *testing.T) {
	// Clearing a tenant that has no hold succeeds silently and returns
	// the row unchanged — the service layer's "quiet on no-op" audit
	// behavior depends on this repo contract.
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	id := insertSuspendedTenant(t, tx, "No-Hold Studios", nil, nil)

	row, err := q.ClearTenantLegalHold(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, id, row.ID)
	assert.False(t, row.HoldUntil.Valid)
	assert.False(t, row.FreezeReason.Valid)
}

func TestSetTenantLegalHold_AppliesIndefiniteHold_SkipsSweeper(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	// Unheld suspended tenant is a sweeper candidate.
	id := insertSuspendedTenant(t, tx, "To-Hold Studios", nil, nil)
	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(), Limit: 100,
	})
	require.NoError(t, err)
	require.True(t, containsTenant(rows, id), "sanity: unheld tenant is a candidate before hold")

	// Apply an indefinite hold via the new write path.
	held, err := q.SetTenantLegalHold(context.Background(), iamdb.SetTenantLegalHoldParams{
		ID:           id,
		HoldUntil:    pgtype.Timestamptz{}, // NULL = indefinite
		FreezeReason: pgtype.Text{String: "support escalation", Valid: true},
	})
	require.NoError(t, err)
	assert.True(t, held.FreezeReason.Valid)
	assert.Equal(t, "support escalation", held.FreezeReason.String)
	assert.False(t, held.HoldUntil.Valid, "indefinite hold => hold_until NULL")
	assert.Equal(t, "suspended", held.Status, "status must be unchanged by a hold write")

	// Sweeper now skips it.
	rows, err = q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(), Limit: 100,
	})
	require.NoError(t, err)
	assert.False(t, containsTenant(rows, id), "held tenant must be skipped by the sweeper")
}

// TestSetTenantLegalHold_AppliesDatedHold_SkipsSweeperUntilExpiry mirrors
// TestSetTenantLegalHold_AppliesIndefiniteHold_SkipsSweeper above but for a
// dated hold_until (#119 fix-wave FIX 2): the write must round-trip the
// future timestamp + reason, leave status untouched, and the sweeper must
// skip the tenant while the hold is still active.
func TestSetTenantLegalHold_AppliesDatedHold_SkipsSweeperUntilExpiry(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	id := insertSuspendedTenant(t, tx, "Dated-Hold Studios", nil, nil)

	future := time.Now().UTC().Add(45 * 24 * time.Hour)
	reason := "retention-extended: ticket-119"
	held, err := q.SetTenantLegalHold(context.Background(), iamdb.SetTenantLegalHoldParams{
		ID:           id,
		HoldUntil:    pgtype.Timestamptz{Time: future, Valid: true},
		FreezeReason: pgtype.Text{String: reason, Valid: true},
	})
	require.NoError(t, err)
	require.True(t, held.HoldUntil.Valid, "dated hold must round-trip hold_until")
	assert.WithinDuration(t, future, held.HoldUntil.Time, time.Second)
	require.True(t, held.FreezeReason.Valid)
	assert.Equal(t, reason, held.FreezeReason.String)
	assert.Equal(t, "suspended", held.Status, "status must be unchanged by a hold write")

	rows, err := q.ListTenantsDueForLifecycle(context.Background(), iamdb.ListTenantsDueForLifecycleParams{
		Column1: time.Now().UTC(), Limit: 100,
	})
	require.NoError(t, err)
	assert.False(t, containsTenant(rows, id), "tenant on a future-dated hold must be skipped by the sweeper")
}

// TestSetTenantLegalHold_PreservesSuspendedAtAnchor asserts that applying a
// legal hold does not disturb the suspended_at anchor the retention clock is
// measured from (#119 fix-wave FIX 2). insertSuspendedTenant backdates
// suspended_at to now()-45 days; SetTenantLegalHold's backing SQL only
// touches hold_until/freeze_reason, so the anchor must survive unchanged and
// deactivated_at must remain unset.
func TestSetTenantLegalHold_PreservesSuspendedAtAnchor(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	id := insertSuspendedTenant(t, tx, "Anchor Studios", nil, nil)
	wantSuspendedAt := time.Now().UTC().Add(-45 * 24 * time.Hour)

	held, err := q.SetTenantLegalHold(context.Background(), iamdb.SetTenantLegalHoldParams{
		ID:           id,
		HoldUntil:    pgtype.Timestamptz{}, // indefinite
		FreezeReason: pgtype.Text{String: "support escalation", Valid: true},
	})
	require.NoError(t, err)
	require.True(t, held.SuspendedAt.Valid, "suspended_at anchor must remain set after a hold write")
	assert.WithinDuration(t, wantSuspendedAt, held.SuspendedAt.Time, time.Minute,
		"suspended_at must be unchanged from the value insertSuspendedTenant set")
	assert.False(t, held.DeactivatedAt.Valid, "deactivated_at must remain unset")
}

// TestPostgresSetTenantLegalHold_UnknownID_ReturnsErrTenantNotFound exercises
// the *Postgres wrapper (not the raw sqlc Queries) for an id that doesn't
// exist, proving pgx.ErrNoRows maps to iam.ErrTenantNotFound the same way
// Postgres.ClearTenantLegalHold already does (#119 fix-wave FIX 2). No insert
// is needed — a random UUID is guaranteed not to match — so this is cheap
// enough to run straight off the pool without the tx-rollback harness.
func TestPostgresSetTenantLegalHold_UnknownID_ReturnsErrTenantNotFound(t *testing.T) {
	pool := testdb.Open(t)
	repo := iamdb.NewPostgres(pool)

	_, err := repo.SetTenantLegalHold(context.Background(), uuid.New(), nil, "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, iam.ErrTenantNotFound)
}

func containsTenant(rows []iamdb.Tenant, id uuid.UUID) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}
