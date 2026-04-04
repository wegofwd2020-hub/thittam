package vertical

import (
	"strings"

	"github.com/shopspring/decimal"
)

// FindBudgetCategory returns the BudgetCategory with the given ID, or nil.
func (c *Config) FindBudgetCategory(id string) *BudgetCategory {
	for i := range c.BudgetCategories {
		if c.BudgetCategories[i].ID == id {
			return &c.BudgetCategories[i]
		}
	}
	return nil
}

// BudgetCategoryIDs returns a comma-separated list of valid category IDs.
func (c *Config) BudgetCategoryIDs() string {
	ids := make([]string, len(c.BudgetCategories))
	for i, cat := range c.BudgetCategories {
		ids[i] = cat.ID
	}
	return strings.Join(ids, ", ")
}

// FindPhaseType returns the PhaseType with the given ID, or nil.
func (c *Config) FindPhaseType(id string) *PhaseType {
	for i := range c.PhaseTypes {
		if c.PhaseTypes[i].ID == id {
			return &c.PhaseTypes[i]
		}
	}
	return nil
}

// CanTransitionTo checks whether this PhaseType allows a transition to targetID.
func (p *PhaseType) CanTransitionTo(targetID string) bool {
	for _, t := range p.AllowedTransitions {
		if t == targetID {
			return true
		}
	}
	return false
}

// FindExpenseCategory returns the ExpenseCategory with the given ID, or nil.
func (c *Config) FindExpenseCategory(id string) *ExpenseCategory {
	for i := range c.ExpenseCategories {
		if c.ExpenseCategories[i].ID == id {
			return &c.ExpenseCategories[i]
		}
	}
	return nil
}

// LimitForRole returns the approval limit for a role, or nil if not configured.
func (a *ApprovalWorkflow) LimitForRole(role string) *decimal.Decimal {
	for _, limit := range a.Limits {
		if limit.Role == role {
			amt := limit.MaxAmount
			return &amt
		}
	}
	return nil
}

// FindBudgetTemplate returns the BudgetTemplate with the given name, or nil.
func (c *Config) FindBudgetTemplate(name string) *BudgetTemplate {
	for i := range c.BudgetTemplates {
		if c.BudgetTemplates[i].Name == name {
			return &c.BudgetTemplates[i]
		}
	}
	return nil
}

// FindInventoryCategory returns the InventoryCategory with the given ID, or nil.
func (c *Config) FindInventoryCategory(id string) *InventoryCategory {
	for i := range c.InventoryCategories {
		if c.InventoryCategories[i].ID == id {
			return &c.InventoryCategories[i]
		}
	}
	return nil
}

// FindReportDefinition returns the ReportDefinition with the given ID, or nil.
func (c *Config) FindReportDefinition(id string) *ReportDefinition {
	for i := range c.ReportDefinitions {
		if c.ReportDefinitions[i].ID == id {
			return &c.ReportDefinitions[i]
		}
	}
	return nil
}
