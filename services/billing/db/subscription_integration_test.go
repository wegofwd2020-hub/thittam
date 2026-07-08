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

// TestSubscriptionRoundTrip_SuspendFields proves the migrated billing schema
// (#130) accepts what the live repo writes — in particular the suspend fields
// (suspended_at, status='suspended') that migration 001 was missing before the
// reconcile. Guards against re-drift between code and migration.
func TestSubscriptionRoundTrip_SuspendFields(t *testing.T) {
	pool := testdb.Open(t) // owner role; connects to a migrated thittam_test
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	tenantID := uuid.New()
	// FK parent: subscriptions.tenant_id REFERENCES tenants(id). Seed one.
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active')`,
		tenantID, "Billing IT "+tenantID.String()[:8], "bil-"+tenantID.String()[:8])
	require.NoError(t, err, "seed tenant")
	t.Cleanup(func() {
		// ON DELETE CASCADE removes the subscription too.
		_, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		Plan:               "starter",
		Status:             "active",
		BillingCycle:       "monthly",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub), "create subscription")

	// The suspend write — exercises the columns 001 was missing.
	suspendedAt := now
	sub.Status = "suspended"
	sub.SuspendedAt = &suspendedAt
	sub.UpdatedAt = now
	require.NoError(t, repo.UpdateSubscription(ctx, sub), "suspend subscription")

	got, err := repo.GetSubscriptionByTenant(ctx, tenantID)
	require.NoError(t, err, "get subscription")
	assert.Equal(t, "suspended", got.Status)
	require.NotNil(t, got.SuspendedAt, "suspended_at must persist (the column 001 lacked)")
}
