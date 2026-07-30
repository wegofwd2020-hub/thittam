# Deploying permission-gating changes (#166)

A change that adds or moves an `interceptor.RequirePermission("X")` gate depends on permission `X`
existing on the relevant roles. Two audiences need it:

- **New tenants** get it from `systemRoles` (`services/iam/service.go`) at `CreateTenant`.
- **Existing tenants** get it only when a `migrations/iam` backfill runs
  (`UPDATE roles … array_append(permissions, 'X') …`).

## The rule

**Run `make migrate-all` before rolling the new service code.** Deploying code-first makes every
existing tenant return `PermissionDenied` on the newly-gated RPCs until the backfill migration runs —
migration-first is harmless (the permission simply exists before anything checks it).

## What the automated checks do and do not cover

- `services/iam/authz_coverage_test.go` (rides `go test ./...`) guarantees that for every gated
  permission a grant EXISTS — it is in `systemRoles`, and every non-founding one has a backfill
  migration. It does NOT guarantee the migration has RUN in a given environment. Ordering is still the
  operator's responsibility.
- CI `Migration Validate (up + down)` runs against a fresh EMPTY database, so a data migration like
  `020_seed_read_permissions` executes against zero rows. It validates SQL syntax and that `down` does
  not error — NOT that the grant reaches any real tenant's roles. Semantic coverage lives in the slice
  integration tests, not that job.
