package vertical_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/services/budget"
	"github.com/wegofwd2020/thittam/services/inventory"
	"github.com/wegofwd2020/thittam/services/project"
	"github.com/wegofwd2020/thittam/services/reporting"
)

func TestIsolation_MovieCantUseCloudInfrastructure(t *testing.T) {
	t.Parallel()
	svc := budget.NewService(&budgetMock{})
	err := svc.CreateLineItem(ctxWith(MovieProductionConfig()), &budget.BudgetLineItem{
		BudgetID: uuid.New(), TenantID: uuid.New(), CategoryID: "cloud_infrastructure",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, budget.ErrInvalidCategory)
}

func TestIsolation_SoftwareDevCantUseAboveTheLine(t *testing.T) {
	t.Parallel()
	svc := budget.NewService(&budgetMock{})
	err := svc.CreateLineItem(ctxWith(SoftwareDevelopmentConfig()), &budget.BudgetLineItem{
		BudgetID: uuid.New(), TenantID: uuid.New(), CategoryID: "above_the_line",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, budget.ErrInvalidCategory)
}

func TestIsolation_DiscoveryPhaseRejectedForMovie(t *testing.T) {
	t.Parallel()
	svc := project.NewService(&projectMock{})
	err := svc.CreatePhase(ctxWith(MovieProductionConfig()), &project.Phase{
		ProductionID: uuid.New(), TenantID: uuid.New(), PhaseType: "discovery",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, project.ErrInvalidPhaseType)
}

func TestIsolation_TenderPhaseRejectedForSoftware(t *testing.T) {
	t.Parallel()
	svc := project.NewService(&projectMock{})
	err := svc.CreatePhase(ctxWith(SoftwareDevelopmentConfig()), &project.Phase{
		ProductionID: uuid.New(), TenantID: uuid.New(), PhaseType: "tender",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, project.ErrInvalidPhaseType)
}

func TestIsolation_ConstructionCoAHasRetentionReceivable(t *testing.T) {
	t.Parallel()
	var found bool
	for _, acct := range ConstructionConfig().DefaultChartOfAccounts {
		if acct.Code == "1200" && acct.Name == "Retention Receivable" {
			found = true
		}
	}
	assert.True(t, found, "construction should have Retention Receivable")
}

func TestIsolation_MovieCoADoesNotHaveRetentionReceivable(t *testing.T) {
	t.Parallel()
	for _, acct := range MovieProductionConfig().DefaultChartOfAccounts {
		assert.NotEqual(t, "Retention Receivable", acct.Name)
	}
}

func TestIsolation_SoftwareDevCoAHasUnbilledRevenue(t *testing.T) {
	t.Parallel()
	var found bool
	for _, acct := range SoftwareDevelopmentConfig().DefaultChartOfAccounts {
		if acct.Code == "1200" && acct.Name == "Unbilled Revenue (WIP)" {
			found = true
		}
	}
	assert.True(t, found, "software-dev should have Unbilled Revenue")
}

func TestIsolation_MovieDoesNotHaveUnbilledRevenue(t *testing.T) {
	t.Parallel()
	for _, acct := range MovieProductionConfig().DefaultChartOfAccounts {
		assert.NotEqual(t, "Unbilled Revenue (WIP)", acct.Name)
	}
}

func TestIsolation_EventsCantUseHeavyMachinery(t *testing.T) {
	t.Parallel()
	svc := inventory.NewService(&inventoryMock{status: "available"})
	err := svc.CreateAsset(ctxWith(EventsManagementConfig()), &inventory.Asset{
		TenantID: uuid.New(), CategoryID: "heavy_machinery", Name: "Crane",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, inventory.ErrInvalidCategory)
}

func TestIsolation_ConstructionCantUseAVEquipment(t *testing.T) {
	t.Parallel()
	svc := inventory.NewService(&inventoryMock{status: "available"})
	err := svc.CreateAsset(ctxWith(ConstructionConfig()), &inventory.Asset{
		TenantID: uuid.New(), CategoryID: "av_equipment", Name: "Speaker",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, inventory.ErrInvalidCategory)
}

func TestIsolation_EventReportNotForConstruction(t *testing.T) {
	t.Parallel()
	svc := reporting.NewService(&reportingMock{})
	_, err := svc.GetReportDefinition(ctxWith(ConstructionConfig()), "event_pl")
	require.Error(t, err)
	assert.ErrorIs(t, err, reporting.ErrReportNotFound)
}

func TestIsolation_BOQReportNotForEvents(t *testing.T) {
	t.Parallel()
	svc := reporting.NewService(&reportingMock{})
	_, err := svc.GetReportDefinition(ctxWith(EventsManagementConfig()), "boq_variance")
	require.Error(t, err)
	assert.ErrorIs(t, err, reporting.ErrReportNotFound)
}

func TestIsolation_DifferentApprovalLimitsPerVertical(t *testing.T) {
	t.Parallel()
	eventsLimit := EventsManagementConfig().ApprovalWorkflow.LimitForRole("accountant")
	constructionLimit := ConstructionConfig().ApprovalWorkflow.LimitForRole("accountant")
	require.NotNil(t, eventsLimit)
	require.NotNil(t, constructionLimit)
	assert.False(t, eventsLimit.Equal(*constructionLimit))
}

func TestConcurrency_NoConfigBleed(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	errors := make([]error, len(allVerticals))
	for i, tc := range allVerticals {
		i, tc := i, tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc := project.NewService(&projectMock{})
			errors[i] = svc.CreatePhase(ctxWith(tc.config), &project.Phase{
				ProductionID: uuid.New(), TenantID: uuid.New(), PhaseType: tc.config.PhaseTypes[0].ID,
			})
		}()
	}
	wg.Wait()
	for i, err := range errors {
		assert.NoError(t, err, "goroutine %d should succeed", i)
	}
}

func TestConcurrency_CrossVerticalRejection(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	var movieErr, softErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		svc := budget.NewService(&budgetMock{})
		movieErr = svc.CreateLineItem(ctxWith(MovieProductionConfig()), &budget.BudgetLineItem{
			BudgetID: uuid.New(), TenantID: uuid.New(), CategoryID: "cloud_infrastructure",
		})
	}()
	go func() {
		defer wg.Done()
		svc := budget.NewService(&budgetMock{})
		softErr = svc.CreateLineItem(ctxWith(SoftwareDevelopmentConfig()), &budget.BudgetLineItem{
			BudgetID: uuid.New(), TenantID: uuid.New(), CategoryID: "above_the_line",
		})
	}()
	wg.Wait()
	assert.Error(t, movieErr)
	assert.Error(t, softErr)
}
