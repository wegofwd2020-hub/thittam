package vertical

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func testConfig() *Config {
	return &Config{
		ID: "test-vertical",
		PhaseTypes: []PhaseType{
			{ID: "design", Label: "Design", Order: 1, AllowedTransitions: []string{"development", "review"}},
			{ID: "development", Label: "Development", Order: 2, AllowedTransitions: []string{"review"}},
			{ID: "review", Label: "Review", Order: 3, AllowedTransitions: []string{"archived"}},
			{ID: "archived", Label: "Archived", Order: 4, AllowedTransitions: []string{}},
		},
		BudgetCategories: []BudgetCategory{
			{ID: "personnel", Label: "Personnel", DefaultAccountCode: "5100"},
			{ID: "infrastructure", Label: "Infrastructure", DefaultAccountCode: "5200"},
		},
		BudgetTemplates: []BudgetTemplate{
			{Name: "Standard Project", Description: "Default template"},
			{Name: "Fixed-Price", Description: "Fixed-price template"},
		},
		ExpenseCategories: []ExpenseCategory{
			{ID: "cloud", Label: "Cloud Infrastructure", TaxTreatment: "input_gst", DefaultAccountCode: "5300", RequiresPO: true},
		},
		ApprovalWorkflow: ApprovalWorkflow{
			Limits: []ApprovalLimit{
				{Role: "manager", MaxAmount: decimal.NewFromInt(100000)},
				{Role: "director", MaxAmount: decimal.NewFromInt(500000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		ReportDefinitions: []ReportDefinition{
			{ID: "burn_rate", Name: "Burn Rate", DataSources: []string{"budget-planning"}},
		},
	}
}

func TestFindBudgetCategory(t *testing.T) {
	cfg := testConfig()

	tests := []struct {
		name   string
		id     string
		wantID string
		wantOK bool
	}{
		{"found", "personnel", "personnel", true},
		{"found second", "infrastructure", "infrastructure", true},
		{"not found", "nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.FindBudgetCategory(tt.id)
			if tt.wantOK {
				assert.NotNil(t, got)
				assert.Equal(t, tt.wantID, got.ID)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestFindBudgetCategory_EmptySlice(t *testing.T) {
	cfg := &Config{}
	assert.Nil(t, cfg.FindBudgetCategory("anything"))
}

func TestBudgetCategoryIDs(t *testing.T) {
	tests := []struct {
		name       string
		categories []BudgetCategory
		want       string
	}{
		{"multiple", []BudgetCategory{{ID: "a"}, {ID: "b"}, {ID: "c"}}, "a, b, c"},
		{"single", []BudgetCategory{{ID: "only"}}, "only"},
		{"empty", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{BudgetCategories: tt.categories}
			assert.Equal(t, tt.want, cfg.BudgetCategoryIDs())
		})
	}
}

func TestFindPhaseType(t *testing.T) {
	cfg := testConfig()

	got := cfg.FindPhaseType("development")
	assert.NotNil(t, got)
	assert.Equal(t, "Development", got.Label)

	assert.Nil(t, cfg.FindPhaseType("nonexistent"))
}

func TestCanTransitionTo(t *testing.T) {
	cfg := testConfig()
	design := cfg.FindPhaseType("design")
	archived := cfg.FindPhaseType("archived")

	tests := []struct {
		name   string
		phase  *PhaseType
		target string
		want   bool
	}{
		{"allowed", design, "development", true},
		{"allowed second", design, "review", true},
		{"not allowed", design, "archived", false},
		{"empty transitions", archived, "design", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.phase.CanTransitionTo(tt.target))
		})
	}
}

func TestFindExpenseCategory(t *testing.T) {
	cfg := testConfig()

	got := cfg.FindExpenseCategory("cloud")
	assert.NotNil(t, got)
	assert.Equal(t, "Cloud Infrastructure", got.Label)
	assert.True(t, got.RequiresPO)

	assert.Nil(t, cfg.FindExpenseCategory("nonexistent"))
}

func TestLimitForRole(t *testing.T) {
	cfg := testConfig()

	tests := []struct {
		name    string
		role    string
		wantAmt *int64
	}{
		{"manager", "manager", int64Ptr(100000)},
		{"director", "director", int64Ptr(500000)},
		{"no limit", "intern", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.ApprovalWorkflow.LimitForRole(tt.role)
			if tt.wantAmt == nil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.True(t, decimal.NewFromInt(*tt.wantAmt).Equal(*got))
			}
		})
	}
}

func TestLimitForRole_EmptyLimits(t *testing.T) {
	wf := &ApprovalWorkflow{}
	assert.Nil(t, wf.LimitForRole("any"))
}

func TestFindBudgetTemplate(t *testing.T) {
	cfg := testConfig()

	got := cfg.FindBudgetTemplate("Standard Project")
	assert.NotNil(t, got)
	assert.Equal(t, "Default template", got.Description)

	assert.Nil(t, cfg.FindBudgetTemplate("nonexistent"))
}

func TestFindInventoryCategory(t *testing.T) {
	cfg := &Config{
		InventoryCategories: []InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
			{ID: "props", Label: "Props", IsTrackable: false},
		},
	}

	got := cfg.FindInventoryCategory("camera")
	assert.NotNil(t, got)
	assert.Equal(t, "Camera Equipment", got.Label)
	assert.True(t, got.IsTrackable)

	got = cfg.FindInventoryCategory("props")
	assert.NotNil(t, got)
	assert.Equal(t, "Props", got.Label)

	assert.Nil(t, cfg.FindInventoryCategory("nonexistent"))
}

func TestFindInventoryCategory_EmptySlice(t *testing.T) {
	cfg := &Config{}
	assert.Nil(t, cfg.FindInventoryCategory("anything"))
}

func TestFindReportDefinition(t *testing.T) {
	cfg := testConfig()

	got := cfg.FindReportDefinition("burn_rate")
	assert.NotNil(t, got)
	assert.Equal(t, "Burn Rate", got.Name)

	assert.Nil(t, cfg.FindReportDefinition("nonexistent"))
}

func int64Ptr(v int64) *int64 { return &v }

func TestRoleLabelFor_ReturnsConfiguredLabel(t *testing.T) {
	cfg := &Config{
		RoleLabels: map[string]string{
			"manager":     "Executive Producer",
			"coordinator": "Line Producer",
		},
	}
	assert.Equal(t, "Executive Producer", cfg.RoleLabelFor("manager"))
	assert.Equal(t, "Line Producer", cfg.RoleLabelFor("coordinator"))
}

func TestRoleLabelFor_FallsBackToInternalName(t *testing.T) {
	cfg := &Config{RoleLabels: map[string]string{"manager": "Boss"}}
	// Missing key falls back to the internal role name without panic.
	assert.Equal(t, "accountant", cfg.RoleLabelFor("accountant"))
	// Empty value also falls back.
	cfg.RoleLabels["empty"] = ""
	assert.Equal(t, "empty", cfg.RoleLabelFor("empty"))
}

func TestRoleLabelFor_NilMap(t *testing.T) {
	cfg := &Config{}
	assert.Equal(t, "manager", cfg.RoleLabelFor("manager"))
}
