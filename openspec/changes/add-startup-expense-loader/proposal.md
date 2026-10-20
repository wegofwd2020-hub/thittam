# Change: Start-up expense loader (reimbursable liabilities)

## Why

Costs paid before a multi-member LLC's books open are not capital. Under the
operating agreement they are reimbursable to the member who paid them. The
tenant seed has no way to load them, and the finance owner keeps the extract by
hand. The loader turns a reviewed expense extract into draft ledger entries
that the owner can post, with the gates the owner set in place:

- only `candidate` and `candidate-verify` rows load;
- `excluded-*` and `pending*` rows never load;
- rows held for owner confirmation never load, whatever their status;
- an undecided attribution (`TBD`) blocks the whole load;
- the loaded total must reconcile to the expected total, and is never forced.

## What Changes

- **New module** `tools/tenant-onboard/tenant_onboard/startup.py` (Python):
  explicit CSV schema checks, per-row rejection with line numbers, status and
  hold filtering, attribution parsing, `Decimal` arithmetic, reconciliation,
  and SQL rendering.
- **New CLI command** `startup-load`: dry run by default. Prints a line-by-line
  report and exits `2` when a gate blocks. `--write --out <dir>` generates
  `005_startup_expenses.sql` and `startup_manifest.csv`, only when every gate
  passes.
- **Ledger output**: one draft `journal_entries` row per loaded expense, with
  balanced `journal_lines`. Debit `1310` Start-up & Organizational Costs for
  the company share. Credit `2300` Due to Owners (reimbursable), tagged to the
  owed member's `parties` record. Currency is set to `USD` explicitly, because
  the column default is `INR`. Entries are idempotent: `entry_number` is the
  key, and re-running inserts nothing.
- **Amounts**: the original amount and the company share are both kept. The
  share is `amount × attribution`, rounded half-up to cents. The manifest
  carries both; the narration carries both.
- **Not in scope**: posting entries, choosing `1310` sub-buckets (§195 vs §709
  waits on the CPA), attribution decisions, and any tracked tenant data.

## Impact

- `tools/tenant-onboard/` (new module, CLI subcommand, tests, mock fixture).
- `openspec/changes/add-startup-expense-loader/` (this change).
- No migrations, no Go code, no changes to the vertical or existing seeds.
- Generated output goes to `build/` (git-ignored). The real extract and the
  owed member's name never enter tracked files.
