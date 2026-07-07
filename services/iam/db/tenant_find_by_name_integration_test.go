//go:build integration

// Integration test for FindTenantByNormalizedName (#89 Task 2), which will
// power the pre-flight duplicate check in Service.CreateTenant (Task 3).
// Verifies the query applies the SAME normalisation as the tenants_name_ci_unique
// index added in migration 018 (case-insensitive, trimmed, internal whitespace
// collapsed), and that a non-matching name surfaces as pgx.ErrNoRows.
//
// Constructor note: there is no tx-based constructor for the Postgres repo
// wrapper in this codebase (services/iam/db/postgres.go's NewPostgres only
// accepts a *pgxpool.Pool, and this holds across every service). Following the
// existing convention in this package (see tenant_legal_hold_integration_test.go
// and user_roles_scope_integration_test.go), this test exercises the
// sqlc-generated Queries directly via iamdb.New(tx) rather than the Postgres
// wrapper. Postgres.FindTenantByNormalizedName's translation of pgx.ErrNoRows
// to (nil, nil) is a trivial, already-established pattern (identical to
// GetTenant's ErrTenantNotFound mapping) verified by code review / the package
// build, not a DB round trip.
//
// Each test wraps its work in testdb.NewTx so rollback is automatic.

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestFindTenantByNormalizedName_CaseAndWhitespaceVariedLookup_Finds(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	want := insertTenant(t, tx, "Acme  Corp") // two internal spaces

	got, err := q.FindTenantByNormalizedName(context.Background(), "  acme corp ")
	require.NoError(t, err)
	assert.Equal(t, want, got.ID)
}

func TestFindTenantByNormalizedName_NoMatch_ReturnsErrNoRows(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	q := iamdb.New(tx)

	insertTenant(t, tx, "Acme Corp")

	_, err := q.FindTenantByNormalizedName(context.Background(), "Nobody Inc")
	require.ErrorIs(t, err, pgx.ErrNoRows,
		"no matching row must surface as pgx.ErrNoRows so Postgres.FindTenantByNormalizedName can map it to (nil, nil)")
}
