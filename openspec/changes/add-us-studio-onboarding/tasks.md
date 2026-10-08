# Tasks

## 1. Tax treatment enum
- [x] 1.1 Add US values to `validTaxTreatments`; derive error message from the set
- [x] 1.2 Update `schema.json` enum and description
- [x] 1.3 Tests: accepted values, message lists US values, schema ↔ validator parity
- [x] 1.4 Web badge labels for the new values

## 2. software-development-us vertical
- [x] 2.1 Author YAML (stages, Schedule C CoA, USD amounts, 1099/meals/use-tax categories)
- [x] 2.2 Passes `make validate-verticals`
- [x] 2.3 Generated shared migration `007`; drift check target

## 3. tenant-onboard tool
- [x] 3.1 Exception hierarchy with aggregated validation issues
- [x] 3.2 Safe SQL literal rendering
- [x] 3.3 Company config load / local override merge / validation / cross-check
- [x] 3.4 Seed renderer (tenant, admin, products, ledger) — idempotent, transactional
- [x] 3.5 CLI with documented exit codes; `hash-password`
- [x] 3.6 Unit tests with mock fixtures; repo drift tests; PostgreSQL integration tests

## 4. Kaundinya Labs tenant
- [x] 4.1 `seeds/tenants/kaundinya-labs/company.yaml` + `.local.yaml.example`
- [x] 4.2 Make targets and `.gitignore`
- [x] 4.3 Fix `user_roles` conflict target in `seeds/template/new-tenant/002_admin_user.sql`
- [ ] 4.4 Owner fills `company.local.yaml` and loads the tenant
- [ ] 4.5 CPA confirms Schedule C mapping of 6210/6220
- [ ] 4.6 Wire `check-vertical-migrations` and `test-tenant-onboard` into CI
