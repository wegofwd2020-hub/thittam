package expense

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	expensev1 "github.com/wegofwd2020/thittam/gen/expense/v1"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ctxWithTenant(tenantID uuid.UUID) context.Context {
	return tenant.WithID(ctxWithVertical(), tenantID)
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}))
}

// --- CreatePurchaseOrder ---

func TestHandler_CreatePurchaseOrder_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreatePurchaseOrder(ctxWithTenant(tenantID), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: uuid.New().String(),
		PoNumber:     "PO-001",
		VendorName:   "ABC Supplies",
		Amount:       "50000.00",
		Currency:     "INR",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
}

func TestHandler_CreatePurchaseOrder_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePurchaseOrder(ctxWithVertical(), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: uuid.New().String(),
		Amount:       "100.00",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CreatePurchaseOrder_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: "bad",
		Amount:       "100.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreatePurchaseOrder_InvalidAmount(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: uuid.New().String(),
		Amount:       "not-a-number",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreatePurchaseOrder_InvalidBudgetLineID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: uuid.New().String(),
		Amount:       "100.00",
		BudgetLineId: "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreatePurchaseOrder_DefaultCurrency(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Currency: "INR"}, nil
		},
	}))
	resp, err := h.CreatePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: uuid.New().String(),
		Amount:       "100.00",
	})
	require.NoError(t, err)
	assert.Equal(t, "INR", resp.GetCurrency())
}

// --- GetPurchaseOrder ---

func TestHandler_GetPurchaseOrder_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	poID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft"}, nil
		},
	}))

	resp, err := h.GetPurchaseOrder(ctxWithTenant(tenantID), &expensev1.GetPurchaseOrderRequest{Id: poID.String()})
	require.NoError(t, err)
	assert.Equal(t, poID.String(), resp.GetId())
}

func TestHandler_GetPurchaseOrder_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetPurchaseOrder(ctxWithVertical(), &expensev1.GetPurchaseOrderRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetPurchaseOrder_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetPurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.GetPurchaseOrderRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListPurchaseOrders ---

func TestHandler_ListPurchaseOrders_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listPOsFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]PurchaseOrder, error) {
			return []PurchaseOrder{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	}))

	resp, err := h.ListPurchaseOrders(ctxWithTenant(tenantID), &expensev1.ListPurchaseOrdersRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetPurchaseOrders(), 1)
}

func TestHandler_ListPurchaseOrders_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListPurchaseOrders(ctxWithVertical(), &expensev1.ListPurchaseOrdersRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListPurchaseOrders_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListPurchaseOrders(ctxWithTenant(uuid.New()), &expensev1.ListPurchaseOrdersRequest{ProductionId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- SubmitExpense ---

func TestHandler_SubmitExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.SubmitExpense(ctxWithTenant(tenantID), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		CategoryId:   "catering", // catering does not require a PO
		Description:  "Catering costs",
		Amount:       "20000.00",
		Currency:     "INR",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_SubmitExpense_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxWithVertical(), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		Amount:       "100.00",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_SubmitExpense_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxWithTenant(uuid.New()), &expensev1.SubmitExpenseRequest{
		ProductionId: "bad",
		Amount:       "100.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SubmitExpense_InvalidAmount(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxWithTenant(uuid.New()), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		Amount:       "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SubmitExpense_InvalidBudgetLineID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxWithTenant(uuid.New()), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		Amount:       "100.00",
		BudgetLineId: "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SubmitExpense_InvalidPurchaseOrderID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxWithTenant(uuid.New()), &expensev1.SubmitExpenseRequest{
		ProductionId:    uuid.New().String(),
		Amount:          "100.00",
		PurchaseOrderId: "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetExpense ---

func TestHandler_GetExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "pending"}, nil
		},
	}))

	resp, err := h.GetExpense(ctxWithTenant(tenantID), &expensev1.GetExpenseRequest{Id: expID.String()})
	require.NoError(t, err)
	assert.Equal(t, expID.String(), resp.GetId())
}

func TestHandler_GetExpense_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetExpense(ctxWithVertical(), &expensev1.GetExpenseRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetExpense_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetExpense(ctxWithTenant(uuid.New()), &expensev1.GetExpenseRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListExpenses ---

func TestHandler_ListExpenses_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]Expense, error) {
			return []Expense{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	}))

	resp, err := h.ListExpenses(ctxWithTenant(tenantID), &expensev1.ListExpensesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetExpenses(), 1)
}

func TestHandler_ListExpenses_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListExpenses(ctxWithVertical(), &expensev1.ListExpensesRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- ApproveExpense ---

func TestHandler_ApproveExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	// Build a vertical config that includes an approval limit for the empty role
	// (the handler passes uuid.Nil as approverID and "" as approverRole).
	cfg := movieProductionConfig()
	cfg.ApprovalWorkflow.Limits = append(cfg.ApprovalWorkflow.Limits, vertical.ApprovalLimit{
		Role:      "",
		MaxAmount: decimal.NewFromInt(999999999),
	})
	ctx := tenant.WithID(vertical.WithConfig(context.Background(), cfg), tenantID)

	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			callCount++
			status := "submitted"
			if callCount > 1 {
				status = "approved"
			}
			return &Expense{ID: id, TenantID: tid, Status: status, Amount: decimal.NewFromInt(5000)}, nil
		},
	}))

	resp, err := h.ApproveExpense(ctx, &expensev1.ApproveExpenseRequest{ExpenseId: expID.String()})
	require.NoError(t, err)
	assert.Equal(t, "approved", resp.GetStatus())
}

func TestHandler_ApproveExpense_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveExpense(ctxWithVertical(), &expensev1.ApproveExpenseRequest{ExpenseId: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ApproveExpense_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveExpense(ctxWithTenant(uuid.New()), &expensev1.ApproveExpenseRequest{ExpenseId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreatePettyCashAdvance ---

func TestHandler_CreatePettyCashAdvance_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreatePettyCashAdvance(ctxWithTenant(tenantID), &expensev1.CreatePettyCashAdvanceRequest{
		ProductionId: uuid.New().String(),
		IssuedTo:     uuid.New().String(),
		Amount:       "5000.00",
		Purpose:      "Petrol",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreatePettyCashAdvance_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePettyCashAdvance(ctxWithVertical(), &expensev1.CreatePettyCashAdvanceRequest{
		ProductionId: uuid.New().String(),
		IssuedTo:     uuid.New().String(),
		Amount:       "100.00",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_CreatePettyCashAdvance_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePettyCashAdvance(ctxWithTenant(uuid.New()), &expensev1.CreatePettyCashAdvanceRequest{
		ProductionId: "bad",
		IssuedTo:     uuid.New().String(),
		Amount:       "100.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreatePettyCashAdvance_InvalidIssuedTo(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePettyCashAdvance(ctxWithTenant(uuid.New()), &expensev1.CreatePettyCashAdvanceRequest{
		ProductionId: uuid.New().String(),
		IssuedTo:     "not-a-uuid",
		Amount:       "100.00",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreatePettyCashAdvance_InvalidAmount(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePettyCashAdvance(ctxWithTenant(uuid.New()), &expensev1.CreatePettyCashAdvanceRequest{
		ProductionId: uuid.New().String(),
		IssuedTo:     uuid.New().String(),
		Amount:       "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetPettyCashAdvance ---

func TestHandler_GetPettyCashAdvance_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	pcID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "open"}, nil
		},
	}))

	resp, err := h.GetPettyCashAdvance(ctxWithTenant(tenantID), &expensev1.GetPettyCashAdvanceRequest{Id: pcID.String()})
	require.NoError(t, err)
	assert.Equal(t, pcID.String(), resp.GetId())
}

func TestHandler_GetPettyCashAdvance_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetPettyCashAdvance(ctxWithVertical(), &expensev1.GetPettyCashAdvanceRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetPettyCashAdvance_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetPettyCashAdvance(ctxWithTenant(uuid.New()), &expensev1.GetPettyCashAdvanceRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListPettyCashAdvances ---

func TestHandler_ListPettyCashAdvances_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listPettyCashFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]PettyCashAdvance, error) {
			return []PettyCashAdvance{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	}))

	resp, err := h.ListPettyCashAdvances(ctxWithTenant(tenantID), &expensev1.ListPettyCashAdvancesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetAdvances(), 1)
}

func TestHandler_ListPettyCashAdvances_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListPettyCashAdvances(ctxWithVertical(), &expensev1.ListPettyCashAdvancesRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- Vertical metadata ---

func TestHandler_GetExpenseCategories(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetExpenseCategories(ctxWithVertical(), &expensev1.GetExpenseCategoriesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetCategories(), 3)
	assert.Equal(t, "location_rental", resp.GetCategories()[0].GetId())
	assert.True(t, resp.GetCategories()[0].GetRequiresPo())
}

func TestHandler_GetApprovalLimits(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetApprovalLimits(ctxWithVertical(), &expensev1.GetApprovalLimitsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetLimits(), 2)
	assert.Equal(t, "coordinator", resp.GetLimits()[0].GetRole())
	assert.Equal(t, "200000.00", resp.GetLimits()[0].GetMaxAmount())
	assert.Equal(t, "1000000.00", resp.GetDualApprovalAbove())
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrExpenseNotFound, codes.NotFound},
		{ErrPONotFound, codes.NotFound},
		{ErrAlreadyApproved, codes.FailedPrecondition},
		{ErrApprovalLimitExceeded, codes.FailedPrecondition},
		{ErrDualApprovalRequired, codes.FailedPrecondition},
		{ErrPORequired, codes.FailedPrecondition},
		{ErrInvalidCategory, codes.InvalidArgument},
		{ErrInsufficientBudget, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
