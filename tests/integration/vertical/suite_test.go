package vertical_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"github.com/wegofwd2020/thittam/services/budget"
	"github.com/wegofwd2020/thittam/services/expense"
	"github.com/wegofwd2020/thittam/services/inventory"
	"github.com/wegofwd2020/thittam/services/project"
	"github.com/wegofwd2020/thittam/services/reporting"
)

type verticalTestCase struct {
	name   string
	config *vertical.Config

	validPhaseType           string
	validPhaseTransition     [2]string // [from, to]
	invalidPhaseTransition   [2]string
	validBudgetCat           string
	invalidBudgetCat         string
	validExpenseCat          string
	expenseCatRequiresPO     bool
	noPoExpenseCat           string
	approvalRole             string
	approvalLimit            int64
	overLimitAmount          int64
	validInventoryCat        string
	invalidInventoryCat      string
	validReportID            string
	invalidReportID          string
	projectLabel             string
	phaseLabel               string
	teamMemberLabel          string
	rateUnit                 string
}

var allVerticals = []verticalTestCase{
	{
		name: "movie-production", config: MovieProductionConfig(),
		validPhaseType: "development",
		validPhaseTransition: [2]string{"development", "pre_production"},
		invalidPhaseTransition: [2]string{"development", "post_production"},
		validBudgetCat: "above_the_line", invalidBudgetCat: "cloud_infrastructure",
		validExpenseCat: "location_rental", expenseCatRequiresPO: true, noPoExpenseCat: "",
		approvalRole: "coordinator", approvalLimit: 200000, overLimitAmount: 300000,
		validInventoryCat: "camera", invalidInventoryCat: "laptop",
		validReportID: "cost_report", invalidReportID: "burn_rate",
		projectLabel: "Production", phaseLabel: "Phase", teamMemberLabel: "Crew Member", rateUnit: "day",
	},
	{
		name: "software-development", config: SoftwareDevelopmentConfig(),
		validPhaseType: "discovery",
		validPhaseTransition: [2]string{"development", "qa"},
		invalidPhaseTransition: [2]string{"development", "go_live"},
		validBudgetCat: "personnel", invalidBudgetCat: "above_the_line",
		validExpenseCat: "contractor_invoice", expenseCatRequiresPO: true, noPoExpenseCat: "salary",
		approvalRole: "accountant", approvalLimit: 50000, overLimitAmount: 60000,
		validInventoryCat: "laptop", invalidInventoryCat: "camera",
		validReportID: "burn_rate", invalidReportID: "cost_report",
		projectLabel: "Project", phaseLabel: "Milestone", teamMemberLabel: "Team Member", rateUnit: "hour",
	},
	{
		name: "construction", config: ConstructionConfig(),
		validPhaseType: "tender",
		validPhaseTransition: [2]string{"tender", "pre_construction"},
		invalidPhaseTransition: [2]string{"tender", "structure"},
		validBudgetCat: "labour", invalidBudgetCat: "venue",
		validExpenseCat: "material_purchase", expenseCatRequiresPO: true, noPoExpenseCat: "labour_wages",
		approvalRole: "accountant", approvalLimit: 50000, overLimitAmount: 60000,
		validInventoryCat: "heavy_machinery", invalidInventoryCat: "av_equipment",
		validReportID: "boq_variance", invalidReportID: "event_pl",
		projectLabel: "Contract", phaseLabel: "Work Package", teamMemberLabel: "Site Worker", rateUnit: "day",
	},
	{
		name: "events-management", config: EventsManagementConfig(),
		validPhaseType: "concept",
		validPhaseTransition: [2]string{"concept", "planning"},
		invalidPhaseTransition: [2]string{"concept", "live"},
		validBudgetCat: "venue", invalidBudgetCat: "personnel",
		validExpenseCat: "venue_payment", expenseCatRequiresPO: true, noPoExpenseCat: "petty_cash",
		approvalRole: "accountant", approvalLimit: 25000, overLimitAmount: 30000,
		validInventoryCat: "av_equipment", invalidInventoryCat: "heavy_machinery",
		validReportID: "event_pl", invalidReportID: "boq_variance",
		projectLabel: "Event", phaseLabel: "Stage", teamMemberLabel: "Event Staff", rateUnit: "day",
	},
}

func ctxWith(cfg *vertical.Config) context.Context {
	return vertical.WithConfig(context.Background(), cfg)
}

// =============================================================================
// Per-Vertical Tests (×4 verticals × 7 test types = 28+ subtests)
// =============================================================================

func TestPhaseTransition_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/valid", func(t *testing.T) {
			t.Parallel()
			svc := project.NewService(&phaseReturnMock{phaseType: tc.validPhaseTransition[0]})
			err := svc.UpdatePhaseStatus(ctxWith(tc.config), uuid.New(), uuid.New(), tc.validPhaseTransition[1])
			require.NoError(t, err)
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			t.Parallel()
			svc := project.NewService(&phaseReturnMock{phaseType: tc.invalidPhaseTransition[0]})
			err := svc.UpdatePhaseStatus(ctxWith(tc.config), uuid.New(), uuid.New(), tc.invalidPhaseTransition[1])
			require.Error(t, err)
			assert.ErrorIs(t, err, project.ErrInvalidTransition)
		})
	}
}

func TestCreatePhase_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/valid", func(t *testing.T) {
			t.Parallel()
			svc := project.NewService(&projectMock{})
			err := svc.CreatePhase(ctxWith(tc.config), &project.Phase{
				ProductionID: uuid.New(), TenantID: uuid.New(), PhaseType: tc.validPhaseType,
			})
			require.NoError(t, err)
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			t.Parallel()
			svc := project.NewService(&projectMock{})
			err := svc.CreatePhase(ctxWith(tc.config), &project.Phase{
				ProductionID: uuid.New(), TenantID: uuid.New(), PhaseType: "nonexistent_phase",
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, project.ErrInvalidPhaseType)
		})
	}
}

func TestBudgetCategory_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/valid", func(t *testing.T) {
			t.Parallel()
			svc := budget.NewService(&budgetMock{})
			err := svc.CreateLineItem(ctxWith(tc.config), &budget.BudgetLineItem{
				BudgetID: uuid.New(), TenantID: uuid.New(), CategoryID: tc.validBudgetCat,
			})
			require.NoError(t, err)
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			t.Parallel()
			svc := budget.NewService(&budgetMock{})
			err := svc.CreateLineItem(ctxWith(tc.config), &budget.BudgetLineItem{
				BudgetID: uuid.New(), TenantID: uuid.New(), CategoryID: tc.invalidBudgetCat,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, budget.ErrInvalidCategory)
		})
	}
}

func TestExpenseApprovalLimit_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/limit_exists", func(t *testing.T) {
			t.Parallel()
			limit := tc.config.ApprovalWorkflow.LimitForRole(tc.approvalRole)
			require.NotNil(t, limit, "role %s should have a limit in %s", tc.approvalRole, tc.name)
			assert.True(t, limit.Equal(decimal.NewFromInt(tc.approvalLimit)))
		})
		t.Run(tc.name+"/over_limit_amount_exceeds", func(t *testing.T) {
			t.Parallel()
			limit := tc.config.ApprovalWorkflow.LimitForRole(tc.approvalRole)
			require.NotNil(t, limit)
			overAmount := decimal.NewFromInt(tc.overLimitAmount)
			assert.True(t, overAmount.GreaterThan(*limit),
				"overLimitAmount %s should exceed role limit %s", overAmount, limit)
		})
	}
}

func TestExpensePORequirement_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/po_required_category", func(t *testing.T) {
			t.Parallel()
			svc := expense.NewService(&expenseMock{})
			err := svc.SubmitExpense(ctxWith(tc.config), &expense.Expense{
				TenantID: uuid.New(), CategoryID: tc.validExpenseCat,
				Amount: decimal.NewFromInt(10000),
			})
			if tc.expenseCatRequiresPO {
				require.Error(t, err)
				assert.ErrorIs(t, err, expense.ErrPORequired)
			}
		})
		if tc.noPoExpenseCat != "" {
			t.Run(tc.name+"/no_po_succeeds", func(t *testing.T) {
				t.Parallel()
				svc := expense.NewService(&expenseMock{})
				err := svc.SubmitExpense(ctxWith(tc.config), &expense.Expense{
					TenantID: uuid.New(), CategoryID: tc.noPoExpenseCat,
					Amount: decimal.NewFromInt(10000),
				})
				require.NoError(t, err)
			})
		}
	}
}

func TestInventoryCategory_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/valid", func(t *testing.T) {
			t.Parallel()
			svc := inventory.NewService(&inventoryMock{status: "available"})
			err := svc.CreateAsset(ctxWith(tc.config), &inventory.Asset{
				TenantID: uuid.New(), CategoryID: tc.validInventoryCat, Name: "Test",
			})
			require.NoError(t, err)
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			t.Parallel()
			svc := inventory.NewService(&inventoryMock{status: "available"})
			err := svc.CreateAsset(ctxWith(tc.config), &inventory.Asset{
				TenantID: uuid.New(), CategoryID: tc.invalidInventoryCat, Name: "Test",
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, inventory.ErrInvalidCategory)
		})
	}
}

func TestReportDefinition_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name+"/valid", func(t *testing.T) {
			t.Parallel()
			svc := reporting.NewService(&reportingMock{})
			rd, err := svc.GetReportDefinition(ctxWith(tc.config), tc.validReportID)
			require.NoError(t, err)
			assert.Equal(t, tc.validReportID, rd.ID)
		})
		t.Run(tc.name+"/invalid", func(t *testing.T) {
			t.Parallel()
			svc := reporting.NewService(&reportingMock{})
			_, err := svc.GetReportDefinition(ctxWith(tc.config), tc.invalidReportID)
			require.Error(t, err)
			assert.ErrorIs(t, err, reporting.ErrReportNotFound)
		})
	}
}

func TestEntityLabels_AllVerticals(t *testing.T) {
	t.Parallel()
	for _, tc := range allVerticals {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := project.NewService(&projectMock{})
			labels := svc.GetEntityLabels(ctxWith(tc.config))
			assert.Equal(t, tc.projectLabel, labels.Project)
			assert.Equal(t, tc.phaseLabel, labels.Phase)
			assert.Equal(t, tc.teamMemberLabel, labels.TeamMember)
			assert.Equal(t, tc.rateUnit, labels.RateUnit)
		})
	}
}
