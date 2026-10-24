package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
)

// --- SetAccountCostBehavior ---

func TestHandler_SetAccountCostBehavior_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	accountID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAccountByIDFn: func(_ context.Context, _, id uuid.UUID) (*Account, error) {
			return &Account{ID: id, TenantID: tenantID, AccountType: "expense"}, nil
		},
		setCostBehaviorFn: func(_ context.Context, _, id uuid.UUID, cb string) (*Account, error) {
			return &Account{ID: id, TenantID: tenantID, AccountType: "expense", CostBehavior: cb}, nil
		},
	}), allowAllPerm{})

	resp, err := h.SetAccountCostBehavior(callerCtx(tenantID), &ledgerv1.SetAccountCostBehaviorRequest{
		TenantId:     tenantID.String(),
		Id:           accountID.String(),
		CostBehavior: "fixed",
	})
	require.NoError(t, err)
	assert.Equal(t, accountID.String(), resp.GetId())
	assert.Equal(t, "fixed", resp.GetCostBehavior())
}

func TestHandler_SetAccountCostBehavior_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SetAccountCostBehavior(callerCtx(uuid.New()), &ledgerv1.SetAccountCostBehaviorRequest{
		TenantId: "bad", Id: uuid.New().String(), CostBehavior: "fixed",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetAccountCostBehavior_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().SetAccountCostBehavior(callerCtx(tid), &ledgerv1.SetAccountCostBehaviorRequest{
		TenantId: tid.String(), Id: "bad", CostBehavior: "fixed",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetAccountCostBehavior_InvalidValueMapsToInvalidArgument(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().SetAccountCostBehavior(callerCtx(tid), &ledgerv1.SetAccountCostBehaviorRequest{
		TenantId: tid.String(), Id: uuid.New().String(), CostBehavior: "semi",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetAccountCostBehavior_NotExpenseMapsToInvalidArgument(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAccountByIDFn: func(_ context.Context, tenantID, id uuid.UUID) (*Account, error) {
			return &Account{ID: id, TenantID: tenantID, AccountType: "asset"}, nil
		},
	}), allowAllPerm{})
	_, err := h.SetAccountCostBehavior(callerCtx(tid), &ledgerv1.SetAccountCostBehaviorRequest{
		TenantId: tid.String(), Id: uuid.New().String(), CostBehavior: "fixed",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetAccountCostBehavior_AdminRequired(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		setCostBehaviorFn: func(context.Context, uuid.UUID, uuid.UUID, string) (*Account, error) {
			t.Fatal("permission gate must fire before the repository is written")
			return nil, nil
		},
	}), denyPerm{})
	_, err := h.SetAccountCostBehavior(callerCtx(tid), &ledgerv1.SetAccountCostBehaviorRequest{
		TenantId: tid.String(), Id: uuid.New().String(), CostBehavior: "fixed",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- GetMonthlyCostBehavior ---

func TestHandler_GetMonthlyCostBehavior_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		monthlyCostBehaviorFn: func(context.Context, uuid.UUID, string, string) ([]MonthlyCostBehaviorRow, error) {
			return []MonthlyCostBehaviorRow{{
				Month:        "2026-01",
				Fixed:        decimal.RequireFromString("1000"),
				Variable:     decimal.RequireFromString("250.5"),
				Unclassified: decimal.RequireFromString("40"),
				Total:        decimal.RequireFromString("1290.5"),
			}}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetMonthlyCostBehavior(callerCtx(tid), &ledgerv1.GetMonthlyCostBehaviorRequest{
		TenantId:  tid.String(),
		FromMonth: "2026-01",
		ToMonth:   "2026-01",
	})
	require.NoError(t, err)
	require.Len(t, resp.GetMonths(), 1)
	m := resp.GetMonths()[0]
	assert.Equal(t, "2026-01", m.GetMonth())
	assert.Equal(t, "1000.00", m.GetFixed())
	assert.Equal(t, "250.50", m.GetVariable())
	assert.Equal(t, "40.00", m.GetUnclassified())
	assert.Equal(t, "1290.50", m.GetTotal())
}

func TestHandler_GetMonthlyCostBehavior_InvalidRangeMapsToInvalidArgument(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetMonthlyCostBehavior(callerCtx(tid), &ledgerv1.GetMonthlyCostBehaviorRequest{
		TenantId: tid.String(), FromMonth: "2026-03", ToMonth: "2026-01",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetMonthlyCostBehavior_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetMonthlyCostBehavior(callerCtx(uuid.New()), &ledgerv1.GetMonthlyCostBehaviorRequest{
		TenantId: "bad", FromMonth: "2026-01", ToMonth: "2026-01",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetMonthlyCostBehavior_MemberDenied(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		monthlyCostBehaviorFn: func(context.Context, uuid.UUID, string, string) ([]MonthlyCostBehaviorRow, error) {
			t.Fatal("permission gate must fire before the repository is read")
			return nil, nil
		},
	}), denyPerm{})
	_, err := h.GetMonthlyCostBehavior(callerCtx(tid), &ledgerv1.GetMonthlyCostBehaviorRequest{
		TenantId: tid.String(), FromMonth: "2026-01", ToMonth: "2026-01",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
