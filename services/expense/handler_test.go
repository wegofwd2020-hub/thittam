package expense

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	expensev1 "github.com/wegofwd2020/thittam/gen/expense/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
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

// ctxWithCaller injects the vertical config, the caller's tenant, and the given caller.
func ctxWithCaller(caller interceptor.CallerInfo) context.Context {
	return interceptor.WithCaller(tenant.WithID(ctxWithVertical(), caller.TenantID), caller)
}

// ctxTenantNoCaller injects vertical + tenant but NO caller (to exercise the caller guard).
func ctxTenantNoCaller(tenantID uuid.UUID) context.Context {
	return tenant.WithID(ctxWithVertical(), tenantID)
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{})).WithPermissionChecker(allowAllPerm{})
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

func TestHandler_CreatePurchaseOrder_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePurchaseOrder(ctxTenantNoCaller(uuid.New()), &expensev1.CreatePurchaseOrderRequest{ProductionId: uuid.New().String(), VendorName: "v", Amount: "100"})
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
	})).WithPermissionChecker(allowAllPerm{})
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
	})).WithPermissionChecker(allowAllPerm{})

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

func TestHandler_GetPurchaseOrder_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getPOFn: func(_ context.Context, _, _ uuid.UUID) (*PurchaseOrder, error) {
			t.Fatal("repository reached: GetPurchaseOrder must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetPurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.GetPurchaseOrderRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListPurchaseOrders ---

func TestHandler_ListPurchaseOrders_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listPOsFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]PurchaseOrder, error) {
			return []PurchaseOrder{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

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

func TestHandler_ListPurchaseOrders_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		listPOsFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]PurchaseOrder, error) {
			t.Fatal("repository reached: ListPurchaseOrders must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListPurchaseOrders(ctxWithTenant(uuid.New()), &expensev1.ListPurchaseOrdersRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ApprovePurchaseOrder ---

func TestHandler_ApprovePurchaseOrder_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	poID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID, Roles: []string{"manager"}}
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			callCount++
			st := "draft"
			if callCount > 1 {
				st = "approved"
			}
			return &PurchaseOrder{ID: id, TenantID: tid, Status: st, Amount: decimal.NewFromInt(5000)}, nil
		},
		updatePOFn: func(_ context.Context, _ *PurchaseOrder) error { return nil },
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ApprovePurchaseOrder(ctxWithCaller(caller), &expensev1.ApprovePurchaseOrderRequest{Id: poID.String()})
	require.NoError(t, err)
	assert.Equal(t, "approved", resp.GetStatus())
}

func TestHandler_ApprovePurchaseOrder_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApprovePurchaseOrder(ctxTenantNoCaller(uuid.New()), &expensev1.ApprovePurchaseOrderRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ApprovePurchaseOrder_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getPOFn: func(context.Context, uuid.UUID, uuid.UUID) (*PurchaseOrder, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ApprovePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.ApprovePurchaseOrderRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ApprovePurchaseOrder_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApprovePurchaseOrder(ctxWithVertical(), &expensev1.ApprovePurchaseOrderRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ApprovePurchaseOrder_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApprovePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.ApprovePurchaseOrderRequest{Id: "bad"})
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

func TestHandler_SubmitExpense_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxTenantNoCaller(uuid.New()), &expensev1.SubmitExpenseRequest{ProductionId: uuid.New().String(), CategoryId: "catering", Amount: "100"})
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
	})).WithPermissionChecker(allowAllPerm{})

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

func TestHandler_GetExpense_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, _, _ uuid.UUID) (*Expense, error) {
			t.Fatal("repository reached: GetExpense must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetExpense(ctxWithTenant(uuid.New()), &expensev1.GetExpenseRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListExpenses ---

func TestHandler_ListExpenses_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]Expense, error) {
			return []Expense{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListExpenses(ctxWithTenant(tenantID), &expensev1.ListExpensesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetExpenses(), 1)
}

func TestHandler_ListExpenses_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListExpenses(ctxWithVertical(), &expensev1.ListExpensesRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListExpenses_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]Expense, error) {
			t.Fatal("repository reached: ListExpenses must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListExpenses(ctxWithTenant(uuid.New()), &expensev1.ListExpensesRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ApproveExpense ---

func TestHandler_ApproveExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID, Roles: []string{"manager"}}
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			callCount++
			st := "submitted"
			if callCount > 1 {
				st = "approved"
			}
			return &Expense{ID: id, TenantID: tid, Status: st, Amount: decimal.NewFromInt(5000)}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.ApproveExpense(ctxWithCaller(caller), &expensev1.ApproveExpenseRequest{ExpenseId: expID.String()})
	require.NoError(t, err)
	assert.Equal(t, "approved", resp.GetStatus())
}

func TestHandler_ApproveExpense_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveExpense(ctxTenantNoCaller(uuid.New()), &expensev1.ApproveExpenseRequest{ExpenseId: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
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

// --- RejectExpense ---

func TestHandler_RejectExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	rejecter := uuid.New()
	caller := interceptor.CallerInfo{UserID: rejecter, TenantID: tenantID}
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			callCount++
			if callCount > 1 {
				// second call: GetExpense after reject → return the rejected record
				return &Expense{ID: id, TenantID: tid, Status: "rejected", Amount: decimal.NewFromInt(5000), RejectedBy: &rejecter}, nil
			}
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(5000)}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.RejectExpense(ctxWithCaller(caller), &expensev1.RejectExpenseRequest{ExpenseId: expID.String(), Reason: "over budget"})
	require.NoError(t, err)
	assert.Equal(t, "rejected", resp.GetStatus())
	assert.Equal(t, rejecter.String(), resp.GetRejectedBy())
}

func TestHandler_RejectExpense_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxTenantNoCaller(uuid.New()),
		&expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "over budget"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_RejectExpense_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(context.Context, uuid.UUID, uuid.UUID) (*Expense, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})
	_, err := h.RejectExpense(ctxWithTenant(uuid.New()), &expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_RejectExpense_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxWithVertical(), &expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_RejectExpense_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxWithTenant(uuid.New()), &expensev1.RejectExpenseRequest{ExpenseId: "bad", Reason: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_RejectExpense_EmptyReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxWithTenant(uuid.New()), &expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: ""})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = newHandler().RejectExpense(ctxWithTenant(uuid.New()), &expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "   "})
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
	})).WithPermissionChecker(allowAllPerm{})

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

func TestHandler_GetPettyCashAdvance_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, _, _ uuid.UUID) (*PettyCashAdvance, error) {
			t.Fatal("repository reached: GetPettyCashAdvance must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetPettyCashAdvance(ctxWithTenant(uuid.New()), &expensev1.GetPettyCashAdvanceRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- ListPettyCashAdvances ---

func TestHandler_ListPettyCashAdvances_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listPettyCashFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]PettyCashAdvance, error) {
			return []PettyCashAdvance{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.ListPettyCashAdvances(ctxWithTenant(tenantID), &expensev1.ListPettyCashAdvancesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetAdvances(), 1)
}

func TestHandler_ListPettyCashAdvances_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListPettyCashAdvances(ctxWithVertical(), &expensev1.ListPettyCashAdvancesRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_ListPettyCashAdvances_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		listPettyCashFn: func(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]PettyCashAdvance, error) {
			t.Fatal("repository reached: ListPettyCashAdvances must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.ListPettyCashAdvances(ctxWithTenant(uuid.New()), &expensev1.ListPettyCashAdvancesRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- SettlePettyCash ---

func TestHandler_SettlePettyCash_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	pcID := uuid.New()
	holder := uuid.New()
	caller := interceptor.CallerInfo{UserID: holder, TenantID: tenantID}
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			callCount++
			st := "issued"
			if callCount > 1 {
				st = "settled"
			}
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: st, Amount: decimal.NewFromInt(5000), IssuedTo: holder}, nil
		},
		updatePettyCashFn: func(_ context.Context, _ *PettyCashAdvance) error { return nil },
	})).WithPermissionChecker(allowAllPerm{})

	resp, err := h.SettlePettyCash(ctxWithCaller(caller), &expensev1.SettlePettyCashRequest{Id: pcID.String(), UnspentAmount: "500.00"})
	require.NoError(t, err)
	assert.Equal(t, "settled", resp.GetStatus())
}

func TestHandler_SettlePettyCash_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(context.Context, uuid.UUID, uuid.UUID) (*PettyCashAdvance, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.SettlePettyCash(ctxWithTenant(uuid.New()), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "500.00"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SettlePettyCash_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SettlePettyCash(ctxWithVertical(), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "500.00"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_SettlePettyCash_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SettlePettyCash(ctxWithTenant(uuid.New()), &expensev1.SettlePettyCashRequest{Id: "bad", UnspentAmount: "500.00"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SettlePettyCash_InvalidAmount(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SettlePettyCash(ctxWithTenant(uuid.New()), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "abc"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SettlePettyCash_NegativeAmount(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SettlePettyCash(ctxWithTenant(uuid.New()), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "-5.00"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SettlePettyCash_ExceedsAdvance(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	holder := uuid.New()
	caller := interceptor.CallerInfo{UserID: holder, TenantID: tenantID}
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.RequireFromString("100.00"), IssuedTo: holder}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})

	_, err := h.SettlePettyCash(ctxWithCaller(caller), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "150.00"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SettlePettyCash_NotHolder(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID}
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: uuid.New()}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.SettlePettyCash(ctxWithCaller(caller), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "10.00"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SettlePettyCash_HolderSucceeds(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	holder := uuid.New()
	caller := interceptor.CallerInfo{UserID: holder, TenantID: tenantID}
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: holder}, nil
		},
		updatePettyCashFn: func(_ context.Context, _ *PettyCashAdvance) error { return nil },
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.SettlePettyCash(ctxWithCaller(caller), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "10.00"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_SettlePettyCash_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SettlePettyCash(ctxTenantNoCaller(uuid.New()), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "10.00"})
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

// --- fail-closed regression (#138) ---

// TestSubmitExpense_NoPermissionChecker_Denies proves the fail-open is closed:
// a handler with no PermissionChecker wired must deny writes, not silently
// skip the check. Without this test, someone could reinstate the
// the nil-checker guard around RequirePermission and nothing would catch it.
func TestSubmitExpense_NoPermissionChecker_Denies(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{})) // deliberately no WithPermissionChecker

	_, err := h.SubmitExpense(ctxWithTenant(uuid.New()), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		CategoryId:   "catering",
		Amount:       "1000.00",
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

func TestHandler_CreatePurchaseOrder_SetsRaisedBy(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID}
	var saved *PurchaseOrder
	h := NewHandler(NewService(&mockRepo{
		createPOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.CreatePurchaseOrder(ctxWithCaller(caller), &expensev1.CreatePurchaseOrderRequest{ProductionId: uuid.New().String(), VendorName: "v", Amount: "100"})
	require.NoError(t, err)
	assert.Equal(t, caller.UserID, saved.RaisedBy)
}

func TestHandler_SubmitExpense_SetsSubmittedBy(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID}
	var saved *Expense
	h := NewHandler(NewService(&mockRepo{
		createExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.SubmitExpense(ctxWithCaller(caller), &expensev1.SubmitExpenseRequest{ProductionId: uuid.New().String(), CategoryId: "catering", Amount: "100"})
	require.NoError(t, err)
	assert.Equal(t, caller.UserID, saved.SubmittedBy)
}
