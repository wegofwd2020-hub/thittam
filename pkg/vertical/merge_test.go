package vertical

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseConfig() *Config {
	return &Config{
		ID:      "movie-production",
		Name:    "Movie & Film Production",
		Version: "1.0.0",
		EntityLabels: EntityLabels{
			Project:          "Production",
			ProjectPlural:    "Productions",
			Phase:            "Phase",
			PhasePlural:      "Phases",
			TeamMember:       "Crew Member",
			TeamMemberPlural: "Crew Members",
			RateLabel:        "Day Rate",
			RateUnit:         "day",
		},
		BudgetCategories: []BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
			{ID: "below_the_line", Label: "Below the Line", DefaultAccountCode: "5200"},
		},
		ExpenseCategories: []ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
			{ID: "equipment_rental", Label: "Equipment Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		InventoryCategories: []InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
			{ID: "props", Label: "Props", IsTrackable: false},
		},
		PhaseTypes: []PhaseType{
			{ID: "development", Label: "Development", Order: 1, AllowedTransitions: []string{"pre_production"}},
			{ID: "pre_production", Label: "Pre-Production", Order: 2, AllowedTransitions: []string{"production"}},
			{ID: "production", Label: "Production", Order: 3, AllowedTransitions: []string{}},
		},
		ReportDefinitions: []ReportDefinition{
			{ID: "cost_report", Name: "Daily Cost Report"},
		},
		ApprovalWorkflow: ApprovalWorkflow{
			Limits: []ApprovalLimit{
				{Role: "coordinator", MaxAmount: decimal.NewFromInt(200000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
	}
}

func TestApplyOverride_NilOverride(t *testing.T) {
	t.Parallel()
	result, err := ApplyOverride(baseConfig(), nil)
	require.NoError(t, err)
	assert.Equal(t, "Production", result.EntityLabels.Project)
	assert.Len(t, result.BudgetCategories, 2)
}

func TestApplyOverride_EmptyOverride(t *testing.T) {
	t.Parallel()
	result, err := ApplyOverride(baseConfig(), []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "Production", result.EntityLabels.Project)
}

func TestApplyOverride_EntityLabelOverride(t *testing.T) {
	t.Parallel()
	override := `{
		"entity_labels": {
			"override": {
				"project": "Engagement",
				"team_member": "Consultant"
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Equal(t, "Engagement", result.EntityLabels.Project)
	assert.Equal(t, "Consultant", result.EntityLabels.TeamMember)
	// Unchanged fields preserved
	assert.Equal(t, "Productions", result.EntityLabels.ProjectPlural)
	assert.Equal(t, "Phase", result.EntityLabels.Phase)
	assert.Equal(t, "day", result.EntityLabels.RateUnit)
}

func TestApplyOverride_AppendBudgetCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"budget_categories": {
			"append": [
				{"id": "rd_grant", "label": "R&D Grant Funded", "default_account_code": "6900"}
			]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.BudgetCategories, 3)
	assert.Equal(t, "rd_grant", result.BudgetCategories[2].ID)
	assert.Equal(t, "R&D Grant Funded", result.BudgetCategories[2].Label)
}

func TestApplyOverride_OverrideBudgetCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"budget_categories": {
			"override": {
				"above_the_line": {"id": "above_the_line", "label": "Key Talent", "default_account_code": "5100"}
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.BudgetCategories, 2)
	assert.Equal(t, "Key Talent", result.BudgetCategories[0].Label) // overridden
	assert.Equal(t, "Below the Line", result.BudgetCategories[1].Label) // unchanged
}

func TestApplyOverride_RemoveBudgetCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"budget_categories": {
			"remove": ["below_the_line"]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.BudgetCategories, 1)
	assert.Equal(t, "above_the_line", result.BudgetCategories[0].ID)
}

func TestApplyOverride_CombinedOperations(t *testing.T) {
	t.Parallel()
	override := `{
		"entity_labels": {
			"override": {"project": "Film"}
		},
		"budget_categories": {
			"remove": ["below_the_line"],
			"append": [
				{"id": "vfx", "label": "Visual Effects", "default_account_code": "5400"}
			]
		},
		"expense_categories": {
			"override": {
				"location_rental": {"id": "location_rental", "label": "Venue Rental", "tax_treatment": "input_gst", "default_account_code": "5200", "requires_po": true}
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)

	assert.Equal(t, "Film", result.EntityLabels.Project)
	assert.Len(t, result.BudgetCategories, 2) // removed 1, appended 1
	assert.Equal(t, "above_the_line", result.BudgetCategories[0].ID)
	assert.Equal(t, "vfx", result.BudgetCategories[1].ID)
	assert.Equal(t, "Venue Rental", result.ExpenseCategories[0].Label)
}

func TestApplyOverride_PhaseTypeAppend(t *testing.T) {
	t.Parallel()
	override := `{
		"phase_types": {
			"append": [
				{"id": "on_hold", "label": "On Hold", "order": 10, "is_billable": false, "allowed_transitions": ["development"]}
			]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.PhaseTypes, 4)
	assert.Equal(t, "on_hold", result.PhaseTypes[3].ID)
}

func TestApplyOverride_InventoryCategoryRemove(t *testing.T) {
	t.Parallel()
	override := `{
		"inventory_categories": {
			"remove": ["props"]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.InventoryCategories, 1)
	assert.Equal(t, "camera", result.InventoryCategories[0].ID)
}

func TestApplyOverride_ReportDefinitionAppend(t *testing.T) {
	t.Parallel()
	override := `{
		"report_definitions": {
			"append": [
				{"id": "custom_report", "name": "Custom Enterprise Report", "description": "Bespoke"}
			]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.ReportDefinitions, 2)
	assert.Equal(t, "custom_report", result.ReportDefinitions[1].ID)
}

func TestApplyOverride_DoesNotMutateBase(t *testing.T) {
	t.Parallel()
	base := baseConfig()
	override := `{
		"entity_labels": {"override": {"project": "Changed"}},
		"budget_categories": {"remove": ["above_the_line"]}
	}`

	result, err := ApplyOverride(base, []byte(override))
	require.NoError(t, err)

	// Result is changed
	assert.Equal(t, "Changed", result.EntityLabels.Project)
	assert.Len(t, result.BudgetCategories, 1)

	// Base is NOT changed
	assert.Equal(t, "Production", base.EntityLabels.Project)
	assert.Len(t, base.BudgetCategories, 2)
}

func TestApplyOverride_FlatMergeFallback(t *testing.T) {
	t.Parallel()
	// Non-structured override: directly sets top-level fields (backward compat
	// with PostgreSQL || shallow merge where entity_labels is fully replaced)
	override := `{"id": "custom-movie", "name": "Custom Movie Production"}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	// Flat merge replaces top-level keys
	assert.Equal(t, "custom-movie", result.ID)
	assert.Equal(t, "Custom Movie Production", result.Name)
	// Other fields preserved
	assert.Equal(t, "Production", result.EntityLabels.Project)
}

func TestApplyOverride_InvalidJSON(t *testing.T) {
	t.Parallel()
	result, err := ApplyOverride(baseConfig(), []byte(`not json`))
	require.NoError(t, err) // falls back to base unchanged
	assert.Equal(t, "Production", result.EntityLabels.Project)
}

func TestDeepCopyConfig(t *testing.T) {
	t.Parallel()
	original := baseConfig()
	copy := deepCopyConfig(original)

	copy.EntityLabels.Project = "Modified"
	copy.BudgetCategories = append(copy.BudgetCategories, BudgetCategory{ID: "new"})

	// Original unchanged
	assert.Equal(t, "Production", original.EntityLabels.Project)
	assert.Len(t, original.BudgetCategories, 2)
}

// ─── ExpenseCategory missing branches ────────────────────────────────────────

func TestApplyOverride_AppendExpenseCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"expense_categories": {
			"append": [
				{"id": "catering", "label": "Catering", "tax_treatment": "input_gst", "default_account_code": "5300", "requires_po": false}
			]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.ExpenseCategories, 3)
	assert.Equal(t, "catering", result.ExpenseCategories[2].ID)
	assert.Equal(t, "Catering", result.ExpenseCategories[2].Label)
}

func TestApplyOverride_RemoveExpenseCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"expense_categories": {
			"remove": ["equipment_rental"]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.ExpenseCategories, 1)
	assert.Equal(t, "location_rental", result.ExpenseCategories[0].ID)
}

// ─── InventoryCategory missing branches ──────────────────────────────────────

func TestApplyOverride_AppendInventoryCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"inventory_categories": {
			"append": [
				{"id": "lighting", "label": "Lighting Rigs", "is_trackable": true}
			]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.InventoryCategories, 3)
	assert.Equal(t, "lighting", result.InventoryCategories[2].ID)
	assert.True(t, result.InventoryCategories[2].IsTrackable)
}

func TestApplyOverride_OverrideInventoryCategory(t *testing.T) {
	t.Parallel()
	override := `{
		"inventory_categories": {
			"override": {
				"props": {"id": "props", "label": "Set Dressing Props", "is_trackable": true}
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.InventoryCategories, 2)
	assert.Equal(t, "Set Dressing Props", result.InventoryCategories[1].Label)
	assert.True(t, result.InventoryCategories[1].IsTrackable)
	// Unchanged item preserved
	assert.Equal(t, "Camera Equipment", result.InventoryCategories[0].Label)
}

// ─── PhaseType missing branches ──────────────────────────────────────────────

func TestApplyOverride_OverridePhaseType(t *testing.T) {
	t.Parallel()
	override := `{
		"phase_types": {
			"override": {
				"development": {"id": "development", "label": "Principal Photography", "order": 2, "allowed_transitions": ["post_production"]}
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.PhaseTypes, 3)
	assert.Equal(t, "Principal Photography", result.PhaseTypes[0].Label)
	assert.Equal(t, []string{"post_production"}, result.PhaseTypes[0].AllowedTransitions)
	// Unchanged phases preserved
	assert.Equal(t, "pre_production", result.PhaseTypes[1].ID)
}

func TestApplyOverride_RemovePhaseType(t *testing.T) {
	t.Parallel()
	override := `{
		"phase_types": {
			"remove": ["pre_production", "production"]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.PhaseTypes, 1)
	assert.Equal(t, "development", result.PhaseTypes[0].ID)
}

// ─── ReportDefinition missing branches ───────────────────────────────────────

func TestApplyOverride_OverrideReportDefinition(t *testing.T) {
	t.Parallel()
	override := `{
		"report_definitions": {
			"override": {
				"cost_report": {"id": "cost_report", "name": "Weekly Cost Summary", "description": "Summarised weekly"}
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Len(t, result.ReportDefinitions, 1)
	assert.Equal(t, "Weekly Cost Summary", result.ReportDefinitions[0].Name)
}

func TestApplyOverride_RemoveReportDefinition(t *testing.T) {
	t.Parallel()
	override := `{
		"report_definitions": {
			"remove": ["cost_report"]
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Empty(t, result.ReportDefinitions)
}

// ─── EntityLabels — remaining 4 fields ───────────────────────────────────────

func TestApplyOverride_EntityLabelOverride_AllRemainingFields(t *testing.T) {
	t.Parallel()
	override := `{
		"entity_labels": {
			"override": {
				"phase_plural":       "Acts",
				"team_member_plural": "Freelancers",
				"rate_label":         "Hourly Rate",
				"rate_unit":          "hour"
			}
		}
	}`

	result, err := ApplyOverride(baseConfig(), []byte(override))
	require.NoError(t, err)
	assert.Equal(t, "Acts", result.EntityLabels.PhasePlural)
	assert.Equal(t, "Freelancers", result.EntityLabels.TeamMemberPlural)
	assert.Equal(t, "Hourly Rate", result.EntityLabels.RateLabel)
	assert.Equal(t, "hour", result.EntityLabels.RateUnit)
	// Untouched fields remain
	assert.Equal(t, "Production", result.EntityLabels.Project)
	assert.Equal(t, "Productions", result.EntityLabels.ProjectPlural)
	assert.Equal(t, "Phase", result.EntityLabels.Phase)
	assert.Equal(t, "Crew Member", result.EntityLabels.TeamMember)
}

func TestOverride_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	override := Override{
		EntityLabels: &OverrideMap{
			Override: map[string]interface{}{
				"project": "Film",
			},
		},
		BudgetCategories: &ArrayOverride{
			Append: []json.RawMessage{
				json.RawMessage(`{"id":"new_cat","label":"New Category"}`),
			},
			Remove: []string{"old_cat"},
		},
	}

	data, err := json.Marshal(override)
	require.NoError(t, err)

	var decoded Override
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.NotNil(t, decoded.EntityLabels)
	assert.NotNil(t, decoded.BudgetCategories)
	assert.Len(t, decoded.BudgetCategories.Append, 1)
	assert.Equal(t, []string{"old_cat"}, decoded.BudgetCategories.Remove)
}
