# Permission-gate coverage test (#166) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-30
**Issue:** #166 (infra: nothing enforces migration-before-deploy ordering for permission migrations)
**Branch:** `chore/authz-coverage-test-166` off `main` (f864649)
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

Make it impossible to merge a permission gate that lacks its grant, and impossible to add a
newly-gated permission without a backfill migration for existing tenants — the mechanically-preventable
core of #166. A pure source-parsing Go test enforces three consistency invariants between the code's
`RequirePermission` gates, the IAM `systemRoles` seed (new tenants), and the `migrations/iam/` backfills
(existing tenants). It rides the existing `go test ./...` (Test & Coverage CI job) — no `ci.yml`, k8s,
or DB change. A short ops note covers the operational half the test cannot: deploy ordering.

## Context (grounding facts, `main` @ f864649)

- **The hazard (#166):** a code-first prod deploy gates RPCs on a permission that a not-yet-run
  migration grants → every existing tenant gets `PermissionDenied` until `make migrate-all` runs. Both
  the gate and the migration ship in the *same PR*; the gap is prod apply-order plus the risk of a gate
  shipping with no grant at all.
- **#166 is already behind the rollouts, not ahead.** Slices D/E/F/G all shipped (`migrations/iam/020`
  read perms, `021` document, `022` billing, `023` notifications; PRs #168/#169/#170/#171 done). This
  is now forward-looking hardening, not a block on an imminent slice.
- **Options 1 & 2 are not achievable in-repo.** There are **zero Dockerfiles** in the repo (service
  images built out-of-band); `migrate` is the golang-migrate CLI fetched from GitHub releases at CI time
  — no migrate-capable image to run in an initContainer/Job. And every service Deployment mounts
  `DATABASE_URL` from Secret key `runtime_url` (thittam_app, least-privilege, #122), while migrations
  need the owner `url` key — a DSN mismatch. Option 3 (boot assertion) needs a new IAM "permission
  exists" RPC + a per-service permission manifest that does not exist. All deferred as non-goals.
- **Gates:** `interceptor.RequirePermission(ctx, checker, perm)` (`pkg/interceptor/permission.go:62`),
  one call per gated RPC, scattered across `services/*/handler.go`. The third argument is USUALLY a
  string literal (`"expense:read"`), but the **ledger service passes package-local string consts**
  (`services/ledger/handler.go:26-29`: `permLedgerRead="ledger:read"`, `permLedgerWrite="ledger:write"`,
  `permLedgerPost="ledger:post"`, `permLedgerAdmin="ledger:admin"`) at 12 call sites. So the extraction
  MUST resolve same-package string consts, not treat them as unverifiable. Full current gated set (24):
  the 20 literals `billing:manage, billing:read, budget:approve, budget:read, budget:write,
  document:delete, document:read, document:write, expense:approve, expense:read, expense:submit,
  inventory:checkout, inventory:read, inventory:write, notifications:manage, notifications:read,
  production:read, production:write, report:read, resource:manage` PLUS the 4 ledger consts
  `ledger:read, ledger:write, ledger:post, ledger:admin`. (`iam` may additionally gate `user:manage` via
  the `permUserManage` const — the test resolves it the same way; finalize the exact set from the first
  run.) No `RequireRole(...)` call gates on a permission string (RequireRole takes a role constant).
- **`systemRoles`** (`services/iam/service.go:66`, unexported `var systemRoles []struct{…; Permissions
  []string}`) is the per-tenant role seed applied at `CreateTenant` (`service.go:827` loops it). A
  permission must be here or **new** tenants never receive it.
- **Backfill migrations** (`migrations/iam/*.up.sql`) patch **existing** tenants via
  `UPDATE roles SET permissions = array_append(permissions, 'X') WHERE is_system = true AND name IN (…)
  AND NOT ('X' = ANY(permissions))`. Permissions granted by a backfill today (9):
  `billing:manage, billing:read, document:delete, document:read, document:write, expense:read,
  inventory:read, notifications:manage, notifications:read`. `roles.permissions` is a `TEXT[]` column
  (`migrations/iam/007_create_roles.up.sql`); no join table.
- **Founding permissions = gated − backfilled (15):** `budget:approve, budget:read, budget:write,
  expense:approve, expense:submit, inventory:checkout, inventory:write, production:read,
  production:write, report:read, resource:manage, ledger:read, ledger:write, ledger:post,
  ledger:admin` (+ `user:manage` if `iam` gates it via RequirePermission). These predate the backfill
  era — they entered `systemRoles` before the tenants that would need a backfill existed, so they
  legitimately have no backfill migration. The implementer finalizes this list empirically from the
  first test run (every gated-non-backfilled permission is either genuinely founding → allowlist, or a
  real missing-backfill bug → fix).
- **`Migration Validate` CI** (`ci.yml:152`) runs `migrate up`/`down -all` against a **fresh empty**
  `postgres:16` with no tenant seeding, so a data migration like `020` runs its `UPDATE roles … WHERE
  is_system` against zero rows — syntax + down-doesn't-error only, never a grant-matrix check.

## Design

### Artifact 1 — `services/iam/authz_coverage_test.go` (package `iam`)

Placed in `package iam` so it reads the unexported `systemRoles` directly (robust — no AST parse of the
seed). It scans sibling packages' source and the migration SQL from disk. Repo root is located via
`runtime.Caller` on the test file (`services/iam/authz_coverage_test.go` → root two levels up).

Sets it computes:
- **S (seeded):** every permission string in `systemRoles[].Permissions` (direct, in-package).
- **G (gated):** walk `<root>/services/`, `go/parser`-parse every non-`_test.go` `.go` file, grouped by
  directory (package). First, per package, collect a `constName → value` map from every
  `const name = "…"` string declaration in that package's files. Then find every call whose function is
  the selector `…RequirePermission` and take the **third argument**: a `*ast.BasicLit` STRING → its
  unquoted value; an `*ast.Ident` that resolves in the same package's const map → that value (this is how
  the 12 ledger gates on `permLedger*` and any `permUserManage` gate are captured). An argument that is
  neither a string literal nor a resolvable same-package string const (a cross-package selector, a
  concatenation, a computed value) → **fail the test** naming file/line: *"RequirePermission permission
  arg is neither a string literal nor a resolvable same-package const — the coverage test cannot verify
  it; use a literal/const or extend the resolver."* This keeps the gate set statically knowable while
  supporting the existing const-based gates.
- **B (backfilled):** read every `<root>/migrations/iam/*.up.sql`, regex
  `array_append\(permissions,\s*'([^']+)'\)` → capture group into B.
- **F (founding):** a hardcoded, documented allowlist var = the 11 founding permissions above, with a
  comment: *"Permissions gated before the backfill-migration era; they entered systemRoles before any
  tenant needing a backfill existed, so they have no migrations/iam backfill. Adding to this list is a
  reviewable exception — prefer adding a backfill migration."*

Assertions (each failure lists the offending permissions):
1. **`G ⊆ S`** — every gated permission is in `systemRoles`. Catches gating a permission no role seeds,
   and typos. Failure message: *"gated but not in systemRoles (new tenants can never be granted it): …"*.
2. **`(G \ F) ⊆ B`** — every gated permission that is **not** founding must be granted by a backfill
   migration. This is #166's core: a newly-gated permission cannot merge without an existing-tenant
   backfill. Failure: *"gated, not founding, and no migrations/iam backfill grants it — existing tenants
   will get PermissionDenied until a backfill migration is added: …"*.
3. **`B ⊆ S`** — every backfilled permission is also in `systemRoles` (the "both halves" the migration
   comments require, checked from the migration side; no backfill grants a permission new tenants miss).
   Failure: *"backfilled but not in systemRoles (existing tenants get it, new tenants do not): …"*.

Additionally assert **`F ⊆ G`** — no stale entry in the founding allowlist that is no longer gated
(keeps the allowlist honest; a removed gate must drop from F). Failure: *"founding allowlist entry is no
longer gated anywhere — remove it from F: …"*.

The test is pure file parsing: no DB, runs under `go test ./... -short`, needs no `ci.yml` change (the
Test & Coverage job already runs `go test`).

### Artifact 2 — deploy-ordering note `docs/operations/permission-migration-ordering.md`

Short doc (the `docs/operations/` dir exists from #123). States: (1) when a change adds a
permission-gating RPC, run `make migrate-all` **before** rolling the new service code — code-first makes
existing tenants return `PermissionDenied` until the backfill runs; (2) the coverage test
(`services/iam/authz_coverage_test.go`) guarantees the backfill migration *exists*, not that it has
*run* in prod — ordering is still the operator's responsibility; (3) `Migration Validate` in CI is
syntax-only (empty DB), not semantic grant coverage. Link the test and `migrations/iam/`.

## Testing

- The coverage test IS the test — it must pass against the current tree on first run. Expect: G = the 24
  listed (incl. the 4 ledger consts; + `user:manage` if iam gates it), S ⊇ G, B = the 9 listed, F = the
  15 founding, so `G⊆S`, `(G\F)⊆B`, `B⊆S`, `F⊆G` all hold.
  **If the first run fails, that is a real finding**, not a test bug: either a gate is missing from
  `systemRoles`/backfill (fix the seed/migration), or the founding allowlist is wrong (reclassify).
  Resolve by correcting the code under test or the allowlist — never by loosening an assertion to hide a
  real gap.
- Prove the test has teeth: temporarily add a bogus `RequirePermission(ctx, h.perm, "zzz:bogus")` to one
  handler, confirm assertion 1 fails; remove it. Temporarily add a non-founding gated permission with no
  backfill, confirm assertion 2 fails; revert. (Do these as throwaway local edits, not commits.)
- Gates: `go test ./services/iam/ -run TestAuthzCoverage -v`; `go vet ./...`; `go build ./...`;
  `gofmt -l` the new file. No proto/sqlc/migration.

## Non-goals

- initContainer / migration Job / Helm pre-deploy hook (issue options 1 & 2) — not achievable in-repo
  (no Dockerfile, owner-vs-runtime DSN mismatch); would not be CI-testable.
- A boot-time runtime assertion (issue option 3) — needs a new IAM "does permission exist" RPC and a
  per-service permission manifest; deferred.
- Auto-running migrations or changing the deploy pipeline.
- Retroactively adding backfill migrations for the 11 founding permissions (they are correctly founding;
  the allowlist records that).
- Semantic testing of the migration grant matrix in CI (still covered by slice integration tests).

## Review weight

Low runtime risk (a test + a doc; no production code path changes). The review that matters: the test's
extraction is correct (finds all `RequirePermission` literals, rejects non-literals), the founding
allowlist is justified permission-by-permission, and the three assertions have real teeth (the bogus-gate
demonstration). No senior/security-domain gate (no iam/ledger business-logic or money change).
