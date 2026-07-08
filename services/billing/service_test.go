package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock repository ---

type mockRepo struct {
	createSubscriptionFn        func(ctx context.Context, s *Subscription) error
	getSubscriptionByTenantFn   func(ctx context.Context, tenantID uuid.UUID) (*Subscription, error)
	updateSubscriptionFn        func(ctx context.Context, s *Subscription) error
	createInvoiceFn             func(ctx context.Context, inv *Invoice) error
	getInvoiceFn                func(ctx context.Context, tenantID, id uuid.UUID) (*Invoice, error)
	listInvoicesFn              func(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Invoice, error)
	updateInvoiceFn             func(ctx context.Context, inv *Invoice) error
	nextInvoiceSeqFn            func(ctx context.Context, year int) (int, error)
	createPaymentMethodFn       func(ctx context.Context, pm *PaymentMethod) error
	getPaymentMethodFn          func(ctx context.Context, id uuid.UUID) (*PaymentMethod, error)
	listPaymentMethodsFn        func(ctx context.Context, tenantID uuid.UUID) ([]PaymentMethod, error)
	updatePaymentMethodFn       func(ctx context.Context, pm *PaymentMethod) error
	deletePaymentMethodFn       func(ctx context.Context, id uuid.UUID) error
	clearDefaultPaymentMethodsFn func(ctx context.Context, tenantID uuid.UUID) error
	createUsageRecordFn         func(ctx context.Context, u *UsageRecord) error
	latestUsageRecordFn         func(ctx context.Context, tenantID uuid.UUID) (*UsageRecord, error)
	createDunningAttemptFn      func(ctx context.Context, d *DunningAttempt) error
	listDunningAttemptsFn       func(ctx context.Context, invoiceID uuid.UUID) ([]DunningAttempt, error)

	// Outbox (#126)
	suspendSubscriptionWithOutboxFn func(ctx context.Context, sub *Subscription, subject string, payload []byte) error
	claimUnsentOutboxFn             func(ctx context.Context, limit int) ([]*OutboxEvent, error)
	markOutboxSentFn                func(ctx context.Context, id uuid.UUID) error
	recordOutboxFailureFn           func(ctx context.Context, id uuid.UUID, errMsg string) error
	deleteSentOutboxOlderThanFn     func(ctx context.Context, cutoff time.Time) (int64, error)
}

func (m *mockRepo) CreateSubscription(ctx context.Context, s *Subscription) error {
	if m.createSubscriptionFn != nil {
		return m.createSubscriptionFn(ctx, s)
	}
	return nil
}
func (m *mockRepo) GetSubscriptionByTenant(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	if m.getSubscriptionByTenantFn != nil {
		return m.getSubscriptionByTenantFn(ctx, tenantID)
	}
	return nil, ErrSubscriptionNotFound
}
func (m *mockRepo) UpdateSubscription(ctx context.Context, s *Subscription) error {
	if m.updateSubscriptionFn != nil {
		return m.updateSubscriptionFn(ctx, s)
	}
	return nil
}
func (m *mockRepo) CreateInvoice(ctx context.Context, inv *Invoice) error {
	if m.createInvoiceFn != nil {
		return m.createInvoiceFn(ctx, inv)
	}
	return nil
}
func (m *mockRepo) GetInvoice(ctx context.Context, tenantID, id uuid.UUID) (*Invoice, error) {
	if m.getInvoiceFn != nil {
		return m.getInvoiceFn(ctx, tenantID, id)
	}
	return &Invoice{ID: id, TenantID: tenantID, Status: "pending"}, nil
}
func (m *mockRepo) ListInvoices(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Invoice, error) {
	if m.listInvoicesFn != nil {
		return m.listInvoicesFn(ctx, tenantID, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateInvoice(ctx context.Context, inv *Invoice) error {
	if m.updateInvoiceFn != nil {
		return m.updateInvoiceFn(ctx, inv)
	}
	return nil
}
func (m *mockRepo) NextInvoiceSeq(ctx context.Context, year int) (int, error) {
	if m.nextInvoiceSeqFn != nil {
		return m.nextInvoiceSeqFn(ctx, year)
	}
	return 1, nil
}
func (m *mockRepo) CreatePaymentMethod(ctx context.Context, pm *PaymentMethod) error {
	if m.createPaymentMethodFn != nil {
		return m.createPaymentMethodFn(ctx, pm)
	}
	return nil
}
func (m *mockRepo) GetPaymentMethod(ctx context.Context, id uuid.UUID) (*PaymentMethod, error) {
	if m.getPaymentMethodFn != nil {
		return m.getPaymentMethodFn(ctx, id)
	}
	return &PaymentMethod{ID: id, TenantID: uuid.New()}, nil
}
func (m *mockRepo) ListPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]PaymentMethod, error) {
	if m.listPaymentMethodsFn != nil {
		return m.listPaymentMethodsFn(ctx, tenantID)
	}
	return nil, nil
}
func (m *mockRepo) UpdatePaymentMethod(ctx context.Context, pm *PaymentMethod) error {
	if m.updatePaymentMethodFn != nil {
		return m.updatePaymentMethodFn(ctx, pm)
	}
	return nil
}
func (m *mockRepo) DeletePaymentMethod(ctx context.Context, id uuid.UUID) error {
	if m.deletePaymentMethodFn != nil {
		return m.deletePaymentMethodFn(ctx, id)
	}
	return nil
}
func (m *mockRepo) ClearDefaultPaymentMethods(ctx context.Context, tenantID uuid.UUID) error {
	if m.clearDefaultPaymentMethodsFn != nil {
		return m.clearDefaultPaymentMethodsFn(ctx, tenantID)
	}
	return nil
}
func (m *mockRepo) CreateUsageRecord(ctx context.Context, u *UsageRecord) error {
	if m.createUsageRecordFn != nil {
		return m.createUsageRecordFn(ctx, u)
	}
	return nil
}
func (m *mockRepo) LatestUsageRecord(ctx context.Context, tenantID uuid.UUID) (*UsageRecord, error) {
	if m.latestUsageRecordFn != nil {
		return m.latestUsageRecordFn(ctx, tenantID)
	}
	return nil, ErrSubscriptionNotFound
}
func (m *mockRepo) CreateDunningAttempt(ctx context.Context, d *DunningAttempt) error {
	if m.createDunningAttemptFn != nil {
		return m.createDunningAttemptFn(ctx, d)
	}
	return nil
}
func (m *mockRepo) ListDunningAttempts(ctx context.Context, invoiceID uuid.UUID) ([]DunningAttempt, error) {
	if m.listDunningAttemptsFn != nil {
		return m.listDunningAttemptsFn(ctx, invoiceID)
	}
	return nil, nil
}

// --- Outbox (#126) ---

func (m *mockRepo) SuspendSubscriptionWithOutbox(ctx context.Context, sub *Subscription, subject string, payload []byte) error {
	if m.suspendSubscriptionWithOutboxFn != nil {
		return m.suspendSubscriptionWithOutboxFn(ctx, sub, subject, payload)
	}
	return nil
}
func (m *mockRepo) ClaimUnsentOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	if m.claimUnsentOutboxFn != nil {
		return m.claimUnsentOutboxFn(ctx, limit)
	}
	return nil, nil
}
func (m *mockRepo) MarkOutboxSent(ctx context.Context, id uuid.UUID) error {
	if m.markOutboxSentFn != nil {
		return m.markOutboxSentFn(ctx, id)
	}
	return nil
}
func (m *mockRepo) RecordOutboxFailure(ctx context.Context, id uuid.UUID, errMsg string) error {
	if m.recordOutboxFailureFn != nil {
		return m.recordOutboxFailureFn(ctx, id, errMsg)
	}
	return nil
}
func (m *mockRepo) DeleteSentOutboxOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if m.deleteSentOutboxOlderThanFn != nil {
		return m.deleteSentOutboxOlderThanFn(ctx, cutoff)
	}
	return 0, nil
}

// --- Fixtures ---

var (
	fixedTenantID = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixedInvID    = uuid.MustParse("a1000000-0000-0000-0000-000000000002")
	fixedPMID     = uuid.MustParse("a1000000-0000-0000-0000-000000000003")
)

func starterSub() *Subscription {
	now := time.Now().UTC()
	trial := now.Add(TrialDays * 24 * time.Hour)
	return &Subscription{
		ID:                 uuid.New(),
		TenantID:           fixedTenantID,
		Plan:               "starter",
		Status:             "active",
		BillingCycle:       "monthly",
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   now.AddDate(0, 1, 0),
		TrialEndsAt:        &trial,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// --- Tests: CreateSubscription ---

func TestCreateSubscription_NewTenant(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	sub, err := svc.CreateSubscription(context.Background(), fixedTenantID, "starter", "monthly")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, sub.ID)
	assert.Equal(t, "starter", sub.Plan)
	assert.Equal(t, "trialing", sub.Status)
	assert.Equal(t, "monthly", sub.BillingCycle)
	require.NotNil(t, sub.TrialEndsAt)
	assert.True(t, sub.TrialEndsAt.After(time.Now()))
}

func TestCreateSubscription_DefaultBillingCycle(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	sub, err := svc.CreateSubscription(context.Background(), fixedTenantID, "professional", "")
	require.NoError(t, err)
	assert.Equal(t, "monthly", sub.BillingCycle)
}

func TestCreateSubscription_AnnualCycle(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	sub, err := svc.CreateSubscription(context.Background(), fixedTenantID, "starter", "annual")
	require.NoError(t, err)
	// Annual period end should be ~1 year out.
	diff := sub.CurrentPeriodEnd.Sub(sub.CurrentPeriodStart)
	assert.True(t, diff > 364*24*time.Hour)
}

func TestCreateSubscription_InvalidPlan(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	_, err := svc.CreateSubscription(context.Background(), fixedTenantID, "unicorn", "monthly")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPlan)
}

func TestCreateSubscription_AlreadyExists(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	_, err := svc.CreateSubscription(context.Background(), fixedTenantID, "starter", "monthly")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionAlreadyExists)
}

// --- Tests: GetSubscription ---

func TestGetSubscription_Success(t *testing.T) {
	t.Parallel()
	expected := starterSub()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return expected, nil
		},
	})

	sub, err := svc.GetSubscription(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "starter", sub.Plan)
}

func TestGetSubscription_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	_, err := svc.GetSubscription(context.Background(), fixedTenantID)
	require.Error(t, err)
}

// --- Tests: UpgradeSubscription ---

func TestUpgradeSubscription_StarterToProfessional(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	sub, err := svc.UpgradeSubscription(context.Background(), fixedTenantID, "professional")
	require.NoError(t, err)
	assert.Equal(t, "professional", sub.Plan)
}

func TestUpgradeSubscription_TrialingBecomesActive(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Status = "trialing"
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
	})

	sub, err := svc.UpgradeSubscription(context.Background(), fixedTenantID, "professional")
	require.NoError(t, err)
	assert.Equal(t, "active", sub.Status)
}

func TestUpgradeSubscription_InvalidPlan(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	_, err := svc.UpgradeSubscription(context.Background(), fixedTenantID, "unicorn")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPlan)
}

func TestUpgradeSubscription_CancelledRejected(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Status = "cancelled"
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
	})

	_, err := svc.UpgradeSubscription(context.Background(), fixedTenantID, "professional")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionCancelled)
}

func TestUpgradeSubscription_SuspendedRejected(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Status = "suspended"
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
	})

	_, err := svc.UpgradeSubscription(context.Background(), fixedTenantID, "professional")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionSuspended)
}

// --- Tests: CancelSubscription ---

func TestCancelSubscription_SetsTimestamp(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	sub, err := svc.CancelSubscription(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", sub.Status)
	require.NotNil(t, sub.CancelledAt)
}

func TestCancelSubscription_AlreadyCancelled(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Status = "cancelled"
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
	})

	_, err := svc.CancelSubscription(context.Background(), fixedTenantID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionCancelled)
}

// --- Tests: CheckPlanLimit ---

func TestCheckPlanLimit_StarterProductionAtLimit(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	result, err := svc.CheckPlanLimit(context.Background(), CheckLimitRequest{
		TenantID:           fixedTenantID,
		Resource:           "production",
		CurrentProductions: 2, // starter limit
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "production limit reached")
}

func TestCheckPlanLimit_StarterProductionUnderLimit(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	result, err := svc.CheckPlanLimit(context.Background(), CheckLimitRequest{
		TenantID:           fixedTenantID,
		Resource:           "production",
		CurrentProductions: 1,
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheckPlanLimit_StarterUserAtLimit(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	result, err := svc.CheckPlanLimit(context.Background(), CheckLimitRequest{
		TenantID:     fixedTenantID,
		Resource:     "user",
		CurrentUsers: 10, // starter limit
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "user limit reached")
}

func TestCheckPlanLimit_EnterpriseUnlimited(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Plan = "enterprise"
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
	})

	result, err := svc.CheckPlanLimit(context.Background(), CheckLimitRequest{
		TenantID:           fixedTenantID,
		Resource:           "production",
		CurrentProductions: 9999,
	})
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestCheckPlanLimit_SuspendedDenied(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Status = "suspended"
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
	})

	result, err := svc.CheckPlanLimit(context.Background(), CheckLimitRequest{
		TenantID: fixedTenantID,
		Resource: "production",
	})
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "suspended")
}

func TestCheckPlanLimit_UnknownResource(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	_, err := svc.CheckPlanLimit(context.Background(), CheckLimitRequest{
		TenantID: fixedTenantID,
		Resource: "banana",
	})
	require.Error(t, err)
}

// --- Tests: CreateInvoice ---

func TestCreateInvoice_GeneratesIDAndNumber(t *testing.T) {
	t.Parallel()
	var captured Invoice
	svc := NewService(&mockRepo{
		nextInvoiceSeqFn: func(_ context.Context, _ int) (int, error) { return 42, nil },
		createInvoiceFn: func(_ context.Context, inv *Invoice) error {
			captured = *inv
			return nil
		},
	})

	inv := &Invoice{
		TenantID:       fixedTenantID,
		SubscriptionID: uuid.New(),
		Amount:         decimal.NewFromInt(4999),
		Currency:       "INR",
		DueDate:        time.Now().AddDate(0, 0, 15),
	}
	err := svc.CreateInvoice(context.Background(), inv)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, captured.ID)
	assert.Equal(t, "INV-2026-00042", captured.InvoiceNumber)
	assert.Equal(t, "pending", captured.Status)
}

func TestCreateInvoice_GSTAppliedForINR(t *testing.T) {
	t.Parallel()
	var captured Invoice
	svc := NewService(&mockRepo{
		createInvoiceFn: func(_ context.Context, inv *Invoice) error {
			captured = *inv
			return nil
		},
	})

	inv := &Invoice{
		TenantID: fixedTenantID,
		Amount:   decimal.NewFromInt(10000),
		Currency: "INR",
	}
	err := svc.CreateInvoice(context.Background(), inv)
	require.NoError(t, err)
	// 18% of 10000 = 1800
	assert.Equal(t, "1800.00", captured.TaxAmount.StringFixed(2))
	assert.Equal(t, "11800.00", captured.TotalAmount.StringFixed(2))
}

func TestCreateInvoice_NoGSTForForeignCurrency(t *testing.T) {
	t.Parallel()
	var captured Invoice
	svc := NewService(&mockRepo{
		createInvoiceFn: func(_ context.Context, inv *Invoice) error {
			captured = *inv
			return nil
		},
	})

	inv := &Invoice{
		TenantID: fixedTenantID,
		Amount:   decimal.NewFromInt(200),
		Currency: "USD",
	}
	err := svc.CreateInvoice(context.Background(), inv)
	require.NoError(t, err)
	assert.True(t, captured.TaxAmount.IsZero())
	assert.Equal(t, "200.00", captured.TotalAmount.StringFixed(2))
}

// --- Tests: MarkInvoicePaid ---

func TestMarkInvoicePaid_SetsPaidAt(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	inv, err := svc.MarkInvoicePaid(context.Background(), fixedTenantID, fixedInvID, "rzp_txn_001")
	require.NoError(t, err)
	assert.Equal(t, "paid", inv.Status)
	require.NotNil(t, inv.PaidAt)
	assert.Equal(t, "rzp_txn_001", inv.GatewayTxnID)
}

func TestMarkInvoicePaid_AlreadyPaid(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, id uuid.UUID) (*Invoice, error) {
			return &Invoice{ID: id, Status: "paid"}, nil
		},
	})

	_, err := svc.MarkInvoicePaid(context.Background(), fixedTenantID, fixedInvID, "txn")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvoiceAlreadyPaid)
}

func TestMarkInvoicePaid_PastDueTransitionsToActive(t *testing.T) {
	t.Parallel()
	s := starterSub()
	s.Status = "past_due"
	var updatedSub *Subscription
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return s, nil
		},
		updateSubscriptionFn: func(_ context.Context, sub *Subscription) error {
			updatedSub = sub
			return nil
		},
	})

	_, err := svc.MarkInvoicePaid(context.Background(), fixedTenantID, fixedInvID, "txn")
	require.NoError(t, err)
	require.NotNil(t, updatedSub)
	assert.Equal(t, "active", updatedSub.Status)
}

// --- Tests: ListInvoices ---

func TestListInvoices_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listInvoicesFn: func(_ context.Context, _ uuid.UUID, limit, _ int) ([]Invoice, error) {
			capturedLimit = limit
			return nil, nil
		},
	})
	_, _ = svc.ListInvoices(context.Background(), fixedTenantID, 0, 0)
	assert.Equal(t, 20, capturedLimit)
}

func TestListInvoices_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listInvoicesFn: func(_ context.Context, _ uuid.UUID, limit, _ int) ([]Invoice, error) {
			capturedLimit = limit
			return nil, nil
		},
	})
	_, _ = svc.ListInvoices(context.Background(), fixedTenantID, 9999, 0)
	assert.Equal(t, 20, capturedLimit)
}

// --- Tests: PaymentMethods ---

func TestAddPaymentMethod_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	pm := &PaymentMethod{
		TenantID:    fixedTenantID,
		Type:        "card",
		DisplayName: "Visa ending 4242",
	}
	err := svc.AddPaymentMethod(context.Background(), pm)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, pm.ID)
}

func TestSetDefaultPaymentMethod_ClearsOldDefault(t *testing.T) {
	t.Parallel()
	var clearCalled bool
	var updatedPM *PaymentMethod
	svc := NewService(&mockRepo{
		clearDefaultPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) error {
			clearCalled = true
			return nil
		},
		updatePaymentMethodFn: func(_ context.Context, pm *PaymentMethod) error {
			updatedPM = pm
			return nil
		},
	})

	err := svc.SetDefaultPaymentMethod(context.Background(), fixedTenantID, fixedPMID)
	require.NoError(t, err)
	assert.True(t, clearCalled)
	require.NotNil(t, updatedPM)
	assert.True(t, updatedPM.IsDefault)
}

func TestRemovePaymentMethod_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getPaymentMethodFn: func(_ context.Context, _ uuid.UUID) (*PaymentMethod, error) {
			return nil, ErrPaymentMethodNotFound
		},
	})

	err := svc.RemovePaymentMethod(context.Background(), fixedPMID)
	require.Error(t, err)
}

// --- Tests: Usage ---

func TestRecordUsage_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	u := &UsageRecord{
		TenantID:          fixedTenantID,
		ActiveProductions: 1,
		UserCount:         5,
		StorageGB:         decimal.NewFromFloat(1.2),
	}
	err := svc.RecordUsage(context.Background(), u)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, u.ID)
	assert.False(t, u.RecordedAt.IsZero())
}

func TestGetUsageSummary_NoUsageRecordYet(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
		// LatestUsageRecord returns error → service returns zeroed record
	})

	summary, err := svc.GetUsageSummary(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "starter", summary.Plan)
	assert.Equal(t, 2, summary.Limits.MaxActiveProductions)
	assert.Equal(t, 0, summary.Usage.ActiveProductions)
}

func TestGetUsageSummary_WithRecord(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
		latestUsageRecordFn: func(_ context.Context, tenantID uuid.UUID) (*UsageRecord, error) {
			return &UsageRecord{
				TenantID:          tenantID,
				ActiveProductions: 1,
				UserCount:         3,
				StorageGB:         decimal.NewFromFloat(0.5),
			}, nil
		},
	})

	summary, err := svc.GetUsageSummary(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Usage.ActiveProductions)
	assert.Equal(t, 10, summary.Limits.MaxUsers)
}

// --- Tests: Dunning ---

func TestRecordDunningAttempt_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	d := &DunningAttempt{
		InvoiceID:     fixedInvID,
		AttemptNumber: 1,
		Result:        "card_declined",
	}
	err := svc.RecordDunningAttempt(context.Background(), d)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, d.ID)
	assert.False(t, d.AttemptedAt.IsZero())
}

func TestSuspendSubscription_SetsSuspendedAt(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	sub, err := svc.SuspendSubscription(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "suspended", sub.Status)
	require.NotNil(t, sub.SuspendedAt)
}

// fakeEventPublisher captures PublishSubscriptionSuspended calls for assertions.
type fakeEventPublisher struct {
	calls []*Subscription
	err   error
}

func (f *fakeEventPublisher) PublishSubscriptionSuspended(_ context.Context, sub *Subscription) error {
	f.calls = append(f.calls, sub)
	return f.err
}

func TestSuspendSubscription_PublishesEvent(t *testing.T) {
	t.Parallel()
	pub := &fakeEventPublisher{}
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	}).WithPublisher(pub)

	sub, err := svc.SuspendSubscription(context.Background(), fixedTenantID)
	require.NoError(t, err)
	require.Len(t, pub.calls, 1)
	assert.Same(t, sub, pub.calls[0])
	assert.Equal(t, "suspended", pub.calls[0].Status)
}

func TestSuspendSubscription_NilPublisherIsNoOp(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	})

	assert.NotPanics(t, func() {
		sub, err := svc.SuspendSubscription(context.Background(), fixedTenantID)
		require.NoError(t, err)
		assert.Equal(t, "suspended", sub.Status)
	})
}

func TestSuspendSubscription_PublishErrorDoesNotFailOp(t *testing.T) {
	t.Parallel()
	pub := &fakeEventPublisher{err: fmt.Errorf("nats: connection closed")}
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
	}).WithPublisher(pub)

	sub, err := svc.SuspendSubscription(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "suspended", sub.Status)
	require.Len(t, pub.calls, 1)
}

// --- Tests: PlanLimitsByName ---

func TestPlanLimitsByName_AllPlansPresent(t *testing.T) {
	t.Parallel()
	for _, plan := range []string{"starter", "professional", "enterprise"} {
		_, ok := PlanLimitsByName[plan]
		assert.True(t, ok, "PlanLimitsByName must contain %q", plan)
	}
}

func TestPlanLimitsByName_EnterpriseIsUnlimited(t *testing.T) {
	t.Parallel()
	limits := PlanLimitsByName["enterprise"]
	assert.Equal(t, -1, limits.MaxActiveProductions)
	assert.Equal(t, -1, limits.MaxUsers)
	assert.Equal(t, -1, limits.MaxStorageGB)
}

func TestPlanLimitsByName_StarterIsRestrictive(t *testing.T) {
	t.Parallel()
	limits := PlanLimitsByName["starter"]
	assert.Equal(t, 2, limits.MaxActiveProductions)
	assert.Equal(t, 10, limits.MaxUsers)
	assert.Equal(t, 5, limits.MaxStorageGB)
}

// --- Tests: MarkInvoiceOverdue ---

func TestMarkInvoiceOverdue_SetsStatus(t *testing.T) {
	t.Parallel()
	inv := &Invoice{ID: fixedInvID, TenantID: fixedTenantID, Status: "pending"}
	svc := NewService(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			return inv, nil
		},
	})

	got, err := svc.MarkInvoiceOverdue(context.Background(), fixedTenantID, fixedInvID)
	require.NoError(t, err)
	assert.Equal(t, "overdue", got.Status)
}

func TestMarkInvoiceOverdue_RepoError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			return nil, ErrInvoiceNotFound
		},
	})

	_, err := svc.MarkInvoiceOverdue(context.Background(), fixedTenantID, fixedInvID)
	require.Error(t, err)
}

// --- Tests: ListDunningAttempts ---

func TestListDunningAttempts_ReturnsAttempts(t *testing.T) {
	t.Parallel()
	want := []DunningAttempt{
		{ID: uuid.MustParse("a1000000-0000-0000-0000-000000000010"), InvoiceID: fixedInvID, AttemptNumber: 1, Result: "card_declined"},
		{ID: uuid.MustParse("a1000000-0000-0000-0000-000000000011"), InvoiceID: fixedInvID, AttemptNumber: 2, Result: "card_declined"},
	}
	svc := NewService(&mockRepo{
		listDunningAttemptsFn: func(_ context.Context, _ uuid.UUID) ([]DunningAttempt, error) {
			return want, nil
		},
	})

	got, err := svc.ListDunningAttempts(context.Background(), fixedInvID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListDunningAttempts_Empty(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	got, err := svc.ListDunningAttempts(context.Background(), fixedInvID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// --- Tests: ListPaymentMethods cap ---

func TestListPaymentMethods_CappedAt20(t *testing.T) {
	t.Parallel()
	methods := make([]PaymentMethod, 25)
	for i := range methods {
		methods[i] = PaymentMethod{ID: uuid.New(), TenantID: fixedTenantID}
	}
	svc := NewService(&mockRepo{
		listPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) {
			return methods, nil
		},
	})

	got, err := svc.ListPaymentMethods(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Len(t, got, 20, "must be capped at 20")
}

// --- Tests: SetDefaultPaymentMethod ---

func TestSetDefaultPaymentMethod_ClearsOthersAndSetsDefault(t *testing.T) {
	t.Parallel()
	cleared := false
	updated := false
	svc := NewService(&mockRepo{
		getPaymentMethodFn: func(_ context.Context, id uuid.UUID) (*PaymentMethod, error) {
			return &PaymentMethod{ID: id, TenantID: fixedTenantID}, nil
		},
		clearDefaultPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) error {
			cleared = true
			return nil
		},
		updatePaymentMethodFn: func(_ context.Context, pm *PaymentMethod) error {
			updated = true
			assert.True(t, pm.IsDefault)
			return nil
		},
	})

	err := svc.SetDefaultPaymentMethod(context.Background(), fixedTenantID, fixedPMID)
	require.NoError(t, err)
	assert.True(t, cleared, "must clear existing defaults")
	assert.True(t, updated, "must mark new default")
}
