package vertical

import (
	"encoding/json"
)

// Override represents a tenant's config override with structured operations.
// Each section supports "append" (add new items), "override" (modify existing),
// and "remove" (delete by ID).
type Override struct {
	EntityLabels        *OverrideMap    `json:"entity_labels,omitempty"`
	BudgetCategories    *ArrayOverride  `json:"budget_categories,omitempty"`
	ExpenseCategories   *ArrayOverride  `json:"expense_categories,omitempty"`
	InventoryCategories *ArrayOverride  `json:"inventory_categories,omitempty"`
	PhaseTypes          *ArrayOverride  `json:"phase_types,omitempty"`
	ReportDefinitions   *ArrayOverride  `json:"report_definitions,omitempty"`
	BudgetTemplates     *ArrayOverride  `json:"budget_templates,omitempty"`
	ApprovalWorkflow    *OverrideMap    `json:"approval_workflow,omitempty"`
	CustomFields        *OverrideMap    `json:"custom_fields,omitempty"`
}

// OverrideMap applies key-level overrides to a map/struct.
type OverrideMap struct {
	Override map[string]interface{} `json:"override,omitempty"`
}

// ArrayOverride applies append/override/remove operations to an array of items with IDs.
type ArrayOverride struct {
	Append   []json.RawMessage      `json:"append,omitempty"`
	Override map[string]json.RawMessage `json:"override,omitempty"` // keyed by ID
	Remove   []string               `json:"remove,omitempty"`
}

// hasOperations returns true if the override has at least one structured operation.
func (o *Override) hasOperations() bool {
	return o.EntityLabels != nil ||
		o.BudgetCategories != nil ||
		o.ExpenseCategories != nil ||
		o.InventoryCategories != nil ||
		o.PhaseTypes != nil ||
		o.ReportDefinitions != nil ||
		o.BudgetTemplates != nil ||
		o.ApprovalWorkflow != nil ||
		o.CustomFields != nil
}

// ApplyOverride applies a structured override to a base Config, returning a new Config.
// The base config is not modified.
func ApplyOverride(base *Config, overrideJSON []byte) (*Config, error) {
	if len(overrideJSON) == 0 || string(overrideJSON) == "null" || string(overrideJSON) == "{}" {
		return base, nil
	}

	// Try structured override format first
	var override Override
	if err := json.Unmarshal(overrideJSON, &override); err != nil || !override.hasOperations() {
		// Fall back to flat JSONB merge (backward compatibility with shallow || merge)
		return applyFlatMerge(base, overrideJSON)
	}

	// Deep copy the base config
	merged := deepCopyConfig(base)

	// Apply entity label overrides
	if override.EntityLabels != nil && override.EntityLabels.Override != nil {
		applyEntityLabelOverrides(&merged.EntityLabels, override.EntityLabels.Override)
	}

	// Apply array overrides for each section
	if override.BudgetCategories != nil {
		merged.BudgetCategories = applyBudgetCategoryOverride(merged.BudgetCategories, override.BudgetCategories)
	}
	if override.ExpenseCategories != nil {
		merged.ExpenseCategories = applyExpenseCategoryOverride(merged.ExpenseCategories, override.ExpenseCategories)
	}
	if override.InventoryCategories != nil {
		merged.InventoryCategories = applyInventoryCategoryOverride(merged.InventoryCategories, override.InventoryCategories)
	}
	if override.PhaseTypes != nil {
		merged.PhaseTypes = applyPhaseTypeOverride(merged.PhaseTypes, override.PhaseTypes)
	}
	if override.ReportDefinitions != nil {
		merged.ReportDefinitions = applyReportDefinitionOverride(merged.ReportDefinitions, override.ReportDefinitions)
	}

	return merged, nil
}

// deepCopyConfig creates a deep copy via JSON round-trip.
func deepCopyConfig(cfg *Config) *Config {
	data, _ := json.Marshal(cfg)
	var copy Config
	json.Unmarshal(data, &copy)
	return &copy
}

// applyFlatMerge handles backward-compatible flat JSONB overrides (non-structured).
func applyFlatMerge(base *Config, overrideJSON []byte) (*Config, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]interface{}
	json.Unmarshal(baseJSON, &baseMap)

	var overrideMap map[string]interface{}
	if err := json.Unmarshal(overrideJSON, &overrideMap); err != nil {
		return base, nil // invalid override, return base unchanged
	}

	// Shallow merge: override keys replace base keys
	for k, v := range overrideMap {
		baseMap[k] = v
	}

	mergedJSON, _ := json.Marshal(baseMap)
	var merged Config
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return base, nil
	}
	return &merged, nil
}

// applyEntityLabelOverrides replaces individual entity label fields.
func applyEntityLabelOverrides(labels *EntityLabels, overrides map[string]interface{}) {
	if v, ok := overrides["project"].(string); ok {
		labels.Project = v
	}
	if v, ok := overrides["project_plural"].(string); ok {
		labels.ProjectPlural = v
	}
	if v, ok := overrides["phase"].(string); ok {
		labels.Phase = v
	}
	if v, ok := overrides["phase_plural"].(string); ok {
		labels.PhasePlural = v
	}
	if v, ok := overrides["team_member"].(string); ok {
		labels.TeamMember = v
	}
	if v, ok := overrides["team_member_plural"].(string); ok {
		labels.TeamMemberPlural = v
	}
	if v, ok := overrides["rate_label"].(string); ok {
		labels.RateLabel = v
	}
	if v, ok := overrides["rate_unit"].(string); ok {
		labels.RateUnit = v
	}
}

// --- Array override helpers (append/override/remove by ID) ---

func applyBudgetCategoryOverride(base []BudgetCategory, op *ArrayOverride) []BudgetCategory {
	// Remove
	if len(op.Remove) > 0 {
		removeSet := toSet(op.Remove)
		filtered := base[:0]
		for _, item := range base {
			if !removeSet[item.ID] {
				filtered = append(filtered, item)
			}
		}
		base = filtered
	}

	// Override existing by ID
	for id, raw := range op.Override {
		for i := range base {
			if base[i].ID == id {
				json.Unmarshal(raw, &base[i])
				break
			}
		}
	}

	// Append new
	for _, raw := range op.Append {
		var item BudgetCategory
		if json.Unmarshal(raw, &item) == nil {
			base = append(base, item)
		}
	}

	return base
}

func applyExpenseCategoryOverride(base []ExpenseCategory, op *ArrayOverride) []ExpenseCategory {
	if len(op.Remove) > 0 {
		removeSet := toSet(op.Remove)
		filtered := base[:0]
		for _, item := range base {
			if !removeSet[item.ID] {
				filtered = append(filtered, item)
			}
		}
		base = filtered
	}
	for id, raw := range op.Override {
		for i := range base {
			if base[i].ID == id {
				json.Unmarshal(raw, &base[i])
				break
			}
		}
	}
	for _, raw := range op.Append {
		var item ExpenseCategory
		if json.Unmarshal(raw, &item) == nil {
			base = append(base, item)
		}
	}
	return base
}

func applyInventoryCategoryOverride(base []InventoryCategory, op *ArrayOverride) []InventoryCategory {
	if len(op.Remove) > 0 {
		removeSet := toSet(op.Remove)
		filtered := base[:0]
		for _, item := range base {
			if !removeSet[item.ID] {
				filtered = append(filtered, item)
			}
		}
		base = filtered
	}
	for id, raw := range op.Override {
		for i := range base {
			if base[i].ID == id {
				json.Unmarshal(raw, &base[i])
				break
			}
		}
	}
	for _, raw := range op.Append {
		var item InventoryCategory
		if json.Unmarshal(raw, &item) == nil {
			base = append(base, item)
		}
	}
	return base
}

func applyPhaseTypeOverride(base []PhaseType, op *ArrayOverride) []PhaseType {
	if len(op.Remove) > 0 {
		removeSet := toSet(op.Remove)
		filtered := base[:0]
		for _, item := range base {
			if !removeSet[item.ID] {
				filtered = append(filtered, item)
			}
		}
		base = filtered
	}
	for id, raw := range op.Override {
		for i := range base {
			if base[i].ID == id {
				json.Unmarshal(raw, &base[i])
				break
			}
		}
	}
	for _, raw := range op.Append {
		var item PhaseType
		if json.Unmarshal(raw, &item) == nil {
			base = append(base, item)
		}
	}
	return base
}

func applyReportDefinitionOverride(base []ReportDefinition, op *ArrayOverride) []ReportDefinition {
	if len(op.Remove) > 0 {
		removeSet := toSet(op.Remove)
		filtered := base[:0]
		for _, item := range base {
			if !removeSet[item.ID] {
				filtered = append(filtered, item)
			}
		}
		base = filtered
	}
	for id, raw := range op.Override {
		for i := range base {
			if base[i].ID == id {
				json.Unmarshal(raw, &base[i])
				break
			}
		}
	}
	for _, raw := range op.Append {
		var item ReportDefinition
		if json.Unmarshal(raw, &item) == nil {
			base = append(base, item)
		}
	}
	return base
}

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
