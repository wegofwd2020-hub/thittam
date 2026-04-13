package vertical

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_JSONRoundTrip(t *testing.T) {
	depYears := 5
	cfg := &Config{
		ID:          "movie-production",
		Name:        "Movie & Film Production",
		Version:     "1.0.0",
		Description: "For film production companies",
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
		PhaseTypes: []PhaseType{
			{ID: "pre_production", Label: "Pre-Production", Order: 1, IsBillable: false, AllowedTransitions: []string{"production"}},
			{ID: "production", Label: "Production", Order: 2, IsBillable: true, AllowedTransitions: []string{"post_production"}},
		},
		BudgetCategories: []BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", Description: "Key talent", DefaultAccountCode: "5100"},
		},
		BudgetTemplates: []BudgetTemplate{
			{
				Name:        "Feature Film",
				Description: "Standard feature film budget",
				LineItems: []TemplateLineItem{
					{CategoryID: "above_the_line", Description: "Director", AccountCode: "5100", DefaultAmount: decimal.NewFromInt(500000), IsRequired: true},
				},
			},
		},
		ExpenseCategories: []ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: ApprovalWorkflow{
			Limits: []ApprovalLimit{
				{Role: "coordinator", MaxAmount: decimal.NewFromInt(200000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true, DepreciationYears: &depYears},
			{ID: "props", Label: "Props", IsTrackable: false, DepreciationYears: nil},
		},
		ReportDefinitions: []ReportDefinition{
			{ID: "cost_report", Name: "Cost Report", Description: "Daily cost report", DataSources: []string{"budget-planning", "expense-tracking"}, DefaultColumns: []string{"category", "budget", "actual", "variance"}},
		},
		CustomFields: CustomFields{
			Project: []CustomField{
				{Key: "call_sheet_time", Label: "Call Sheet Time", Type: "text", Required: false},
			},
			Expense: []CustomField{
				{Key: "vendor_gstin", Label: "Vendor GSTIN", Type: "text", Required: true},
			},
		},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	var decoded Config
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, cfg.ID, decoded.ID)
	assert.Equal(t, cfg.EntityLabels, decoded.EntityLabels)
	assert.Equal(t, len(cfg.PhaseTypes), len(decoded.PhaseTypes))
	assert.Equal(t, cfg.PhaseTypes[0].AllowedTransitions, decoded.PhaseTypes[0].AllowedTransitions)
	assert.True(t, cfg.ApprovalWorkflow.DualApprovalAbove.Equal(decoded.ApprovalWorkflow.DualApprovalAbove))
	assert.True(t, cfg.BudgetTemplates[0].LineItems[0].DefaultAmount.Equal(decoded.BudgetTemplates[0].LineItems[0].DefaultAmount))

	// DepreciationYears pointer behavior
	assert.NotNil(t, decoded.InventoryCategories[0].DepreciationYears)
	assert.Equal(t, 5, *decoded.InventoryCategories[0].DepreciationYears)
	assert.Nil(t, decoded.InventoryCategories[1].DepreciationYears)
}
