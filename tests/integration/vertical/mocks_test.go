package vertical_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/budget"
	"github.com/wegofwd2020/thittam/services/expense"
	"github.com/wegofwd2020/thittam/services/inventory"
	"github.com/wegofwd2020/thittam/services/project"
	"github.com/wegofwd2020/thittam/services/reporting"
)

// --- project mock ---

type projectMock struct{}

func (m *projectMock) CreateProduction(ctx context.Context, p *project.Production) error { return nil }
func (m *projectMock) GetProduction(ctx context.Context, tid, id uuid.UUID) (*project.Production, error) { return nil, nil }
func (m *projectMock) ListProductions(ctx context.Context, tid uuid.UUID, status string, limit, offset int) ([]project.Production, error) { return nil, nil }
func (m *projectMock) UpdateProduction(ctx context.Context, p *project.Production) error { return nil }
func (m *projectMock) ArchiveProduction(ctx context.Context, tid, id uuid.UUID) error { return nil }
func (m *projectMock) CreatePhase(ctx context.Context, p *project.Phase) error { return nil }
func (m *projectMock) ListPhases(ctx context.Context, tid, pid uuid.UUID, limit, offset int) ([]project.Phase, error) { return nil, nil }
func (m *projectMock) GetPhase(ctx context.Context, tid, id uuid.UUID) (*project.Phase, error) { return nil, nil }
func (m *projectMock) UpdatePhaseStatus(ctx context.Context, tid, id uuid.UUID, status string) error { return nil }
func (m *projectMock) AddCrewMember(ctx context.Context, c *project.CrewMember) error { return nil }
func (m *projectMock) ListCrewMembers(ctx context.Context, tid, pid uuid.UUID, limit, offset int) ([]project.CrewMember, error) { return nil, nil }
func (m *projectMock) RemoveCrewMember(ctx context.Context, tid, id uuid.UUID) error { return nil }

type phaseReturnMock struct {
	projectMock
	phaseType string
}
func (m *phaseReturnMock) GetPhase(ctx context.Context, tid, id uuid.UUID) (*project.Phase, error) {
	return &project.Phase{ID: id, TenantID: tid, PhaseType: m.phaseType, Status: "active"}, nil
}

// --- budget mock ---

type budgetMock struct{}

func (m *budgetMock) CreateBudget(ctx context.Context, b *budget.Budget) error { return nil }
func (m *budgetMock) GetBudget(ctx context.Context, tid, id uuid.UUID) (*budget.Budget, error) { return nil, nil }
func (m *budgetMock) ListBudgets(ctx context.Context, tid, pid uuid.UUID, status string, limit, offset int) ([]budget.Budget, error) { return nil, nil }
func (m *budgetMock) UpdateBudgetStatus(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error { return nil }
func (m *budgetMock) CreateLineItem(ctx context.Context, li *budget.BudgetLineItem) error { return nil }
func (m *budgetMock) GetLineItem(ctx context.Context, tid, id uuid.UUID) (*budget.BudgetLineItem, error) { return nil, nil }
func (m *budgetMock) ListLineItems(ctx context.Context, tid, budgetID uuid.UUID, limit, offset int) ([]budget.BudgetLineItem, error) { return nil, nil }
func (m *budgetMock) UpdateLineItemActuals(ctx context.Context, tid, id uuid.UUID, actual, committed decimal.Decimal) error { return nil }
func (m *budgetMock) CheckLineAvailability(ctx context.Context, tid, id uuid.UUID) (decimal.Decimal, error) { return decimal.NewFromInt(1000000), nil }

// --- expense mock ---

type expenseMock struct{}

func (m *expenseMock) CreatePurchaseOrder(ctx context.Context, po *expense.PurchaseOrder) error { return nil }
func (m *expenseMock) GetPurchaseOrder(ctx context.Context, tid, id uuid.UUID) (*expense.PurchaseOrder, error) { return nil, nil }
func (m *expenseMock) ListPurchaseOrders(ctx context.Context, tid, pid uuid.UUID, status string, limit, offset int) ([]expense.PurchaseOrder, error) { return nil, nil }
func (m *expenseMock) UpdatePurchaseOrder(ctx context.Context, po *expense.PurchaseOrder) error { return nil }
func (m *expenseMock) CreateExpense(ctx context.Context, e *expense.Expense) error { return nil }
func (m *expenseMock) GetExpense(ctx context.Context, tid, id uuid.UUID) (*expense.Expense, error) { return nil, nil }
func (m *expenseMock) ListExpenses(ctx context.Context, tid, pid uuid.UUID, status string, limit, offset int) ([]expense.Expense, error) { return nil, nil }
func (m *expenseMock) UpdateExpense(ctx context.Context, e *expense.Expense) error { return nil }
func (m *expenseMock) RejectExpense(ctx context.Context, tid, id uuid.UUID, reason string) error { return nil }
func (m *expenseMock) CreatePettyCashAdvance(ctx context.Context, pc *expense.PettyCashAdvance) error { return nil }
func (m *expenseMock) GetPettyCashAdvance(ctx context.Context, tid, id uuid.UUID) (*expense.PettyCashAdvance, error) { return nil, nil }
func (m *expenseMock) ListPettyCashAdvances(ctx context.Context, tid, pid uuid.UUID, status string, limit, offset int) ([]expense.PettyCashAdvance, error) { return nil, nil }
func (m *expenseMock) UpdatePettyCashAdvance(ctx context.Context, pc *expense.PettyCashAdvance) error { return nil }

// --- inventory mock ---

type inventoryMock struct{ status string }

func (m *inventoryMock) CreateAsset(ctx context.Context, a *inventory.Asset) error { return nil }
func (m *inventoryMock) GetAsset(ctx context.Context, tid, id uuid.UUID) (*inventory.Asset, error) { return &inventory.Asset{ID: id, TenantID: tid, Status: m.status}, nil }
func (m *inventoryMock) ListAssets(ctx context.Context, tid uuid.UUID, status string, limit, offset int) ([]inventory.Asset, error) { return nil, nil }
func (m *inventoryMock) UpdateAssetStatus(ctx context.Context, tid, id uuid.UUID, status string) error { return nil }
func (m *inventoryMock) CheckOutAsset(ctx context.Context, c *inventory.AssetCheckout) error { return nil }
func (m *inventoryMock) CheckInAsset(ctx context.Context, tid, checkoutID uuid.UUID, conditionIn string) error { return nil }
func (m *inventoryMock) GetCheckout(ctx context.Context, tid, id uuid.UUID) (*inventory.AssetCheckout, error) { return &inventory.AssetCheckout{ID: id, TenantID: tid}, nil }
func (m *inventoryMock) ListCheckouts(ctx context.Context, tid, assetID uuid.UUID) ([]inventory.AssetCheckout, error) { return nil, nil }

// --- reporting mock ---

type reportingMock struct{}

func (m *reportingMock) GetExpenseFacts(ctx context.Context, tid, pid uuid.UUID) ([]reporting.ExpenseFact, error) { return nil, nil }
func (m *reportingMock) GetBudgetFacts(ctx context.Context, tid, pid uuid.UUID) ([]reporting.BudgetFact, error) { return nil, nil }
func (m *reportingMock) GetDashboardSummary(ctx context.Context, tid uuid.UUID) (*reporting.DashboardSummary, error) { return &reporting.DashboardSummary{TenantID: tid}, nil }
func (m *reportingMock) GetPortfolioOverview(ctx context.Context, tid uuid.UUID) (*reporting.PortfolioOverview, error) { return &reporting.PortfolioOverview{TenantID: tid}, nil }
func (m *reportingMock) GetFinancialSummary(ctx context.Context, tid uuid.UUID) (*reporting.FinancialSummary, error) { return &reporting.FinancialSummary{TenantID: tid}, nil }
func (m *reportingMock) GetApprovalPipeline(ctx context.Context, tid uuid.UUID) (*reporting.ApprovalPipeline, error) { return &reporting.ApprovalPipeline{TenantID: tid}, nil }
func (m *reportingMock) GetTeamUtilization(ctx context.Context, tid uuid.UUID) (*reporting.TeamUtilization, error) { return &reporting.TeamUtilization{TenantID: tid}, nil }
func (m *reportingMock) GetComplianceStatus(ctx context.Context, tid uuid.UUID) (*reporting.ComplianceStatus, error) { return &reporting.ComplianceStatus{TenantID: tid}, nil }
