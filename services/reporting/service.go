package reporting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Service implements reporting-analytics business logic.
type Service struct {
	repo Repository
}

// NewService creates a reporting-analytics service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetReportDefinition validates the report ID against the vertical config and returns it.
func (s *Service) GetReportDefinition(ctx context.Context, reportID string) (*vertical.ReportDefinition, error) {
	if reportID == "" {
		return nil, ErrInvalidReportID
	}

	vcfg := vertical.MustFromContext(ctx)

	rd := vcfg.FindReportDefinition(reportID)
	if rd == nil {
		return nil, fmt.Errorf("%w: %q is not valid for vertical %s", ErrReportNotFound, reportID, vcfg.ID)
	}

	return rd, nil
}

// ListReportDefinitions returns all report definitions for this tenant's vertical.
func (s *Service) ListReportDefinitions(ctx context.Context) []vertical.ReportDefinition {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.ReportDefinitions
}

// GetExpenseFacts retrieves expense facts for a production.
func (s *Service) GetExpenseFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]ExpenseFact, error) {
	return s.repo.GetExpenseFacts(ctx, tenantID, productionID)
}

// GetBudgetFacts retrieves budget facts for a production.
func (s *Service) GetBudgetFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]BudgetFact, error) {
	return s.repo.GetBudgetFacts(ctx, tenantID, productionID)
}

// GetDashboardSummary returns a high-level dashboard using entity labels from the vertical.
func (s *Service) GetDashboardSummary(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error) {
	vcfg := vertical.MustFromContext(ctx)

	summary, err := s.repo.GetDashboardSummary(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get dashboard: %w", err)
	}

	// Enrich with vertical entity labels
	summary.ProjectLabel = vcfg.EntityLabels.ProjectPlural

	return summary, nil
}
