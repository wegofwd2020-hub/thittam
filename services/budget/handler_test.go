package budget

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	budgetv1 "github.com/wegofwd2020/thittam/gen/budget/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// allowAllPerm grants every permission. Authorization semantics are covered by
// pkg/interceptor's own tests; these handler tests exercise handler logic.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm denies every permission, so a denial test can prove the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}

// ctxWithTenant injects vertical config, a tenant ID, and a synthetic caller
// (RequirePermission now runs unconditionally, so every handler test needs a
// caller in context or it fails Unauthenticated before reaching handler logic).
func ctxWithTenant(tenantID uuid.UUID) context.Context {
	ctx := tenant.WithID(ctxWithVertical(), tenantID)
	return interceptor.WithCaller(ctx, interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID})
}

// callerCtx is for handlers that do not check tenant.IDFromContext themselves
// (e.g. UpdateLineItemActuals) but still hit RequirePermission.
func callerCtx() context.Context {
	return interceptor.WithCaller(ctxWithVertical(), interceptor.CallerInfo{UserID: uuid.New(), TenantID: uuid.New()})
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{})).WithPermissionChecker(allowAllPerm{})
}

// --- CreateBudget ---

func TestHandler_CreateBudget_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateBudget(ctxWithTenant(tenantID), &budgetv1.CreateBudgetRequest{
		ProductionId: uuid.New().String(),
		Label:        "Q1 Budget",
		Currency:     "INR",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
}

func TestHandler_CreateBudget_NoTenant(t *testing.T) {
	t.Parallel()
	h := newHandler()

	_, err := h.CreateBudget(ctxWithVertical(), &budgetv1.CreateBudgetRequest{
		ProductionId: uuid.New().String(),
		Label:        "Budget",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CreateBudget_InvalidProductionID(t *testing.T) {
	t.Parallel()
	h := newHandler()

	_, err := h.CreateBudget(ctxWithTenant(uuid.New()), &budgetv1.CreateBudgetRequest{
		ProductionId: "not-a-uuid",
		Label:        "Budget",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateBudget_DefaultCurrency(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tid, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tid, Status: "draft", Currency: "INR"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.CreateBudget(ctxWithTenant(tenantID), &budgetv1.CreateBudgetRequest{
		ProductionId: uuid.New().String(),
		Label:        "No Currency",
		// Currency omitted — should default to INR
	})
	require.NoError(t, err)
	assert.Equal(t, "INR", resp.GetCurrency())
}

// --- GetBudget ---

func TestHandler_GetBudget_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	budgetID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tid, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tid, Label: "Test", Status: "draft"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.GetBudget(ctxWithTenant(tenantID), &budgetv1.GetBudgetRequest{Id: budgetID.String()})
	require.NoError(t, err)
	assert.Equal(t, budgetID.String(), resp.GetId())
}

func TestHandler_GetBudget_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetBudget(ctxWithVertical(), &budgetv1.GetBudgetRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetBudget_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetBudget(ctxWithTenant(uuid.New()), &budgetv1.GetBudgetRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetBudget_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, _, _ uuid.UUID) (*Budget, error) {
			return nil, ErrBudgetNotFound
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.GetBudget(ctxWithTenant(uuid.New()), &budgetv1.GetBudgetRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_GetBudget_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getBudgetFn: func(context.Context, uuid.UUID, uuid.UUID) (*Budget, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetBudget(ctxWithTenant(tenantID), &budgetv1.GetBudgetRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListBudgets ---

func TestHandler_ListBudgets_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listBudgetsFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]Budget, error) {
			return []Budget{{ID: uuid.New(), TenantID: tenantID, Status: "draft"}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListBudgets(ctxWithTenant(tenantID), &budgetv1.ListBudgetsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetBudgets(), 1)
}

func TestHandler_ListBudgets_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListBudgets(ctxWithVertical(), &budgetv1.ListBudgetsRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListBudgets_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListBudgets(ctxWithTenant(uuid.New()), &budgetv1.ListBudgetsRequest{ProductionId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListBudgets_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listBudgetsFn: func(context.Context, uuid.UUID, uuid.UUID, string, int, int) ([]Budget, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListBudgets(ctxWithTenant(tenantID), &budgetv1.ListBudgetsRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- SubmitBudget ---

func TestHandler_SubmitBudget_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	budgetID := uuid.New()
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tid, id uuid.UUID) (*Budget, error) {
			callCount++
			status := "draft"
			if callCount > 1 {
				status = "submitted"
			}
			return &Budget{ID: id, TenantID: tid, Status: status}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.SubmitBudget(ctxWithTenant(tenantID), &budgetv1.SubmitBudgetRequest{Id: budgetID.String()})
	require.NoError(t, err)
	assert.Equal(t, "submitted", resp.GetStatus())
}

func TestHandler_SubmitBudget_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitBudget(ctxWithVertical(), &budgetv1.SubmitBudgetRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_SubmitBudget_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitBudget(ctxWithTenant(uuid.New()), &budgetv1.SubmitBudgetRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ApproveBudget ---

func TestHandler_ApproveBudget_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	budgetID := uuid.New()
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tid, id uuid.UUID) (*Budget, error) {
			callCount++
			status := "submitted"
			if callCount > 1 {
				status = "approved"
			}
			return &Budget{ID: id, TenantID: tid, Status: status}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ApproveBudget(ctxWithTenant(tenantID), &budgetv1.ApproveBudgetRequest{Id: budgetID.String()})
	require.NoError(t, err)
	assert.Equal(t, "approved", resp.GetStatus())
}

func TestHandler_ApproveBudget_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveBudget(ctxWithVertical(), &budgetv1.ApproveBudgetRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ApproveBudget_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveBudget(ctxWithTenant(uuid.New()), &budgetv1.ApproveBudgetRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateBudgetFromTemplate ---

func TestHandler_CreateBudgetFromTemplate_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateBudgetFromTemplate(ctxWithTenant(tenantID), &budgetv1.CreateBudgetFromTemplateRequest{
		ProductionId: uuid.New().String(),
		Label:        "From Template",
		TemplateName: "standard_feature", // matches template name in movieProductionConfig
		Currency:     "INR",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreateBudgetFromTemplate_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateBudgetFromTemplate(ctxWithVertical(), &budgetv1.CreateBudgetFromTemplateRequest{
		ProductionId: uuid.New().String(),
		Label:        "X",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CreateBudgetFromTemplate_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateBudgetFromTemplate(ctxWithTenant(uuid.New()), &budgetv1.CreateBudgetFromTemplateRequest{
		ProductionId: "bad",
		Label:        "X",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateLineItem ---

func TestHandler_CreateLineItem_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateLineItem(ctxWithTenant(tenantID), &budgetv1.CreateLineItemRequest{
		BudgetId:       uuid.New().String(),
		CategoryId:     "above_the_line",
		Description:    "Director fee",
		AccountCode:    "5100",
		BudgetedAmount: "500000.00",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreateLineItem_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateLineItem(ctxWithVertical(), &budgetv1.CreateLineItemRequest{
		BudgetId:       uuid.New().String(),
		CategoryId:     "above_the_line",
		BudgetedAmount: "1000.00",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CreateLineItem_InvalidBudgetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateLineItem(ctxWithTenant(uuid.New()), &budgetv1.CreateLineItemRequest{
		BudgetId:       "bad",
		BudgetedAmount: "1000.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateLineItem_InvalidAmount(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateLineItem(ctxWithTenant(uuid.New()), &budgetv1.CreateLineItemRequest{
		BudgetId:       uuid.New().String(),
		BudgetedAmount: "not-a-number",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetLineItem ---

func TestHandler_GetLineItem_Success(t *testing.T) {
	t.Parallel()
	itemID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getLineItemFn: func(_ context.Context, id uuid.UUID) (*BudgetLineItem, error) {
			return &BudgetLineItem{ID: id, CategoryID: "above_the_line"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.GetLineItem(callerCtx(), &budgetv1.GetLineItemRequest{Id: itemID.String()})
	require.NoError(t, err)
	assert.Equal(t, itemID.String(), resp.GetId())
}

func TestHandler_GetLineItem_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetLineItem(callerCtx(), &budgetv1.GetLineItemRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetLineItem_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getLineItemFn: func(_ context.Context, _ uuid.UUID) (*BudgetLineItem, error) {
			return nil, ErrLineItemNotFound
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.GetLineItem(callerCtx(), &budgetv1.GetLineItemRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_GetLineItem_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getLineItemFn: func(context.Context, uuid.UUID) (*BudgetLineItem, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetLineItem(callerCtx(), &budgetv1.GetLineItemRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListLineItems ---

func TestHandler_ListLineItems_Success(t *testing.T) {
	t.Parallel()
	budgetID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listLineItemsFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]BudgetLineItem, error) {
			return []BudgetLineItem{{ID: uuid.New(), CategoryID: "above_the_line"}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListLineItems(callerCtx(), &budgetv1.ListLineItemsRequest{BudgetId: budgetID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetLineItems(), 1)
}

func TestHandler_ListLineItems_InvalidBudgetID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListLineItems(callerCtx(), &budgetv1.ListLineItemsRequest{BudgetId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListLineItems_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		listLineItemsFn: func(context.Context, uuid.UUID, int, int) ([]BudgetLineItem, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListLineItems(callerCtx(), &budgetv1.ListLineItemsRequest{BudgetId: uuid.New().String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- UpdateLineItemActuals ---

func TestHandler_UpdateLineItemActuals_Success(t *testing.T) {
	t.Parallel()
	itemID := uuid.New()
	h := newHandler()

	resp, err := h.UpdateLineItemActuals(callerCtx(), &budgetv1.UpdateLineItemActualsRequest{
		Id:              itemID.String(),
		ActualAmount:    "100000.00",
		CommittedAmount: "50000.00",
	})
	require.NoError(t, err)
	assert.Equal(t, itemID.String(), resp.GetId())
}

func TestHandler_UpdateLineItemActuals_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateLineItemActuals(callerCtx(), &budgetv1.UpdateLineItemActualsRequest{
		Id: "bad", ActualAmount: "100.00", CommittedAmount: "50.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateLineItemActuals_InvalidActual(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateLineItemActuals(callerCtx(), &budgetv1.UpdateLineItemActualsRequest{
		Id: uuid.New().String(), ActualAmount: "bad", CommittedAmount: "50.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateLineItemActuals_InvalidCommitted(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateLineItemActuals(callerCtx(), &budgetv1.UpdateLineItemActualsRequest{
		Id: uuid.New().String(), ActualAmount: "100.00", CommittedAmount: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CheckLineAvailability ---

func TestHandler_CheckLineAvailability_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		checkLineAvailabilityFn: func(_ context.Context, _ uuid.UUID) (decimal.Decimal, error) {
			return decimal.NewFromInt(1000000), nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.CheckLineAvailability(callerCtx(), &budgetv1.CheckLineAvailabilityRequest{
		LineItemId: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, "1000000.00", resp.GetAvailableAmount())
}

func TestHandler_CheckLineAvailability_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckLineAvailability(callerCtx(), &budgetv1.CheckLineAvailabilityRequest{LineItemId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckLineAvailability_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		checkLineAvailabilityFn: func(context.Context, uuid.UUID) (decimal.Decimal, error) {
			t.Fatal("gate must fire before the repository is read")
			return decimal.Decimal{}, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.CheckLineAvailability(callerCtx(), &budgetv1.CheckLineAvailabilityRequest{
		LineItemId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- Vertical metadata ---

func TestHandler_GetBudgetCategories(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetBudgetCategories(ctxWithVertical(), &budgetv1.GetBudgetCategoriesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetCategories(), 2)
	assert.Equal(t, "above_the_line", resp.GetCategories()[0].GetId())
}

func TestHandler_GetBudgetTemplates(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetBudgetTemplates(ctxWithVertical(), &budgetv1.GetBudgetTemplatesRequest{})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// --- fail-closed regression (#138) ---

// TestCreateBudget_NoPermissionChecker_Denies proves the fail-open is closed:
// a handler with no PermissionChecker wired must deny writes, not silently
// skip the check. Without this test, someone could reinstate the
// the nil-checker guard around RequirePermission and nothing would catch it.
func TestCreateBudget_NoPermissionChecker_Denies(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{})) // deliberately no WithPermissionChecker

	_, err := h.CreateBudget(ctxWithTenant(uuid.New()), &budgetv1.CreateBudgetRequest{
		ProductionId: uuid.New().String(),
		Label:        "Q1 Budget",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err), "a missing permission checker must deny, not skip the check")
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrBudgetNotFound, codes.NotFound},
		{ErrLineItemNotFound, codes.NotFound},
		{ErrBudgetLocked, codes.FailedPrecondition},
		{ErrBudgetNotApproved, codes.FailedPrecondition},
		{ErrAlreadyApproved, codes.FailedPrecondition},
		{ErrInvalidCategory, codes.InvalidArgument},
		{ErrInsufficientFunds, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
