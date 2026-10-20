## ADDED Requirements

### Requirement: Explicit extract schema

The loader SHALL read the expense extract CSV with an explicit header check.
The required columns are `date, vendor, reference, description, amount_usd,
status, attribution, source, notes`. Extra columns SHALL be ignored.

#### Scenario: Required column missing
- **WHEN** the extract lacks any required column
- **THEN** the loader exits `2` naming the missing columns and loads nothing

#### Scenario: File unreadable or malformed
- **WHEN** the file is missing, not UTF-8, or not valid CSV
- **THEN** the loader exits `2` with the file path and reason, and no output is written

### Requirement: Row rejection with line numbers

A row with a missing or malformed `date`, `amount_usd` or `status` SHALL be
rejected and reported with its file line number. A `status` outside the known
set SHALL be rejected. Rejection of one row SHALL NOT stop the others.

#### Scenario: Missing date
- **WHEN** a row has an empty `date`
- **THEN** the report lists `line N: missing date`

#### Scenario: Unknown status
- **WHEN** a row has a status that is neither loadable nor a known skip status
- **THEN** the report lists `line N: unknown status '<value>'`

### Requirement: Status and hold filtering

Only `candidate` and `candidate-verify` rows SHALL load. Rows whose status
begins with `excluded-` or `pending` SHALL NOT load. Rows whose vendor begins
with `Michigan Secretary of State` SHALL NOT load under any status until the
owner confirms them.

#### Scenario: Excluded and pending rows
- **WHEN** a row has status `excluded-duplicate`, `excluded-net-zero`, or `pending-unpaid`
- **THEN** the row is skipped and the report gives its status as the reason

#### Scenario: Held vendor
- **WHEN** a Secretary of State row has status `candidate`
- **THEN** the row is skipped with reason `held: owner must confirm before loading`

### Requirement: Attribution

Attribution SHALL be a percentage of the amount, `0%` to `100%`, with up to
four decimal places. A `0%` row SHALL be skipped. A row whose attribution is
`TBD` SHALL block the load: the loader SHALL list every such row and SHALL NOT
choose a percentage. A loadable row whose attribution is neither a percentage
nor `TBD` SHALL be rejected.

#### Scenario: TBD blocks loading
- **WHEN** any loadable row has attribution `TBD`
- **THEN** the loader lists every TBD line, exits `2`, and writes no SQL or manifest

#### Scenario: Partial attribution
- **WHEN** a loadable row has amount `45.00` and attribution `50%`
- **THEN** the company share is `22.50`
- **AND** the original amount `45.00` is kept in the manifest and the narration

#### Scenario: Rounding
- **WHEN** the company share has more than two decimal places
- **THEN** it is rounded half-up to cents using decimal arithmetic

### Requirement: Reconciliation

The loader SHALL total the candidate original amounts (loadable rows plus TBD
rows) and compare the total with an expected value. The default expected
value is the v2 candidate total, `1752.57`, and `--expect-total` overrides it.
A difference SHALL block the load and SHALL be reported line by line. The
loader SHALL NOT adjust any amount or add a plug to force a match.

#### Scenario: Mismatch
- **WHEN** the candidate total differs from the expected total
- **THEN** the report shows the difference and every loadable and TBD line, and the loader exits `2`

#### Scenario: Match
- **WHEN** the candidate total equals the expected total and no TBD rows remain
- **THEN** the dry run reports `READY` and exits `0`

### Requirement: Reimbursable-liability entries

Each loaded row SHALL become one draft journal entry with two lines: a debit
to account `1310` for the company share, and a credit to account `2300` for
the same amount. The credit line SHALL be tagged to the owed member's party
record. Every line SHALL use currency `USD`. Entries SHALL NOT be booked as
capital contributions.

#### Scenario: Balanced draft
- **WHEN** the loader writes SQL
- **THEN** each entry has debit total equal to credit total, status `draft`, and currency `USD` on every line

#### Scenario: Owed member without a name
- **WHEN** the owed member has no `name` in the local override
- **THEN** rendering fails with a message naming the key to set

### Requirement: Idempotent load

Each entry SHALL carry `entry_number` `SU-<YYYYMMDD>-<reference with non-alphanumerics removed>`,
and the insert SHALL use `ON CONFLICT (entry_number) DO NOTHING`. Lines SHALL
insert only when their entry was new. Applying the same SQL twice SHALL leave
the data unchanged.

#### Scenario: Re-apply
- **WHEN** the generated SQL is applied twice
- **THEN** the second run inserts no entries and no lines

#### Scenario: Key collision
- **WHEN** two loadable rows produce the same `entry_number`
- **THEN** rendering fails naming both lines

### Requirement: Privacy

The loader SHALL NOT write the EIN, the address, or member names into any
tracked file, fixture, or test. Tests SHALL use the mock fixture only. Generated
output SHALL be written to a git-ignored directory.

#### Scenario: Tests
- **WHEN** the test suite runs
- **THEN** it reads only `tests/fixtures/` files
