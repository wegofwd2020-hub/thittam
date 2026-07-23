# expense and inventory reads — new vocabulary and the first permission backfill — design

**Issue:** #139, slice D. **Branch:** `fix/expense-inventory-reads-139d`, base `d77ebaa` (`main`).
**Follows:** #138, #144, #146, #149, slice A (#155), slice C (#158), #157 (#161), slice B (#164).
**Policy table:** `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`.

## 1. What this slice is

Nine read RPCs return tenant business data with no permission check:

- **expense (6):** `GetPurchaseOrder`, `ListPurchaseOrders`, `GetExpense`, `ListExpenses`, `GetPettyCashAdvance`, `ListPettyCashAdvances`
- **inventory (3):** `GetAsset`, `ListAssets`, `ListCheckouts`

Unlike slice C, **no existing permission string fits**, so this is the first slice to add vocabulary — and therefore the first to need a **backfill for existing tenants**. The mechanism it establishes is what slices E, F and G reuse.

`GetExpenseCategories`, `GetApprovalLimits` and `GetInventoryCategories` are **not** gated. They read vertical configuration, not tenant data — decision D3, same as the four config lookups slice C left alone.

## 2. The D10 backfill is one migration, not a per-schema job

**A correction to a premise carried since the policy table.** #139's notes recorded D10 as "the largest hidden cost in #139": that `seedSystemRoles` runs only at tenant creation, so a new permission string reaches new tenants only, and every existing `tenant_<uuid>` schema would need a separate backfill. That framing was wrong.

`roles` is a **single shared table in the public schema**, multi-tenant by column:

```sql
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    is_system   BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (tenant_id, name)
);
```

Three facts establish that the public copy is the live one:

1. `make migrate-all` applies `migrations/iam` to plain `$(DB_URL)` with **no `search_path`** (`Makefile`).
2. `cmd/iam/main.go:64` opens `pgxpool.New(ctx, dbURL)` with **no `search_path`**, and no `search_path` appears in any config, env file or k8s manifest.
3. `seedSystemRoles` writes through that same pool via `repo.CreateRole`.

So updating every existing tenant's roles is **one `UPDATE` against one table**.

Per-tenant schemas do exist — `CreateTenant` runs all twelve migration directories against `tenant_<uuid>` (`cmd/iam/main.go:170-181`) — but every service reads the public copies filtered by `tenant_id`. That matches what #157's whole-branch review found independently: the `tenant_id` predicate is the sole isolation control for these services, not defence-in-depth behind a schema boundary. **This slice does not attempt to resolve that architectural inconsistency**; it only records why the backfill is simple.

## 3. Vocabulary and grants

### 3.1 `expense:read` — why no existing string works

| role | `expense:submit` | `expense:approve` |
|---|:-:|:-:|
| super_admin, accountant, project_supervisor | ✓ | ✓ |
| manager, coordinator | — | ✓ |
| member | ✓ | — |
| inventory_manager | — | — |

Gating the reads on `expense:submit` locks out `manager` and `coordinator`; on `expense:approve`, it locks out `member`. `RequirePermission` takes a single string, so no combination is expressible. `expense:read` is genuinely required.

**Granted to `super_admin`, `manager`, `coordinator`, `accountant`, `project_supervisor` — deliberately NOT `member`.** This matches `budget:read`'s shape exactly (slice C).

**The cost of excluding `member`, stated plainly:** `ListExpenses` is tenant-scoped only —

```sql
SELECT * FROM expenses WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR production_id = $2) AND ($3 = '' OR status = $3)
```

— with no `submitted_by` filter. Granting `member` the permission would expose every colleague's amounts, vendors and categories to the lowest-privilege role. Withholding it means **a member cannot read back an expense they submitted**, because no self-scoped path exists. That gap is real, is not closed here, and is filed as a follow-up issue (§7). It is the same shape as the open `ListNotifications` defect — tenant-scoped but not user-scoped.

### 3.2 `inventory:read` — the grant matrix is wrong, not the string

`inventory:read` already exists but is granted to **`inventory_manager` alone** (`service.go:106`) — not even `super_admin`. Gating the three reads on it today would lock out every role that can currently check an asset out, which is why slice C deferred them here.

**Add `inventory:read` to `super_admin`, `manager`, `coordinator`, `project_supervisor`** — exactly the roles that already hold `inventory:checkout`. `inventory_manager` already has it. `accountant` holds no inventory permission and is left alone.

Being able to check an asset out while being unable to see the asset list is incoherent; this makes the grant set match the workflow.

## 4. The migration

`migrations/iam/020_seed_read_permissions.{up,down}.sql`. Next free number is 020 (019 is `tenant_purge_requests`).

```sql
-- up
UPDATE roles
SET permissions = array_append(permissions, 'expense:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('expense:read' = ANY (permissions));

UPDATE roles
SET permissions = array_append(permissions, 'inventory:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'project_supervisor')
  AND NOT ('inventory:read' = ANY (permissions));
```

```sql
-- down
UPDATE roles SET permissions = array_remove(permissions, 'expense:read')
WHERE is_system = true;

UPDATE roles SET permissions = array_remove(permissions, 'inventory:read')
WHERE is_system = true AND name <> 'inventory_manager';
```

Four properties matter:

- **Idempotent.** The `NOT (... = ANY (permissions))` guard means re-running appends nothing. This is required, not decorative: `migrations/iam` runs against the public schema via `make migrate-all` **and** against every new `tenant_<uuid>` at `CreateTenant`, so the statement executes in more than one context.
- **`is_system = true` only.** Custom roles are a tenant's own business; the migration must not edit them. (No RPC mints roles today — `CreateRole` exists only at the repository layer, called solely by `seedSystemRoles` — but the guard is correct regardless.)
- **The down migration preserves `inventory_manager`'s pre-existing grant.** `inventory:read` was on that role before this slice; a blind `array_remove` across all system roles would strip something this migration did not add. `expense:read` is new to every role, so its removal needs no exclusion.
- **New tenants are covered by code, not by this migration.** `systemRoles` in `services/iam/service.go` is edited in the same change so `seedSystemRoles` grants both strings going forward. **Both halves are required**: the migration covers existing tenants, the code covers future ones. Shipping either alone leaves a population without the permission.

### 4.1 Deploy ordering — the one operational hazard

**The migration must run before the new service code.** If the gates land first, every existing tenant loses expense and inventory read access until the migration runs. The reverse order is harmless: the permission exists but nothing checks it yet.

This is the first slice in #139 where deploy order matters. State it in the PR body.

## 5. Design

Nine gate insertions, following the pattern slice C established. In each handler, immediately after the existing tenant block and before any `uuid.Parse`:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}
```

Guard order is **tenant → permission → parse**, matching the write RPCs already in these files. Permission strings are inline literals, matching how `expense` and `inventory` already pass `"expense:submit"` / `"inventory:checkout"`.

Both services already carry `perm interceptor.PermissionChecker` and the `WithPermissionChecker` setter and already gate their writes, so no wiring, no `NewHandler` signature change, and no `cmd/` change is needed. No repository or service signature changes. No proto change.

## 6. Testing

**Denial tests** for each newly gated RPC, proving the gate fires before the repository is reached. Both services have an `allowAllPerm` double and **neither has a `denyPerm`** — each task adds one, mirroring `services/project/handler_test.go`.

**The denial-test rule:** install the `t.Fatal` on the first repository fn the gated path would call, and confirm which fn that is by reading the handler. A status-code-only assertion can pass against ungated code when a mock default produces the same code by another route.

### 6.1 Flip predictions — measured, not estimated

Tests that pass a valid tenant but build the handler without a checker leave `h.perm` nil, and after #138 `RequirePermission` on a nil checker returns `Internal`. Repair by adding `.WithPermissionChecker(allowAllPerm{})` — never by weakening an assertion.

**The distinction that decides the count:** `newHandler()` in both services already returns `NewHandler(NewService(&mockRepo{})).WithPermissionChecker(allowAllPerm{})`. Tests that call it therefore pass the new gate untouched. Only tests that build the handler inline — `NewHandler(NewService(&mockRepo{...}))` with a custom mock and **no** setter — leave `h.perm` nil and flip.

**expense — exactly 6**, all `_Success`:
`GetPurchaseOrder_Success`, `ListPurchaseOrders_Success`, `GetExpense_Success`, `ListExpenses_Success`, `GetPettyCashAdvance_Success`, `ListPettyCashAdvances_Success`

**inventory — exactly 3**, all `_Success`:
`GetAsset_Success`, `ListAssets_Success`, `ListCheckouts_Success`

**These must NOT flip, for two distinct reasons:**

- *Already hold a checker via `newHandler()`* — expense `GetPurchaseOrder_InvalidID`, `ListPurchaseOrders_InvalidProductionID`, `GetExpense_InvalidID`, `GetPettyCashAdvance_InvalidID`; inventory `GetAsset_InvalidID`, `ListCheckouts_InvalidAssetID`, and `ListCheckouts_PassesCallerTenantToRepo` (added by #157, supplies `allowAllPerm{}` explicitly). These reach the gate, pass it, and still assert `InvalidArgument` from the parse below.
- *Short-circuit before the gate* — all six expense `_NoTenant` tests and all three inventory `_NoTenant` tests pass `ctxWithVertical()` with no tenant, so `tenant.IDFromContext` returns `Unauthenticated` first.

An earlier draft of this section predicted 10 and 5 by scanning test bodies for `allowAllPerm` without accounting for `newHandler()` supplying it indirectly. Corrected by measurement.

**If either count differs, stop and report.** Both slice B tasks mispredicted — one by two, one by one — and each miss exposed something real: a broken verification command, and a test that had been passing for the wrong reason.

Use `go test ./services/<svc>/ -count=1 2>&1 | grep -- '--- FAIL' | sort` to count. **Do not anchor with `'^\s+--- FAIL'`** — that form matches nothing against this repo's output and silently reports zero.

**The migration needs its own test.** CI's `Migration Validate (up + down)` job is the authoritative gate. Add an integration test under `services/iam/db/` (build tag `//go:build integration`, `testdb.Open(t)` — see `tenant_find_by_name_integration_test.go`) that seeds a system role without the new strings, runs the update, and asserts the permission is appended exactly once when applied twice. **Integration tests are not compiled by default commands**; `go vet -tags=integration ./services/iam/db/` is the only local signal the file builds.

**Coverage** must not regress: `expense` and `inventory` are in the `others ≥ 75%` tier; `iam` is 87.3% with a floor of 85%.

## 7. Out of scope

- **Members cannot read their own expenses.** §3.1. Needs a `submitted_by` filter and a proto decision about how a caller expresses "mine". **Filed as #165.**
- **`ListNotifications` is tenant-scoped but not user-scoped** — the same defect class, already recorded, belongs to slice G.
- **The public-schema / per-tenant-schema inconsistency** (§2). Recorded, not resolved.
- **Slices E, F, G** (document, billing, notifications — 35 RPCs, zero gates) and **H** (#159, the tenant-isolation audit). They reuse this slice's migration pattern.
- **`inventory:retire`** remains dead vocabulary, granted to `inventory_manager` and checked nowhere. Decision D7; no RPC needs it yet.

## 8. Constraints

- Security change. Senior review; 2 approvals.
- **No Docker, no database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. Use `pkg/testdb` (skips without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. **CI's real-Postgres and Migration Validate jobs are the authoritative gates. This binds delegated subagents — state it in their instructions.**
- **This is the first slice with a migration.** `migrations/` will show a diff — expected here, unlike every previous slice. `gen/` must still be empty.
- **Do not run `make generate-sqlc`.** Nothing here touches `queries.sql`; codegen is repo-wide and would dirty `services/billing/` with pre-existing drift (#160).
- Whole-tree `go vet ./...` is the gate. No interface changes are expected in this slice — if an implementer finds one needed, that is a signal something is wrong.
- `errcheck` runs in CI; `golangci-lint` is not installed locally.
- `gofmt -l services/iam` flags `service.go` and `lifecycle_test.go` on a clean `main`. Pre-existing — do not reformat.
- **Verification commands containing `$`, `(`, `)` or quotes must use `grep -F`.** Three greps that could only ever return zero were shipped across #157 and slice B; a check that cannot fail manufactures false confidence.
- `gh pr checks` before declaring the PR ready.
