# Design: US studio onboarding

## Context

Tenant data in local/dev deployments lives in `public` with `tenant_id`
columns; vertical definitions live as JSONB in `vertical_definitions` and are
read per request (`vd.config || tv.config_override`). The ledger's
`SeedChartOfAccounts` resolves `parent_code` by looking up already-created
accounts, so CoA order matters.

## Goals / Non-Goals

- Goals: one command to (re)load a real tenant safely; no private data in git;
  generated artefacts can't silently drift from their YAML source.
- Non-goals: replacing the `CreateTenant` saga; fixing the `productions.status`
  CHECK constraint; posting opening balances (done in the app with real numbers).

## Decisions

- **Generate, don't hand-write, the vertical migration.** The YAML stays the
  single source of truth; `check-vertical-migrations` fails CI on drift.
  JSON is pretty-printed so migration diffs are reviewable.
- **Seed SQL is generated into `build/` and never committed.** The committed
  inputs are `company.yaml` (public-safe) and the generator.
- **Private fields via `company.local.yaml` (git-ignored), deep-merged.**
  `REPLACE_ME` markers in the public file make a missing override fail with a
  precise message instead of loading placeholder data.
- **Password hash supplied at apply time** as a psql variable, checked for
  bcrypt shape inside the SQL. Make reads it from the environment because make
  would otherwise expand the `$` characters in a bcrypt hash.
- **Parents resolved by code** (`SELECT id FROM accounts WHERE code = …`), not
  by generated UUID, so the seed composes with accounts the ledger service may
  already have created.
- **Idempotency**: every insert has `ON CONFLICT … DO NOTHING`; each file is one
  transaction under `ON_ERROR_STOP`.
- **Stage → status mapping** lives in the generator with a per-tenant override
  (`stage_status_map`) until `productions.status` becomes vertical-aware.
- **Python** for the tool (owner's standard for new application code); Go
  remains for the in-service validator because it runs inside the services.

## Risks / Trade-offs

- Seed path bypasses `CreateTenant` side effects (e.g. NATS events, billing
  saga). Acceptable for a single self-hosted tenant; documented in the runbook.
- Role permissions are duplicated from the template; `test_repo_drift.py`
  fails if they diverge.
- Schedule C line mapping is advisory; the CPA confirms it.

## Migration Plan

1. `make migrate-all` (applies shared `007`).
2. Create `company.local.yaml`; `make tenant-validate TENANT=kaundinya-labs`.
3. `ADMIN_PASSWORD_HASH='…' make seed-tenant TENANT=kaundinya-labs`.

Rollback: `007.down.sql` refuses while a tenant is bound (FK) — by design.
