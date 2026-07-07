# Design: Finish #89 — enforce UNIQUE on `tenants.name` (case-insensitive, whitespace-normalized)

**Issue:** [#89](https://github.com/wegofwd2020-hub/thittam/issues/89)
**Date:** 2026-07-07
**Scope:** iam service only
**Branch:** `feat/iam-tenant-name-unique-89`

## Context

Issue #91 (CLOSED) already shipped the core of case-insensitive tenant-name
uniqueness:

- Migration `015_tenants_unique_name` — `CREATE UNIQUE INDEX tenants_name_ci_unique ON tenants (lower(trim(name)))`
- `Service.CreateTenant` normalizes the name with `strings.Join(strings.Fields(name), " ")` (trim + collapse internal whitespace, preserve case) — `services/iam/service.go:437`
- `postgres.go` maps pg unique-violation `23505` on constraint `tenants_name_ci_unique` to the domain sentinel `ErrTenantNameTaken` — `services/iam/db/postgres.go:326,341`
- `grpcError` maps `ErrTenantNameTaken` to `codes.AlreadyExists` — `services/iam/handler.go:661`
- Integration test `services/iam/db/tenant_name_unique_integration_test.go` covers case-insensitive / all-caps / leading-trailing-whitespace / distinct-names.

Issue #89 is a stricter superset of #91 whose acceptance criteria are **not**
met. This design closes the remaining gaps, staying inside the iam service.
The public self-serve registration path (`pkg/registration`) also bypasses this
enforcement — that is out of scope here (it is #89's "every layer" *spirit*, not
in its written checklist) and is noted as follow-up.

## Gaps being closed

1. **Whitespace-normalized index.** The index is `lower(trim(name))` — trim
   only. #89's own example wants `"acme  corp"` (double space) to collide with
   `"acme corp"`. `trim` does not collapse *internal* whitespace, so today they
   do not collide at the DB layer. The service collapses via `strings.Fields`,
   but any writer that bypasses the service (e.g. registration) can still insert
   both. The DB backstop must collapse internal whitespace too.
2. **Error must name the colliding tenant's UUID.** Current mapping returns the
   bare sentinel message. #89 requires the `ALREADY_EXISTS` error to name the
   existing tenant's UUID.
3. **Pre-flight duplicate check.** #89 wants the common path to return a clean
   `ALREADY_EXISTS` from a pre-insert lookup, with the raw `23505` only as the
   concurrent-race backstop. Today there is no pre-flight query.
4. **Concurrent-create race test.** #89's checklist requires a race test; none
   exists.
5. **Normalization unit test.** No unit test asserts the `strings.Fields`
   normalization or the `ErrTenantNameTaken` mapping.

## Component 1 — Migration `018_tenants_name_collapse_whitespace`

Rebuild the unique index to collapse internal whitespace, **keeping the same
index name** so `isUniqueViolationOn(err, "tenants_name_ci_unique")`
(`postgres.go:341`) continues to match.

`migrations/iam/018_tenants_name_collapse_whitespace.up.sql`:

```sql
DROP INDEX tenants_name_ci_unique;
CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (regexp_replace(lower(trim(name)), '\s+', ' ', 'g'));
```

`018_...down.sql` restores the trim-only index:

```sql
DROP INDEX tenants_name_ci_unique;
CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (lower(trim(name)));
```

Non-concurrent (`tenants` is a small, low-write table). Before applying,
`scripts/audit-tenant-name-collisions.sql` must be updated to group by the new
expression so any pre-existing internal-whitespace duplicates are surfaced
before `CREATE UNIQUE INDEX` fails. Seed data (`XYZ_CBA Productions Pvt. Ltd.`,
`XYZ Construction LLC`) does not collide under the new expression.

The new expression matches the service's `strings.Fields` collapse, so
application-normalized names and the DB index agree.

## Component 2 — Pre-flight check + UUID-naming error

**sqlc query** — `services/iam/db/queries.sql`:

```sql
-- name: FindTenantByNormalizedName :one
SELECT * FROM tenants
WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
    = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')
LIMIT 1;
```

Uses the unique index. Run `sqlc generate` to regenerate.

**Repository interface** (`services/iam/repository.go`) — add:

```go
FindTenantByNormalizedName(ctx context.Context, name string) (*Tenant, error)
```

Implement on the Postgres repo (`services/iam/db/postgres.go`) and on
`mockRepo` (`services/iam/service_test.go`). Returns `(nil, nil)` on no-rows so
callers treat "not found" as "free to create".

**Error constructor** (`services/iam/errors.go`) — keep the sentinel, add:

```go
// tenantNameTakenErr wraps ErrTenantNameTaken with the colliding tenant's
// UUID so the ALREADY_EXISTS surfaced to the caller names the existing tenant.
func tenantNameTakenErr(id uuid.UUID) error {
    return fmt.Errorf("%w (existing tenant %s)", ErrTenantNameTaken, id)
}
```

`errors.Is(err, ErrTenantNameTaken)` still matches, so `grpcError`
(`handler.go:661`) keeps mapping it to `codes.AlreadyExists`, now with the UUID
in the message.

**`Service.CreateTenant`** (`services/iam/service.go`) — after normalizing the
name and before insert, pre-flight:

```go
if existing, err := s.repo.FindTenantByNormalizedName(ctx, tenant.Name); err != nil {
    return nil, err
} else if existing != nil {
    return nil, tenantNameTakenErr(existing.ID)
}
```

**`postgres.go` race path** — where `isUniqueViolationOn(err, "tenants_name_ci_unique")`
is already handled, look up the colliding row and return `tenantNameTakenErr(id)`
instead of the bare sentinel, so both the common and race paths name the UUID.

## Component 3 — Tests

**Unit** (`services/iam/service_test.go`, `mockRepo`):

- Normalization: names with internal double-spaces, tabs, and leading/trailing
  whitespace → assert the persisted name is collapsed to single spaces.
- Pre-flight: `mockRepo.FindTenantByNormalizedName` returns an existing tenant →
  `CreateTenant` returns an error where `errors.Is(err, ErrTenantNameTaken)` is
  true **and** `err.Error()` contains the existing tenant's UUID.

**Integration** (`services/iam/db/tenant_name_unique_integration_test.go`,
`//go:build integration`, real Postgres via `pkg/testdb`):

- `_InternalWhitespaceCollapsed`: insert `"Acme  Corp"` (double space), then
  `"Acme Corp"` → the second insert fails with `23505` on
  `tenants_name_ci_unique` (proves migration 018's collapse).
- `TestCreateTenant_ConcurrentSameName_Race`: N goroutines create the same name
  concurrently → assert exactly one success and the rest map to
  `ErrTenantNameTaken`.

## Acceptance criteria (from #89)

- [x] Migration adds a case-insensitive, whitespace-normalized UNIQUE index — **collapse added by 018**
- [x] Existing duplicates resolved via pre-migration audit — **audit script updated to new expression; seed data clear**
- [x] `CreateTenant` returns `ALREADY_EXISTS` (not `INTERNAL`) naming the colliding tenant's UUID — **pre-flight + race path via `tenantNameTakenErr`**
- [x] Concurrent-create race → exactly one success, others `ALREADY_EXISTS` — **race integration test**
- [x] Table-driven test: exact / case-only / whitespace-only / race — **unit + integration**
- [ ] `tenant-onboarding.md` §7 gap entry removed — **docs repo change, tracked separately**

## Out of scope (follow-up)

- Public registration path (`pkg/registration`) name normalization + `23505`
  mapping — the "every layer" spirit of #89, but not in its written checklist.
- Fuzzy/soft-match duplicate detection ("Acme LLC" vs "Acme, LLC") — #89
  explicitly defers this.

## Files touched

- `migrations/iam/018_tenants_name_collapse_whitespace.up.sql` (new)
- `migrations/iam/018_tenants_name_collapse_whitespace.down.sql` (new)
- `scripts/audit-tenant-name-collisions.sql` (edit expression)
- `services/iam/db/queries.sql` (new query) + regenerated sqlc
- `services/iam/repository.go` (interface method)
- `services/iam/db/postgres.go` (impl + race-path UUID lookup)
- `services/iam/errors.go` (`tenantNameTakenErr` constructor)
- `services/iam/service.go` (pre-flight check)
- `services/iam/service_test.go` (`mockRepo` method + unit tests)
- `services/iam/db/tenant_name_unique_integration_test.go` (new cases)

Coverage: iam threshold ≥85%. Review: iam change → senior engineer + 2 approvals.
