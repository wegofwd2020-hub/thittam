package vertical

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTestData(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return data
}

func TestValidate_ValidMovieProduction(t *testing.T) {
	data := readTestData(t, "movie-production.yaml")
	errs, err := Validate(data)
	require.NoError(t, err)
	assert.Empty(t, errs)
}

func TestValidate_InvalidYAML(t *testing.T) {
	_, err := Validate([]byte("{{not yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid YAML")
}

func TestValidate_MissingSections(t *testing.T) {
	data := readTestData(t, "invalid-missing-sections.yaml")
	errs, err := Validate(data)
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	fields := make(map[string]bool)
	for _, e := range errs {
		fields[e.Field] = true
	}

	assert.True(t, fields["vertical.phase_types"], "should report missing phase_types")
	assert.True(t, fields["vertical.budget_categories"], "should report missing budget_categories")
	assert.True(t, fields["vertical.expense_categories"], "should report missing expense_categories")
	assert.True(t, fields["vertical.inventory_categories"], "should report missing inventory_categories")
	assert.True(t, fields["vertical.default_chart_of_accounts"], "should report missing chart_of_accounts")
	assert.True(t, fields["vertical.report_definitions"], "should report missing report_definitions")
}

func TestValidate_CyclicPhases(t *testing.T) {
	data := readTestData(t, "invalid-cyclic-phases.yaml")
	errs, err := Validate(data)
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var hasCycleErr, hasTerminalErr bool
	for _, e := range errs {
		if e.Message == "phase transition graph contains a cycle" {
			hasCycleErr = true
		}
		if e.Message == "must have at least one terminal state (phase with empty allowed_transitions)" {
			hasTerminalErr = true
		}
	}
	assert.True(t, hasCycleErr, "should detect cycle in phase transitions")
	assert.True(t, hasTerminalErr, "should detect missing terminal state")
}

func TestValidate_DuplicateIDs(t *testing.T) {
	data := readTestData(t, "invalid-duplicate-ids.yaml")
	errs, err := Validate(data)
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var hasDupErr bool
	for _, e := range errs {
		if e.Field == "vertical.phase_types" && e.Message == `duplicate id "design"` {
			hasDupErr = true
		}
	}
	assert.True(t, hasDupErr, "should detect duplicate phase type ID")
}

func TestValidate_InvalidAccountCodeRef(t *testing.T) {
	yaml := `
vertical:
  id: bad-refs
  name: Bad References
  version: 1.0.0
  description: Test

  entity_labels:
    project: Project
    project_plural: Projects
    phase: Phase
    phase_plural: Phases
    team_member: Member
    team_member_plural: Members
    rate_label: Rate
    rate_unit: day

  phase_types:
    - id: start
      label: Start
      order: 1
      allowed_transitions: [done]
    - id: done
      label: Done
      order: 2
      allowed_transitions: []

  budget_categories:
    - id: general
      label: General
      description: General
      default_account_code: "9999"

  expense_categories:
    - id: misc
      label: Misc
      tax_treatment: none
      default_account_code: "8888"
      requires_po: false

  approval_workflow:
    limits:
      - role: manager
        max_amount: 100000
    dual_approval_above: 500000

  inventory_categories:
    - id: equip
      label: Equipment
      is_trackable: true

  default_chart_of_accounts:
    - code: "5000"
      name: General
      account_type: expense

  report_definitions:
    - id: summary
      name: Summary
      description: Basic
      data_sources: [budget-planning]
      default_columns: [total]
`
	errs, err := Validate([]byte(yaml))
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var hasBudgetRef, hasExpenseRef bool
	for _, e := range errs {
		if e.Field == "vertical.budget_categories[0].default_account_code" {
			hasBudgetRef = true
		}
		if e.Field == "vertical.expense_categories[0].default_account_code" {
			hasExpenseRef = true
		}
	}
	assert.True(t, hasBudgetRef, "should detect invalid account code in budget category")
	assert.True(t, hasExpenseRef, "should detect invalid account code in expense category")
}

func TestValidate_MissingEntityLabels(t *testing.T) {
	yaml := `
vertical:
  id: no-labels
  name: No Labels
  version: 1.0.0
  description: Test

  entity_labels:
    project: ""
    project_plural: ""
    phase: ""
    phase_plural: ""
    team_member: ""
    team_member_plural: ""
    rate_label: ""
    rate_unit: ""

  phase_types:
    - id: done
      label: Done
      order: 1
      allowed_transitions: []

  budget_categories:
    - id: general
      label: General
      description: General
      default_account_code: "5000"

  expense_categories:
    - id: misc
      label: Misc
      tax_treatment: none
      default_account_code: "5000"
      requires_po: false

  approval_workflow:
    limits: []
    dual_approval_above: 0

  inventory_categories:
    - id: equip
      label: Equipment
      is_trackable: true

  default_chart_of_accounts:
    - code: "5000"
      name: General
      account_type: expense

  report_definitions:
    - id: summary
      name: Summary
      description: Basic
      data_sources: [budget-planning]
      default_columns: [total]
`
	errs, err := Validate([]byte(yaml))
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	labelFields := 0
	for _, e := range errs {
		if len(e.Field) > len("vertical.entity_labels.") {
			labelFields++
		}
	}
	assert.Equal(t, 8, labelFields, "should report all 8 missing entity label fields")
}

func TestValidate_TemplateCategoryRef(t *testing.T) {
	yaml := `
vertical:
  id: bad-template
  name: Bad Template
  version: 1.0.0
  description: Test

  entity_labels:
    project: Project
    project_plural: Projects
    phase: Phase
    phase_plural: Phases
    team_member: Member
    team_member_plural: Members
    rate_label: Rate
    rate_unit: day

  phase_types:
    - id: done
      label: Done
      order: 1
      allowed_transitions: []

  budget_categories:
    - id: personnel
      label: Personnel
      description: People
      default_account_code: "5000"

  budget_templates:
    - name: Default
      description: Default template
      line_items:
        - category_id: nonexistent_category
          description: Bad ref
          account_code: "5000"
          is_required: true

  expense_categories:
    - id: misc
      label: Misc
      tax_treatment: none
      default_account_code: "5000"
      requires_po: false

  approval_workflow:
    limits: []
    dual_approval_above: 0

  inventory_categories:
    - id: equip
      label: Equipment
      is_trackable: true

  default_chart_of_accounts:
    - code: "5000"
      name: General
      account_type: expense

  report_definitions:
    - id: summary
      name: Summary
      description: Basic
      data_sources: [budget-planning]
      default_columns: [total]
`
	errs, err := Validate([]byte(yaml))
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var hasCatRef bool
	for _, e := range errs {
		if e.Field == "vertical.budget_templates[0].line_items[0].category_id" {
			hasCatRef = true
		}
	}
	assert.True(t, hasCatRef, "should detect invalid category_id in budget template line item")
}
