//go:build integration

// Integration test for the notifications self-scope fix (#139 D9): the
// personal inbox (GetNotification / ListNotifications) must be scoped to the
// caller's own recipient id, not just the tenant. Exercises the actual
// Postgres repository (services/notifications/db.Postgres), not a double, so
// the AND recipient_id = $N predicate wired into GetNotification (:161) and
// ListNotifications (:191) is the thing under test.
//
// Uses pkg/testdb (SKIPs without THITTAM_TEST_DSN); inserts rows directly via
// the pool and cleans them up in t.Cleanup, following the pattern in
// services/iam/db/tenant_find_by_name_integration_test.go — the Postgres
// wrapper under test needs a *pgxpool.Pool, not a tx, so this cannot use
// testdb.NewTx's auto-rollback.
package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/notifications"
	notificationsdb "github.com/wegofwd2020/thittam/services/notifications/db"
)

func TestNotifications_SelfScoped_ListAndGetOnlyReturnCallersOwnRows(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := notificationsdb.NewPostgres(pool)

	tenantID := uuid.New()
	recipientA := uuid.New()
	recipientB := uuid.New()

	notifA := insertNotificationLog(t, pool, tenantID, recipientA, "email", "sent")
	notifB := insertNotificationLog(t, pool, tenantID, recipientB, "email", "sent")

	// ListNotifications as A must return only A's row, never B's.
	gotA, err := repo.ListNotifications(ctx, tenantID, recipientA, "", "", 20, 0)
	require.NoError(t, err)
	ids := make([]uuid.UUID, len(gotA))
	for i, n := range gotA {
		ids[i] = n.ID
	}
	assert.Contains(t, ids, notifA, "A's own notification must be listed")
	assert.NotContains(t, ids, notifB, "B's notification must never appear in A's list")

	// GetNotification for B's id, called as A, must be indistinguishable from
	// a truly missing row — ErrNotificationNotFound, not a permission error
	// that would confirm B's row exists (no existence oracle).
	_, err = repo.GetNotification(ctx, tenantID, recipientA, notifB)
	assert.ErrorIs(t, err, notifications.ErrNotificationNotFound)

	// A can still read A's own notification.
	got, err := repo.GetNotification(ctx, tenantID, recipientA, notifA)
	require.NoError(t, err)
	assert.Equal(t, notifA, got.ID)
}

// backfillNotificationsRead is the exact statement from
// migrations/iam/023_seed_notifications_permissions.up.sql. Keep the two in sync.
const backfillNotificationsRead = `
UPDATE roles SET permissions = array_append(permissions, 'notifications:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('notifications:read' = ANY (permissions))`

// backfillNotificationsManage is the exact statement from
// migrations/iam/023_seed_notifications_permissions.up.sql. Keep the two in sync.
const backfillNotificationsManage = `
UPDATE roles SET permissions = array_append(permissions, 'notifications:manage')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('notifications:manage' = ANY (permissions))`

// backfillNotificationsReadDown is the exact statement from
// migrations/iam/023_seed_notifications_permissions.down.sql. Keep the two in sync.
const backfillNotificationsReadDown = `
UPDATE roles SET permissions = array_remove(permissions, 'notifications:read')   WHERE is_system = true`

// backfillNotificationsManageDown is the exact statement from
// migrations/iam/023_seed_notifications_permissions.down.sql. Keep the two in sync.
const backfillNotificationsManageDown = `
UPDATE roles SET permissions = array_remove(permissions, 'notifications:manage') WHERE is_system = true`

// TestMigration023_GrantsNotificationsPermissionsIdempotently applies
// migration 023's up statements to a fresh tenant schema, runs them twice,
// and asserts super_admin and manager each hold both notifications:read and
// notifications:manage exactly once — the property the migration depends on
// since it runs against the public schema via `make migrate-all` AND against
// every new tenant_<uuid> at CreateTenant (a non-idempotent statement would
// duplicate entries). It then applies the down statements and asserts both
// strings are gone.
func TestMigration023_GrantsNotificationsPermissionsIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Notifications Backfill Test "+tenantID.String(), "notif-backfill-"+tenantID.String())
	require.NoError(t, err)

	// super_admin and manager as they exist BEFORE the migration: neither
	// notifications permission present yet.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'super_admin', $3, true)`,
		uuid.New(), tenantID, []string{"production:read", "billing:read", "billing:manage"})
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'manager', $3, true)`,
		uuid.New(), tenantID, []string{"production:read", "billing:read", "billing:manage"})
	require.NoError(t, err)

	applyNotificationsUp := func() {
		_, err := tx.Exec(ctx, backfillNotificationsRead)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, backfillNotificationsManage)
		require.NoError(t, err)
	}

	// First application appends both permissions.
	applyNotificationsUp()
	// Second application (migration 023 run again) must be a no-op.
	applyNotificationsUp()

	for _, role := range []string{"super_admin", "manager"} {
		var perms []string
		require.NoError(t, tx.QueryRow(ctx,
			`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`,
			tenantID, role).Scan(&perms))
		require.Contains(t, perms, "notifications:read", "%s must hold notifications:read", role)
		require.Contains(t, perms, "notifications:manage", "%s must hold notifications:manage", role)
		require.Equal(t, 1, countOccurrences(perms, "notifications:read"),
			"re-running the migration must not duplicate notifications:read for %s", role)
		require.Equal(t, 1, countOccurrences(perms, "notifications:manage"),
			"re-running the migration must not duplicate notifications:manage for %s", role)
	}

	// Down removes both strings from both roles.
	_, err = tx.Exec(ctx, backfillNotificationsReadDown)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, backfillNotificationsManageDown)
	require.NoError(t, err)

	for _, role := range []string{"super_admin", "manager"} {
		var perms []string
		require.NoError(t, tx.QueryRow(ctx,
			`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = $2`,
			tenantID, role).Scan(&perms))
		require.NotContains(t, perms, "notifications:read", "down migration must remove notifications:read from %s", role)
		require.NotContains(t, perms, "notifications:manage", "down migration must remove notifications:manage from %s", role)
	}
}

func countOccurrences(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}

// insertNotificationLog inserts a notification_log row directly via the pool
// (the Postgres wrapper under test needs a *pgxpool.Pool, not a tx) and
// registers a t.Cleanup to delete it so it doesn't leak into other tests.
func insertNotificationLog(t *testing.T, pool *pgxpool.Pool, tenantID, recipientID uuid.UUID, channel, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO notification_log (id, tenant_id, recipient_id, channel, event_type, status)
		 VALUES ($1, $2, $3, $4, 'test.event', $5)`,
		id, tenantID, recipientID, channel, status)
	require.NoError(t, err, "insert notification_log")
	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), `DELETE FROM notification_log WHERE id = $1`, id)
		assert.NoError(t, err, "cleanup: delete notification_log %s", id)
	})
	return id
}
