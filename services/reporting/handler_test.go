package reporting

import (
	"context"
	"testing"

	"github.com/google/uuid"
	reportingv1 "github.com/wegofwd2020/thittam/gen/reporting/v1"
	"github.com/wegofwd2020/thittam/pkg/tenant"
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

// --- GetReportDefinition ---

func TestHandler_GetReportDefinition_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().GetReportDefinition(ctxWithVertical(), &reportingv1.GetReportDefinitionRequest{
		ReportId: "cost_report",
	})
	require.NoError(t, err)
	assert.Equal(t, "cost_report", resp.GetId())
	assert.Equal(t, "Daily Cost Report", resp.GetName())
}

func TestHandler_GetReportDefinition_NotFound(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetReportDefinition(ctxWithVertical(), &reportingv1.GetReportDefinitionRequest{
		ReportId: "nonexistent",
	})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- ListReportDefinitions ---

func TestHandler_ListReportDefinitions_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().ListReportDefinitions(ctxWithVertical(), &reportingv1.ListReportDefinitionsRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.GetDefinitions(), 2)
	assert.Equal(t, "cost_report", resp.GetDefinitions()[0].GetId())
}

// --- GetExpenseFacts ---

func TestHandler_GetExpenseFacts_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFactsFn: func(_ context.Context, _, _ uuid.UUID) ([]ExpenseFact, error) {
			return []ExpenseFact{{ExpenseID: uuid.New(), ProductionID: prodID, TenantID: tenantID, CategoryID: "location_rental"}}, nil
		},
	}))

	resp, err := h.GetExpenseFacts(ctxWithTenant(tenantID), &reportingv1.GetExpenseFactsRequest{
		ProductionId: prodID.String(),
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetFacts(), 1)
}

func TestHandler_GetExpenseFacts_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetExpenseFacts(ctxWithVertical(), &reportingv1.GetExpenseFactsRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetExpenseFacts_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetExpenseFacts(ctxWithTenant(uuid.New()), &reportingv1.GetExpenseFactsRequest{
		ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetBudgetFacts ---

func TestHandler_GetBudgetFacts_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getBudgetFactsFn: func(_ context.Context, _, _ uuid.UUID) ([]BudgetFact, error) {
			return []BudgetFact{{BudgetID: uuid.New(), ProductionID: prodID, TenantID: tenantID}}, nil
		},
	}))

	resp, err := h.GetBudgetFacts(ctxWithTenant(tenantID), &reportingv1.GetBudgetFactsRequest{
		ProductionId: prodID.String(),
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetFacts(), 1)
}

func TestHandler_GetBudgetFacts_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetBudgetFacts(ctxWithVertical(), &reportingv1.GetBudgetFactsRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_GetBudgetFacts_InvalidProductionID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetBudgetFacts(ctxWithTenant(uuid.New()), &reportingv1.GetBudgetFactsRequest{
		ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetDashboardSummary ---

func TestHandler_GetDashboardSummary_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	resp, err := newHandler().GetDashboardSummary(ctxWithTenant(tenantID), &reportingv1.GetDashboardSummaryRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
	assert.Equal(t, int32(5), resp.GetProjectCount())
	assert.Equal(t, "1000000.00", resp.GetTotalBudgeted())
}

func TestHandler_GetDashboardSummary_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDashboardSummary(ctxWithVertical(), &reportingv1.GetDashboardSummaryRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrReportNotFound, codes.NotFound},
		{ErrInvalidReportID, codes.InvalidArgument},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
