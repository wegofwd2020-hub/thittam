//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/billing"
	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
)

// TestDunningAttempts_TenantIsolation proves ListDunningAttempts and
// CreateDunningAttempt are scoped to the tenant that owns the parent invoice,
// even though dunning_attempts itself carries no tenant_id column (#173).
func TestDunningAttempts_TenantIsolation(t *testing.T) {
	pool := testdb.Open(t) // runtime role (thittam_app since #122); DML only
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	tenantA := uuid.New()
	tenantB := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active'), ($4, $5, $6, 'US', 'USD', 'active')`,
		tenantA, "Dunning IT A "+tenantA.String()[:8], "dun-a-"+tenantA.String()[:8],
		tenantB, "Dunning IT B "+tenantB.String()[:8], "dun-b-"+tenantB.String()[:8])
	require.NoError(t, err, "seed tenants")
	t.Cleanup(func() {
		// Delete child-first. invoices.tenant_id and invoices.subscription_id are
		// plain REFERENCES with NO ON DELETE action (migrations/billing/001:29-30),
		// so deleting tenants while invoices exist raises an FK violation — which
		// an ignored Exec error would hide, leaking every row.
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM dunning_attempts WHERE invoice_id IN (SELECT id FROM invoices WHERE tenant_id IN ($1, $2))`,
			`DELETE FROM invoices WHERE tenant_id IN ($1, $2)`,
			`DELETE FROM subscriptions WHERE tenant_id IN ($1, $2)`,
			`DELETE FROM tenants WHERE id IN ($1, $2)`,
		} {
			_, err := pool.Exec(ctx, q, tenantA, tenantB)
			assert.NoError(t, err, "cleanup: %s", q)
		}
	})

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID:                 uuid.New(),
		TenantID:           tenantA,
		Plan:               "starter",
		Status:             "active",
		BillingCycle:       "monthly",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub), "create subscription for tenant A")

	invAID := uuid.New()
	invA := &billing.Invoice{
		ID:             invAID,
		TenantID:       tenantA,
		SubscriptionID: sub.ID,
		InvoiceNumber:  "INV-DUN-" + invAID.String(),
		Plan:           "starter",
		Amount:         decimal.NewFromInt(100),
		TaxAmount:      decimal.Zero,
		TotalAmount:    decimal.NewFromInt(100),
		Currency:       "USD",
		Status:         "pending",
		DueDate:        now.AddDate(0, 0, 14),
		PeriodStart:    now,
		PeriodEnd:      now.AddDate(0, 1, 0),
		CreatedAt:      now,
	}
	require.NoError(t, repo.CreateInvoice(ctx, invA), "create invoice for tenant A")

	attempt := &billing.DunningAttempt{
		ID:            uuid.New(),
		InvoiceID:     invA.ID,
		AttemptNumber: 1,
		Result:        "card_declined",
		AttemptedAt:   now,
	}
	require.NoError(t, repo.CreateDunningAttempt(ctx, tenantA, attempt), "seed dunning attempt for tenant A")

	// Cross-tenant list must return zero rows, not tenant A's attempts.
	gotB, err := repo.ListDunningAttempts(ctx, tenantB, invA.ID)
	require.NoError(t, err)
	assert.Empty(t, gotB, "tenant B must not see tenant A's dunning attempts")

	// Same-tenant list must still work (regression guard on the JOIN).
	gotA, err := repo.ListDunningAttempts(ctx, tenantA, invA.ID)
	require.NoError(t, err)
	require.Len(t, gotA, 1, "tenant A must still see its own dunning attempts")
	assert.Equal(t, attempt.ID, gotA[0].ID)
	assert.Equal(t, "card_declined", gotA[0].Result)

	// Cross-tenant create must fail — the invoice does not belong to tenant B —
	// and must not insert anything.
	forged := &billing.DunningAttempt{
		ID:            uuid.New(),
		InvoiceID:     invA.ID,
		AttemptNumber: 2,
		Result:        "card_declined",
		AttemptedAt:   now,
	}
	err = repo.CreateDunningAttempt(ctx, tenantB, forged)
	require.ErrorIs(t, err, billing.ErrInvoiceNotFound)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM dunning_attempts WHERE id = $1`, forged.ID).Scan(&count))
	assert.Equal(t, 0, count, "cross-tenant create must not insert a row")
}
