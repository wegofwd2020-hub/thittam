package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	billingv1 "github.com/wegofwd2020/thittam/gen/billing/v1"
	documentv1 "github.com/wegofwd2020/thittam/gen/document/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// callerCtx returns a context carrying a verified caller in tenant tid, as
// UnaryAuthInterceptor would have produced from a valid token (#138).
// Handler tests bypass the interceptor, so they must inject the caller themselves.
func callerCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}

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

// allowAllPerm is a PermissionChecker double that grants every check.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm is a PermissionChecker double that denies every check.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}

// newHandlerWithRepo builds a Handler backed by the given mock repo.
func newHandlerWithRepo(r Repository) *Handler {
	return NewHandler(NewService(r), allowAllPerm{})
}

// newHandlerWithRepoDeny builds a Handler backed by the given mock repo whose
// permission checker denies every check, for the denial tests.
func newHandlerWithRepoDeny(r Repository) *Handler {
	return NewHandler(NewService(r), denyPerm{})
}

// --- mock document client ---

type mockDocClient struct {
	getDownloadURLFn func(ctx context.Context, req *documentv1.GetDownloadURLRequest, opts ...grpc.CallOption) (*documentv1.DownloadURL, error)
}

func (m *mockDocClient) GetDownloadURL(ctx context.Context, req *documentv1.GetDownloadURLRequest, opts ...grpc.CallOption) (*documentv1.DownloadURL, error) {
	if m.getDownloadURLFn != nil {
		return m.getDownloadURLFn(ctx, req, opts...)
	}
	return &documentv1.DownloadURL{
		Url:       "https://storage.example.com/invoice.pdf?sig=abc",
		ExpiresAt: timestamppb.New(time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)),
	}, nil
}

// Satisfy the full documentv1.DocumentServiceClient interface — only GetDownloadURL is used.
func (m *mockDocClient) InitiateUpload(_ context.Context, _ *documentv1.InitiateUploadRequest, _ ...grpc.CallOption) (*documentv1.UploadURL, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) ConfirmUpload(_ context.Context, _ *documentv1.ConfirmUploadRequest, _ ...grpc.CallOption) (*documentv1.Document, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) GetDocument(_ context.Context, _ *documentv1.GetDocumentRequest, _ ...grpc.CallOption) (*documentv1.Document, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) ListDocuments(_ context.Context, _ *documentv1.ListDocumentsRequest, _ ...grpc.CallOption) (*documentv1.ListDocumentsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) DeleteDocument(_ context.Context, _ *documentv1.DeleteDocumentRequest, _ ...grpc.CallOption) (*documentv1.DeleteDocumentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) MoveDocument(_ context.Context, _ *documentv1.MoveDocumentRequest, _ ...grpc.CallOption) (*documentv1.Document, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) CreateVersion(_ context.Context, _ *documentv1.CreateVersionRequest, _ ...grpc.CallOption) (*documentv1.UploadURL, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) ConfirmVersion(_ context.Context, _ *documentv1.ConfirmVersionRequest, _ ...grpc.CallOption) (*documentv1.DocumentVersion, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) ListVersions(_ context.Context, _ *documentv1.ListVersionsRequest, _ ...grpc.CallOption) (*documentv1.ListVersionsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) CreateFolder(_ context.Context, _ *documentv1.CreateFolderRequest, _ ...grpc.CallOption) (*documentv1.Folder, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) ListFolders(_ context.Context, _ *documentv1.ListFoldersRequest, _ ...grpc.CallOption) (*documentv1.ListFoldersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
}
func (m *mockDocClient) RestoreVersion(_ context.Context, _ *documentv1.RestoreVersionRequest, _ ...grpc.CallOption) (*documentv1.Document, error) {
	return nil, status.Error(codes.Unimplemented, "not used in billing tests")
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
		resp, err := h.GetSubscription(callerCtx(fixedTenantID), &billingv1.GetSubscriptionRequest{
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
		_, err := h.GetSubscription(callerCtx(fixedTenantID), &billingv1.GetSubscriptionRequest{
			TenantId: fixedTenantID.String(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("invalid_tenant_id", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{})
		_, err := h.GetSubscription(callerCtx(fixedTenantID), &billingv1.GetSubscriptionRequest{
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
		resp, err := h.CreateSubscription(callerCtx(fixedTenantID), &billingv1.CreateSubscriptionRequest{
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
		_, err := h.CreateSubscription(callerCtx(fixedTenantID), &billingv1.CreateSubscriptionRequest{
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
		_, err := h.CreateSubscription(callerCtx(fixedTenantID), &billingv1.CreateSubscriptionRequest{
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
		resp, err := h.UpgradeSubscription(callerCtx(fixedTenantID), &billingv1.UpgradeSubscriptionRequest{
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
		_, err := h.UpgradeSubscription(callerCtx(fixedTenantID), &billingv1.UpgradeSubscriptionRequest{
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
		resp, err := h.CancelSubscription(callerCtx(fixedTenantID), &billingv1.CancelSubscriptionRequest{
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
		resp, err := h.CheckPlanLimit(callerCtx(fixedTenantID), &billingv1.CheckPlanLimitRequest{
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
		resp, err := h.CheckPlanLimit(callerCtx(fixedTenantID), &billingv1.CheckPlanLimitRequest{
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
		resp, err := h.ListInvoices(callerCtx(fixedTenantID), &billingv1.ListInvoicesRequest{
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
		resp, err := h.GetInvoice(callerCtx(fixedTenantID), &billingv1.GetInvoiceRequest{
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
		resp, err := h.AddPaymentMethod(callerCtx(fixedTenantID), &billingv1.AddPaymentMethodRequest{
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
		_, err := h.AddPaymentMethod(callerCtx(fixedTenantID), &billingv1.AddPaymentMethodRequest{
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
			getPaymentMethodFn: func(_ context.Context, _, _ uuid.UUID) (*PaymentMethod, error) {
				return pm, nil
			},
		})
		_, err := h.RemovePaymentMethod(callerCtx(fixedTenantID), &billingv1.RemovePaymentMethodRequest{
			TenantId:        fixedTenantID.String(),
			PaymentMethodId: fixedPMID.String(),
		})
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithRepo(&mockRepo{
			getPaymentMethodFn: func(_ context.Context, _, _ uuid.UUID) (*PaymentMethod, error) {
				return nil, ErrPaymentMethodNotFound
			},
		})
		_, err := h.RemovePaymentMethod(callerCtx(fixedTenantID), &billingv1.RemovePaymentMethodRequest{
			TenantId:        fixedTenantID.String(),
			PaymentMethodId: fixedPMID.String(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestHandler_RemovePaymentMethod_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	var gotGetTenant, gotDelTenant uuid.UUID
	delCalled := false
	repo := &mockRepo{
		getPaymentMethodFn: func(_ context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
			gotGetTenant = tenantID
			return &PaymentMethod{ID: id, TenantID: tenantID}, nil
		},
		deletePaymentMethodFn: func(_ context.Context, tenantID, id uuid.UUID) error {
			gotDelTenant = tenantID
			delCalled = true
			return nil
		},
	}
	h := newHandlerWithRepo(repo)

	_, err := h.RemovePaymentMethod(callerCtx(callerTenant), &billingv1.RemovePaymentMethodRequest{
		PaymentMethodId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.True(t, delCalled)
	require.Equal(t, callerTenant, gotGetTenant, "GetPaymentMethod must receive the caller's tenant")
	require.Equal(t, callerTenant, gotDelTenant, "the DELETE must be scoped to the caller's tenant")
}

func TestHandler_SetDefaultPaymentMethod_PassesCallerTenantToGet(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	var gotGetTenant uuid.UUID
	repo := &mockRepo{
		getPaymentMethodFn: func(_ context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
			gotGetTenant = tenantID
			return &PaymentMethod{ID: id, TenantID: tenantID}, nil
		},
		clearDefaultPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) error { return nil },
		updatePaymentMethodFn:        func(_ context.Context, _ *PaymentMethod) error { return nil },
		listPaymentMethodsFn:         func(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) { return nil, nil },
	}
	h := newHandlerWithRepo(repo)

	_, _ = h.SetDefaultPaymentMethod(callerCtx(callerTenant), &billingv1.SetDefaultPaymentMethodRequest{
		PaymentMethodId: uuid.New().String(),
	})

	require.Equal(t, callerTenant, gotGetTenant,
		"SetDefaultPaymentMethod must fetch the payment method scoped to the caller's tenant, not a bare id")
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
		resp, err := h.ListPaymentMethods(callerCtx(fixedTenantID), &billingv1.ListPaymentMethodsRequest{
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
		resp, err := h.GetUsageSummary(callerCtx(fixedTenantID), &billingv1.GetUsageSummaryRequest{
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
	_, err := h.HandlePaymentWebhook(callerCtx(fixedTenantID), &billingv1.WebhookPayload{
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

// --- DownloadInvoice ---

func newHandlerWithDocClient(r Repository, doc documentv1.DocumentServiceClient) *Handler {
	return NewHandlerWithDeps(NewService(r), allowAllPerm{}, doc)
}

var fixedPDFDocID = uuid.MustParse("a1000000-0000-0000-0000-000000000099")

func TestHandler_DownloadInvoice_Success(t *testing.T) {
	t.Parallel()
	pdfID := fixedPDFDocID
	inv := fixedInvoice()
	inv.PDFDocumentID = &pdfID

	h := newHandlerWithDocClient(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			return inv, nil
		},
	}, &mockDocClient{})

	resp, err := h.DownloadInvoice(callerCtx(fixedTenantID), &billingv1.DownloadInvoiceRequest{
		TenantId:  fixedTenantID.String(),
		InvoiceId: fixedInvID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, "https://storage.example.com/invoice.pdf?sig=abc", resp.Url)
	assert.Equal(t, "2026-04-13T00:00:00Z", resp.ExpiresAt)
}

func TestHandler_DownloadInvoice_NoPDF_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	inv := fixedInvoice() // PDFDocumentID is nil

	h := newHandlerWithDocClient(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			return inv, nil
		},
	}, &mockDocClient{})

	_, err := h.DownloadInvoice(callerCtx(fixedTenantID), &billingv1.DownloadInvoiceRequest{
		TenantId:  fixedTenantID.String(),
		InvoiceId: fixedInvID.String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_DownloadInvoice_NilDocClient_ReturnsUnavailable(t *testing.T) {
	t.Parallel()
	pdfID := fixedPDFDocID
	inv := fixedInvoice()
	inv.PDFDocumentID = &pdfID

	// nil docClient — service not configured
	h := newHandlerWithDocClient(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			return inv, nil
		},
	}, nil)

	_, err := h.DownloadInvoice(callerCtx(fixedTenantID), &billingv1.DownloadInvoiceRequest{
		TenantId:  fixedTenantID.String(),
		InvoiceId: fixedInvID.String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestHandler_DownloadInvoice_DocServiceError_ReturnsInternal(t *testing.T) {
	t.Parallel()
	pdfID := fixedPDFDocID
	inv := fixedInvoice()
	inv.PDFDocumentID = &pdfID

	h := newHandlerWithDocClient(&mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			return inv, nil
		},
	}, &mockDocClient{
		getDownloadURLFn: func(_ context.Context, _ *documentv1.GetDownloadURLRequest, _ ...grpc.CallOption) (*documentv1.DownloadURL, error) {
			return nil, errors.New("document service unavailable")
		},
	})

	_, err := h.DownloadInvoice(callerCtx(fixedTenantID), &billingv1.DownloadInvoiceRequest{
		TenantId:  fixedTenantID.String(),
		InvoiceId: fixedInvID.String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestHandler_DownloadInvoice_InvalidTenantID(t *testing.T) {
	t.Parallel()
	h := newHandlerWithDocClient(&mockRepo{}, &mockDocClient{})
	_, err := h.DownloadInvoice(callerCtx(fixedTenantID), &billingv1.DownloadInvoiceRequest{
		TenantId:  "not-a-uuid",
		InvoiceId: fixedInvID.String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_DownloadInvoice_InvoiceNotFound(t *testing.T) {
	t.Parallel()
	h := newHandlerWithDocClient(&mockRepo{}, &mockDocClient{})
	_, err := h.DownloadInvoice(callerCtx(fixedTenantID), &billingv1.DownloadInvoiceRequest{
		TenantId:  fixedTenantID.String(),
		InvoiceId: fixedInvID.String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- Tenant boundary (#144) ---

func TestHandler_CrossTenantRead_Denied(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	victimTenant := uuid.New()
	require.NotEqual(t, callerTenant, victimTenant)

	h := newHandlerWithRepo(&mockRepo{
		listInvoicesFn: func(context.Context, uuid.UUID, int, int) ([]Invoice, error) {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil, nil
		},
	})

	_, err := h.ListInvoices(callerCtx(callerTenant), &billingv1.ListInvoicesRequest{
		TenantId: victimTenant.String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListInvoices_UsesTokenTenant(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenant uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		listInvoicesFn: func(_ context.Context, tenantID uuid.UUID, _, _ int) ([]Invoice, error) {
			gotTenant = tenantID
			return nil, nil
		},
	})

	// Request carries NO tenant at all: the token supplies it.
	_, err := h.ListInvoices(callerCtx(tid), &billingv1.ListInvoicesRequest{})
	require.NoError(t, err)
	assert.Equal(t, tid, gotTenant, "the repository must receive the token's tenant")
}

// --- Authorization denial (#139) ---
//
// One test per gated RPC. Each installs t.Fatal on the first repo/service fn
// its happy path reaches, so a passing gate is provable: if the gate did not
// fire, the mock would panic the test via t.Fatal rather than merely return
// PermissionDenied by chance.

func TestHandler_GetSubscription_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: GetSubscription must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.GetSubscription(callerCtx(uuid.New()), &billingv1.GetSubscriptionRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListInvoices_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		listInvoicesFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]Invoice, error) {
			t.Fatal("repository reached: ListInvoices must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.ListInvoices(callerCtx(uuid.New()), &billingv1.ListInvoicesRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetInvoice_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			t.Fatal("repository reached: GetInvoice must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.GetInvoice(callerCtx(uuid.New()), &billingv1.GetInvoiceRequest{InvoiceId: fixedInvID.String()})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_DownloadInvoice_Denied(t *testing.T) {
	t.Parallel()
	// The gate fires before the invoice fetch, so before the doc client is
	// ever touched — the tripwire is the invoice fetch fn, not a doc-client call.
	repo := &mockRepo{
		getInvoiceFn: func(_ context.Context, _, _ uuid.UUID) (*Invoice, error) {
			t.Fatal("repository reached: DownloadInvoice must deny before fetching the invoice")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.DownloadInvoice(callerCtx(uuid.New()), &billingv1.DownloadInvoiceRequest{InvoiceId: fixedInvID.String()})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListPaymentMethods_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		listPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) {
			t.Fatal("repository reached: ListPaymentMethods must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.ListPaymentMethods(callerCtx(uuid.New()), &billingv1.ListPaymentMethodsRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetUsageSummary_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: GetUsageSummary must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.GetUsageSummary(callerCtx(uuid.New()), &billingv1.GetUsageSummaryRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CheckPlanLimit_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: CheckPlanLimit must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.CheckPlanLimit(callerCtx(uuid.New()), &billingv1.CheckPlanLimitRequest{Resource: "production"})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CreateSubscription_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: CreateSubscription must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.CreateSubscription(callerCtx(uuid.New()), &billingv1.CreateSubscriptionRequest{Plan: "starter"})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_UpgradeSubscription_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: UpgradeSubscription must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.UpgradeSubscription(callerCtx(uuid.New()), &billingv1.UpgradeSubscriptionRequest{NewPlan: "professional"})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CancelSubscription_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: CancelSubscription must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.CancelSubscription(callerCtx(uuid.New()), &billingv1.CancelSubscriptionRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_AddPaymentMethod_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		createPaymentMethodFn: func(_ context.Context, _ *PaymentMethod) error {
			t.Fatal("repository reached: AddPaymentMethod must deny before writing")
			return nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.AddPaymentMethod(callerCtx(uuid.New()), &billingv1.AddPaymentMethodRequest{Type: "card"})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_RemovePaymentMethod_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getPaymentMethodFn: func(_ context.Context, _, _ uuid.UUID) (*PaymentMethod, error) {
			t.Fatal("repository reached: RemovePaymentMethod must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.RemovePaymentMethod(callerCtx(uuid.New()), &billingv1.RemovePaymentMethodRequest{PaymentMethodId: fixedPMID.String()})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SetDefaultPaymentMethod_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getPaymentMethodFn: func(_ context.Context, _, _ uuid.UUID) (*PaymentMethod, error) {
			t.Fatal("repository reached: SetDefaultPaymentMethod must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.SetDefaultPaymentMethod(callerCtx(uuid.New()), &billingv1.SetDefaultPaymentMethodRequest{PaymentMethodId: fixedPMID.String()})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
