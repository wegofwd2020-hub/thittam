package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	billingv1 "github.com/wegofwd2020/thittam/gen/billing/v1"
)

// Deterministic UUIDs for handler test fixtures.
// fixedTenantID, fixedInvID, fixedPMID are shared with service_test.go.
var (
	fixedSubID = uuid.MustParse("b2000000-0000-0000-0000-000000000002")
	fixedNow   = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
)

func fixedSub() *Subscription {
	trialEnd := fixedNow.Add(14 * 24 * time.Hour)
	return &Subscription{
		ID:                 fixedSubID,
		TenantID:           fixedTenantID,
		Plan:               "starter",
		Status:             "trialing",
		BillingCycle:       "monthly",
		CurrentPeriodStart: fixedNow,
		CurrentPeriodEnd:   fixedNow.AddDate(0, 1, 0),
		TrialEndsAt:        &trialEnd,
		CreatedAt:          fixedNow,
		UpdatedAt:          fixedNow,
	}
}

func fixedInvoice() *Invoice {
	return &Invoice{
		ID:             fixedInvID,
		TenantID:       fixedTenantID,
		SubscriptionID: fixedSubID,
		InvoiceNumber:  "INV-2026-00001",
		Plan:           "starter",
		Amount:         decimal.NewFromFloat(1000),
		TaxAmount:      decimal.NewFromFloat(180),
		TotalAmount:    decimal.NewFromFloat(1180),
		Currency:       "INR",
		Status:         "pending",
		PeriodStart:    fixedNow,
		PeriodEnd:      fixedNow.AddDate(0, 1, 0),
		DueDate:        fixedNow.AddDate(0, 0, 7),
		CreatedAt:      fixedNow,
	}
}

func fixedPM() *PaymentMethod {
	return &PaymentMethod{
		ID:          fixedPMID,
		TenantID:    fixedTenantID,
		Type:        "card",
		DisplayName: "Visa ending 4242",
		IsDefault:   true,
		CreatedAt:   fixedNow,
	}
}

// newHandlerWithRepo builds a Handler backed by the given mock repo.
func newHandlerWithRepo(r Repository) *Handler {
	return NewHandler(NewService(r))
}

// --- GetSubscription ---

func TestHandler_GetSubscription(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
		})
		resp, err := h.GetSubscription(context.Background(), &billingv1.GetSubscriptionRequest{
			TenantId: fixedTenantID.String(),
		})
		require.NoError(t, err)
		assert.Equal(t, fixedSubID.String(), resp.Id)
		assert.Equal(t, "starter", resp.Plan)
		assert.Equal(t, "trialing", resp.Status)
		assert.NotNil(t, resp.TrialEndsAt)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{})
		_, err := h.GetSubscription(context.Background(), &billingv1.GetSubscriptionRequest{
			TenantId: fixedTenantID.String(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("invalid_tenant_id", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{})
		_, err := h.GetSubscription(context.Background(), &billingv1.GetSubscriptionRequest{
			TenantId: "not-a-uuid",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tenant_id")
	})
}

// --- CreateSubscription ---

func TestHandler_CreateSubscription(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{
			// GetSubscriptionByTenant returns not-found so no existing sub.
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return nil, ErrSubscriptionNotFound
			},
		})
		resp, err := h.CreateSubscription(context.Background(), &billingv1.CreateSubscriptionRequest{
			TenantId:     fixedTenantID.String(),
			Plan:         "professional",
			BillingCycle: "annual",
		})
		require.NoError(t, err)
		assert.Equal(t, "professional", resp.Plan)
		assert.Equal(t, "trialing", resp.Status)
		assert.Equal(t, "annual", resp.BillingCycle)
	})

	t.Run("already_exists", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return fixedSub(), nil
			},
		})
		_, err := h.CreateSubscription(context.Background(), &billingv1.CreateSubscriptionRequest{
			TenantId: fixedTenantID.String(),
			Plan:     "starter",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("invalid_plan", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return nil, ErrSubscriptionNotFound
			},
		})
		_, err := h.CreateSubscription(context.Background(), &billingv1.CreateSubscriptionRequest{
			TenantId: fixedTenantID.String(),
			Plan:     "ultra",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid plan")
	})
}

// --- UpgradeSubscription ---

func TestHandler_UpgradeSubscription(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
		})
		resp, err := h.UpgradeSubscription(context.Background(), &billingv1.UpgradeSubscriptionRequest{
			TenantId: fixedTenantID.String(),
			NewPlan:  "professional",
		})
		require.NoError(t, err)
		assert.Equal(t, "professional", resp.Plan)
		// trialing → active on first upgrade
		assert.Equal(t, "active", resp.Status)
	})

	t.Run("cancelled_subscription", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		sub.Status = "cancelled"
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
		})
		_, err := h.UpgradeSubscription(context.Background(), &billingv1.UpgradeSubscriptionRequest{
			TenantId: fixedTenantID.String(),
			NewPlan:  "professional",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
	})
}

// --- CancelSubscription ---

func TestHandler_CancelSubscription(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
		})
		resp, err := h.CancelSubscription(context.Background(), &billingv1.CancelSubscriptionRequest{
			TenantId: fixedTenantID.String(),
		})
		require.NoError(t, err)
		assert.Equal(t, "cancelled", resp.Status)
		assert.NotNil(t, resp.CancelledAt)
	})
}

// --- CheckPlanLimit ---

func TestHandler_CheckPlanLimit(t *testing.T) {
	t.Parallel()

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		sub.Status = "active"
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
		})
		resp, err := h.CheckPlanLimit(context.Background(), &billingv1.CheckPlanLimitRequest{
			TenantId:           fixedTenantID.String(),
			Resource:           "production",
			CurrentProductions: 1, // starter allows 2
		})
		require.NoError(t, err)
		assert.True(t, resp.Allowed)
	})

	t.Run("exceeded", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		sub.Status = "active"
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
		})
		resp, err := h.CheckPlanLimit(context.Background(), &billingv1.CheckPlanLimitRequest{
			TenantId:           fixedTenantID.String(),
			Resource:           "production",
			CurrentProductions: 2, // at limit
		})
		require.NoError(t, err)
		assert.False(t, resp.Allowed)
		assert.Contains(t, resp.Reason, "production limit reached")
	})
}

// --- ListInvoices ---

func TestHandler_ListInvoices(t *testing.T) {
	t.Parallel()

	t.Run("returns_list", func(t *testing.T) {
		t.Parallel()
		inv := fixedInvoice()
		h := newHandlerWithRepo(&mockRepo{
			listInvoicesFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]Invoice, error) {
				return []Invoice{*inv}, nil
			},
		})
		resp, err := h.ListInvoices(context.Background(), &billingv1.ListInvoicesRequest{
			TenantId: fixedTenantID.String(),
		})
		require.NoError(t, err)
		require.Len(t, resp.Invoices, 1)
		assert.Equal(t, "INV-2026-00001", resp.Invoices[0].InvoiceNumber)
		assert.Equal(t, "1000.00", resp.Invoices[0].Amount)
		assert.Equal(t, "180.00", resp.Invoices[0].TaxAmount)
		assert.Equal(t, "1180.00", resp.Invoices[0].TotalAmount)
	})
}

// --- GetInvoice ---

func TestHandler_GetInvoice(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		inv := fixedInvoice()
		h := newHandlerWithRepo(&mockRepo{
			getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
				return inv, nil
			},
		})
		resp, err := h.GetInvoice(context.Background(), &billingv1.GetInvoiceRequest{
			TenantId:  fixedTenantID.String(),
			InvoiceId: fixedInvID.String(),
		})
		require.NoError(t, err)
		assert.Equal(t, fixedInvID.String(), resp.Id)
		assert.Equal(t, "INR", resp.Currency)
	})
}

// --- AddPaymentMethod ---

func TestHandler_AddPaymentMethod(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{})
		resp, err := h.AddPaymentMethod(context.Background(), &billingv1.AddPaymentMethodRequest{
			TenantId:      fixedTenantID.String(),
			Type:          "card",
			DisplayName:   "Visa ending 4242",
			RazorpayToken: "tok_test_123",
		})
		require.NoError(t, err)
		assert.Equal(t, "card", resp.Type)
		assert.Equal(t, "Visa ending 4242", resp.DisplayName)
	})

	t.Run("invalid_expires_at", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{})
		_, err := h.AddPaymentMethod(context.Background(), &billingv1.AddPaymentMethodRequest{
			TenantId:    fixedTenantID.String(),
			Type:        "card",
			DisplayName: "Visa ending 4242",
			ExpiresAt:   "not-a-date",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid expires_at")
	})
}

// --- RemovePaymentMethod ---

func TestHandler_RemovePaymentMethod(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		pm := fixedPM()
		h := newHandlerWithRepo(&mockRepo{
			getPaymentMethodFn: func(_ context.Context, _ uuid.UUID) (*PaymentMethod, error) {
				return pm, nil
			},
		})
		_, err := h.RemovePaymentMethod(context.Background(), &billingv1.RemovePaymentMethodRequest{
			TenantId:        fixedTenantID.String(),
			PaymentMethodId: fixedPMID.String(),
		})
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{
			getPaymentMethodFn: func(_ context.Context, _ uuid.UUID) (*PaymentMethod, error) {
				return nil, ErrPaymentMethodNotFound
			},
		})
		_, err := h.RemovePaymentMethod(context.Background(), &billingv1.RemovePaymentMethodRequest{
			TenantId:        fixedTenantID.String(),
			PaymentMethodId: fixedPMID.String(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// --- ListPaymentMethods ---

func TestHandler_ListPaymentMethods(t *testing.T) {
	t.Parallel()

	t.Run("returns_list", func(t *testing.T) {
		t.Parallel()
		pm := fixedPM()
		h := newHandlerWithRepo(&mockRepo{
			listPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) {
				return []PaymentMethod{*pm}, nil
			},
		})
		resp, err := h.ListPaymentMethods(context.Background(), &billingv1.ListPaymentMethodsRequest{
			TenantId: fixedTenantID.String(),
		})
		require.NoError(t, err)
		require.Len(t, resp.PaymentMethods, 1)
		assert.Equal(t, "card", resp.PaymentMethods[0].Type)
		assert.True(t, resp.PaymentMethods[0].IsDefault)
	})
}

// --- GetUsageSummary ---

func TestHandler_GetUsageSummary(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		sub := fixedSub()
		sub.Status = "active"
		usage := &UsageRecord{
			ID:                uuid.MustParse("e5000000-0000-0000-0000-000000000005"),
			TenantID:          fixedTenantID,
			ActiveProductions: 1,
			UserCount:         3,
			StorageGB:         decimal.NewFromFloat(0.5),
			RecordedAt:        fixedNow,
		}
		h := newHandlerWithRepo(&mockRepo{
			getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
				return sub, nil
			},
			latestUsageRecordFn: func(_ context.Context, _ uuid.UUID) (*UsageRecord, error) {
				return usage, nil
			},
		})
		resp, err := h.GetUsageSummary(context.Background(), &billingv1.GetUsageSummaryRequest{
			TenantId: fixedTenantID.String(),
		})
		require.NoError(t, err)
		assert.Equal(t, "starter", resp.Plan)
		assert.Equal(t, int32(1), resp.ActiveProductions)
		assert.Equal(t, int32(3), resp.UserCount)
		assert.Equal(t, "0.50", resp.StorageGb)
		assert.Equal(t, int32(2), resp.MaxProductions)
		assert.Equal(t, int32(10), resp.MaxUsers)
	})
}

// --- HandlePaymentWebhook ---

func TestHandler_HandlePaymentWebhook(t *testing.T) {
	t.Parallel()

	h := newHandlerWithRepo(&mockRepo{})
	_, err := h.HandlePaymentWebhook(context.Background(), &billingv1.WebhookPayload{
		Gateway: "razorpay",
		Event:   "payment.captured",
	})
	require.NoError(t, err)
}

// --- grpcErr mapping ---

func TestGrpcErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err      error
		wantCode string
	}{
		{ErrSubscriptionNotFound, "NotFound"},
		{ErrSubscriptionAlreadyExists, "AlreadyExists"},
		{ErrInvoiceNotFound, "NotFound"},
		{ErrPaymentMethodNotFound, "NotFound"},
		{ErrInvalidPlan, "InvalidArgument"},
		{ErrDowngradeNotAllowed, "FailedPrecondition"},
		{ErrSubscriptionCancelled, "FailedPrecondition"},
		{ErrSubscriptionSuspended, "FailedPrecondition"},
		{ErrInvoiceAlreadyPaid, "FailedPrecondition"},
		{ErrPlanLimitExceeded, "ResourceExhausted"},
		{ErrNoDefaultPaymentMethod, "FailedPrecondition"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.err.Error(), func(t *testing.T) {
			t.Parallel()
			s := grpcErr(tc.err)
			assert.Contains(t, s.Error(), tc.wantCode)
		})
	}
}
