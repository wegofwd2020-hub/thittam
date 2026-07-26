package vertical

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// VerticalDefinition is the top-level YAML structure for vertical files.
// It includes fields not present in the runtime Config (e.g. chart of accounts).
type VerticalDefinition struct {
	Vertical VerticalYAML `yaml:"vertical"`
}

// VerticalYAML extends Config with fields only used at registration time.
type VerticalYAML struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`

	EntityLabels        EntityLabelsYAML        `yaml:"entity_labels"`
	PhaseTypes          []PhaseTypeYAML         `yaml:"phase_types"`
	BudgetCategories    []BudgetCategoryYAML    `yaml:"budget_categories"`
	BudgetTemplates     []BudgetTemplateYAML    `yaml:"budget_templates"`
	ExpenseCategories   []ExpenseCategoryYAML   `yaml:"expense_categories"`
	ApprovalWorkflow    ApprovalWorkflowYAML    `yaml:"approval_workflow"`
	InventoryCategories []InventoryCategoryYAML `yaml:"inventory_categories"`
	ReportDefinitions   []ReportDefinitionYAML  `yaml:"report_definitions"`
	CustomFields        *CustomFieldsYAML       `yaml:"custom_fields"`

	DefaultChartOfAccounts []ChartOfAccountEntry `yaml:"default_chart_of_accounts"`

	RoleLabels map[string]string `yaml:"role_labels"`
}

// SystemRoleNames is the closed set of internal role names seeded by the iam
// service for every tenant. Vertical YAML files must provide a label for every
// entry in this set. Keep in sync with services/iam.systemRoles.
var SystemRoleNames = []string{
	"super_admin",
	"manager",
	"coordinator",
	"accountant",
	"member",
	"inventory_manager",
	"project_supervisor",
}

type EntityLabelsYAML struct {
	Project          string `yaml:"project"`
	ProjectPlural    string `yaml:"project_plural"`
	Phase            string `yaml:"phase"`
	PhasePlural      string `yaml:"phase_plural"`
	TeamMember       string `yaml:"team_member"`
	TeamMemberPlural string `yaml:"team_member_plural"`
	RateLabel        string `yaml:"rate_label"`
	RateUnit         string `yaml:"rate_unit"`
}

type PhaseTypeYAML struct {
	ID                 string   `yaml:"id"`
	Label              string   `yaml:"label"`
	Order              int      `yaml:"order"`
	IsBillable         bool     `yaml:"is_billable"`
	AllowedTransitions []string `yaml:"allowed_transitions"`
}

type BudgetCategoryYAML struct {
	ID                 string `yaml:"id"`
	Label              string `yaml:"label"`
	Description        string `yaml:"description"`
	DefaultAccountCode string `yaml:"default_account_code"`
}

type BudgetTemplateYAML struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	LineItems   []TemplateLineItemYAML `yaml:"line_items"`
}

type TemplateLineItemYAML struct {
	CategoryID    string  `yaml:"category_id"`
	Description   string  `yaml:"description"`
	AccountCode   string  `yaml:"account_code"`
	DefaultAmount float64 `yaml:"default_amount"`
	IsRequired    bool    `yaml:"is_required"`
}

type ExpenseCategoryYAML struct {
	ID                 string `yaml:"id"`
	Label              string `yaml:"label"`
	TaxTreatment       string `yaml:"tax_treatment"`
	DefaultAccountCode string `yaml:"default_account_code"`
	RequiresPO         bool   `yaml:"requires_po"`
}

type ApprovalWorkflowYAML struct {
	Limits            []ApprovalLimitYAML `yaml:"limits"`
	DualApprovalAbove float64             `yaml:"dual_approval_above"`
}

type ApprovalLimitYAML struct {
	Role      string  `yaml:"role"`
	MaxAmount float64 `yaml:"max_amount"`
}

type InventoryCategoryYAML struct {
	ID                string `yaml:"id"`
	Label             string `yaml:"label"`
	IsTrackable       bool   `yaml:"is_trackable"`
	DepreciationYears *int   `yaml:"depreciation_years"`
}

type ReportDefinitionYAML struct {
	ID             string   `yaml:"id"`
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	DataSources    []string `yaml:"data_sources"`
	DefaultColumns []string `yaml:"default_columns"`
}

type CustomFieldsYAML struct {
	Project []CustomFieldYAML `yaml:"project"`
	Expense []CustomFieldYAML `yaml:"expense"`
}

type CustomFieldYAML struct {
	Key      string   `yaml:"key"`
	Label    string   `yaml:"label"`
	Type     string   `yaml:"type"`
	Options  []string `yaml:"options"`
	Required bool     `yaml:"required"`
}

// ChartOfAccountEntry is defined in types.go with both json and yaml tags.

// ValidationError represents a single validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate parses and validates a vertical YAML definition.
// Returns (nil, error) for parse failures.
// Returns ([]ValidationError, nil) for semantic validation failures.
// Returns (nil, nil) for a valid definition.
func Validate(yamlData []byte) ([]ValidationError, error) {
	var def VerticalDefinition
	if err := yaml.Unmarshal(yamlData, &def); err != nil {
		return nil, fmt.Errorf("vertical: invalid YAML: %w", err)
	}

	v := def.Vertical
	var errs []ValidationError

	// Required top-level fields
	if v.ID == "" {
		errs = append(errs, ValidationError{"vertical.id", "required"})
	}
	if v.Name == "" {
		errs = append(errs, ValidationError{"vertical.name", "required"})
	}
	if v.Version == "" {
		errs = append(errs, ValidationError{"vertical.version", "required"})
	}

	// Required entity labels
	errs = append(errs, validateEntityLabels(v.EntityLabels)...)

	// Required sections
	if len(v.PhaseTypes) == 0 {
		errs = append(errs, ValidationError{"vertical.phase_types", "at least one phase type required"})
	}
	if len(v.BudgetCategories) == 0 {
		errs = append(errs, ValidationError{"vertical.budget_categories", "at least one budget category required"})
	}
	if len(v.ExpenseCategories) == 0 {
		errs = append(errs, ValidationError{"vertical.expense_categories", "at least one expense category required"})
	}
	if len(v.InventoryCategories) == 0 {
		errs = append(errs, ValidationError{"vertical.inventory_categories", "at least one inventory category required"})
	}
	if len(v.DefaultChartOfAccounts) == 0 {
		errs = append(errs, ValidationError{"vertical.default_chart_of_accounts", "required"})
	}
	if len(v.ReportDefinitions) == 0 {
		errs = append(errs, ValidationError{"vertical.report_definitions", "at least one report definition required"})
	}

	// Build account code set for cross-referencing
	accountCodes := make(map[string]bool, len(v.DefaultChartOfAccounts))
	for _, entry := range v.DefaultChartOfAccounts {
		accountCodes[entry.Code] = true
	}

	// Duplicate ID checks
	errs = append(errs, checkDuplicateIDs("vertical.phase_types", phaseTypeIDs(v.PhaseTypes))...)
	errs = append(errs, checkDuplicateNames("vertical.phase_types", phaseTypeLabels(v.PhaseTypes))...)
	errs = append(errs, checkDuplicateIDs("vertical.budget_categories", budgetCategoryIDs(v.BudgetCategories))...)
	errs = append(errs, checkDuplicateIDs("vertical.expense_categories", expenseCategoryIDs(v.ExpenseCategories))...)
	errs = append(errs, checkDuplicateIDs("vertical.inventory_categories", inventoryCategoryIDs(v.InventoryCategories))...)
	errs = append(errs, checkDuplicateIDs("vertical.report_definitions", reportDefinitionIDs(v.ReportDefinitions))...)
	errs = append(errs, checkDuplicateNames("vertical.budget_templates", budgetTemplateNames(v.BudgetTemplates))...)

	// Account code cross-references
	for i, cat := range v.BudgetCategories {
		if cat.DefaultAccountCode != "" && !accountCodes[cat.DefaultAccountCode] {
			errs = append(errs, ValidationError{
				fmt.Sprintf("vertical.budget_categories[%d].default_account_code", i),
				fmt.Sprintf("account code %q not found in default_chart_of_accounts", cat.DefaultAccountCode),
			})
		}
	}
	for i, cat := range v.ExpenseCategories {
		if cat.DefaultAccountCode != "" && !accountCodes[cat.DefaultAccountCode] {
			errs = append(errs, ValidationError{
				fmt.Sprintf("vertical.expense_categories[%d].default_account_code", i),
				fmt.Sprintf("account code %q not found in default_chart_of_accounts", cat.DefaultAccountCode),
			})
		}
		// Enum check for tax_treatment.
		if cat.TaxTreatment != "" && !validTaxTreatments[cat.TaxTreatment] {
			errs = append(errs, ValidationError{
				fmt.Sprintf("vertical.expense_categories[%d].tax_treatment", i),
				fmt.Sprintf("invalid value %q: must be one of input_gst, tds_applicable, none", cat.TaxTreatment),
			})
		}
	}

	// Budget template line item validation
	budgetCatIDs := make(map[string]bool, len(v.BudgetCategories))
	for _, cat := range v.BudgetCategories {
		budgetCatIDs[cat.ID] = true
	}
	for i, tmpl := range v.BudgetTemplates {
		for j, item := range tmpl.LineItems {
			if !budgetCatIDs[item.CategoryID] {
				errs = append(errs, ValidationError{
					fmt.Sprintf("vertical.budget_templates[%d].line_items[%d].category_id", i, j),
					fmt.Sprintf("category %q not found in budget_categories", item.CategoryID),
				})
			}
			if item.AccountCode != "" && !accountCodes[item.AccountCode] {
				errs = append(errs, ValidationError{
					fmt.Sprintf("vertical.budget_templates[%d].line_items[%d].account_code", i, j),
					fmt.Sprintf("account code %q not found in default_chart_of_accounts", item.AccountCode),
				})
			}
		}
	}

	// Phase transition validation
	if len(v.PhaseTypes) > 0 {
		errs = append(errs, validatePhaseTransitions(v.PhaseTypes)...)
	}

	// Role label coverage: every system role must have a label entry.
	// Unknown keys in role_labels are not an error — they may reference roles
	// added in a future iam release before this YAML is updated.
	for _, name := range SystemRoleNames {
		if _, ok := v.RoleLabels[name]; !ok {
			errs = append(errs, ValidationError{
				"vertical.role_labels",
				fmt.Sprintf("missing label for system role %q", name),
			})
		}
	}

	// Approval-limit roles must be canonical RBAC role names — otherwise
	// MaxLimitForRoles(caller.Roles) never matches and approvals fail-closed (#199).
	validRole := make(map[string]bool, len(SystemRoleNames))
	for _, name := range SystemRoleNames {
		validRole[name] = true
	}
	for _, lim := range v.ApprovalWorkflow.Limits {
		if !validRole[lim.Role] {
			errs = append(errs, ValidationError{
				"vertical.approval_workflow.limits",
				fmt.Sprintf("limit role %q is not a system role", lim.Role),
			})
		}
	}

	if len(errs) > 0 {
		return errs, nil
	}
	return nil, nil
}

// validRateUnits is the closed set of allowed rate_unit values.
// Expanding this set requires a vertical schema version bump.
var validRateUnits = map[string]bool{
	"day":   true,
	"hour":  true,
	"month": true,
	"fixed": true,
}

// validTaxTreatments is the closed set of allowed tax_treatment values.
var validTaxTreatments = map[string]bool{
	"input_gst":      true,
	"tds_applicable": true,
	"none":           true,
}

func validateEntityLabels(el EntityLabelsYAML) []ValidationError {
	var errs []ValidationError
	fields := map[string]string{
		"project":            el.Project,
		"project_plural":     el.ProjectPlural,
		"phase":              el.Phase,
		"phase_plural":       el.PhasePlural,
		"team_member":        el.TeamMember,
		"team_member_plural": el.TeamMemberPlural,
		"rate_label":         el.RateLabel,
		"rate_unit":          el.RateUnit,
	}
	for name, val := range fields {
		if val == "" {
			errs = append(errs, ValidationError{
				"vertical.entity_labels." + name, "required",
			})
		}
	}
	// Enum check: only validate when non-empty (empty already reported above).
	if el.RateUnit != "" && !validRateUnits[el.RateUnit] {
		errs = append(errs, ValidationError{
			"vertical.entity_labels.rate_unit",
			fmt.Sprintf("invalid value %q: must be one of day, hour, month, fixed", el.RateUnit),
		})
	}
	return errs
}

// validatePhaseTransitions checks for cycles and at least one terminal state.
func validatePhaseTransitions(phases []PhaseTypeYAML) []ValidationError {
	var errs []ValidationError

	ids := make(map[string]bool, len(phases))
	adj := make(map[string][]string, len(phases))
	for _, p := range phases {
		ids[p.ID] = true
		adj[p.ID] = p.AllowedTransitions
	}

	// Check transitions reference valid phase IDs
	for _, p := range phases {
		for _, t := range p.AllowedTransitions {
			if !ids[t] {
				errs = append(errs, ValidationError{
					fmt.Sprintf("vertical.phase_types[%s].allowed_transitions", p.ID),
					fmt.Sprintf("references unknown phase %q", t),
				})
			}
		}
	}

	// Check for at least one terminal state
	hasTerminal := false
	for _, p := range phases {
		if len(p.AllowedTransitions) == 0 {
			hasTerminal = true
			break
		}
	}
	if !hasTerminal {
		errs = append(errs, ValidationError{
			"vertical.phase_types",
			"must have at least one terminal state (phase with empty allowed_transitions)",
		})
	}

	// Cycle detection via DFS (white=0, gray=1, black=2)
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(phases))
	for _, p := range phases {
		color[p.ID] = white
	}

	var hasCycle bool
	var dfs func(node string)
	dfs = func(node string) {
		if hasCycle {
			return
		}
		color[node] = gray
		for _, next := range adj[node] {
			if !ids[next] {
				continue // skip invalid refs, already reported
			}
			switch color[next] {
			case gray:
				hasCycle = true
				return
			case white:
				dfs(next)
			}
		}
		color[node] = black
	}

	for _, p := range phases {
		if color[p.ID] == white {
			dfs(p.ID)
		}
	}
	if hasCycle {
		errs = append(errs, ValidationError{
			"vertical.phase_types",
			"phase transition graph contains a cycle",
		})
	}

	return errs
}

func checkDuplicateIDs(field string, ids []string) []ValidationError {
	seen := make(map[string]bool, len(ids))
	var errs []ValidationError
	for _, id := range ids {
		if seen[id] {
			errs = append(errs, ValidationError{field, fmt.Sprintf("duplicate id %q", id)})
		}
		seen[id] = true
	}
	return errs
}

func checkDuplicateNames(field string, names []string) []ValidationError {
	seen := make(map[string]bool, len(names))
	var errs []ValidationError
	for _, name := range names {
		if seen[name] {
			errs = append(errs, ValidationError{field, fmt.Sprintf("duplicate name %q", name)})
		}
		seen[name] = true
	}
	return errs
}

func phaseTypeIDs(pts []PhaseTypeYAML) []string {
	ids := make([]string, len(pts))
	for i, p := range pts {
		ids[i] = p.ID
	}
	return ids
}

func phaseTypeLabels(pts []PhaseTypeYAML) []string {
	labels := make([]string, len(pts))
	for i, p := range pts {
		labels[i] = p.Label
	}
	return labels
}

func budgetCategoryIDs(cats []BudgetCategoryYAML) []string {
	ids := make([]string, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	return ids
}

func expenseCategoryIDs(cats []ExpenseCategoryYAML) []string {
	ids := make([]string, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	return ids
}

func inventoryCategoryIDs(cats []InventoryCategoryYAML) []string {
	ids := make([]string, len(cats))
	for i, c := range cats {
		ids[i] = c.ID
	}
	return ids
}

func reportDefinitionIDs(defs []ReportDefinitionYAML) []string {
	ids := make([]string, len(defs))
	for i, d := range defs {
		ids[i] = d.ID
	}
	return ids
}

func budgetTemplateNames(tmpls []BudgetTemplateYAML) []string {
	names := make([]string, len(tmpls))
	for i, t := range tmpls {
		names[i] = t.Name
	}
	return names
}
