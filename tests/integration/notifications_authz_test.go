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
