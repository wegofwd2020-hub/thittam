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
