//go:build integration

// Integration test for Postgres.ActivateUser (#162 Task 2). ActivateUser is
// backed by the conditional ReactivateUser query
// (`UPDATE users SET status = 'active' WHERE id = $1 AND tenant_id = $2 AND
// status = 'deactivated'`) — sqlc validates the SELECT-* RETURNING expansion
// but not the bare WHERE-clause literal, so this is the authoritative proof
// the guard actually runs against Postgres (see reference_sqlc_where_clause_blind_spot).
//
// Constructor note: mirrors user_status_preserve_integration_test.go — the
// Postgres wrapper needs a *pgxpool.Pool, so seed data goes in directly
// through the pool with t.Cleanup, not a rollback-on-cleanup tx.

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

// insertUserWithStatusViaPool inserts a tenant + a user with the given status
// directly through the pool (not a tx), since the Postgres wrapper under test
// needs a *pgxpool.Pool. Registers a t.Cleanup that deletes both rows so they
// don't leak into other tests.
func insertUserWithStatusViaPool(t *testing.T, pool *pgxpool.Pool, status string) (tenantID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	tenantID = uuid.New()
	userID = uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'US', 'USD')`,
		tenantID, "test-tenant-"+tenantID.String()[:8], "test-"+tenantID.String()[:8])
	require.NoError(t, err, "insert tenant")

	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, tenant_id, email, display_name, password_hash, status)
		 VALUES ($1, $2, $3, $4, 'x', $5)`,
		userID, tenantID, userID.String()+"@test.local", "Original Name", status)
	require.NoError(t, err, "insert user")

	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		assert.NoError(t, err, "cleanup: delete user %s", userID)
		_, err = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant %s", tenantID)
	})

	return tenantID, userID
}

// TestPostgresActivateUser_DeactivatedToActive proves the happy path: a
// 'deactivated' user is restored to 'active'.
func TestPostgresActivateUser_DeactivatedToActive(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	tenantID, userID := insertUserWithStatusViaPool(t, pool, "deactivated")

	err := repo.ActivateUser(ctx, tenantID, userID)
	require.NoError(t, err)

	var status string
	err = pool.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "active", status)
}

// TestPostgresActivateUser_AlreadyActive_ReturnsErrNotDeactivated proves the
// guard: an already-'active' user is not force-activated, and the bare WHERE
// literal (status = 'deactivated') really is enforced by Postgres, not just
// by sqlc's generated Go — a bug here would silently no-op instead of erroring.
func TestPostgresActivateUser_AlreadyActive_ReturnsErrNotDeactivated(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	tenantID, userID := insertUserWithStatusViaPool(t, pool, "active")

	err := repo.ActivateUser(ctx, tenantID, userID)
	require.Error(t, err)
	assert.ErrorIs(t, err, iam.ErrNotDeactivated)

	var status string
	err = pool.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "active", status, "an already-active user must be left untouched")
}

// TestPostgresActivateUser_MissingUser_ReturnsErrUserNotFound proves the
// other disambiguation branch: no row at all (wrong id or wrong tenant).
func TestPostgresActivateUser_MissingUser_ReturnsErrUserNotFound(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	err := repo.ActivateUser(ctx, uuid.New(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, iam.ErrUserNotFound)
}
