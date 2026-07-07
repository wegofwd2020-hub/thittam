# Design: registration tenant-name uniqueness parity

**Follows:** #89 (iam tenant-name uniqueness) — the "enforce at every layer" follow-up
**Date:** 2026-07-07
**Scope:** `pkg/registration` only
**Branch:** `feat/registration-name-parity`

## Context

The public self-serve signup path (`pkg/registration`) writes to the **same shared
`tenants` table** the iam service uses (same DB, same `tenants_name_ci_unique`
index defined in `migrations/iam/`). #89 gave `services/iam/Service.CreateTenant`
full tenant-name uniqueness: name normalization (`strings.Join(strings.Fields(name)," ")`),
a pre-flight duplicate check, and a `23505`→`ErrTenantNameTaken` mapping surfaced
as gRPC `AlreadyExists`.

The registration path bypasses all of it. Today it:

- only `strings.TrimSpace`es `CompanyName` (`request.go:87`) — no internal-whitespace
  collapse, so `"Acme  Corp"` is stored verbatim and diverges from iam's canonical form;
- has **no** pre-flight name check (it pre-flights email and slug, but not name);
- does **not** inspect `pgconn.PgError` / `23505` in `db/store.go` — a duplicate name
  surfaces as a raw wrapped Postgres error, not a clean conflict.

The DB unique index already blocks the second insert, so this is about surfacing a
**clean, typed conflict** instead of a raw pg error, and about storing a **canonical
name** consistent with iam.

**Important scope fact:** registration is **not wired to any handler/cmd yet** —
`NewPipeline`/`NewOrchestrator` have no production callers (only tests), and there is
no HTTP/gRPC error mapper for registration errors. So this work makes the *library*
return a clean `ErrTenantNameTaken`; the HTTP-409 mapping is deferred until
registration is actually wired up (a separate future feature).

**Do not mirror slug handling.** On slug collision, registration silently
auto-suffixes with a UUID fragment (`pipeline.go:102-105`, `saga.go:267-269`), and
`ErrSlugTaken` is dead code. Name parity must mirror **iam's clean-error model**, not
slug's silent auto-suffix.

## Component 1 — Normalization + sentinel

`pkg/registration/request.go` — in `RegisterRequest.Validate()` (line 87), replace:

```go
r.CompanyName = strings.TrimSpace(r.CompanyName)
```

with:

```go
// Collapse internal whitespace runs and trim, matching iam's canonical form
// and the tenants_name_ci_unique index (regexp_replace(lower(trim(name)),'\s+',' ','g')).
r.CompanyName = strings.Join(strings.Fields(r.CompanyName), " ")
```

`strings.Fields` splits on any whitespace run and drops leading/trailing, so the join
yields the same canonical form as the DB index and iam. The existing required-check
and 2–200 length check (`request.go:92-97`) run after, unchanged.

`pkg/registration/errors.go` — add the sentinel:

```go
// ErrTenantNameTaken is returned when a tenant already exists with the same
// company name (case-insensitive, whitespace-collapsed). Enforced by the shared
// tenants_name_ci_unique index; mirrors iam.ErrTenantNameTaken.
ErrTenantNameTaken = errors.New("registration: company name already taken")
```

## Component 2 — Pre-flight duplicate check

**sqlc query** — `pkg/registration/db/queries.sql`:

```sql
-- name: TenantExistsByNormalizedName :one
SELECT EXISTS (
    SELECT 1 FROM tenants
    WHERE regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
        = regexp_replace(lower(trim($1)), '\s+', ' ', 'g')
);
```

Uses the `tenants_name_ci_unique` index. `sqlc generate` to regenerate.

**Port** — `pkg/registration/ports.go`, on the TenantStore interface (next to
`TenantExistsBySlug` at :21):

```go
// TenantExistsByNormalizedName returns true if a tenant already exists with
// this name under case-insensitive, trimmed, whitespace-collapsed comparison.
TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error)
```

**Store impl** — `pkg/registration/db/store.go`, on both `Store` and `TxStore`:

```go
func (s *Store) TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error) {
    return s.q.TenantExistsByNormalizedName(ctx, name)
}
```

**Pipeline wiring** — `pkg/registration/pipeline.go`, beside the slug pre-flight
(~:98), before `CreateTenant`:

```go
if taken, err := p.tenants.TenantExistsByNormalizedName(ctx, req.CompanyName); err != nil {
    return result, fmt.Errorf("check tenant name: %w", err)
} else if taken {
    return result, ErrTenantNameTaken
}
```

**Saga wiring** — `pkg/registration/saga.go`, in the step-1 region (~:267), same
check → `o.failSaga(..., StepCreateTenant, ErrTenantNameTaken)` (step 1 hasn't
completed, so no compensation is needed — matches the existing failSaga pattern).

The pre-flight uses `req.CompanyName`, which `Validate()` has already normalized to
the canonical form.

## Component 3 — 23505 backstop in the store

`pkg/registration/db/store.go` — `CreateTenant` on both `Store` (:35) and `TxStore`
(:133). Add a package-level helper (mirror `services/iam/db/postgres.go:346`):

```go
// isUniqueViolationOn reports whether err is a Postgres unique_violation (23505)
// on the named constraint/index. Parameterized for symmetry with iam's helper.
func isUniqueViolationOn(err error, constraint string) bool {
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) {
        return false
    }
    return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
```

In `CreateTenant`, wrap the sqlc error:

```go
t, err := s.q.CreateTenant(ctx, CreateTenantParams{Name: name, Slug: slug, Plan: plan})
if err != nil {
    if isUniqueViolationOn(err, "tenants_name_ci_unique") {
        return uuid.Nil, ErrTenantNameTaken
    }
    return uuid.Nil, fmt.Errorf("create tenant: %w", err)
}
```

Requires importing `github.com/jackc/pgx/v5/pgconn` and `errors` in `store.go`. This
closes the pre-flight TOCTOU window: two concurrent same-name signups — one wins, the
other's insert hits `23505` and now returns the clean sentinel.

## Component 4 — Tests

**Unit** (`pkg/registration/*_test.go`, in-memory function-field mocks):

- `TestRegisterRequest_Validate` (`request`/`pipeline_test.go:510`): add cases —
  internal double-space and tab in `CompanyName` collapse to single spaces;
  leading/trailing trimmed.
- `TestPipeline_TenantNameTaken` (mirror `TestPipeline_EmailTaken` at
  `pipeline_test.go:314`): inject `tenantExistsByNormalizedNameFn` returning
  `(true, nil)` → `errors.Is(err, ErrTenantNameTaken)`. Add the mock field to
  `mockTenantStore`.
- Saga equivalent (`saga_test.go`): same injection → failSaga with
  `ErrTenantNameTaken`.
- `TestIsUniqueViolationOn` (`db/store_test.go`): synthetic
  `&pgconn.PgError{Code:"23505", ConstraintName:"tenants_name_ci_unique"}` → true;
  wrong code / wrong constraint / non-pg error → false.

**Integration** (`//go:build integration`, `pkg/testdb`, shared `thittam_test`):

- New `pkg/registration/db/tenant_name_taken_integration_test.go`: construct a real
  `Store` (via the package's real constructor — confirm the constructor name during
  implementation), `CreateTenant` with `"Parity  Studios"` then `"parity studios"`
  → second returns `ErrTenantNameTaken` (not a raw pg error). Also assert
  `TenantExistsByNormalizedName` returns true for a case/whitespace-varied lookup and
  false for an absent name.

## Acceptance criteria

- [ ] `CompanyName` is normalized (trim + internal-whitespace collapse) in `Validate()`.
- [ ] `registration.ErrTenantNameTaken` sentinel exists.
- [ ] Pipeline and saga pre-flight the normalized name and return `ErrTenantNameTaken` on an existing tenant.
- [ ] `Store`/`TxStore` `CreateTenant` maps a `23505` on `tenants_name_ci_unique` to `ErrTenantNameTaken`.
- [ ] Unit tests: normalization, pipeline pre-flight, saga pre-flight, `isUniqueViolationOn`.
- [ ] Integration test: real store duplicate → `ErrTenantNameTaken`.
- [ ] registration coverage ≥ 75%.

## Out of scope (follow-up)

- Wiring registration to a production handler/endpoint + HTTP `409`/`AlreadyExists`
  mapping — registration has no production caller today; that is a separate feature.
- Naming the colliding tenant's UUID in the error (iam does this; registration
  returns a bare sentinel — no handler consumes a UUID yet).

## Files touched

- `pkg/registration/request.go` (normalization)
- `pkg/registration/errors.go` (sentinel)
- `pkg/registration/ports.go` (port method)
- `pkg/registration/db/queries.sql` (+ regenerated sqlc)
- `pkg/registration/db/store.go` (existence impl on Store+TxStore, 23505 backstop, helper)
- `pkg/registration/pipeline.go` (pre-flight)
- `pkg/registration/saga.go` (pre-flight)
- `pkg/registration/pipeline_test.go` / `saga_test.go` / `db/store_test.go` (unit tests + mock field)
- `pkg/registration/db/tenant_name_taken_integration_test.go` (new integration test)

Review: touches shared-tenancy onboarding → treat as a security-adjacent change (senior review advisable).
