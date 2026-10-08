## ADDED Requirements

### Requirement: Company Configuration Validation

The tool SHALL validate a tenant's `company.yaml`, deep-merged with an
optional `company.local.yaml`, and SHALL report every problem in one run,
each located by a dotted field path.

#### Scenario: Valid config
- **WHEN** `tenant_onboard validate --config <company.yaml>` runs on a complete config
- **THEN** it exits 0 and prints the product, account and open-period counts

#### Scenario: Placeholder left unfilled
- **WHEN** any merged value still contains `REPLACE_ME`
- **THEN** it exits 2 and names each such field

#### Scenario: Constraint violations caught before the database
- **WHEN** the config has an invalid UUID, slug, plan, ISO currency or country code, US ZIP, email, or date
- **THEN** it exits 2 listing each violation

#### Scenario: Vertical cross-check
- **WHEN** a product uses a stage the bound vertical does not define, or a stage with no `productions.status` mapping
- **THEN** it exits 2 naming the product and stage

### Requirement: Private Data Stays Out of Version Control

Street address and login email for a real tenant SHALL be supplied only via
`seeds/tenants/<slug>/company.local.yaml`, which SHALL be git-ignored, and
the admin password SHALL only ever be supplied as a bcrypt hash at apply time.

#### Scenario: Committed config is public-safe
- **WHEN** the repo test-suite runs
- **THEN** the committed `company.yaml` has `REPLACE_ME` for address and email
- **AND** `.gitignore` covers `seeds/tenants/*/company.local.yaml`

#### Scenario: Hash missing or malformed at apply time
- **WHEN** `002_admin.sql` is applied without `-v admin_password_hash=…`, or with a value that is not a bcrypt hash
- **THEN** psql exits non-zero before any row is written

### Requirement: Idempotent Seed Generation

`tenant_onboard tenant` SHALL write `001_tenant.sql`, `002_admin.sql`,
`003_products.sql`, `004_ledger.sql`; each SHALL run in one transaction with
`ON_ERROR_STOP`, and re-applying all four SHALL succeed without changes.

#### Scenario: Re-run is a no-op
- **WHEN** the four files are applied twice to a migrated database
- **THEN** both runs succeed and row counts are unchanged after the second

#### Scenario: Chart of accounts hierarchy
- **WHEN** `004_ledger.sql` is applied
- **THEN** every vertical account exists for the tenant with its parent resolved by code
- **AND** accounting periods are open from the opening month through fiscal year end

#### Scenario: Composes with pre-existing accounts
- **WHEN** a parent account already exists with a different id
- **THEN** children are linked to that existing account

### Requirement: Generated Vertical Migrations

The tool SHALL render a vertical YAML into an idempotent `vertical_definitions`
upsert migration and SHALL detect drift between the YAML and the committed
migration.

#### Scenario: Drift detected
- **WHEN** the committed migration differs from what the YAML generates
- **THEN** `check-vertical-migration` exits 1 and prints a unified diff

#### Scenario: Child account before parent
- **WHEN** a vertical lists an account before its `parent_code`
- **THEN** generation fails with exit 2 explaining the ordering rule

### Requirement: CLI Exit Codes

The CLI SHALL use exit codes 0 (ok), 1 (check failed), 2 (bad input),
3 (cannot write output), 70 (unexpected internal error, traceback with `--debug`).

#### Scenario: Unwritable output
- **WHEN** `--out` points at an existing file
- **THEN** it exits 3 and leaves no partial `.sql` files
