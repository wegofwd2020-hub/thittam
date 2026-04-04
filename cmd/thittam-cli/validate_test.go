package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVerticalSummary_Valid(t *testing.T) {
	t.Parallel()
	yaml := `
vertical:
  id: test-vertical
  name: Test Vertical
  version: 1.0.0
  description: A test vertical.
  entity_labels:
    project: Project
    project_plural: Projects
    phase: Phase
    phase_plural: Phases
    team_member: Member
    team_member_plural: Members
    rate_label: Rate
    rate_unit: hour
  phase_types:
    - id: start
      label: Start
      order: 1
      is_billable: false
      allowed_transitions: [done]
    - id: done
      label: Done
      order: 2
      is_billable: false
      allowed_transitions: []
  budget_categories:
    - id: cat1
      label: Category 1
      description: Test
      default_account_code: "5000"
  budget_templates:
    - name: Template 1
      description: Test
      line_items:
        - category_id: cat1
          description: Item 1
          account_code: "5000"
          is_required: true
  expense_categories:
    - id: exp1
      label: Expense 1
      tax_treatment: none
      default_account_code: "5000"
      requires_po: false
  approval_workflow:
    limits:
      - role: admin
        max_amount: 100000
    dual_approval_above: 100000
  inventory_categories:
    - id: inv1
      label: Inventory 1
      is_trackable: true
  default_chart_of_accounts:
    - code: "5000"
      name: Expenses
      account_type: expense
  report_definitions:
    - id: rep1
      name: Report 1
      description: Test
      data_sources: [budget-planning]
      default_columns: [col1]
  custom_fields:
    project:
      - key: field1
        label: Field 1
        type: text
        required: false
    expense: []
`
	summary, err := parseVerticalSummary([]byte(yaml))
	require.NoError(t, err)

	assert.Equal(t, "test-vertical", summary.ID)
	assert.Equal(t, "Test Vertical", summary.Name)
	assert.Equal(t, "1.0.0", summary.Version)
	assert.Equal(t, 2, summary.PhaseCount)
	assert.Equal(t, 1, summary.BudgetCatCount)
	assert.Equal(t, 1, summary.BudgetTemplateCount)
	assert.Equal(t, 1, summary.ExpenseCatCount)
	assert.Equal(t, 1, summary.InventoryCatCount)
	assert.Equal(t, 1, summary.CoACount)
	assert.Equal(t, 1, summary.ReportCount)
}

func TestParseVerticalSummary_InvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := parseVerticalSummary([]byte("not: valid: yaml: ["))
	assert.Error(t, err)
}
