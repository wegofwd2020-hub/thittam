//go:build integration

// Integration test for Postgres.UpdateUser (#139 slice B, Task 3). Before
// this fix the UPDATE statement wrote status unconditionally, so a client
// updating only display_name sent status: "" and silently wiped the column
// to the empty string. status is security-critical: pkg/auth/local.go
// refuses login for 'deactivated' and 'invited' accounts, and the empty
// string matches neither case, so a wipe silently reactivated a deactivated
// account.
//
// Constructor note: Postgres.UpdateUser is a raw inline statement (not
// sqlc), and NewPostgres only accepts a *pgxpool.Pool (see
// tenant_find_by_name_integration_test.go for the same caveat), so this test
// exercises the wrapper via the pool rather than a rollback-on-cleanup tx,
// and cleans up the row it inserts with t.Cleanup.

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

// insertDeactivatedUserViaPool inserts a tenant + a user with
// status = 'deactivated' directly through the pool (not a tx), since the
// Postgres wrapper under test needs a *pgxpool.Pool. Registers a t.Cleanup
// that deletes both rows so they don't leak into other tests.
func insertDeactivatedUserViaPool(t *testing.T, pool *pgxpool.Pool) (tenantID, userID uuid.UUID) {
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
		 VALUES ($1, $2, $3, $4, 'x', 'deactivated')`,
		userID, tenantID, userID.String()+"@test.local", "Original Name")
	require.NoError(t, err, "insert user")

	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		assert.NoError(t, err, "cleanup: delete user %s", userID)
		_, err = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant %s", tenantID)
	})

	return tenantID, userID
}

// TestPostgresUpdateUser_EmptyStatusPreservesExistingStatus proves the fixed
// UPDATE statement: calling Postgres.UpdateUser with Status: "" (as an
// ordinary display-name-only edit does) must leave a deactivated account
// deactivated, while still applying the display_name change -- proving the
// UPDATE ran rather than silently matching zero rows.
func TestPostgresUpdateUser_EmptyStatusPreservesExistingStatus(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	tenantID, userID := insertDeactivatedUserViaPool(t, pool)

	err := repo.UpdateUser(ctx, &iam.User{
		ID:          userID,
		TenantID:    tenantID,
		DisplayName: "Renamed",
		Status:      "", // deliberately empty -- must not wipe the column
	})
	require.NoError(t, err)

	var displayName, status string
	err = pool.QueryRow(ctx,
		`SELECT display_name, status FROM users WHERE id = $1`, userID,
	).Scan(&displayName, &status)
	require.NoError(t, err)

	assert.Equal(t, "Renamed", displayName, "display_name should have been updated")
	assert.Equal(t, "deactivated", status, "an empty status must not wipe the existing value")
}
