package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	reportingv1 "github.com/wegofwd2020/thittam/gen/reporting/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ctxWithTenant(tenantID uuid.UUID) context.Context {
	return tenant.WithID(ctxWithVertical(), tenantID)
}

// ctxWithCaller layers a verified caller onto the vertical+tenant context, as
// UnaryAuthInterceptor would. RequirePermission needs caller.UserID.
func ctxWithCaller(tenantID uuid.UUID) context.Context {
	return interceptor.WithCaller(ctxWithTenant(tenantID), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tenantID,
		Roles:    []string{"member"},
	})
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}), allowAllPerm{})
}

type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm proves a gate fires before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return false, nil
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
	}), allowAllPerm{})

	resp, err := h.GetExpenseFacts(ctxWithCaller(tenantID), &reportingv1.GetExpenseFactsRequest{
		ProductionId: prodID.String(),
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetFacts(), 1)
}

func TestHandler_GetExpenseFacts_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFactsFn: func(context.Context, uuid.UUID, uuid.UUID) ([]ExpenseFact, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetExpenseFacts(ctxWithCaller(tenantID), &reportingv1.GetExpenseFactsRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
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
	_, err := newHandler().GetExpenseFacts(ctxWithCaller(uuid.New()), &reportingv1.GetExpenseFactsRequest{
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
	}), allowAllPerm{})

	resp, err := h.GetBudgetFacts(ctxWithCaller(tenantID), &reportingv1.GetBudgetFactsRequest{
		ProductionId: prodID.String(),
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetFacts(), 1)
}

func TestHandler_GetBudgetFacts_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getBudgetFactsFn: func(context.Context, uuid.UUID, uuid.UUID) ([]BudgetFact, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetBudgetFacts(ctxWithCaller(tenantID), &reportingv1.GetBudgetFactsRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
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
	_, err := newHandler().GetBudgetFacts(ctxWithCaller(uuid.New()), &reportingv1.GetBudgetFactsRequest{
		ProductionId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetDashboardSummary ---

func TestHandler_GetDashboardSummary_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	resp, err := newHandler().GetDashboardSummary(ctxWithCaller(tenantID), &reportingv1.GetDashboardSummaryRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
	assert.Equal(t, int32(5), resp.GetProjectCount())
	assert.Equal(t, "1000000.00", resp.GetTotalBudgeted())
}

func TestHandler_GetDashboardSummary_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getDashboardSummaryFn: func(context.Context, uuid.UUID) (*DashboardSummary, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetDashboardSummary(ctxWithCaller(tenantID), &reportingv1.GetDashboardSummaryRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetDashboardSummary_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetDashboardSummary(ctxWithVertical(), &reportingv1.GetDashboardSummaryRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetPortfolioOverview ---

func TestHandler_GetPortfolioOverview_Success(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPortfolioOverviewFn: func(_ context.Context, tid uuid.UUID) (*PortfolioOverview, error) {
			return &PortfolioOverview{
				TenantID:     tid,
				ProjectLabel: "Productions",
				TotalActive:  3,
				TotalBudget:  decimal.RequireFromString("100.00"),
				TotalActual:  decimal.RequireFromString("40.00"),
				// HealthCounts is recomputed by Service.GetPortfolioOverview from Projects'
				// Health field (computeHealthCounts), so it is derived below, not set here.
				Projects: []ProjectSummary{{ID: uuid.New(), Title: "P1", BudgetTotal: decimal.RequireFromString("50.00"), Health: "on_track"}},
			}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetPortfolioOverview(ctxWithCaller(tenantID), &reportingv1.GetPortfolioOverviewRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.TenantId)
	assert.Equal(t, int32(3), resp.TotalActive)
	assert.Equal(t, "100.00", resp.TotalBudget)          // decimal → StringFixed(2)
	assert.Equal(t, int32(1), resp.HealthCounts.OnTrack) // nested message; computed from Projects[0].Health
	require.Len(t, resp.Projects, 1)
	assert.Equal(t, "50.00", resp.Projects[0].BudgetTotal)
}

func TestHandler_GetPortfolioOverview_Denied(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPortfolioOverviewFn: func(context.Context, uuid.UUID) (*PortfolioOverview, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetPortfolioOverview(ctxWithCaller(tenantID), &reportingv1.GetPortfolioOverviewRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetPortfolioOverview_NoTenant(t *testing.T) {
	_, err := newHandler().GetPortfolioOverview(ctxWithVertical(), &reportingv1.GetPortfolioOverviewRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetFinancialSummary ---

func TestHandler_GetFinancialSummary_Success(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getFinancialSummaryFn: func(_ context.Context, tid uuid.UUID) (*FinancialSummary, error) {
			return &FinancialSummary{
				TenantID:      tid,
				TotalBudgeted: decimal.RequireFromString("200.00"),
				ByCategory:    []CategorySpend{{CategoryName: "cam", Actual: decimal.RequireFromString("10.00")}},
			}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetFinancialSummary(ctxWithCaller(tenantID), &reportingv1.GetFinancialSummaryRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.TenantId)
	assert.Equal(t, "200.00", resp.TotalBudgeted)
	require.Len(t, resp.ByCategory, 1)
	assert.Equal(t, "10.00", resp.ByCategory[0].Actual)
}

func TestHandler_GetFinancialSummary_Denied(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getFinancialSummaryFn: func(context.Context, uuid.UUID) (*FinancialSummary, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetFinancialSummary(ctxWithCaller(tenantID), &reportingv1.GetFinancialSummaryRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetFinancialSummary_NoTenant(t *testing.T) {
	_, err := newHandler().GetFinancialSummary(ctxWithVertical(), &reportingv1.GetFinancialSummaryRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetApprovalPipeline ---

func TestHandler_GetApprovalPipeline_Success(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getApprovalPipelineFn: func(_ context.Context, tid uuid.UUID) (*ApprovalPipeline, error) {
			// ByAge and PendingItems[i].AgeHours are recomputed by Service.GetApprovalPipeline
			// from SubmittedAt (computeAgeBands), so ByAge is derived below, not set here.
			return &ApprovalPipeline{
				TenantID:     tid,
				TotalPending: 4,
				PendingItems: []PendingApproval{{Amount: decimal.RequireFromString("5.00"), SubmittedAt: time.Now().Add(-48 * time.Hour)}},
			}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetApprovalPipeline(ctxWithCaller(tenantID), &reportingv1.GetApprovalPipelineRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.TenantId)
	assert.Equal(t, int32(4), resp.TotalPending)
	assert.Equal(t, int32(1), resp.ByAge.Days_1To_3) // 48h-old item falls in the 1-3 day band
	require.Len(t, resp.PendingItems, 1)
	assert.Equal(t, "5.00", resp.PendingItems[0].Amount)
	assert.NotNil(t, resp.PendingItems[0].SubmittedAt)
}

func TestHandler_GetApprovalPipeline_Denied(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getApprovalPipelineFn: func(context.Context, uuid.UUID) (*ApprovalPipeline, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetApprovalPipeline(ctxWithCaller(tenantID), &reportingv1.GetApprovalPipelineRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetApprovalPipeline_NoTenant(t *testing.T) {
	_, err := newHandler().GetApprovalPipeline(ctxWithVertical(), &reportingv1.GetApprovalPipelineRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetTeamUtilization ---

func TestHandler_GetTeamUtilization_Success(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTeamUtilizationFn: func(_ context.Context, tid uuid.UUID) (*TeamUtilization, error) {
			// UtilizationPct is recomputed by Service.GetTeamUtilization from
			// Assigned/TotalMembers, so it is derived below, not set here.
			return &TeamUtilization{
				TenantID:     tid,
				TotalMembers: 5,
				Assigned:     3, // 3/5 = 60.00%
				ByProject:    []ProjectTeam{{ProjectTitle: "P1", MemberCount: 2}},
			}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetTeamUtilization(ctxWithCaller(tenantID), &reportingv1.GetTeamUtilizationRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.TenantId)
	assert.Equal(t, int32(5), resp.TotalMembers)
	assert.Equal(t, "60.00", resp.UtilizationPct)
	require.Len(t, resp.ByProject, 1)
	assert.Equal(t, int32(2), resp.ByProject[0].MemberCount)
}

func TestHandler_GetTeamUtilization_Denied(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTeamUtilizationFn: func(context.Context, uuid.UUID) (*TeamUtilization, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetTeamUtilization(ctxWithCaller(tenantID), &reportingv1.GetTeamUtilizationRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetTeamUtilization_NoTenant(t *testing.T) {
	_, err := newHandler().GetTeamUtilization(ctxWithVertical(), &reportingv1.GetTeamUtilizationRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- GetComplianceStatus ---

func TestHandler_GetComplianceStatus_Success(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getComplianceStatusFn: func(_ context.Context, tid uuid.UUID) (*ComplianceStatus, error) {
			return &ComplianceStatus{
				TenantID:         tid,
				OverdueApprovals: 1,
				RecentViolations: []ComplianceItem{{Severity: "high", DetectedAt: time.Now()}},
			}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetComplianceStatus(ctxWithCaller(tenantID), &reportingv1.GetComplianceStatusRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.TenantId)
	assert.Equal(t, int32(1), resp.OverdueApprovals)
	require.Len(t, resp.RecentViolations, 1)
	assert.Equal(t, "high", resp.RecentViolations[0].Severity)
	assert.NotNil(t, resp.RecentViolations[0].DetectedAt)
}

func TestHandler_GetComplianceStatus_Denied(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getComplianceStatusFn: func(context.Context, uuid.UUID) (*ComplianceStatus, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetComplianceStatus(ctxWithCaller(tenantID), &reportingv1.GetComplianceStatusRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetComplianceStatus_NoTenant(t *testing.T) {
	_, err := newHandler().GetComplianceStatus(ctxWithVertical(), &reportingv1.GetComplianceStatusRequest{})
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
