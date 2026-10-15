## ADDED Requirements

### Requirement: Entity-Driven Owner Equity

A company config MAY declare an `entity:` block with a `tax_classification`
and a mapping of `members`, each with a `label`, an optional `name`, and an
`ownership_pct`. When present, the ledger seed SHALL append owner-equity
accounts derived from it after the vertical's accounts. A vertical whose chart
leaves owner equity to the tenant SHALL require the block.

#### Scenario: Partnership accounts
- **WHEN** `tax_classification` is `partnership_1065` with members in file order
- **THEN** `004_ledger.sql` creates for member *n* `30n0 Capital — …`, `31n0 Distributions — …` and `605n Guaranteed Payments — …`
- **AND** creates `3910 Current-Year Earnings (clearing)` and `6050 Guaranteed Payments to Partners` under `6000`

#### Scenario: Single owner
- **WHEN** `tax_classification` is `disregarded_schedule_c` with one member
- **THEN** `004_ledger.sql` creates `3100 Owner's Contributions` and `3200 Owner's Draws` under `3000`

#### Scenario: Names stay private
- **WHEN** `company.local.yaml` supplies `entity.members.<key>.name`
- **THEN** generated account names use that name instead of the label
- **AND** the committed `company.yaml` holds labels only

#### Scenario: Invalid ownership
- **WHEN** percentages do not total exactly 100, a share is not in (0, 100], a partnership has fewer than 2 or more than 9 members, or a single owner has more than one
- **THEN** validation exits 2 naming each field

#### Scenario: Unsupported classification
- **WHEN** `tax_classification` is `s_corp_1120s` or `c_corp_1120`
- **THEN** validation exits 2 saying it is not supported yet

#### Scenario: Block absent
- **WHEN** a tenant of a vertical that defines its own equity omits `entity:`
- **THEN** the generated seed is unchanged
- **WHEN** a tenant of `software-development-us` omits it
- **THEN** validation exits 2 naming `entity`

#### Scenario: Accounts fit the vertical
- **WHEN** a generated code clashes with a vertical account, or a needed parent is missing or of another type
- **THEN** validation exits 2 describing the conflict

### Requirement: Legacy Single-Owner Accounts Corrected In Place

For a tenant first seeded from `software-development-us` v1.0.0, the ledger
seed SHALL rename `3000 Owner's Capital` to `Equity` and `2300 Due to Owner
(reimbursable)` to `Due to Owners (reimbursable)`, and for a partnership SHALL
deactivate `3100 Owner's Contributions` and `3200 Owner's Draws`. Each
statement SHALL match on code and the old name, SHALL NOT delete rows, and
SHALL be a no-op on re-run.

#### Scenario: Re-seed after the upgrade
- **WHEN** a v1.0.0-seeded partnership tenant is re-seeded twice
- **THEN** member accounts exist, `3100`/`3200` are inactive with history kept, and the second run changes nothing

#### Scenario: Renamed in the app
- **WHEN** an owner renamed `3000` in the app before re-seeding
- **THEN** the seed leaves that name alone

## MODIFIED Requirements

### Requirement: Generated Vertical Migrations

The tool SHALL render a vertical YAML into an idempotent `vertical_definitions`
upsert migration and SHALL detect drift between the YAML and the committed
migration. When given `--restores <earlier up.sql>`, the down migration SHALL
restore that earlier definition instead of deleting the vertical. Only the
newest migration per vertical SHALL be generated; earlier ones are frozen.

#### Scenario: Drift detected
- **WHEN** the committed migration differs from what the YAML generates
- **THEN** `check-vertical-migration` exits 1 and prints a unified diff

#### Scenario: Child account before parent
- **WHEN** a vertical lists an account before its `parent_code`
- **THEN** generation fails with exit 2 explaining the ordering rule

#### Scenario: Update migration rolls back to the previous version
- **WHEN** `008` is generated with `--restores 007…up.sql` and later rolled back
- **THEN** the vertical is restored to the `007` definition and stays registered

#### Scenario: Wrong file to restore
- **WHEN** `--restores` names a file that is missing or registers another vertical
- **THEN** generation exits 2
