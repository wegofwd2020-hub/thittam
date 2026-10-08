# Change: Onboard US software studios (first tenant: Kaundinya Labs LLC)

## Why

Thittam is being dogfooded as the management-accounting system for
Kaundinya Labs LLC, a single-member Michigan LLC whose books open on
20 Oct 2026. Three gaps blocked that:

1. The vertical schema's `tax_treatment` enum only knows Indian GST/TDS, so a
   US vertical cannot describe 1099-NEC contractors, sales/use tax or the 50%
   meals limit.
2. No vertical fits a US product studio: `software-development` is an INR,
   client-services (agency) model with a GST/TDS chart of accounts.
3. The only tenant-creation paths are the `CreateTenant` gRPC saga (needs the
   full service mesh running) or hand-edited seed SQL with `<PLACEHOLDER>`s,
   which is error-prone, unvalidated, and would put a real business address
   into a public repository.

## What Changes

- **vertical-tax-treatment**: add `us_1099_nec`, `us_sales_tax_paid`,
  `us_use_tax`, `us_meals_50pct` to the closed enum (Go validator,
  `schema.json`, web badge). Existing values are unchanged.
- **software-development-us vertical**: new YAML with product-lifecycle
  stages, USD amounts and a Schedule C-aligned chart of accounts; registered
  by a *generated* shared migration (`007`).
- **tenant-onboarding**: new Python tool `tools/tenant-onboard` that
  - validates a `company.yaml` (+ git-ignored `company.local.yaml`) against
    DB constraints and the bound vertical,
  - generates ordered, idempotent seed SQL into `build/seeds/<slug>/`,
  - generates vertical migrations from YAML and detects drift.
- `seeds/tenants/kaundinya-labs/` — the first real tenant definition.
- Make targets: `tenant-validate`, `tenant-generate`, `seed-tenant`,
  `vertical-migrations`, `check-vertical-migrations`, `test-tenant-onboard`.
- **Fix**: `seeds/template/new-tenant/002_admin_user.sql` used
  `ON CONFLICT (user_id, role_id)`, which errors since iam migration 012
  replaced that key with an expression index.

## Impact

- Affected specs: `vertical-tax-treatment` (modified), `tenant-onboarding` (new)
- Affected code: `pkg/vertical/{validator.go,schema.json,types.go}`,
  `pkg/vertical/configs/software-development-us.yaml`,
  `migrations/shared/007_*`, `web/src/components/expenses/tax-treatment-badge.tsx`,
  `tools/tenant-onboard/**`, `seeds/tenants/**`, `Makefile`, `.gitignore`
- Not breaking: enum is additive; no existing vertical or tenant changes.
- Known limitation carried forward: `productions.status` is still the movie
  lifecycle; stages are mapped onto it (docs/multi-tenancy.md §7).
