package expense

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	expensev1 "github.com/wegofwd2020/thittam/gen/expense/v1"
)

func TestHandler_SubmitExpense_InvalidPartyID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxWithTenant(uuid.New()), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		CategoryId:   "catering",
		Amount:       "100.00",
		Currency:     "INR",
		PartyId:      "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreatePurchaseOrder_InvalidPartyID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreatePurchaseOrder(ctxWithTenant(uuid.New()), &expensev1.CreatePurchaseOrderRequest{
		ProductionId: uuid.New().String(),
		VendorName:   "Acme Catering",
		Amount:       "100.00",
		PartyId:      "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListExpenses_ByParty_NeedsExpenseRead(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{}), denyPerm{})
	_, err := h.ListExpenses(ctxWithTenant(uuid.New()), &expensev1.ListExpensesRequest{PartyId: uuid.New().String()})
	assert.Error(t, err)
}

func TestHandler_ListExpenses_ByParty_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListExpenses(ctxWithTenant(uuid.New()), &expensev1.ListExpensesRequest{PartyId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListExpenses_ByParty_MapsPartyID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	pid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listExpensesByPartyFn: func(_ context.Context, _, partyID uuid.UUID, _, _ int) ([]Expense, error) {
			assert.Equal(t, pid, partyID)
			return []Expense{{ID: uuid.New(), PartyID: &partyID, CategoryID: "catering", Status: "submitted"}}, nil
		},
	}), allowAllPerm{})
	resp, err := h.ListExpenses(ctxWithTenant(tid), &expensev1.ListExpensesRequest{PartyId: pid.String()})
	require.NoError(t, err)
	require.Len(t, resp.GetExpenses(), 1)
	assert.Equal(t, pid.String(), resp.GetExpenses()[0].GetPartyId())
}

func TestHandler_ListPurchaseOrders_ByParty_NeedsExpenseRead(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{}), denyPerm{})
	_, err := h.ListPurchaseOrders(ctxWithTenant(uuid.New()), &expensev1.ListPurchaseOrdersRequest{PartyId: uuid.New().String()})
	assert.Error(t, err)
}

func TestHandler_ListPurchaseOrders_ByParty_MapsPartyID(t *testing.T) {
	t.Parallel()
	pid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listPOsByPartyFn: func(_ context.Context, _, partyID uuid.UUID, _, _ int) ([]PurchaseOrder, error) {
			return []PurchaseOrder{{ID: uuid.New(), PartyID: &partyID, VendorName: "Acme", PONumber: "PO-1"}}, nil
		},
	}), allowAllPerm{})
	resp, err := h.ListPurchaseOrders(ctxWithTenant(uuid.New()), &expensev1.ListPurchaseOrdersRequest{PartyId: pid.String()})
	require.NoError(t, err)
	require.Len(t, resp.GetPurchaseOrders(), 1)
	assert.Equal(t, pid.String(), resp.GetPurchaseOrders()[0].GetPartyId())
}
