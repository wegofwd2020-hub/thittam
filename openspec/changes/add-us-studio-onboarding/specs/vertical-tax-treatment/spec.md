## MODIFIED Requirements

### Requirement: Expense Category Tax Treatment

Each expense category in a vertical SHALL declare a `tax_treatment` drawn from
a closed set: `input_gst`, `tds_applicable`, `us_1099_nec`,
`us_sales_tax_paid`, `us_use_tax`, `us_meals_50pct`, `none`. The validator,
`schema.json` and the web badge SHALL agree on this set.

#### Scenario: US treatment accepted
- **WHEN** a vertical declares an expense category with `tax_treatment: us_1099_nec`
- **THEN** `Validate` reports no `tax_treatment` error

#### Scenario: Unknown treatment rejected with the allowed list
- **WHEN** a vertical declares `tax_treatment: vat_standard`
- **THEN** `Validate` reports an error on `vertical.expense_categories[i].tax_treatment`
- **AND** the message lists every allowed value, including the US values

#### Scenario: Schema and validator stay in lockstep
- **WHEN** the test-suite runs
- **THEN** the `tax_treatment` enum in `schema.json` equals the validator's set

#### Scenario: Existing verticals unaffected
- **WHEN** `make validate-verticals` runs after this change
- **THEN** every pre-existing vertical still validates without errors
