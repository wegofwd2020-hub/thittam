//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/billing"
	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
)

func TestOutbox_SuspendWritesAtomically_ThenClaimAndMark(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	tenantID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active')`,
		tenantID, "Outbox IT "+tenantID.String()[:8], "obx-"+tenantID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, tenantID) })

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID: uuid.New(), TenantID: tenantID, Plan: "starter", Status: "active", BillingCycle: "monthly",
		CurrentPeriodStart: now, CurrentPeriodEnd: now.AddDate(0, 1, 0), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub))

	// Atomic suspend + outbox.
	sus := now
	sub.Status = "suspended"
	sub.SuspendedAt = &sus
	sub.UpdatedAt = now
	require.NoError(t, repo.SuspendSubscriptionWithOutbox(ctx, sub,
		"thittam.billing.subscription.suspended", []byte(`{"subscription_id":"x"}`)))

	// Suspend persisted...
	got, err := repo.GetSubscriptionByTenant(ctx, tenantID)
	require.NoError(t, err)
	assert.Equal(t, "suspended", got.Status)

	// ...and exactly one claimable outbox row for this tenant.
	claimed, err := repo.ClaimUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	var mine *billing.OutboxEvent
	for _, e := range claimed {
		if e.TenantID == tenantID {
			mine = e
		}
	}
	require.NotNil(t, mine, "the suspend wrote a claimable outbox row")
	assert.Equal(t, 1, mine.Attempts, "claim increments attempts")

	require.NoError(t, repo.MarkOutboxSent(ctx, mine.ID))

	// After marking sent, it is no longer claimable.
	again, err := repo.ClaimUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	for _, e := range again {
		assert.NotEqual(t, mine.ID, e.ID, "sent row must not be re-claimed")
	}
}

func TestOutboxDLQ_MoveReplayAndStats(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	// Baseline: other tests' rows may already be present.
	base, err := repo.OutboxStats(ctx)
	require.NoError(t, err)

	tenantID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active')`,
		tenantID, "DLQ IT "+tenantID.String()[:8], "dlq-"+tenantID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM event_outbox_dead WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(bg, `DELETE FROM event_outbox WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(bg, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID: uuid.New(), TenantID: tenantID, Plan: "starter", Status: "active", BillingCycle: "monthly",
		CurrentPeriodStart: now, CurrentPeriodEnd: now.AddDate(0, 1, 0), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub))

	sus := now
	sub.Status = "suspended"
	sub.SuspendedAt = &sus
	sub.UpdatedAt = now
	require.NoError(t, repo.SuspendSubscriptionWithOutbox(ctx, sub,
		"thittam.billing.subscription.suspended", []byte(`{"subscription_id":"x"}`)))

	// Find our row among any others.
	ev := claimMine(t, ctx, repo, tenantID)
	origCreatedAt := ev.CreatedAt

	stats, err := repo.OutboxStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, base.Pending+1, stats.Pending, "our suspend added one pending row")
	assert.Equal(t, base.Dead, stats.Dead)
	assert.GreaterOrEqual(t, stats.OldestPendingSeconds, 0.0)

	// Move: atomic, preserves id and created_at.
	require.NoError(t, repo.MoveOutboxToDead(ctx, ev.ID, "stream rejected payload"))

	stats, err = repo.OutboxStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, base.Pending, stats.Pending, "moved row must leave event_outbox")
	assert.Equal(t, base.Dead+1, stats.Dead)

	// A dead row is never claimed again.
	assert.Nil(t, tryClaimMine(t, ctx, repo, tenantID), "dead rows must not be claimable")

	dead, err := repo.ListDeadOutbox(ctx, 100)
	require.NoError(t, err)
	var mineDead *billing.OutboxEvent
	for _, d := range dead {
		if d.ID == ev.ID {
			mineDead = d
		}
	}
	require.NotNil(t, mineDead, "our event is in the DLQ")
	assert.WithinDuration(t, origCreatedAt, mineDead.CreatedAt, time.Second, "created_at preserved")
	require.NotNil(t, mineDead.LastError)
	assert.Contains(t, *mineDead.LastError, "stream rejected")

	// Replay: attempts reset, row claimable again.
	require.NoError(t, repo.ReplayDeadOutbox(ctx, ev.ID))

	stats, err = repo.OutboxStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, base.Pending+1, stats.Pending)
	assert.Equal(t, base.Dead, stats.Dead)

	replayed := claimMine(t, ctx, repo, tenantID)
	assert.Equal(t, ev.ID, replayed.ID)
	assert.Equal(t, 1, replayed.Attempts, "replay reset attempts to 0; this claim incremented it to 1")
	assert.Nil(t, replayed.LastError)
}

func TestOutboxDLQ_ReplayUnknownID(t *testing.T) {
	pool := testdb.Open(t)
	repo := billingdb.NewPostgres(pool)

	err := repo.ReplayDeadOutbox(context.Background(), uuid.New())
	assert.ErrorIs(t, err, billing.ErrOutboxEventNotFound)
}

// claimMine claims a batch and returns this tenant's event, failing if absent.
func claimMine(t *testing.T, ctx context.Context, repo *billingdb.Postgres, tenantID uuid.UUID) *billing.OutboxEvent {
	t.Helper()
	e := tryClaimMine(t, ctx, repo, tenantID)
	require.NotNil(t, e, "expected a claimable outbox row for this tenant")
	return e
}

// tryClaimMine claims a batch and returns this tenant's event, or nil.
func tryClaimMine(t *testing.T, ctx context.Context, repo *billingdb.Postgres, tenantID uuid.UUID) *billing.OutboxEvent {
	t.Helper()
	claimed, err := repo.ClaimUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	for _, e := range claimed {
		if e.TenantID == tenantID {
			return e
		}
	}
	return nil
}
