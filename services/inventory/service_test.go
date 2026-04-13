package inventory

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Mock repository ---

type mockRepo struct {
	createAssetFn       func(ctx context.Context, a *Asset) error
	getAssetFn          func(ctx context.Context, tenantID, id uuid.UUID) (*Asset, error)
	listAssetsFn        func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Asset, error)
	updateAssetStatusFn func(ctx context.Context, tenantID, id uuid.UUID, status string) error
	checkOutAssetFn     func(ctx context.Context, c *AssetCheckout) error
	checkInAssetFn      func(ctx context.Context, checkoutID uuid.UUID, conditionIn string) error
	getCheckoutFn       func(ctx context.Context, id uuid.UUID) (*AssetCheckout, error)
	listCheckoutsFn     func(ctx context.Context, assetID uuid.UUID) ([]AssetCheckout, error)
}

func (m *mockRepo) CreateAsset(ctx context.Context, a *Asset) error {
	if m.createAssetFn != nil {
		return m.createAssetFn(ctx, a)
	}
	return nil
}
func (m *mockRepo) GetAsset(ctx context.Context, tenantID, id uuid.UUID) (*Asset, error) {
	if m.getAssetFn != nil {
		return m.getAssetFn(ctx, tenantID, id)
	}
	return &Asset{ID: id, TenantID: tenantID, Name: "Test Asset", Status: "available"}, nil
}
func (m *mockRepo) ListAssets(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Asset, error) {
	if m.listAssetsFn != nil {
		return m.listAssetsFn(ctx, tenantID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateAssetStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	if m.updateAssetStatusFn != nil {
		return m.updateAssetStatusFn(ctx, tenantID, id, status)
	}
	return nil
}
func (m *mockRepo) CheckOutAsset(ctx context.Context, c *AssetCheckout) error {
	if m.checkOutAssetFn != nil {
		return m.checkOutAssetFn(ctx, c)
	}
	return nil
}
func (m *mockRepo) CheckInAsset(ctx context.Context, checkoutID uuid.UUID, conditionIn string) error {
	if m.checkInAssetFn != nil {
		return m.checkInAssetFn(ctx, checkoutID, conditionIn)
	}
	return nil
}
func (m *mockRepo) GetCheckout(ctx context.Context, id uuid.UUID) (*AssetCheckout, error) {
	if m.getCheckoutFn != nil {
		return m.getCheckoutFn(ctx, id)
	}
	return &AssetCheckout{ID: id}, nil
}
func (m *mockRepo) ListCheckouts(ctx context.Context, assetID uuid.UUID) ([]AssetCheckout, error) {
	if m.listCheckoutsFn != nil {
		return m.listCheckoutsFn(ctx, assetID)
	}
	return nil, nil
}

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
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "coordinator", MaxAmount: decimal.NewFromInt(200000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
			{ID: "lighting", Label: "Lighting Equipment", IsTrackable: true},
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

func TestCreateAsset_ValidCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	asset := &Asset{
		TenantID:      uuid.New(),
		AssetCode:     "CAM-001",
		Name:          "RED V-Raptor",
		CategoryID:    "camera",
		OwnershipType: "owned",
	}
	err := svc.CreateAsset(ctxWithVertical(), asset)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, asset.ID)
}

func TestCreateAsset_InvalidCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	asset := &Asset{
		TenantID:      uuid.New(),
		AssetCode:     "VEH-001",
		Name:          "Grip Truck",
		CategoryID:    "vehicles", // not in movie-production vertical
		OwnershipType: "rented",
	}
	err := svc.CreateAsset(ctxWithVertical(), asset)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCategory)
}

func TestCheckOutAsset_Available(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	assetID := uuid.New()

	svc := NewService(&mockRepo{
		getAssetFn: func(ctx context.Context, tid, id uuid.UUID) (*Asset, error) {
			return &Asset{ID: id, TenantID: tid, Status: "available"}, nil
		},
	})

	checkout := &AssetCheckout{
		AssetID:      assetID,
		ProductionID: uuid.New(),
		TenantID:     tenantID,
		CheckedOutTo: uuid.New(),
		ConditionOut: "good",
	}
	err := svc.CheckOutAsset(context.Background(), checkout)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, checkout.ID)
}

func TestCheckOutAsset_AlreadyCheckedOut(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	assetID := uuid.New()

	svc := NewService(&mockRepo{
		getAssetFn: func(ctx context.Context, tid, id uuid.UUID) (*Asset, error) {
			return &Asset{ID: id, TenantID: tid, Status: "checked_out"}, nil
		},
	})

	checkout := &AssetCheckout{
		AssetID:      assetID,
		ProductionID: uuid.New(),
		TenantID:     tenantID,
		CheckedOutTo: uuid.New(),
	}
	err := svc.CheckOutAsset(context.Background(), checkout)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAssetNotAvailable)
}

func TestCheckInAsset(t *testing.T) {
	t.Parallel()
	var updatedStatus string
	tenantID := uuid.New()
	assetID := uuid.New()
	checkoutID := uuid.New()

	svc := NewService(&mockRepo{
		updateAssetStatusFn: func(ctx context.Context, tid, id uuid.UUID, status string) error {
			updatedStatus = status
			return nil
		},
	})

	err := svc.CheckInAsset(context.Background(), tenantID, checkoutID, assetID, "good")
	require.NoError(t, err)
	assert.Equal(t, "available", updatedStatus)
}

func TestGetInventoryCategories(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	cats := svc.GetInventoryCategories(ctxWithVertical())
	assert.Len(t, cats, 2)
	assert.Equal(t, "camera", cats[0].ID)
	assert.Equal(t, "lighting", cats[1].ID)
}

// --- Additional coverage tests ---

func TestGetAsset_Success(t *testing.T) {
	t.Parallel()
	assetID := uuid.New()
	svc := NewService(&mockRepo{
		getAssetFn: func(_ context.Context, tenantID, id uuid.UUID) (*Asset, error) {
			return &Asset{ID: id, TenantID: tenantID, Status: "available"}, nil
		},
	})

	a, err := svc.GetAsset(context.Background(), uuid.New(), assetID)
	require.NoError(t, err)
	assert.Equal(t, assetID, a.ID)
}

func TestListAssets_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _ string, limit, _ int) ([]Asset, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListAssets(context.Background(), uuid.New(), "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListAssets_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listAssetsFn: func(_ context.Context, _ uuid.UUID, _ string, limit, _ int) ([]Asset, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListAssets(context.Background(), uuid.New(), "", 9999, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListCheckouts_Success(t *testing.T) {
	t.Parallel()
	assetID := uuid.New()
	svc := NewService(&mockRepo{
		listCheckoutsFn: func(_ context.Context, id uuid.UUID) ([]AssetCheckout, error) {
			return []AssetCheckout{{ID: uuid.New(), AssetID: id}}, nil
		},
	})

	checkouts, err := svc.ListCheckouts(context.Background(), assetID)
	require.NoError(t, err)
	assert.Len(t, checkouts, 1)
}

func TestCheckInAsset_Error(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		checkInAssetFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			return fmt.Errorf("db error")
		},
	})

	err := svc.CheckInAsset(context.Background(), uuid.New(), uuid.New(), uuid.New(), "good")
	require.Error(t, err)
}
