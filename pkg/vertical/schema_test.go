package vertical

// schema_test.go — vertical YAML schema validation test suite.
//
// Coverage goals (per issue #16):
//   1. All 4 GA vertical configs pass validation without errors.
//   2. Missing required fields produce descriptive errors.       (validator_test.go)
//   3. Invalid enum values (rate_unit, tax_treatment) are rejected.
//   4. Duplicate phase labels within a vertical are rejected.
//   5. Extra unknown fields are handled gracefully (warn, not panic).
//   6. TestValidateAllProductionVerticals scans pkg/vertical/configs/*.yaml —
//      this is the test run by `make validate-verticals`.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 0. JSON Schema file is valid JSON ────────────────────────────────────────

// TestSchemaJSON verifies that schema.json is well-formed JSON and contains the
// required top-level keys. This ensures the schema file stays in sync with the
// Go validator and is parseable by JSON Schema tooling.
func TestSchemaJSON_IsWellFormed(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("schema.json")
	require.NoError(t, err, "schema.json must exist in pkg/vertical/")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc), "schema.json must be valid JSON")

	// Must have the JSON Schema meta-fields.
	assert.Equal(t, "https://json-schema.org/draft/2020-12/schema", doc["$schema"],
		"schema.json must declare its meta-schema")
	assert.NotEmpty(t, doc["$id"], "schema.json must have an $id")
	assert.NotEmpty(t, doc["title"], "schema.json must have a title")

	// The root must require the top-level `vertical` key.
	props, _ := doc["properties"].(map[string]any)
	require.NotNil(t, props["vertical"], "schema.json must define a 'vertical' property")

	vertical, _ := props["vertical"].(map[string]any)
	required, _ := vertical["required"].([]any)
	requiredStrs := make([]string, len(required))
	for i, r := range required {
		requiredStrs[i], _ = r.(string)
	}

	for _, field := range []string{
		"id", "name", "version", "entity_labels",
		"phase_types", "budget_categories", "expense_categories",
		"inventory_categories", "default_chart_of_accounts", "report_definitions",
	} {
		assert.Contains(t, requiredStrs, field,
			"schema.json vertical.required must include %q", field)
	}
}

// ── 1. All 4 GA verticals ────────────────────────────────────────────────────

// TestValidateAllProductionVerticals is the test run by `make validate-verticals`.
// It reads every *.yaml file under pkg/vertical/configs/ and asserts that each
// passes Validate() without errors. A new vertical YAML added to configs/ is
// automatically picked up by this test.
func TestValidateAllProductionVerticals(t *testing.T) {
	t.Parallel()

	pattern := filepath.Join("configs", "*.yaml")
	paths, err := filepath.Glob(pattern)
	require.NoError(t, err, "glob configs/*.yaml")
	require.NotEmpty(t, paths, "expected at least one vertical YAML in configs/")

	for _, path := range paths {
		path := path // capture for t.Parallel
		verticalID := strings.TrimSuffix(filepath.Base(path), ".yaml")

		t.Run(verticalID, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			require.NoError(t, err, "read %s", path)

			errs, err := Validate(data)
			require.NoError(t, err, "YAML parse error in %s", path)
			assert.Empty(t, errs,
				"vertical %s has validation errors:\n%s",
				verticalID, formatValidationErrors(errs),
			)
		})
	}
}

// TestValidateAllGAVerticalsSmokeTest explicitly names each expected GA vertical
// so that removing or renaming a canonical config is caught independently.
func TestValidateAllGAVerticalsSmokeTest(t *testing.T) {
	t.Parallel()

	required := []string{
		"movie-production",
		"software-development",
		"construction",
		"events-management",
	}

	for _, id := range required {
		id := id
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join("configs", id+".yaml")
			data, err := os.ReadFile(path)
			require.NoError(t, err, "GA vertical config missing: %s", path)

			errs, parseErr := Validate(data)
			require.NoError(t, parseErr, "%s: YAML parse error", id)
			assert.Empty(t, errs, "%s: unexpected validation errors:\n%s", id, formatValidationErrors(errs))
		})
	}
}

// ── 3. Invalid enum values ───────────────────────────────────────────────────

func TestValidate_InvalidEnumValues(t *testing.T) {
	t.Parallel()

	data := readTestData(t, "invalid-bad-enum-values.yaml")
	errs, err := Validate(data)
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	fields := make(map[string]string, len(errs))
	for _, e := range errs {
		fields[e.Field] = e.Message
	}

	assert.Contains(t, fields["vertical.entity_labels.rate_unit"], "weekly",
		"should report the offending value in the rate_unit error")
	assert.Contains(t, fields["vertical.expense_categories[0].tax_treatment"], "banana",
		"should report the offending value in the tax_treatment error")
}

func TestValidate_InvalidRateUnit_InlineYAML(t *testing.T) {
	t.Parallel()

	yaml := minimalValidYAML(t, func(y *minimalVertical) {
		y.RateUnit = "weekly" // not in {day, hour, month, fixed}
	})

	errs, err := Validate([]byte(yaml))
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var found bool
	for _, e := range errs {
		if e.Field == "vertical.entity_labels.rate_unit" {
			found = true
			assert.Contains(t, e.Message, "weekly")
		}
	}
	assert.True(t, found, "expected rate_unit validation error")
}

func TestValidate_ValidRateUnits_AllAccepted(t *testing.T) {
	t.Parallel()

	for _, unit := range []string{"day", "hour", "month", "fixed"} {
		unit := unit
		t.Run(unit, func(t *testing.T) {
			t.Parallel()

			yaml := minimalValidYAML(t, func(y *minimalVertical) {
				y.RateUnit = unit
			})

			errs, err := Validate([]byte(yaml))
			require.NoError(t, err)

			// Filter out any rate_unit errors specifically
			for _, e := range errs {
				assert.NotEqual(t, "vertical.entity_labels.rate_unit", e.Field,
					"rate_unit=%q should be accepted but got error: %s", unit, e.Message)
			}
		})
	}
}

func TestValidate_InvalidTaxTreatment_InlineYAML(t *testing.T) {
	t.Parallel()

	yaml := minimalValidYAML(t, func(y *minimalVertical) {
		y.TaxTreatment = "exempt" // not in {input_gst, tds_applicable, none}
	})

	errs, err := Validate([]byte(yaml))
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var found bool
	for _, e := range errs {
		if strings.Contains(e.Field, "tax_treatment") {
			found = true
			assert.Contains(t, e.Message, "exempt")
		}
	}
	assert.True(t, found, "expected tax_treatment validation error")
}

func TestValidate_ValidTaxTreatments_AllAccepted(t *testing.T) {
	t.Parallel()

	for _, treatment := range []string{"input_gst", "tds_applicable", "none"} {
		treatment := treatment
		t.Run(treatment, func(t *testing.T) {
			t.Parallel()

			yaml := minimalValidYAML(t, func(y *minimalVertical) {
				y.TaxTreatment = treatment
			})

			errs, err := Validate([]byte(yaml))
			require.NoError(t, err)

			for _, e := range errs {
				assert.False(t, strings.Contains(e.Field, "tax_treatment"),
					"tax_treatment=%q should be accepted but got error: %s", treatment, e.Message)
			}
		})
	}
}

// ── 4. Duplicate phase labels ────────────────────────────────────────────────

func TestValidate_DuplicatePhaseLabels(t *testing.T) {
	t.Parallel()

	data := readTestData(t, "invalid-duplicate-phase-labels.yaml")
	errs, err := Validate(data)
	require.NoError(t, err)
	require.NotEmpty(t, errs)

	var hasDupLabelErr bool
	for _, e := range errs {
		if e.Field == "vertical.phase_types" && strings.Contains(e.Message, `duplicate name "Review"`) {
			hasDupLabelErr = true
		}
	}
	assert.True(t, hasDupLabelErr,
		"should reject duplicate phase label 'Review'; got errors: %s", formatValidationErrors(errs))
}

// ── 5. Unknown fields are graceful ───────────────────────────────────────────

// TestValidate_UnknownFieldsGraceful verifies that extra fields not present in
// the schema neither panic nor cause a parse error. yaml.v3 silently ignores
// unknown fields by default; this test documents and locks in that behaviour.
func TestValidate_UnknownFieldsGraceful(t *testing.T) {
	t.Parallel()

	// Insert an unrecognised top-level key inside the vertical block.
	yaml := strings.Replace(
		string(readTestData(t, "movie-production.yaml")),
		"  id: movie-production",
		"  id: movie-production\n  unknown_future_field: some_value\n  another_unknown: 42",
		1,
	)

	// Must not panic, must not return a parse error.
	errs, err := Validate([]byte(yaml))
	require.NoError(t, err, "unknown fields must not cause a parse error")
	assert.Empty(t, errs, "unknown fields must not cause validation errors")
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// minimalVertical holds the mutable parts of a minimal-but-valid YAML fixture.
type minimalVertical struct {
	RateUnit     string
	TaxTreatment string
}

// minimalValidYAML returns a YAML string for a complete minimal vertical,
// with defaults overridden by the mutate function. This avoids repeating 80
// lines of YAML in every test case that only changes one field.
func minimalValidYAML(t *testing.T, mutate func(*minimalVertical)) string {
	t.Helper()

	opts := minimalVertical{
		RateUnit:     "day",
		TaxTreatment: "none",
	}
	if mutate != nil {
		mutate(&opts)
	}

	return `vertical:
  id: test-vertical
  name: Test Vertical
  version: 1.0.0
  description: Minimal valid vertical for testing.

  entity_labels:
    project: Project
    project_plural: Projects
    phase: Phase
    phase_plural: Phases
    team_member: Member
    team_member_plural: Members
    rate_label: Rate
    rate_unit: ` + opts.RateUnit + `

  phase_types:
    - id: active
      label: Active
      order: 1
      is_billable: true
      allowed_transitions: [done]
    - id: done
      label: Done
      order: 2
      is_billable: false
      allowed_transitions: []

  budget_categories:
    - id: general
      label: General
      description: General costs
      default_account_code: "5000"

  budget_templates:
    - name: Standard
      description: Default template
      line_items:
        - category_id: general
          description: General
          account_code: "5000"
          is_required: true

  expense_categories:
    - id: misc
      label: Miscellaneous
      tax_treatment: ` + opts.TaxTreatment + `
      default_account_code: "5000"
      requires_po: false

  approval_workflow:
    limits:
      - role: manager
        max_amount: 100000
    dual_approval_above: 500000

  inventory_categories:
    - id: equipment
      label: Equipment
      is_trackable: true

  default_chart_of_accounts:
    - code: "5000"
      name: General Expenses
      account_type: expense
      parent_code: null

  report_definitions:
    - id: summary
      name: Summary
      description: Basic
      data_sources: [budget-planning]
      default_columns: [total]
`
}

func formatValidationErrors(errs []ValidationError) string {
	var sb strings.Builder
	for _, e := range errs {
		sb.WriteString("  • ")
		sb.WriteString(e.Field)
		sb.WriteString(": ")
		sb.WriteString(e.Message)
		sb.WriteString("\n")
	}
	return sb.String()
}
