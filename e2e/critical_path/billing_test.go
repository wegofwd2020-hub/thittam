package critical_path_test

// Critical Path 8: Billing Subscription Lifecycle
//
// Verifies end-to-end billing flow (in-process, no Razorpay/Stripe):
//   - CreateSubscription → "trialing" with 14-day trial
//   - UpgradeSubscription → plan change recorded
//   - CheckPlanLimit     → enforced against plan config
//   - CreateInvoice + MarkInvoicePaid → invoice transitions to "paid"
//   - AddPaymentMethod + SetDefaultPaymentMethod → default flag toggled
//   - CancelSubscription → status transitions to "cancelled"

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/services/billing"
)

// ── Deterministic IDs ─────────────────────────────────────────────────────

var (
	fixedBillingTenantID = uuid.MustParse("b0000000-0000-0000-0000-000000000001")
	fixedBillingInvID    = uuid.MustParse("b0000000-0000-0000-0000-000000000002")
	fixedBillingPMID     = uuid.MustParse("b0000000-0000-0000-0000-000000000003")
)

// ── In-process mock billing repository ───────────────────────────────────

type billingRepo struct {
	mu            sync.Mutex
	subs          map[uuid.UUID]*billing.Subscription  // keyed by tenantID
	invoices      map[uuid.UUID]*billing.Invoice        // keyed by invoiceID
	paymentMethods map[uuid.UUID]*billing.PaymentMethod // keyed by pmID
	usageRecords  []*billing.UsageRecord
	dunning       []*billing.DunningAttempt
	outbox        []*billing.OutboxEvent
	dead          []*billing.OutboxEvent
	seq           int
}

func newBillingRepo() *billingRepo {
	return &billingRepo{
		subs:           make(map[uuid.UUID]*billing.Subscription),
		invoices:       make(map[uuid.UUID]*billing.Invoice),
		paymentMethods: make(map[uuid.UUID]*billing.PaymentMethod),
	}
}

func (r *billingRepo) CreateSubscription(_ context.Context, s *billing.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[s.TenantID] = s
	return nil
}

func (r *billingRepo) GetSubscriptionByTenant(_ context.Context, tenantID uuid.UUID) (*billing.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.subs[tenantID]
	if !ok {
		return nil, billing.ErrSubscriptionNotFound
	}
	return sub, nil
}

func (r *billingRepo) UpdateSubscription(_ context.Context, s *billing.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[s.TenantID] = s
	return nil
}

func (r *billingRepo) CreateInvoice(_ context.Context, inv *billing.Invoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.invoices[inv.ID] = inv
	return nil
}

func (r *billingRepo) GetInvoice(_ context.Context, _, id uuid.UUID) (*billing.Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.invoices[id]
	if !ok {
		return nil, billing.ErrInvoiceNotFound
	}
	return inv, nil
}

func (r *billingRepo) ListInvoices(_ context.Context, tenantID uuid.UUID, _, _ int) ([]billing.Invoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []billing.Invoice
	for _, inv := range r.invoices {
		if inv.TenantID == tenantID {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (r *billingRepo) UpdateInvoice(_ context.Context, inv *billing.Invoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.invoices[inv.ID] = inv
	return nil
}

func (r *billingRepo) NextInvoiceSeq(_ context.Context, _ int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	return r.seq, nil
}

func (r *billingRepo) CreatePaymentMethod(_ context.Context, pm *billing.PaymentMethod) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paymentMethods[pm.ID] = pm
	return nil
}

func (r *billingRepo) GetPaymentMethod(_ context.Context, tenantID, id uuid.UUID) (*billing.PaymentMethod, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pm, ok := r.paymentMethods[id]
	if !ok || pm.TenantID != tenantID {
		return nil, billing.ErrPaymentMethodNotFound
	}
	return pm, nil
}

func (r *billingRepo) ListPaymentMethods(_ context.Context, tenantID uuid.UUID) ([]billing.PaymentMethod, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []billing.PaymentMethod
	for _, pm := range r.paymentMethods {
		if pm.TenantID == tenantID {
			out = append(out, *pm)
		}
	}
	return out, nil
}

func (r *billingRepo) UpdatePaymentMethod(_ context.Context, pm *billing.PaymentMethod) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paymentMethods[pm.ID] = pm
	return nil
}

func (r *billingRepo) DeletePaymentMethod(_ context.Context, tenantID, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pm, ok := r.paymentMethods[id]
	if !ok || pm.TenantID != tenantID {
		return billing.ErrPaymentMethodNotFound
	}
	delete(r.paymentMethods, id)
	return nil
}

func (r *billingRepo) ClearDefaultPaymentMethods(_ context.Context, tenantID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, pm := range r.paymentMethods {
		if pm.TenantID == tenantID {
			r.paymentMethods[id].IsDefault = false
		}
	}
	return nil
}

func (r *billingRepo) CreateUsageRecord(_ context.Context, u *billing.UsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usageRecords = append(r.usageRecords, u)
	return nil
}

func (r *billingRepo) LatestUsageRecord(_ context.Context, tenantID uuid.UUID) (*billing.UsageRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.usageRecords) - 1; i >= 0; i-- {
		if r.usageRecords[i].TenantID == tenantID {
			return r.usageRecords[i], nil
		}
	}
	return nil, fmt.Errorf("billing: no usage record for tenant")
}

func (r *billingRepo) CreateDunningAttempt(_ context.Context, d *billing.DunningAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dunning = append(r.dunning, d)
	return nil
}

func (r *billingRepo) ListDunningAttempts(_ context.Context, invoiceID uuid.UUID) ([]billing.DunningAttempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []billing.DunningAttempt
	for _, d := range r.dunning {
		if d.InvoiceID == invoiceID {
			out = append(out, *d)
		}
	}
	return out, nil
}

// --- Outbox (#126) ---

func (r *billingRepo) SuspendSubscriptionWithOutbox(_ context.Context, s *billing.Subscription, subject string, payload []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[s.TenantID] = s
	r.outbox = append(r.outbox, &billing.OutboxEvent{
		ID:        uuid.New(),
		Subject:   subject,
		TenantID:  s.TenantID,
		Payload:   payload,
		CreatedAt: time.Now(),
	})
	return nil
}

func (r *billingRepo) ClaimUnsentOutbox(_ context.Context, limit int) ([]*billing.OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*billing.OutboxEvent
	for _, e := range r.outbox {
		if e.SentAt == nil {
			e.Attempts++
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *billingRepo) MarkOutboxSent(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.outbox {
		if e.ID == id {
			now := time.Now()
			e.SentAt = &now
		}
	}
	return nil
}

func (r *billingRepo) RecordOutboxFailure(_ context.Context, id uuid.UUID, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.outbox {
		if e.ID == id {
			e.LastError = &errMsg
		}
	}
	return nil
}

func (r *billingRepo) DeleteSentOutboxOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []*billing.OutboxEvent
	var deleted int64
	for _, e := range r.outbox {
		if e.SentAt != nil && e.SentAt.Before(cutoff) {
			deleted++
			continue
		}
		kept = append(kept, e)
	}
	r.outbox = kept
	return deleted, nil
}

func (r *billingRepo) MoveOutboxToDead(_ context.Context, id uuid.UUID, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []*billing.OutboxEvent
	for _, e := range r.outbox {
		if e.ID == id {
			e.LastError = &errMsg
			r.dead = append(r.dead, e)
			continue
		}
		kept = append(kept, e)
	}
	r.outbox = kept
	return nil
}

func (r *billingRepo) OutboxStats(_ context.Context) (*billing.OutboxStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := &billing.OutboxStats{Dead: int64(len(r.dead))}
	var oldest time.Time
	for _, e := range r.outbox {
		if e.SentAt != nil {
			continue
		}
		s.Pending++
		if oldest.IsZero() || e.CreatedAt.Before(oldest) {
			oldest = e.CreatedAt
		}
	}
	if !oldest.IsZero() {
		s.OldestPendingSeconds = time.Since(oldest).Seconds()
	}
	return s, nil
}

func (r *billingRepo) ListDeadOutbox(_ context.Context, limit int) ([]*billing.OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > len(r.dead) {
		limit = len(r.dead)
	}
	out := make([]*billing.OutboxEvent, limit)
	copy(out, r.dead[:limit])
	return out, nil
}

func (r *billingRepo) ReplayDeadOutbox(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.dead {
		if e.ID == id {
			e.Attempts = 0
			e.LastError = nil
			e.SentAt = nil
			r.outbox = append(r.outbox, e)
			r.dead = append(r.dead[:i], r.dead[i+1:]...)
			return nil
		}
	}
	return billing.ErrOutboxEventNotFound
}

// ── Test: subscription lifecycle ─────────────────────────────────────────

func TestCriticalPath_BillingSubscriptionLifecycle(t *testing.T) {
	t.Parallel()

	repo := newBillingRepo()
	svc := billing.NewService(repo)
	ctx := context.Background()

	// Create subscription — must start trialing with a 14-day trial period.
	sub, err := svc.CreateSubscription(ctx, fixedBillingTenantID, "starter", "monthly")
	require.NoError(t, err)
	assert.Equal(t, "starter", sub.Plan)
	assert.Equal(t, "trialing", sub.Status)
	require.NotNil(t, sub.TrialEndsAt)
	assert.True(t, sub.TrialEndsAt.After(time.Now().Add(13*24*time.Hour)),
		"trial must be at least 13 days in the future")

	// Upgrade plan.
	upgraded, err := svc.UpgradeSubscription(ctx, fixedBillingTenantID, "professional")
	require.NoError(t, err)
	assert.Equal(t, "professional", upgraded.Plan)

	// Cancel subscription.
	cancelled, err := svc.CancelSubscription(ctx, fixedBillingTenantID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", cancelled.Status)
	assert.NotNil(t, cancelled.CancelledAt)
}

// ── Test: invoice paid lifecycle ─────────────────────────────────────────

func TestCriticalPath_BillingInvoicePaid(t *testing.T) {
	t.Parallel()

	repo := newBillingRepo()
	svc := billing.NewService(repo)
	ctx := context.Background()

	// Setup subscription so invoice can be created.
	_, err := svc.CreateSubscription(ctx, fixedBillingTenantID, "starter", "monthly")
	require.NoError(t, err)

	sub, err := repo.GetSubscriptionByTenant(ctx, fixedBillingTenantID)
	require.NoError(t, err)

	now := time.Now().UTC()
	inv := &billing.Invoice{
		ID:             fixedBillingInvID,
		TenantID:       fixedBillingTenantID,
		SubscriptionID: sub.ID,
		InvoiceNumber:  "INV-2026-00001",
		Plan:           "starter",
		Amount:         decimal.NewFromInt(1000),
		TaxAmount:      decimal.NewFromInt(180),
		TotalAmount:    decimal.NewFromInt(1180),
		Currency:       "INR",
		Status:         "pending",
		PeriodStart:    now,
		PeriodEnd:      now.AddDate(0, 1, 0),
		DueDate:        now.AddDate(0, 0, 7),
		CreatedAt:      now,
	}
	require.NoError(t, svc.CreateInvoice(ctx, inv))

	// Mark paid via gateway transaction.
	paid, err := svc.MarkInvoicePaid(ctx, fixedBillingTenantID, fixedBillingInvID, "razorpay_txn_001")
	require.NoError(t, err)
	assert.Equal(t, "paid", paid.Status)
	assert.Equal(t, "razorpay_txn_001", paid.GatewayTxnID)
	assert.NotNil(t, paid.PaidAt)
}

// ── Test: plan limit enforcement ─────────────────────────────────────────

func TestCriticalPath_BillingPlanLimitEnforced(t *testing.T) {
	t.Parallel()

	repo := newBillingRepo()
	svc := billing.NewService(repo)
	ctx := context.Background()

	_, err := svc.CreateSubscription(ctx, fixedBillingTenantID, "starter", "monthly")
	require.NoError(t, err)

	sub, err := repo.GetSubscriptionByTenant(ctx, fixedBillingTenantID)
	require.NoError(t, err)

	// "starter" plan has a maximum number of active productions.
	// Find the limit by checking allowed count.
	limits := billing.PlanLimitsByName[sub.Plan]

	// Under the limit: must be allowed.
	result, err := svc.CheckPlanLimit(ctx, billing.CheckLimitRequest{
		TenantID:           fixedBillingTenantID,
		Resource:           "production",
		CurrentProductions: limits.MaxActiveProductions - 1,
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed, "should be allowed when under the limit")

	// At or over the limit: must be denied.
	result, err = svc.CheckPlanLimit(ctx, billing.CheckLimitRequest{
		TenantID:           fixedBillingTenantID,
		Resource:           "production",
		CurrentProductions: limits.MaxActiveProductions,
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed, "should be denied when at the limit")
	assert.NotEmpty(t, result.Reason)
}

// ── Test: payment method default toggling ────────────────────────────────

func TestCriticalPath_BillingPaymentMethodDefault(t *testing.T) {
	t.Parallel()

	repo := newBillingRepo()
	svc := billing.NewService(repo)
	ctx := context.Background()

	pm1 := &billing.PaymentMethod{
		ID:          fixedBillingPMID,
		TenantID:    fixedBillingTenantID,
		Type:        "card",
		DisplayName: "Visa ending 4242",
		CreatedAt:   time.Now().UTC(),
	}
	pm2 := &billing.PaymentMethod{
		ID:          uuid.MustParse("b0000000-0000-0000-0000-000000000004"),
		TenantID:    fixedBillingTenantID,
		Type:        "card",
		DisplayName: "Mastercard ending 1234",
		CreatedAt:   time.Now().UTC(),
	}

	require.NoError(t, svc.AddPaymentMethod(ctx, pm1))
	require.NoError(t, svc.AddPaymentMethod(ctx, pm2))

	// Set pm2 as default.
	require.NoError(t, svc.SetDefaultPaymentMethod(ctx, fixedBillingTenantID, pm2.ID))

	methods, err := svc.ListPaymentMethods(ctx, fixedBillingTenantID)
	require.NoError(t, err)

	defaults := 0
	for _, m := range methods {
		if m.IsDefault {
			assert.Equal(t, pm2.ID, m.ID, "only pm2 must be the default")
			defaults++
		}
	}
	assert.Equal(t, 1, defaults, "exactly one payment method must be marked default")
}
