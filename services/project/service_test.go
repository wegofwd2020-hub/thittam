package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Mock repository ---

type mockRepo struct {
	createProductionFn  func(ctx context.Context, p *Production) error
	getProductionFn     func(ctx context.Context, tenantID, id uuid.UUID) (*Production, error)
	listProductionsFn   func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Production, error)
	createPhaseFn       func(ctx context.Context, p *Phase) error
	getPhaseFn          func(ctx context.Context, id uuid.UUID) (*Phase, error)
	updatePhaseStatusFn func(ctx context.Context, id uuid.UUID, status string) error
	addCrewFn           func(ctx context.Context, c *CrewMember) error
	listCrewFn          func(ctx context.Context, prodID uuid.UUID) ([]CrewMember, error)
}

func (m *mockRepo) CreateProduction(ctx context.Context, p *Production) error {
	if m.createProductionFn != nil {
		return m.createProductionFn(ctx, p)
	}
	return nil
}
func (m *mockRepo) GetProduction(ctx context.Context, tenantID, id uuid.UUID) (*Production, error) {
	if m.getProductionFn != nil {
		return m.getProductionFn(ctx, tenantID, id)
	}
	return &Production{ID: id, TenantID: tenantID, Title: "Test", Status: "production"}, nil
}
func (m *mockRepo) ListProductions(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Production, error) {
	if m.listProductionsFn != nil {
		return m.listProductionsFn(ctx, tenantID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateProduction(ctx context.Context, p *Production) error { return nil }
func (m *mockRepo) ArchiveProduction(ctx context.Context, tenantID, id uuid.UUID) error {
	return nil
}
func (m *mockRepo) CreatePhase(ctx context.Context, p *Phase) error {
	if m.createPhaseFn != nil {
		return m.createPhaseFn(ctx, p)
	}
	return nil
}
func (m *mockRepo) ListPhases(ctx context.Context, prodID uuid.UUID) ([]Phase, error) {
	return nil, nil
}
func (m *mockRepo) GetPhase(ctx context.Context, id uuid.UUID) (*Phase, error) {
	if m.getPhaseFn != nil {
		return m.getPhaseFn(ctx, id)
	}
	return &Phase{ID: id, PhaseType: "development", Status: "active"}, nil
}
func (m *mockRepo) UpdatePhaseStatus(ctx context.Context, id uuid.UUID, status string) error {
	if m.updatePhaseStatusFn != nil {
		return m.updatePhaseStatusFn(ctx, id, status)
	}
	return nil
}
func (m *mockRepo) AddCrewMember(ctx context.Context, c *CrewMember) error {
	if m.addCrewFn != nil {
		return m.addCrewFn(ctx, c)
	}
	return nil
}
func (m *mockRepo) ListCrewMembers(ctx context.Context, prodID uuid.UUID) ([]CrewMember, error) {
	if m.listCrewFn != nil {
		return m.listCrewFn(ctx, prodID)
	}
	return nil, nil
}
func (m *mockRepo) RemoveCrewMember(ctx context.Context, id uuid.UUID) error { return nil }

// --- Vertical config fixture (movie-production) ---

func movieProductionConfig() *vertical.Config {
	return &vertical.Config{
		ID:      "movie-production",
		Name:    "Movie & Film Production",
		Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project:          "Production",
			ProjectPlural:    "Productions",
			Phase:            "Phase",
			PhasePlural:      "Phases",
			TeamMember:       "Crew Member",
			TeamMemberPlural: "Crew Members",
			RateLabel:        "Day Rate",
			RateUnit:         "day",
		},
		PhaseTypes: []vertical.PhaseType{
			{ID: "development", Label: "Development", Order: 1, IsBillable: false, AllowedTransitions: []string{"pre_production"}},
			{ID: "pre_production", Label: "Pre-Production", Order: 2, IsBillable: false, AllowedTransitions: []string{"production"}},
			{ID: "production", Label: "Production", Order: 3, IsBillable: true, AllowedTransitions: []string{"post_production"}},
			{ID: "post_production", Label: "Post-Production", Order: 4, IsBillable: true, AllowedTransitions: []string{"released"}},
			{ID: "released", Label: "Released", Order: 5, IsBillable: false, AllowedTransitions: []string{}},
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
			{ID: "below_the_line", Label: "Below the Line", DefaultAccountCode: "5200"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(200000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(1000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "cost_report", Name: "Daily Cost Report"},
		},
	}
}

func ctxWithVertical() context.Context {
	return vertical.WithConfig(context.Background(), movieProductionConfig())
}

// --- Tests ---

func TestCreatePhase_ValidType(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	phase := &Phase{
		ProductionID: uuid.New(),
		TenantID:     uuid.New(),
		PhaseType:    "development",
	}
	err := svc.CreatePhase(ctxWithVertical(), phase)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, phase.ID)
}

func TestCreatePhase_InvalidType(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	phase := &Phase{
		ProductionID: uuid.New(),
		TenantID:     uuid.New(),
		PhaseType:    "sprint_planning", // not in movie-production vertical
	}
	err := svc.CreatePhase(ctxWithVertical(), phase)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPhaseType)
}

func TestUpdatePhaseStatus_ValidTransition(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getPhaseFn: func(ctx context.Context, id uuid.UUID) (*Phase, error) {
			return &Phase{ID: id, PhaseType: "development", Status: "active"}, nil
		},
	})

	err := svc.UpdatePhaseStatus(ctxWithVertical(), uuid.New(), "pre_production")
	require.NoError(t, err)
}

func TestUpdatePhaseStatus_InvalidTransition(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getPhaseFn: func(ctx context.Context, id uuid.UUID) (*Phase, error) {
			return &Phase{ID: id, PhaseType: "development", Status: "active"}, nil
		},
	})

	// development → post_production is not allowed (must go through pre_production first)
	err := svc.UpdatePhaseStatus(ctxWithVertical(), uuid.New(), "post_production")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestUpdatePhaseStatus_ReleaseToNothing(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getPhaseFn: func(ctx context.Context, id uuid.UUID) (*Phase, error) {
			return &Phase{ID: id, PhaseType: "released", Status: "active"}, nil
		},
	})

	// released has no allowed transitions
	err := svc.UpdatePhaseStatus(ctxWithVertical(), uuid.New(), "development")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestGetEntityLabels(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	labels := svc.GetEntityLabels(ctxWithVertical())
	assert.Equal(t, "Production", labels.Project)
	assert.Equal(t, "Crew Member", labels.TeamMember)
	assert.Equal(t, "day", labels.RateUnit)
}

func TestGetPhaseTypes(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	pts := svc.GetPhaseTypes(ctxWithVertical())
	assert.Len(t, pts, 5)
	assert.Equal(t, "development", pts[0].ID)
	assert.Equal(t, "released", pts[4].ID)
}

func TestCreateProduction_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	p := &Production{TenantID: uuid.New(), Title: "Test Film"}
	err := svc.CreateProduction(context.Background(), p)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, p.ID)
}

func TestListProductions_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listProductionsFn: func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Production, error) {
			capturedLimit = limit
			return nil, nil
		},
	})
	svc.ListProductions(context.Background(), uuid.New(), "", 0, 0)
	assert.Equal(t, 20, capturedLimit)
}
