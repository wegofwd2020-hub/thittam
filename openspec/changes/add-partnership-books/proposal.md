# Change: Partnership books for multi-member LLCs (Form 1065)

## Why

Kaundinya Labs LLC has two members, 50/50, and its CPA confirmed on
15 Oct 2026 that it files **Form 1065** with a Schedule K-1 per member. The
`software-development-us` vertical (v1.0.0) seeded a single-owner chart:
`3000 Owner's Capital`, `3100 Owner's Contributions`, `3200 Owner's Draws`,
with expenses annotated only with Schedule C lines. A partnership needs a
capital and a distribution account per member, a home for guaranteed
payments (Form 1065 line 10), and a mapping to Form 1065 page 1. The CPA
review packet (#247) proposed the accounts; this change implements them.

Owner equity depends on the entity, not the industry, so it does not belong
in a vertical: putting Member A/B accounts in the vertical would give every
future tenant of it the same two members.

## What Changes

- **software-development-us v1.1.0** (new generated migration `008`):
  - equity is entity-neutral: `3000 Equity` (parent) and `3900 Retained
    Earnings`; `3100`/`3200` move to the tenant tool;
  - adds `1300 Intangible Assets`, `1310 Software & Intellectual Property`,
    `1320 Start-up & Organizational Costs`, `1390 Accumulated Amortization`,
    `6860 Amortization` for the contributed IP and §195/§709 costs;
  - every revenue and expense account is annotated with its Schedule C line
    **and** its tax year 2025 Form 1065 line;
  - `2300` renamed `Due to Owners (reimbursable)`; report
    `schedule_c_rollup` renamed `tax_return_rollup`.
- **tenant-onboarding**: optional `entity:` block in `company.yaml`
  (`tax_classification`, `members` with exact `ownership_pct`), required for
  `software-development-us`. The ledger seed appends per-member capital,
  distribution and guaranteed-payment accounts, and corrects accounts left by
  the v1.0.0 chart (rename `3000`/`2300`; deactivate `3100`/`3200` for a
  partnership), matching on code and old name.
- **Vertical update migrations**: `vertical-migration --restores <earlier
  up.sql>` renders a down migration that restores the earlier definition
  instead of deleting the vertical. Migration `007` is frozen.
- `seeds/tenants/kaundinya-labs`: `entity:` block (labels only; names via the
  git-ignored local file), updated `CPA-REVIEW.md` and README.

## Impact

- Affected specs: `tenant-onboarding` (modified, added requirements).
- Affected code: `pkg/vertical/configs/software-development-us.yaml`,
  `migrations/shared/008_*`, `tools/tenant-onboard/**`, `Makefile`,
  `seeds/tenants/**`.
- Not breaking for other verticals: without `entity:` the seed is unchanged.
- **Breaking for `software-development-us` tenants**: `entity:` is now
  required; the only such tenant (Kaundinya Labs) is updated here.
- Deferred: opening entry (waits on CPA Q1/Q2), K-1 Item L report, year-end
  allocation of `3910`, S-corp / C-corp classifications.
