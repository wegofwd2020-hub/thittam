# Tasks

## 1. Loader
- [x] 1.1 `startup.py`: header check, per-row checks, rejection with line numbers
- [x] 1.2 Status filter, held vendors, attribution parsing, `Decimal` arithmetic
- [x] 1.3 Reconciliation over candidate total (loadable + TBD), never forced
- [x] 1.4 SQL rendering: draft entries, USD, party-tagged credit, idempotent key
- [x] 1.5 CLI `startup-load`: dry run, gates, `--write --out`

## 2. Tests
- [x] 2.1 Mock fixture (`tests/fixtures/startup_mock.csv`, fictitious data only)
- [x] 2.2 Parsing, bad rows, status filter, duplicates, refunds, 0%, TBD, held rows
- [x] 2.3 Reconciliation match and mismatch; rounding; entry keys; SQL shape
- [x] 2.4 CLI gates: TBD blocks with no output, clean dry run, write requires `--out`
- [x] 2.5 Disposable PostgreSQL: migrations, seed, apply twice, identical data

## 3. Owner decisions (pending, not resolved in code)
- [ ] 3.1 Attribution for every TBD row (shared tools and hosting)
- [ ] 3.2 Confirmation of the June 8 and June 23 Secretary of State charges
- [ ] 3.3 Verification of LARA receipt 1237526 on the statement
- [ ] 3.4 October 16–20 charges (not yet provided)
- [ ] 3.5 Hennessey CPA invoice: record when paid (cash method)
- [ ] 3.6 CPA confirmation of the §195 and §709 buckets for `1310`

## 4. Before the real load
- [ ] 4.1 Owner sets every TBD attribution; `startup-load` dry run exits `0`
- [ ] 4.2 Candidate total reconciles to the expected total with zero difference
- [ ] 4.3 Generate with `--write`, apply to the tenant after the seed, review drafts in the app
- [ ] 4.4 Post entries only after the owner approves; never commit the output
