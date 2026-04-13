# Vertical Plugin YAML Schema

> **Audience:** Developers authoring or extending vertical definitions  
> **Schema file:** `pkg/vertical/schema.json` (JSON Schema draft 2020-12)  
> **Validator:** `pkg/vertical/validator.go` — `Validate([]byte) ([]ValidationError, error)`  
> **Configs:** `pkg/vertical/configs/*.yaml` — one file per GA vertical  
> **Updated:** 2026-04-10

---

## Overview

Thittam's vertical plugin system allows the same platform to serve different
industries without code changes. Each industry is described by a YAML file that
defines the terminology, phases, budgeting categories, expense rules, and
reporting definitions used throughout the application.

A vertical YAML file must:

1. Live under `pkg/vertical/configs/` to be picked up by `make validate-verticals`.
2. Conform to the rules in `pkg/vertical/schema.json` and pass `Validate()`.
3. Use a globally unique `id` in kebab-case.

---

## Top-level structure

```yaml
vertical:
  id:          string       # required — kebab-case, e.g. "movie-production"
  name:        string       # required — human-readable
  version:     string       # required — semver e.g. "1.0.0"
  description: string       # optional

  entity_labels: { ... }             # required — all 8 fields
  phase_types: [ ... ]               # required — at least 1
  budget_categories: [ ... ]         # required — at least 1
  budget_templates: [ ... ]          # optional
  expense_categories: [ ... ]        # required — at least 1
  approval_workflow: { ... }         # optional
  inventory_categories: [ ... ]      # required — at least 1
  default_chart_of_accounts: [ ... ] # required — at least 1
  report_definitions: [ ... ]        # required — at least 1
  custom_fields: { ... }             # optional
```

---

## Field reference

### `vertical.id`

| Rule | Value |
|------|-------|
| Format | kebab-case: `[a-z][a-z0-9-]*` |
| Unique | Yes — must not collide with another registered vertical |
| Mutable | No — changing the ID after publication is a breaking change |

```yaml
id: movie-production
```

---

### `vertical.version`

Semantic version `MAJOR.MINOR.PATCH`. Increment:

- **MAJOR** — breaking change to phase graph, removed required field, or changed enum set.
- **MINOR** — new optional section or new enum value (backwards-compatible).
- **PATCH** — label corrections, description updates.

---

### `vertical.entity_labels`

All eight fields are **required**. They replace the generic UI labels throughout
the application.

| Field | Replaces | Example (movie) |
|-------|----------|-----------------|
| `project` | "Project" | "Production" |
| `project_plural` | "Projects" | "Productions" |
| `phase` | "Phase" | "Phase" |
| `phase_plural` | "Phases" | "Phases" |
| `team_member` | "Team Member" | "Crew Member" |
| `team_member_plural` | "Team Members" | "Crew Members" |
| `rate_label` | "Rate" | "Day Rate" |
| `rate_unit` | _(enum)_ | `day` |

**`rate_unit` enum:** `day` · `hour` · `month` · `fixed`

```yaml
entity_labels:
  project: Production
  project_plural: Productions
  phase: Phase
  phase_plural: Phases
  team_member: Crew Member
  team_member_plural: Crew Members
  rate_label: Day Rate
  rate_unit: day
```

---

### `vertical.phase_types`

Defines the project lifecycle as a **directed acyclic graph (DAG)**. Constraints:

- At least one phase required.
- IDs must be unique within the vertical (snake_case: `[a-z][a-z0-9_]*`).
- Labels must be unique within the vertical.
- `allowed_transitions` references other phase IDs — unknown IDs are rejected.
- Must have at least one **terminal state** (a phase with `allowed_transitions: []`).
- The graph must be **acyclic** — cycles are rejected.

```yaml
phase_types:
  - id: development
    label: Development
    order: 1
    is_billable: false
    allowed_transitions: [pre_production]
  - id: pre_production
    label: Pre-Production
    order: 2
    is_billable: false
    allowed_transitions: [production]
  - id: production
    label: Production
    order: 3
    is_billable: true
    allowed_transitions: [released]
  - id: released
    label: Released
    order: 4
    is_billable: false
    allowed_transitions: []   # ← terminal state
```

---

### `vertical.budget_categories`

Cost categories for building a production budget. At least one required.

- `id` must be unique within the vertical.
- `default_account_code` must reference a code in `default_chart_of_accounts`.

```yaml
budget_categories:
  - id: above_the_line
    label: Above the Line
    description: Key creative talent
    default_account_code: "5100"
```

---

### `vertical.budget_templates` _(optional)_

Pre-built budget structures presented in the "New Budget" wizard.

- `name` must be unique within the vertical.
- Each `line_items[].category_id` must reference an ID in `budget_categories`.
- Each `line_items[].account_code` must reference a code in `default_chart_of_accounts`.

```yaml
budget_templates:
  - name: Feature Film
    description: Standard feature film budget
    line_items:
      - category_id: above_the_line
        description: Director fee
        account_code: "5100"
        default_amount: 500000
        is_required: true
```

---

### `vertical.expense_categories`

Categories for submitted expense claims. At least one required.

- `id` must be unique within the vertical.
- `tax_treatment` **enum:** `input_gst` · `tds_applicable` · `none`
- `default_account_code` must reference `default_chart_of_accounts`.
- `requires_po: true` enforces that a purchase order must be linked before
  the expense can be submitted.

```yaml
expense_categories:
  - id: location_rental
    label: Location Rental
    tax_treatment: input_gst
    default_account_code: "5200"
    requires_po: true
```

---

### `vertical.approval_workflow` _(optional)_

Role-based approval limits. Money amounts in the `limits` array are denominated
in the tenant's base currency (typically INR).

```yaml
approval_workflow:
  limits:
    - role: coordinator
      max_amount: 200000
    - role: manager
      max_amount: 1000000
  dual_approval_above: 1000000
```

- `role` must match a system role name defined in `iam.systemRoles`.
- `dual_approval_above` — expenses above this threshold require two approvers.

---

### `vertical.inventory_categories`

Equipment and asset categories. At least one required.

- `is_trackable: true` enables individual checkout/check-in tracking for assets in this category.
- `depreciation_years` is optional; `null` means the category is not subject to depreciation.

```yaml
inventory_categories:
  - id: camera
    label: Camera Equipment
    is_trackable: true
    depreciation_years: 5
  - id: props
    label: Props
    is_trackable: false
    depreciation_years: null
```

---

### `vertical.default_chart_of_accounts`

Seed accounts created for every new tenant using this vertical. At least one required.

- `code` must be unique within the vertical.
- `account_type` **enum:** `asset` · `liability` · `equity` · `revenue` · `expense`
- `parent_code` references another account code (for sub-accounts); `null` = top-level.

All `default_account_code` values in `budget_categories` and `expense_categories`
must appear in this list — the validator cross-checks this.

```yaml
default_chart_of_accounts:
  - code: "5100"
    name: Above the Line Expenses
    account_type: expense
    parent_code: null
```

---

### `vertical.report_definitions`

Report types available in the reporting-analytics service for this vertical.
At least one required.

```yaml
report_definitions:
  - id: cost_report
    name: Daily Cost Report
    description: Budget vs actuals by category
    data_sources: [budget-planning, expense-tracking]
    default_columns: [category, budget, actual, variance]
```

---

### `vertical.custom_fields` _(optional)_

Additional input fields that appear on project creation forms and expense
submission forms.

- `type` **enum:** `text` · `number` · `date` · `select` · `boolean`
- `options` is required (and only meaningful) when `type: select`.

```yaml
custom_fields:
  project:
    - key: call_sheet_time
      label: Call Sheet Time
      type: text
      required: false
  expense:
    - key: vendor_gstin
      label: Vendor GSTIN
      type: text
      required: true
```

---

## Validation rules summary

| Rule | Validator check |
|------|----------------|
| All required fields present | `Validate()` returns `ValidationError` per missing field |
| `rate_unit` is one of `day\|hour\|month\|fixed` | `validateEntityLabels()` |
| `tax_treatment` is one of `input_gst\|tds_applicable\|none` | `Validate()` loops |
| Phase IDs are unique within the vertical | `checkDuplicateIDs()` |
| Phase labels are unique within the vertical | `checkDuplicateNames()` |
| Phase graph is a DAG (no cycles) | `validatePhaseTransitions()` — DFS |
| Phase graph has at least one terminal state | `validatePhaseTransitions()` |
| Unknown `allowed_transitions` target rejected | `validatePhaseTransitions()` |
| Budget/expense `default_account_code` in chart | cross-reference in `Validate()` |
| Template `category_id` in `budget_categories` | cross-reference in `Validate()` |
| Template `account_code` in chart | cross-reference in `Validate()` |
| Unknown fields in YAML | **silently ignored** (yaml.v3 default) |

---

## Adding a new vertical

1. Copy `pkg/vertical/configs/movie-production.yaml` as a starting point.
2. Change `id`, `name`, `version`, and all industry-specific labels/categories.
3. Run `make validate-verticals` locally — this is the same check CI runs.
4. Add the new vertical ID to `TestValidateAllGAVerticalsSmokeTest` in
   `pkg/vertical/schema_test.go` once it reaches GA status.
5. Open a PR; the `validate-verticals` CI job will run automatically.

---

## JSON Schema

The machine-readable schema lives at `pkg/vertical/schema.json`
(JSON Schema draft 2020-12). It can be used with any JSON Schema validator or
IDE plugin for inline YAML validation.

VSCode example (`.vscode/settings.json`):

```json
{
  "yaml.schemas": {
    "./pkg/vertical/schema.json": "pkg/vertical/configs/*.yaml"
  }
}
```

---

## GA verticals

| File | ID | Rate unit |
|------|----|-----------|
| `movie-production.yaml` | `movie-production` | `day` |
| `software-development.yaml` | `software-development` | `month` |
| `construction.yaml` | `construction` | `day` |
| `events-management.yaml` | `events-management` | `day` |
