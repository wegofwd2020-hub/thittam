# Tasks

## 1. Vertical
- [x] 1.1 `software-development-us` v1.1.0: neutral equity, intangibles, amortization
- [x] 1.2 Schedule C + Form 1065 (TY2025) line annotations; `tax_return_rollup`
- [x] 1.3 Schema-valid; category account references still resolve
- [x] 1.4 Generated update migration `008`; `007` frozen; `--restores` down migration
- [ ] 1.5 `make validate-verticals` (Go) in CI

## 2. tenant-onboard
- [x] 2.1 `entity.py`: parsing, exact percentages, classification rules
- [x] 2.2 Generated equity / guaranteed-payment accounts; cross-check with vertical
- [x] 2.3 Legacy v1.0.0 account fixes (rename, deactivate) — idempotent
- [x] 2.4 CLI: entity in `validate` output; `--restores`
- [x] 2.5 Tests with mock fixtures; repo drift tests; PostgreSQL integration

## 3. Kaundinya Labs
- [x] 3.1 `entity:` block (labels), local example with names
- [x] 3.2 `CPA-REVIEW.md` reflects implemented chart; Form 1065 line 21 fix
- [ ] 3.3 Owner re-seeds local books (`make seed-tenant`)
- [ ] 3.4 Opening entry after CPA answers Q1 / Q2
- [ ] 3.5 K-1 Item L report
