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
// and user_roles_scope_integration_test.go), the first two tests below exercise
// the sqlc-generated Queries directly via iamdb.New(tx) rather than the
// Postgres wrapper.
//
// Postgres.FindTenantByNormalizedName's translation of pgx.ErrNoRows to
// (nil, nil) is safety-critical (Task 3's pre-flight duplicate check in
// Service.CreateTenant relies on it) and is NOT exercised by the two tests
// above, since they never touch the wrapper.
// TestFindTenantByNormalizedName_PostgresWrapper_NormalizesAndReturnsNilOnNoMatch
// closes that gap by calling the wrapper method through db.NewPostgres(pool)
// directly. Because the wrapper needs a *pgxpool.Pool (not a tx), it can't
// use the testdb.NewTx rollback pattern — it inserts via the pool and cleans
// up with a t.Cleanup DELETE instead.
//
// The first two tests wrap their work in testdb.NewTx so rollback is automatic.

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
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

// insertTenantViaPool inserts a tenant directly through the pool (not a tx),
// since the Postgres wrapper under test needs a *pgxpool.Pool. Registers a
// t.Cleanup that deletes by the same case/whitespace-normalized comparison
// the tenants_name_ci_unique index and FindTenantByNormalizedName query use,
// so the row doesn't leak into other tests regardless of exact casing.
func insertTenantViaPool(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		id, name, "slug-"+id.String()[:8])
	require.NoError(t, err, "insert tenant %q", name)
	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(),
			`DELETE FROM tenants
			 WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
			     = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')`,
			name)
		assert.NoError(t, err, "cleanup: delete tenant %q", name)
	})
	return id
}

// TestFindTenantByNormalizedName_PostgresWrapper_NormalizesAndReturnsNilOnNoMatch
// exercises the actual Postgres.FindTenantByNormalizedName wrapper (via
// db.NewPostgres), not just the sqlc layer, asserting both halves of its
// contract:
//  1. a match is normalized (case/trim/internal-whitespace) and mapped from
//     the sqlc row to a non-nil *iam.Tenant with the right ID, and
//  2. no match surfaces as (nil, nil) — never an error — which Task 3's
//     pre-flight duplicate check in Service.CreateTenant depends on.
func TestFindTenantByNormalizedName_PostgresWrapper_NormalizesAndReturnsNilOnNoMatch(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	id := insertTenantViaPool(t, pool, "Fixup  Studios") // two internal spaces

	got, err := repo.FindTenantByNormalizedName(ctx, "  fixup studios ")
	require.NoError(t, err)
	require.NotNil(t, got, "expected a match for the normalized name")
	assert.Equal(t, id, got.ID)

	got, err = repo.FindTenantByNormalizedName(ctx, "No Such Tenant 8f3c2e11-49ab")
	require.NoError(t, err, "no-match must not be an error")
	assert.Nil(t, got, "no-match must return a nil tenant")
}
