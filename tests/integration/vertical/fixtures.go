// Package vertical_test provides integration tests for the Vertical Plugin System
// across all 4 registered verticals.
package vertical_test

import (
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// MovieProductionConfig returns the full vertical config for movie-production.
func MovieProductionConfig() *vertical.Config {
	return &vertical.Config{
		ID: "movie-production", Name: "Movie & Film Production", Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project: "Production", ProjectPlural: "Productions",
			Phase: "Phase", PhasePlural: "Phases",
			TeamMember: "Crew Member", TeamMemberPlural: "Crew Members",
			RateLabel: "Day Rate", RateUnit: "day",
		},
		PhaseTypes: []vertical.PhaseType{
			{ID: "development", Label: "Development", Order: 1, IsBillable: false, AllowedTransitions: []string{"pre_production"}},
			{ID: "pre_production", Label: "Pre-Production", Order: 2, IsBillable: false, AllowedTransitions: []string{"production"}},
			{ID: "production", Label: "Production", Order: 3, IsBillable: true, AllowedTransitions: []string{"post_production"}},
			{ID: "post_production", Label: "Post-Production", Order: 4, IsBillable: true, AllowedTransitions: []string{"released"}},
			{ID: "released", Label: "Released", Order: 5, IsBillable: false, AllowedTransitions: []string{}},
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", Description: "Key creative talent", DefaultAccountCode: "5100"},
			{ID: "below_the_line", Label: "Below the Line", Description: "Technical crew and production costs", DefaultAccountCode: "5200"},
			{ID: "post_production", Label: "Post-Production", Description: "Editing, VFX, sound", DefaultAccountCode: "5300"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
			{ID: "equipment_rental", Label: "Equipment Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(200000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(1000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true, DepreciationYears: intPtr(5)},
			{ID: "props", Label: "Props", IsTrackable: false},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "cost_report", Name: "Daily Cost Report", DataSources: []string{"budget-planning", "expense-tracking"}},
		},
		DefaultChartOfAccounts: []vertical.ChartOfAccountEntry{
			{Code: "1000", Name: "Cash and Bank", AccountType: "asset"},
			{Code: "5100", Name: "Above the Line Expenses", AccountType: "expense"},
			{Code: "5200", Name: "Below the Line Expenses", AccountType: "expense"},
			{Code: "5300", Name: "Post-Production Expenses", AccountType: "expense"},
		},
	}
}

// SoftwareDevelopmentConfig returns the full vertical config for software-development.
func SoftwareDevelopmentConfig() *vertical.Config {
	return &vertical.Config{
		ID: "software-development", Name: "Software Development Services", Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project: "Project", ProjectPlural: "Projects",
			Phase: "Milestone", PhasePlural: "Milestones",
			TeamMember: "Team Member", TeamMemberPlural: "Team Members",
			RateLabel: "Hourly Rate", RateUnit: "hour",
		},
		PhaseTypes: []vertical.PhaseType{
			{ID: "discovery", Label: "Discovery & Scoping", Order: 1, IsBillable: true, AllowedTransitions: []string{"design", "proposal", "cancelled"}},
			{ID: "proposal", Label: "Proposal / Estimation", Order: 2, IsBillable: false, AllowedTransitions: []string{"discovery", "design", "cancelled"}},
			{ID: "design", Label: "UX Design & Architecture", Order: 3, IsBillable: true, AllowedTransitions: []string{"development", "discovery"}},
			{ID: "development", Label: "Development (Sprint Cycle)", Order: 4, IsBillable: true, AllowedTransitions: []string{"qa", "design", "on_hold"}},
			{ID: "qa", Label: "Quality Assurance & Testing", Order: 5, IsBillable: true, AllowedTransitions: []string{"uat", "development"}},
			{ID: "uat", Label: "User Acceptance Testing", Order: 6, IsBillable: true, AllowedTransitions: []string{"go_live", "development", "qa"}},
			{ID: "go_live", Label: "Go-Live & Deployment", Order: 7, IsBillable: true, AllowedTransitions: []string{"support", "completed"}},
			{ID: "support", Label: "Post-Launch Support", Order: 8, IsBillable: true, AllowedTransitions: []string{"completed"}},
			{ID: "on_hold", Label: "On Hold", Order: 9, IsBillable: false, AllowedTransitions: []string{"development", "cancelled"}},
			{ID: "completed", Label: "Completed", Order: 10, IsBillable: false, AllowedTransitions: []string{"archived"}},
			{ID: "cancelled", Label: "Cancelled", Order: 11, IsBillable: false, AllowedTransitions: []string{"archived"}},
			{ID: "archived", Label: "Archived", Order: 12, IsBillable: false, AllowedTransitions: []string{}},
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "personnel", Label: "Personnel / Team Cost", DefaultAccountCode: "5000"},
			{ID: "cloud_infrastructure", Label: "Cloud Infrastructure", DefaultAccountCode: "6000"},
			{ID: "software_licenses", Label: "Software Licenses & Tools", DefaultAccountCode: "6100"},
			{ID: "contingency", Label: "Contingency", DefaultAccountCode: "8000"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "salary", Label: "Salary / Payroll", TaxTreatment: "tds_applicable", DefaultAccountCode: "5000", RequiresPO: false},
			{ID: "contractor_invoice", Label: "Contractor Invoice", TaxTreatment: "tds_applicable", DefaultAccountCode: "6600", RequiresPO: true},
			{ID: "cloud_bill", Label: "Cloud Infrastructure Bill", TaxTreatment: "input_gst", DefaultAccountCode: "6001", RequiresPO: false},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "department_head", MaxAmount: decimal.NewFromInt(50000)},
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(500000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(2000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(2000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "laptop", Label: "Laptop / Workstation", IsTrackable: true, DepreciationYears: intPtr(3)},
			{ID: "software_license", Label: "Perpetual Software License", IsTrackable: false, DepreciationYears: intPtr(3)},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "burn_rate", Name: "Project Burn Rate"},
			{ID: "project_pl", Name: "Project Profitability (P&L)"},
			{ID: "resource_utilisation", Name: "Resource Utilisation Report"},
		},
		DefaultChartOfAccounts: []vertical.ChartOfAccountEntry{
			{Code: "1000", Name: "Bank & Cash", AccountType: "asset"},
			{Code: "1200", Name: "Unbilled Revenue (WIP)", AccountType: "asset"},
			{Code: "5000", Name: "Personnel Costs", AccountType: "expense"},
		},
	}
}

// ConstructionConfig returns the full vertical config for construction.
func ConstructionConfig() *vertical.Config {
	return &vertical.Config{
		ID: "construction", Name: "Construction & Civil Engineering", Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project: "Contract", ProjectPlural: "Contracts",
			Phase: "Work Package", PhasePlural: "Work Packages",
			TeamMember: "Site Worker", TeamMemberPlural: "Site Workers",
			RateLabel: "Day Rate", RateUnit: "day",
		},
		PhaseTypes: []vertical.PhaseType{
			{ID: "tender", Label: "Tender & Estimation", Order: 1, AllowedTransitions: []string{"pre_construction", "cancelled"}},
			{ID: "pre_construction", Label: "Pre-Construction", Order: 2, AllowedTransitions: []string{"foundation", "on_hold"}},
			{ID: "foundation", Label: "Foundation & Earthworks", Order: 3, AllowedTransitions: []string{"structure", "pre_construction"}},
			{ID: "structure", Label: "Structural Works", Order: 4, AllowedTransitions: []string{"mep", "finishing", "on_hold"}},
			{ID: "mep", Label: "MEP", Order: 5, AllowedTransitions: []string{"finishing", "structure"}},
			{ID: "finishing", Label: "Finishing & Fit-Out", Order: 6, AllowedTransitions: []string{"handover", "mep"}},
			{ID: "handover", Label: "Handover", Order: 7, AllowedTransitions: []string{"defects_liability", "completed"}},
			{ID: "defects_liability", Label: "Defects Liability", Order: 8, AllowedTransitions: []string{"completed"}},
			{ID: "on_hold", Label: "On Hold", Order: 9, AllowedTransitions: []string{"pre_construction", "foundation", "structure", "cancelled"}},
			{ID: "completed", Label: "Completed", Order: 10, AllowedTransitions: []string{"archived"}},
			{ID: "cancelled", Label: "Cancelled", Order: 11, AllowedTransitions: []string{"archived"}},
			{ID: "archived", Label: "Archived", Order: 12, AllowedTransitions: []string{}},
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "labour", Label: "Labour", DefaultAccountCode: "5000"},
			{ID: "materials", Label: "Materials & Procurement", DefaultAccountCode: "6000"},
			{ID: "plant_equipment", Label: "Plant & Equipment", DefaultAccountCode: "6100"},
			{ID: "subcontractors", Label: "Subcontractors", DefaultAccountCode: "6200"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "labour_wages", Label: "Labour Wages", TaxTreatment: "tds_applicable", DefaultAccountCode: "5000", RequiresPO: false},
			{ID: "material_purchase", Label: "Material Purchase", TaxTreatment: "input_gst", DefaultAccountCode: "6000", RequiresPO: true},
			{ID: "subcontractor_bill", Label: "Subcontractor Bill", TaxTreatment: "tds_applicable", DefaultAccountCode: "6200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "department_head", MaxAmount: decimal.NewFromInt(50000)},
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(500000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(5000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(5000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "heavy_machinery", Label: "Heavy Machinery (Owned)", IsTrackable: true, DepreciationYears: intPtr(10)},
			{ID: "scaffolding", Label: "Scaffolding & Formwork", IsTrackable: true, DepreciationYears: intPtr(7)},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "boq_variance", Name: "BOQ Variance Report"},
			{ID: "progress_bill", Name: "Progress Billing Statement"},
			{ID: "retention_schedule", Name: "Retention Money Schedule"},
		},
		DefaultChartOfAccounts: []vertical.ChartOfAccountEntry{
			{Code: "1000", Name: "Bank & Cash", AccountType: "asset"},
			{Code: "1200", Name: "Retention Receivable", AccountType: "asset"},
			{Code: "5000", Name: "Labour Costs", AccountType: "expense"},
		},
	}
}

// EventsManagementConfig returns the full vertical config for events-management.
func EventsManagementConfig() *vertical.Config {
	return &vertical.Config{
		ID: "events-management", Name: "Events & Experiential Management", Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project: "Event", ProjectPlural: "Events",
			Phase: "Stage", PhasePlural: "Stages",
			TeamMember: "Event Staff", TeamMemberPlural: "Event Staff",
			RateLabel: "Day Rate", RateUnit: "day",
		},
		PhaseTypes: []vertical.PhaseType{
			{ID: "concept", Label: "Concept & Proposal", Order: 1, AllowedTransitions: []string{"planning", "cancelled"}},
			{ID: "planning", Label: "Planning & Design", Order: 2, AllowedTransitions: []string{"production", "concept", "on_hold"}},
			{ID: "production", Label: "Production & Procurement", Order: 3, AllowedTransitions: []string{"rehearsal", "pre_event", "planning", "on_hold"}},
			{ID: "rehearsal", Label: "Rehearsal", Order: 4, AllowedTransitions: []string{"pre_event", "production"}},
			{ID: "pre_event", Label: "Pre-Event Setup", Order: 5, AllowedTransitions: []string{"live", "rehearsal"}},
			{ID: "live", Label: "Live / On-Site", Order: 6, AllowedTransitions: []string{"wrap"}},
			{ID: "wrap", Label: "Wrap & Bump-Out", Order: 7, AllowedTransitions: []string{"post_event"}},
			{ID: "post_event", Label: "Post-Event", Order: 8, AllowedTransitions: []string{"completed"}},
			{ID: "on_hold", Label: "On Hold", Order: 9, AllowedTransitions: []string{"planning", "production", "cancelled"}},
			{ID: "completed", Label: "Completed", Order: 10, AllowedTransitions: []string{"archived"}},
			{ID: "cancelled", Label: "Cancelled", Order: 11, AllowedTransitions: []string{"archived"}},
			{ID: "archived", Label: "Archived", Order: 12, AllowedTransitions: []string{}},
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "venue", Label: "Venue & Space", DefaultAccountCode: "5000"},
			{ID: "av_production", Label: "AV & Technical Production", DefaultAccountCode: "5100"},
			{ID: "catering", Label: "Catering & Beverages", DefaultAccountCode: "5400"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "venue_payment", Label: "Venue Payment", TaxTreatment: "input_gst", DefaultAccountCode: "5001", RequiresPO: true},
			{ID: "talent_fee", Label: "Talent / Performer Fee", TaxTreatment: "tds_applicable", DefaultAccountCode: "5201", RequiresPO: true},
			{ID: "petty_cash", Label: "Petty Cash (On-Site)", TaxTreatment: "none", DefaultAccountCode: "6099", RequiresPO: false},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "department_head", MaxAmount: decimal.NewFromInt(25000)},
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(250000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(2000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(2000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "av_equipment", Label: "AV & Technical Equipment", IsTrackable: true, DepreciationYears: intPtr(5)},
			{ID: "furniture", Label: "Furniture & Seating", IsTrackable: true, DepreciationYears: intPtr(7)},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "event_pl", Name: "Event P&L"},
			{ID: "vendor_tracker", Name: "Vendor & Supplier Tracker"},
			{ID: "cost_per_head", Name: "Cost Per Head Report"},
		},
		DefaultChartOfAccounts: []vertical.ChartOfAccountEntry{
			{Code: "1000", Name: "Bank & Cash", AccountType: "asset"},
			{Code: "2100", Name: "Client Advances Received", AccountType: "liability"},
			{Code: "5000", Name: "Venue Costs", AccountType: "expense"},
		},
	}
}

func intPtr(n int) *int { return &n }
func strPtr(s string) *string { return &s }
