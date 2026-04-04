// Package demo provides UI-driven demo tenant provisioning.
// A super_admin selects a vertical, previews the generated demo data,
// and confirms to create a fully-populated demo tenant.
package demo

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// PreviewRequest is the input for generating a demo preview.
type PreviewRequest struct {
	VerticalID  string `json:"vertical_id"`
	CompanyName string `json:"company_name"`
	Plan        string `json:"plan"`
}

// DemoPreview contains all the data that will be created when provisioned.
// The super_admin reviews this before confirming.
type DemoPreview struct {
	PreviewToken string `json:"preview_token"` // opaque token for confirm step

	// Tenant
	Tenant DemoTenant `json:"tenant"`

	// Vertical config summary
	VerticalID   string `json:"vertical_id"`
	VerticalName string `json:"vertical_name"`

	// What will be created
	Users              []DemoUser           `json:"users"`
	ChartOfAccounts    []vertical.ChartOfAccountEntry `json:"chart_of_accounts"`
	BudgetCategories   []vertical.BudgetCategory      `json:"budget_categories"`
	BudgetTemplates    []vertical.BudgetTemplate      `json:"budget_templates"`
	ExpenseCategories  []vertical.ExpenseCategory     `json:"expense_categories"`
	ApprovalWorkflow   vertical.ApprovalWorkflow      `json:"approval_workflow"`
	InventoryCategories []vertical.InventoryCategory   `json:"inventory_categories"`
	Projects           []DemoProject        `json:"projects"`
	EntityLabels       vertical.EntityLabels `json:"entity_labels"`
}

// DemoTenant is the tenant that will be created.
type DemoTenant struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan string `json:"plan"`
}

// DemoUser is a user that will be created in the demo tenant.
type DemoUser struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Department   string `json:"department,omitempty"`
	TempPassword string `json:"temp_password"` // shown to admin after provisioning
}

// DemoProject is a sample project with budget and expenses.
type DemoProject struct {
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Phase        string          `json:"phase"`
	BudgetTotal  decimal.Decimal `json:"budget_total"`
	ExpenseCount int             `json:"expense_count"`
	Status       string          `json:"status"`
}

// ProvisionRequest confirms a preview and triggers creation.
type ProvisionRequest struct {
	VerticalID   string `json:"vertical_id"`
	CompanyName  string `json:"company_name"`
	Plan         string `json:"plan"`
	PreviewToken string `json:"preview_token"`
}

// ProvisionResult is returned after successful provisioning.
type ProvisionResult struct {
	TenantID    uuid.UUID  `json:"tenant_id"`
	AdminEmail  string     `json:"admin_email"`
	AdminPassword string   `json:"admin_password"`
	UserCount   int        `json:"user_count"`
	ProjectCount int       `json:"project_count"`
	CreatedAt   time.Time  `json:"created_at"`
}
