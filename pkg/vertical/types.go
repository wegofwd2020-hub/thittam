package vertical

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrNotFound is returned when a tenant has no registered vertical.
var ErrNotFound = errors.New("vertical: config not found for tenant")

// Config represents the full vertical configuration for a tenant.
type Config struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`

	EntityLabels        EntityLabels        `json:"entity_labels"`
	PhaseTypes          []PhaseType         `json:"phase_types"`
	BudgetCategories    []BudgetCategory    `json:"budget_categories"`
	BudgetTemplates     []BudgetTemplate    `json:"budget_templates"`
	ExpenseCategories   []ExpenseCategory   `json:"expense_categories"`
	ApprovalWorkflow    ApprovalWorkflow    `json:"approval_workflow"`
	InventoryCategories []InventoryCategory `json:"inventory_categories"`
	ReportDefinitions   []ReportDefinition  `json:"report_definitions"`
	CustomFields        CustomFields        `json:"custom_fields"`
}

type EntityLabels struct {
	Project          string `json:"project"`
	ProjectPlural    string `json:"project_plural"`
	Phase            string `json:"phase"`
	PhasePlural      string `json:"phase_plural"`
	TeamMember       string `json:"team_member"`
	TeamMemberPlural string `json:"team_member_plural"`
	RateLabel        string `json:"rate_label"`
	RateUnit         string `json:"rate_unit"` // "day" | "hour" | "month" | "fixed"
}

type PhaseType struct {
	ID                 string   `json:"id"`
	Label              string   `json:"label"`
	Order              int      `json:"order"`
	IsBillable         bool     `json:"is_billable"`
	AllowedTransitions []string `json:"allowed_transitions"`
}

type BudgetCategory struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	Description        string `json:"description"`
	DefaultAccountCode string `json:"default_account_code"`
}

type BudgetTemplate struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	LineItems   []TemplateLineItem `json:"line_items"`
}

type TemplateLineItem struct {
	CategoryID    string          `json:"category_id"`
	Description   string          `json:"description"`
	AccountCode   string          `json:"account_code"`
	DefaultAmount decimal.Decimal `json:"default_amount,omitempty"`
	IsRequired    bool            `json:"is_required"`
}

type ExpenseCategory struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	TaxTreatment       string `json:"tax_treatment"` // "input_gst" | "tds_applicable" | "none"
	DefaultAccountCode string `json:"default_account_code"`
	RequiresPO         bool   `json:"requires_po"`
}

type ApprovalWorkflow struct {
	Limits            []ApprovalLimit `json:"limits"`
	DualApprovalAbove decimal.Decimal `json:"dual_approval_above"`
}

type ApprovalLimit struct {
	Role      string          `json:"role"`
	MaxAmount decimal.Decimal `json:"max_amount"`
}

type InventoryCategory struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	IsTrackable       bool   `json:"is_trackable"`
	DepreciationYears *int   `json:"depreciation_years"` // nil = not depreciated
}

type ReportDefinition struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	DataSources    []string `json:"data_sources"`
	DefaultColumns []string `json:"default_columns"`
}

type CustomFields struct {
	Project []CustomField `json:"project"`
	Expense []CustomField `json:"expense"`
}

type CustomField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`    // "text" | "number" | "date" | "select"
	Options  []string `json:"options"` // for "select" type
	Required bool     `json:"required"`
}
