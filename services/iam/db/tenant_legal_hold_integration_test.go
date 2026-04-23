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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
	"github.com/wegofwd2020/thittam/pkg/testdb"
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

func containsTenant(rows []iamdb.Tenant, id uuid.UUID) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}
