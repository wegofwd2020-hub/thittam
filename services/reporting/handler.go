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
