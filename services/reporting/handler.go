package reporting

import (
	"context"
	"errors"

	"github.com/google/uuid"
	reportingv1 "github.com/wegofwd2020/thittam/gen/reporting/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC ReportingService.
type Handler struct {
	reportingv1.UnimplementedReportingServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a Handler wrapping the given Service.
//
// perm is required, not optional. cmd/reporting-analytics refuses to start when the
// checker is nil, so a live handler always has one; a nil here is a test or a bug, and
// RequirePermission fails such a call closed with Internal.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}

// Compile-time interface check.
var _ reportingv1.ReportingServiceServer = (*Handler)(nil)

func (h *Handler) GetReportDefinition(ctx context.Context, req *reportingv1.GetReportDefinitionRequest) (*reportingv1.ReportDefinition, error) {
	rd, err := h.svc.GetReportDefinition(ctx, req.GetReportId())
	if err != nil {
		return nil, grpcErr(err)
	}
	return reportDefToProto(rd), nil
}

func (h *Handler) ListReportDefinitions(ctx context.Context, _ *reportingv1.ListReportDefinitionsRequest) (*reportingv1.ListReportDefinitionsResponse, error) {
	defs := h.svc.ListReportDefinitions(ctx)
	out := make([]*reportingv1.ReportDefinition, len(defs))
	for i := range defs {
		out[i] = reportDefToProto(&defs[i])
	}
	return &reportingv1.ListReportDefinitionsResponse{Definitions: out}, nil
}

func (h *Handler) GetExpenseFacts(ctx context.Context, req *reportingv1.GetExpenseFactsRequest) (*reportingv1.GetExpenseFactsResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production_id")
	}

	facts, err := h.svc.GetExpenseFacts(ctx, tenantID, productionID)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*reportingv1.ExpenseFact, len(facts))
	for i := range facts {
		out[i] = expenseFactToProto(&facts[i])
	}
	return &reportingv1.GetExpenseFactsResponse{Facts: out}, nil
}

func (h *Handler) GetBudgetFacts(ctx context.Context, req *reportingv1.GetBudgetFactsRequest) (*reportingv1.GetBudgetFactsResponse, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}

	productionID, err := uuid.Parse(req.GetProductionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid production_id")
	}

	facts, err := h.svc.GetBudgetFacts(ctx, tenantID, productionID)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*reportingv1.BudgetFact, len(facts))
	for i := range facts {
		out[i] = budgetFactToProto(&facts[i])
	}
	return &reportingv1.GetBudgetFactsResponse{Facts: out}, nil
}

func (h *Handler) GetDashboardSummary(ctx context.Context, _ *reportingv1.GetDashboardSummaryRequest) (*reportingv1.DashboardSummary, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}

	summary, err := h.svc.GetDashboardSummary(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}

	return &reportingv1.DashboardSummary{
		TenantId:       summary.TenantID.String(),
		ProjectLabel:   summary.ProjectLabel,
		ProjectCount:   int32(summary.ProjectCount),
		TotalBudgeted:  summary.TotalBudgeted.StringFixed(2),
		TotalActual:    summary.TotalActual.StringFixed(2),
		TotalCommitted: summary.TotalCommitted.StringFixed(2),
		Variance:       summary.Variance.StringFixed(2),
	}, nil
}

func (h *Handler) GetPortfolioOverview(ctx context.Context, _ *reportingv1.GetPortfolioOverviewRequest) (*reportingv1.PortfolioOverview, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetPortfolioOverview(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return portfolioToProto(res), nil
}

func (h *Handler) GetFinancialSummary(ctx context.Context, _ *reportingv1.GetFinancialSummaryRequest) (*reportingv1.FinancialSummary, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetFinancialSummary(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return financialToProto(res), nil
}

func (h *Handler) GetApprovalPipeline(ctx context.Context, _ *reportingv1.GetApprovalPipelineRequest) (*reportingv1.ApprovalPipeline, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetApprovalPipeline(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return approvalToProto(res), nil
}

func (h *Handler) GetTeamUtilization(ctx context.Context, _ *reportingv1.GetTeamUtilizationRequest) (*reportingv1.TeamUtilization, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetTeamUtilization(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return teamToProto(res), nil
}

func (h *Handler) GetComplianceStatus(ctx context.Context, _ *reportingv1.GetComplianceStatusRequest) (*reportingv1.ComplianceStatus, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetComplianceStatus(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return complianceToProto(res), nil
}

// --- proto conversion helpers ---

func reportDefToProto(rd *vertical.ReportDefinition) *reportingv1.ReportDefinition {
	return &reportingv1.ReportDefinition{
		Id:             rd.ID,
		Name:           rd.Name,
		Description:    rd.Description,
		DataSources:    rd.DataSources,
		DefaultColumns: rd.DefaultColumns,
	}
}

func expenseFactToProto(f *ExpenseFact) *reportingv1.ExpenseFact {
	out := &reportingv1.ExpenseFact{
		ExpenseId:    f.ExpenseID.String(),
		ProductionId: f.ProductionID.String(),
		TenantId:     f.TenantID.String(),
		CategoryId:   f.CategoryID,
		Amount:       f.Amount.StringFixed(2),
	}
	if f.BudgetLineID != nil {
		out.BudgetLineId = f.BudgetLineID.String()
	}
	if f.ApprovedAt != nil {
		out.ApprovedAt = timestamppb.New(*f.ApprovedAt)
	}
	return out
}

func budgetFactToProto(f *BudgetFact) *reportingv1.BudgetFact {
	return &reportingv1.BudgetFact{
		BudgetId:       f.BudgetID.String(),
		ProductionId:   f.ProductionID.String(),
		TenantId:       f.TenantID.String(),
		TotalBudgeted:  f.TotalBudgeted.StringFixed(2),
		TotalActual:    f.TotalActual.StringFixed(2),
		TotalCommitted: f.TotalCommitted.StringFixed(2),
	}
}

func portfolioToProto(p *PortfolioOverview) *reportingv1.PortfolioOverview {
	out := &reportingv1.PortfolioOverview{
		TenantId:     p.TenantID.String(),
		ProjectLabel: p.ProjectLabel,
		TotalActive:  int32(p.TotalActive),
		TotalBudget:  p.TotalBudget.StringFixed(2),
		TotalActual:  p.TotalActual.StringFixed(2),
		HealthCounts: &reportingv1.BudgetHealthCount{
			OnTrack:    int32(p.HealthCounts.OnTrack),
			AtRisk:     int32(p.HealthCounts.AtRisk),
			OverBudget: int32(p.HealthCounts.OverBudget),
		},
	}
	for _, ps := range p.Projects {
		item := &reportingv1.ProjectSummary{
			Id:            ps.ID.String(),
			Title:         ps.Title,
			Status:        ps.Status,
			CurrentPhase:  ps.CurrentPhase,
			BudgetTotal:   ps.BudgetTotal.StringFixed(2),
			ActualSpend:   ps.ActualSpend.StringFixed(2),
			BudgetUsedPct: ps.BudgetUsedPct.StringFixed(2),
			Health:        ps.Health,
		}
		if ps.StartDate != nil {
			item.StartDate = *ps.StartDate
		}
		out.Projects = append(out.Projects, item)
	}
	return out
}

func financialToProto(f *FinancialSummary) *reportingv1.FinancialSummary {
	out := &reportingv1.FinancialSummary{
		TenantId:         f.TenantID.String(),
		TotalBudgeted:    f.TotalBudgeted.StringFixed(2),
		TotalActual:      f.TotalActual.StringFixed(2),
		TotalCommitted:   f.TotalCommitted.StringFixed(2),
		TotalAvailable:   f.TotalAvailable.StringFixed(2),
		BurnRatePerMonth: f.BurnRate.StringFixed(2),
	}
	for _, c := range f.ByCategory {
		out.ByCategory = append(out.ByCategory, &reportingv1.CategorySpend{
			CategoryId:   c.CategoryID,
			CategoryName: c.CategoryName,
			Budgeted:     c.Budgeted.StringFixed(2),
			Actual:       c.Actual.StringFixed(2),
			Percentage:   c.Percentage.StringFixed(2),
		})
	}
	for _, m := range f.MonthlyTrend {
		out.MonthlyTrend = append(out.MonthlyTrend, &reportingv1.MonthlySpend{
			Month:  m.Month,
			Actual: m.Actual.StringFixed(2),
			Budget: m.Budget.StringFixed(2),
		})
	}
	return out
}

func approvalToProto(a *ApprovalPipeline) *reportingv1.ApprovalPipeline {
	out := &reportingv1.ApprovalPipeline{
		TenantId:     a.TenantID.String(),
		TotalPending: int32(a.TotalPending),
		TotalAmount:  a.TotalAmount.StringFixed(2),
		ByAge: &reportingv1.ApprovalAgeBands{
			Under_24H:  int32(a.ByAge.Under24h),
			Days_1To_3: int32(a.ByAge.OneToThreeDays),
			Days_3To_7: int32(a.ByAge.ThreeToSevenDays),
			Over_7Days: int32(a.ByAge.OverSevenDays),
		},
	}
	for _, p := range a.PendingItems {
		out.PendingItems = append(out.PendingItems, &reportingv1.PendingApproval{
			Id:           p.ID.String(),
			Type:         p.Type,
			Description:  p.Description,
			Amount:       p.Amount.StringFixed(2),
			SubmittedBy:  p.SubmittedBy,
			SubmittedAt:  timestamppb.New(p.SubmittedAt),
			ProjectTitle: p.ProjectTitle,
			AgeHours:     int32(p.AgeHours),
		})
	}
	return out
}

func teamToProto(t *TeamUtilization) *reportingv1.TeamUtilization {
	out := &reportingv1.TeamUtilization{
		TenantId:        t.TenantID.String(),
		TeamMemberLabel: t.TeamMemberLabel,
		TotalMembers:    int32(t.TotalMembers),
		Assigned:        int32(t.Assigned),
		Available:       int32(t.Available),
		UtilizationPct:  t.UtilizationPct.StringFixed(2),
	}
	for _, pt := range t.ByProject {
		out.ByProject = append(out.ByProject, &reportingv1.ProjectTeam{
			ProjectId:    pt.ProjectID.String(),
			ProjectTitle: pt.ProjectTitle,
			MemberCount:  int32(pt.MemberCount),
		})
	}
	return out
}

func complianceToProto(c *ComplianceStatus) *reportingv1.ComplianceStatus {
	out := &reportingv1.ComplianceStatus{
		TenantId:         c.TenantID.String(),
		OverdueApprovals: int32(c.OverdueApprovals),
		PolicyViolations: int32(c.PolicyViolations),
		AuditFlags:       int32(c.AuditFlags),
	}
	for _, v := range c.RecentViolations {
		out.RecentViolations = append(out.RecentViolations, &reportingv1.ComplianceItem{
			Id:           v.ID.String(),
			Type:         v.Type,
			Description:  v.Description,
			Severity:     v.Severity,
			DetectedAt:   timestamppb.New(v.DetectedAt),
			ProjectTitle: v.ProjectTitle,
		})
	}
	return out
}

// grpcErr maps domain sentinel errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrReportNotFound):
		return status.Error(codes.NotFound, "report definition not found for this vertical")
	case errors.Is(err, ErrInvalidReportID):
		return status.Error(codes.InvalidArgument, "invalid report ID")
	case errors.Is(err, vertical.ErrNotFound):
		return status.Error(codes.FailedPrecondition, "vertical config not found for tenant")
	}
	return status.Error(codes.Internal, "internal error")
}
