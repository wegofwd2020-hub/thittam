# expense + inventory Reads Implementation Plan (#139 slice D)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate nine read RPCs — six in `expense`, three in `inventory` — on `expense:read` and `inventory:read`, and grant those permissions to existing tenants via the first permission backfill in #139.

**Architecture:** Three tasks. Task 1 creates the vocabulary: a migration that appends the two strings to existing `is_system` roles, plus a `systemRoles` edit so new tenants get them. Tasks 2 and 3 insert the gates. **Task 1 must land first** — gating a permission nobody holds locks everyone out.

**Tech Stack:** Go 1.25, golang-migrate, pgx/v5, grpc-go, testify.

**Spec:** `docs/superpowers/specs/2026-07-23-expense-inventory-reads-139d-design.md` (committed on this branch at `dace420`). Read §2 for why the backfill is one `UPDATE` and not a per-schema loop.

## Global Constraints

- **Branch:** `fix/expense-inventory-reads-139d`, already created, base `d77ebaa` (`main`).
- **NO Docker, NO database.** NEVER run `docker compose` with `-v`, `down`, or `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. Use `pkg/testdb` (skips without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. **CI's `Migration Validate (up + down)` and `Integration Tests (real Postgres)` jobs are the authoritative gates.**
- **This slice has a migration** — `migrations/` WILL show a diff, unlike every previous #139 slice. `gen/` must still be empty.
- **Do NOT run `make generate-sqlc`.** Nothing here touches `queries.sql`; codegen is repo-wide and would dirty `services/billing/` with pre-existing drift (issue #160).
- **NEVER `git add -A`.** Use the scoped `git add` in each task's commit step.
- **Whole-tree `go vet ./...` is the completion gate for every task.** No interface changes are expected anywhere in this slice — if one appears necessary, stop and report.
- **Guard order: tenant (`tenant.IDFromContext`) → permission (`RequirePermission`) → `uuid.Parse` → service call.** The gate goes after the existing tenant block and before any parse. This matches the write RPCs already in both files.
- Permission strings are **inline literals**, matching how these services already pass `"expense:submit"` / `"inventory:checkout"`. No constants — that convention belongs to services that own their vocabulary (iam, ledger).
- NO proto change. NO new `NewHandler` parameter — both services already carry `perm` and `WithPermissionChecker`, and already gate their writes. No `cmd/` change.
- `errcheck` runs in CI; `golangci-lint` is NOT installed locally.
- `gofmt -l services/iam` flags `service.go` and `lifecycle_test.go` on a clean `main`. **Pre-existing — do not reformat them.**
- Coverage must not regress: `expense` and `inventory` are in the `others ≥ 75%` tier; `iam` is 87.3% with a floor of 85%.
- **Verification commands containing `$`, `(`, `)` or quotes must use `grep -F`.** Three greps that could only ever return zero shipped across #157 and slice B. A check that cannot fail manufactures false confidence.
- Count flipped tests with `go test ./services/<svc>/ -count=1 2>&1 | grep -- '--- FAIL' | sort`. **Do NOT anchor with `'^\s+--- FAIL'`** — that form matches nothing against this repo's output.

### Why the backfill is one UPDATE

`roles` is a **single shared table in the public schema**, multi-tenant by a `tenant_id` column — not one table per tenant schema. `make migrate-all` applies `migrations/iam` to plain `$(DB_URL)` with no `search_path`; `cmd/iam/main.go:64` opens `pgxpool.New(ctx, dbURL)` with no `search_path`; `seedSystemRoles` writes through that same pool. Per-tenant schemas exist and receive the same migrations at `CreateTenant`, but no service reads them.

**Consequence for Task 1:** the migration must be idempotent, because `migrations/iam` executes against the public schema *and* against every new `tenant_<uuid>`.

---

### Task 1: Vocabulary — the migration and the seed edit

Creates `expense:read` and widens `inventory:read`'s grant set. **Nothing is gated in this task** — it only makes the permissions exist, so Tasks 2 and 3 have something to check.

**Files:**
- Create: `migrations/iam/020_seed_read_permissions.up.sql`
- Create: `migrations/iam/020_seed_read_permissions.down.sql`
- Create: `services/iam/db/role_permission_backfill_integration_test.go`
- Modify: `services/iam/service.go` (the `systemRoles` var, ~lines 66-117)

**Interfaces:**
- Consumes: nothing.
- Produces: the permission strings `"expense:read"` and `"inventory:read"` present on the seeded system roles. Tasks 2 and 3 gate on those exact literals.

- [ ] **Step 1: Record the coverage baseline**

```bash
go test ./services/iam/ -cover -count=1 2>&1 | tail -1
```

Expected `coverage: 87.3% of statements`. Record it; compare at Step 8.

- [ ] **Step 2: Write the up migration**

Create `migrations/iam/020_seed_read_permissions.up.sql`:

```sql
-- 020_seed_read_permissions.up.sql
-- #139 slice D: grant the two read permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change so NEW
-- tenants receive these at seedSystemRoles time. This migration covers the
-- tenants that already exist. Both halves are required.
--
-- Idempotent by necessity, not politeness: migrations/iam runs against the
-- public schema via `make migrate-all` AND against every new tenant_<uuid>
-- schema at CreateTenant, so these statements execute in more than one
-- context. The NOT (... = ANY (permissions)) guard makes a re-run a no-op.
--
-- is_system = true only: custom roles are a tenant's own business.

UPDATE roles
SET permissions = array_append(permissions, 'expense:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('expense:read' = ANY (permissions));

-- inventory:read already existed but was granted to inventory_manager alone —
-- not even super_admin — so gating the inventory reads would have locked out
-- every role that can check an asset out. Widen it to exactly the roles that
-- already hold inventory:checkout.
UPDATE roles
SET permissions = array_append(permissions, 'inventory:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'project_supervisor')
  AND NOT ('inventory:read' = ANY (permissions));
```

- [ ] **Step 3: Write the down migration**

Create `migrations/iam/020_seed_read_permissions.down.sql`:

```sql
-- 020_seed_read_permissions.down.sql
-- Reverse of 020. expense:read is new to every role, so removing it everywhere
-- is correct.

UPDATE roles
SET permissions = array_remove(permissions, 'expense:read')
WHERE is_system = true;

-- inventory_manager held inventory:read BEFORE this migration. A blind
-- array_remove across all system roles would strip a grant this migration
-- never added, so that role is excluded.
UPDATE roles
SET permissions = array_remove(permissions, 'inventory:read')
WHERE is_system = true
  AND name <> 'inventory_manager';
```

- [ ] **Step 4: Edit `systemRoles` so new tenants get the permissions**

In `services/iam/service.go`, add to the permission lists. The five roles gaining `expense:read` are `super_admin`, `manager`, `coordinator`, `accountant`, `project_supervisor` — **not `member`** (see the spec §3.1: `ListExpenses` has no `submitted_by` filter, so granting it to `member` would expose every colleague's amounts). The four gaining `inventory:read` are `super_admin`, `manager`, `coordinator`, `project_supervisor`; `inventory_manager` already has it.

Read the current `systemRoles` block first, then place each new string next to its siblings:

```go
	{"super_admin", []string{
		"production:read", "production:write",
		"budget:read", "budget:write", "budget:approve",
		"expense:read", "expense:submit", "expense:approve",
		"inventory:read", "inventory:checkout",
		"report:read",
		permLedgerRead, permLedgerWrite, permLedgerPost, permLedgerAdmin,
		permUserManage,
	}},
	{"manager", []string{
		"production:read", "production:write",
		"budget:read", "budget:approve",
		"expense:read", "expense:approve",
		"inventory:read", "inventory:checkout",
		"report:read",
		permLedgerRead,
	}},
	{"coordinator", []string{
		"production:read", "production:write",
		"budget:read", "budget:write",
		"expense:read", "expense:approve",
		"inventory:read", "inventory:checkout",
		"report:read",
		permLedgerRead,
	}},
	{"accountant", []string{
		"budget:read",
		"expense:read", "expense:submit", "expense:approve",
		"report:read",
		permLedgerRead, permLedgerWrite, permLedgerPost,
	}},
```

`member` and `inventory_manager` are unchanged. For `project_supervisor`, add `"expense:read"` beside its existing `"expense:submit", "expense:approve"` and `"inventory:read"` beside its `"inventory:checkout"`, preserving the rest of that block exactly as it is.

- [ ] **Step 5: Write the migration's integration test**

Create `services/iam/db/role_permission_backfill_integration_test.go`. **The first line must be the build tag, followed by a blank line** — copy the header shape from `services/iam/db/tenant_find_by_name_integration_test.go` and read that file first for the package's `testdb` conventions.

```go
//go:build integration

// Integration test for migration 020 (#139 slice D). The migration grants
// expense:read and inventory:read to existing system roles. This test proves
// the two properties the migration depends on: it appends the permission, and
// applying it twice appends it only once.
//
// migrations/iam runs against the public schema via `make migrate-all` AND
// against every new tenant_<uuid> at CreateTenant, so a non-idempotent
// statement would duplicate entries.

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

// backfillExpenseRead is the exact statement from
// migrations/iam/020_seed_read_permissions.up.sql. Keep the two in sync.
const backfillExpenseRead = `
UPDATE roles
SET permissions = array_append(permissions, 'expense:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('expense:read' = ANY (permissions))`

func TestMigration020_GrantsExpenseReadIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Backfill Test "+tenantID.String(), "backfill-"+tenantID.String())
	require.NoError(t, err)

	// A system role as it exists BEFORE the migration: no expense:read.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'manager', $3, true)`,
		uuid.New(), tenantID, []string{"budget:read", "expense:approve"})
	require.NoError(t, err)

	// First application appends it.
	_, err = tx.Exec(ctx, backfillExpenseRead)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "expense:read")
	require.Equal(t, 1, countOccurrences(perms, "expense:read"))

	// Second application is a no-op — this is the property that matters,
	// because the statement runs in more than one schema context.
	_, err = tx.Exec(ctx, backfillExpenseRead)
	require.NoError(t, err)

	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Equal(t, 1, countOccurrences(perms, "expense:read"),
		"re-running the migration must not duplicate the permission")
}

func TestMigration020_LeavesMemberAlone(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Member Test "+tenantID.String(), "member-"+tenantID.String())
	require.NoError(t, err)

	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'member', $3, true)`,
		uuid.New(), tenantID, []string{"production:read", "expense:submit"})
	require.NoError(t, err)

	_, err = tx.Exec(ctx, backfillExpenseRead)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'member'`,
		tenantID).Scan(&perms))
	require.NotContains(t, perms, "expense:read",
		"member is deliberately excluded: ListExpenses has no submitted_by filter")
}

func countOccurrences(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
```

If `testdb.NewTx` does not exist with that signature, or the `tenants` insert needs columns this snippet omits, **adapt and report the discrepancy** — read `tenant_find_by_name_integration_test.go` for the working shape rather than guessing.

- [ ] **Step 6: Verify the tagged file compiles**

```bash
go vet -tags=integration ./services/iam/db/
```

Expected: clean, no output.

**This is not optional.** Integration tests carry `//go:build integration` and are **not compiled** by `go test ./services/iam/db/`. A compile error in the new file would otherwise pass every default-tag command silently.

- [ ] **Step 7: Run the integration test locally**

```bash
go test -tags=integration ./services/iam/db/ -run TestMigration020 -v
```

Without `THITTAM_TEST_DSN` this **SKIPs**. That is the expected local outcome. **Do not set up a database, do not fabricate a result, and do not report a SKIP as a pass.** Record in the report that the proof is deferred to CI's `Migration Validate (up + down)` and `Integration Tests (real Postgres)` jobs.

- [ ] **Step 8: Run the full check**

```bash
go vet ./...
go test ./services/iam/ -race -cover -count=1
git diff --stat gen/          # must be empty
git diff --stat migrations/   # EXPECTED to be non-empty this task
```

Expected: vet clean, tests pass, coverage ≥ the Step 1 baseline.

**Flip prediction for this task: zero.** No gate is added here, so no existing test changes behaviour. Any `services/iam` test failure means the `systemRoles` edit broke an assertion about role contents — **stop and report** rather than adjusting the test.

- [ ] **Step 9: Commit**

```bash
git add migrations/iam/020_seed_read_permissions.up.sql \
        migrations/iam/020_seed_read_permissions.down.sql \
        services/iam/db/role_permission_backfill_integration_test.go \
        services/iam/service.go
git commit -m "feat(iam): grant expense:read and inventory:read to system roles (#139)

Creates the vocabulary slice D's gates need. Nothing is gated in this
commit -- gating a permission nobody holds would lock everyone out, so the
grants land first.

expense:read is new. No existing string worked: gating the expense reads on
expense:submit locks out manager and coordinator, on expense:approve locks
out member, and RequirePermission takes a single string. Granted to the same
five roles as budget:read -- deliberately NOT member, because ListExpenses
has no submitted_by filter and would expose every colleague's amounts and
vendors to the lowest-privilege role.

inventory:read already existed but sat on inventory_manager alone, not even
super_admin, so gating the inventory reads would have locked out every role
that can check an asset out. Widened to the four roles already holding
inventory:checkout.

The migration covers existing tenants; the systemRoles edit covers future
ones. Both halves are required. It is idempotent by necessity: migrations/iam
runs against the public schema via make migrate-all AND against every new
tenant_<uuid> at CreateTenant.

The down migration excludes inventory_manager from the inventory:read
removal -- that role held it beforehand, and a blind array_remove would strip
a grant this migration never added.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Gate the six expense reads

**Files:**
- Modify: `services/expense/handler.go` (`GetPurchaseOrder`, `ListPurchaseOrders`, `GetExpense`, `ListExpenses`, `GetPettyCashAdvance`, `ListPettyCashAdvances`)
- Test: `services/expense/handler_test.go`

**Interfaces:**
- Consumes: the `"expense:read"` string, granted by Task 1.
- Produces: nothing later tasks depend on.

- [ ] **Step 1: Record the coverage baseline**

```bash
go test ./services/expense/ -cover -count=1 2>&1 | tail -1
```

- [ ] **Step 2: Add a `denyPerm` double and write the denial tests**

`services/expense/handler_test.go` has `allowAllPerm` but **no `denyPerm`**. Add it, mirroring `services/project/handler_test.go`:

```go
// denyPerm denies every permission, so a denial test can prove the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}
```

Then one denial test per gated RPC. Each `t.Fatal` sits on the repository fn that RPC calls on its happy path — **read the handler to confirm which fn that is** before writing each one. A status-code-only assertion can pass against ungated code when a mock default produces the same code by another route.

```go
func TestHandler_GetExpense_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, _, _ uuid.UUID) (*Expense, error) {
			t.Fatal("repository reached: GetExpense must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetExpense(ctxWithTenant(uuid.New()), &expensev1.GetExpenseRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Write the same shape for the other five. **The fn-field names are abbreviated and do not match the RPC names** — verified against `services/expense/service_test.go`:

| RPC | fn-field | signature |
|---|---|---|
| `GetPurchaseOrder` | `getPOFn` | `func(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error)` |
| `ListPurchaseOrders` | `listPOsFn` | `func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error)` |
| `GetExpense` | `getExpenseFn` | `func(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error)` |
| `ListExpenses` | `listExpensesFn` | `func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]Expense, error)` |
| `GetPettyCashAdvance` | `getPettyCashFn` | `func(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error)` |
| `ListPettyCashAdvances` | `listPettyCashFn` | `func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error)` |

- [ ] **Step 3: Run the denial tests to verify they fail**

```bash
go test ./services/expense/ -run Denied -v
```

Expected: all six FAIL — the handlers have no gate, so each `t.Fatal` fires. This is the teeth check; record what you saw.

- [ ] **Step 4: Insert the six gates**

In `services/expense/handler.go`, add to each of the six read RPCs, immediately after the existing `tenant.IDFromContext` block and **before** the `uuid.Parse`:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}
```

`GetExpense` becomes:

```go
func (h *Handler) GetExpense(ctx context.Context, req *expensev1.GetExpenseRequest) (*expensev1.Expense, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid expense ID")
	}

	e, err := h.svc.GetExpense(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return expenseToProto(e), nil
}
```

Apply the identical insertion to the other five. Do not change any permission string on the four already-gated write RPCs (`CreatePurchaseOrder`, `SubmitExpense`, `ApproveExpense`, `CreatePettyCashAdvance`), and do not gate `GetExpenseCategories` or `GetApprovalLimits` — they read vertical config, not tenant data.

- [ ] **Step 5: Repair exactly six flipped tests**

```bash
go test ./services/expense/ -count=1 2>&1 | grep -- '--- FAIL' | sort
```

**Prediction — exactly these six, all `_Success`:**
`TestHandler_GetPurchaseOrder_Success`, `TestHandler_ListPurchaseOrders_Success`, `TestHandler_GetExpense_Success`, `TestHandler_ListExpenses_Success`, `TestHandler_GetPettyCashAdvance_Success`, `TestHandler_ListPettyCashAdvances_Success`

They build the handler inline as `NewHandler(NewService(&mockRepo{...}))` with a custom mock and no checker, so `h.perm` is nil and `RequirePermission` returns `Internal`. Repair by appending the setter:

```go
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "pending"}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
```

**These must NOT flip:** the four `_InvalidID`/`_InvalidProductionID` tests use `newHandler()`, which already returns a handler with `allowAllPerm{}`; they pass the gate and still assert `InvalidArgument` from the parse below. The six `_NoTenant` tests pass `ctxWithVertical()` with no tenant and short-circuit at `tenant.IDFromContext` before the gate.

**If the count is not exactly 6, STOP and report.** Do not weaken any assertion.

- [ ] **Step 6: Run the full check**

```bash
go vet ./...
go test ./services/expense/ -race -cover -count=1
grep -cF 'interceptor.RequirePermission(ctx' services/expense/handler.go   # must be 10
git diff --stat gen/ migrations/   # must be empty for THIS task
```

The gate count goes from 4 to **10** (four pre-existing writes plus six reads). Note `grep -F` — the pattern contains `(`.

- [ ] **Step 7: Commit**

```bash
git add services/expense/handler.go services/expense/handler_test.go
git commit -m "fix(expense): gate the six read RPCs on expense:read (#139)

GetPurchaseOrder, ListPurchaseOrders, GetExpense, ListExpenses,
GetPettyCashAdvance and ListPettyCashAdvances returned tenant financial data
-- amounts, vendors, categories, approval state -- to any authenticated
member of the tenant.

Gated on expense:read, granted to super_admin, manager, coordinator,
accountant and project_supervisor by the previous commit. Guard order is
tenant -> permission -> parse, matching the write RPCs in the same file.

GetExpenseCategories and GetApprovalLimits are deliberately not gated: they
read vertical configuration rather than tenant data (decision D3), the same
class as the config lookups slice C left alone.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Gate the three inventory reads

**Files:**
- Modify: `services/inventory/handler.go` (`GetAsset`, `ListAssets`, `ListCheckouts`)
- Test: `services/inventory/handler_test.go`

**Interfaces:**
- Consumes: the `"inventory:read"` string, whose grant set Task 1 widened.
- Produces: nothing.

- [ ] **Step 1: Record the coverage baseline**

```bash
go test ./services/inventory/ -cover -count=1 2>&1 | tail -1
```

- [ ] **Step 2: Add a `denyPerm` double and write the denial tests**

`services/inventory/handler_test.go` has `allowAllPerm` but **no `denyPerm`**. Add it:

```go
// denyPerm denies every permission, so a denial test can prove the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}
```

Then three denial tests. The fn-field names and signatures, verified against `services/inventory/service_test.go`:

| RPC | fn-field | signature |
|---|---|---|
| `GetAsset` | `getAssetFn` | `func(ctx context.Context, tenantID, id uuid.UUID) (*Asset, error)` |
| `ListAssets` | `listAssetsFn` | `func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]Asset, error)` |
| `ListCheckouts` | `listCheckoutsFn` | `func(ctx context.Context, tenantID, assetID uuid.UUID) ([]AssetCheckout, error)` |

`listCheckoutsFn` gained its `tenantID` parameter in #157 — the signature above is the current one.

```go
func TestHandler_GetAsset_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getAssetFn: func(_ context.Context, _, _ uuid.UUID) (*Asset, error) {
			t.Fatal("repository reached: GetAsset must deny before querying")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetAsset(ctxWithTenant(uuid.New()), &inventoryv1.GetAssetRequest{
		Id: uuid.New().String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

- [ ] **Step 3: Run the denial tests to verify they fail**

```bash
go test ./services/inventory/ -run Denied -v
```

Expected: all three FAIL against the ungated handlers. Record it.

- [ ] **Step 4: Insert the three gates**

In `services/inventory/handler.go`, after each existing `tenant.IDFromContext` block and before any parse:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "inventory:read"); err != nil {
		return nil, err
	}
```

`GetAsset` becomes:

```go
func (h *Handler) GetAsset(ctx context.Context, req *inventoryv1.GetAssetRequest) (*inventoryv1.Asset, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}

	if err := interceptor.RequirePermission(ctx, h.perm, "inventory:read"); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid asset ID")
	}

	a, err := h.svc.GetAsset(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return assetToProto(a), nil
}
```

`ListCheckouts` gained a `tenant.IDFromContext` block in #157 but still has no permission gate — add one there too, in the same position. Do not change `CreateAsset`'s `"inventory:write"` or the two `"inventory:checkout"` strings, and do not gate `GetInventoryCategories`.

- [ ] **Step 5: Repair exactly three flipped tests**

```bash
go test ./services/inventory/ -count=1 2>&1 | grep -- '--- FAIL' | sort
```

**Prediction — exactly these three, all `_Success`:**
`TestHandler_GetAsset_Success`, `TestHandler_ListAssets_Success`, `TestHandler_ListCheckouts_Success`

Repair by appending `.WithPermissionChecker(allowAllPerm{})` to the inline handler construction.

**These must NOT flip:** `GetAsset_InvalidID` and `ListCheckouts_InvalidAssetID` use `newHandler()`, which already carries `allowAllPerm{}`; `ListCheckouts_PassesCallerTenantToRepo` (added by #157) supplies it explicitly; the three `_NoTenant` tests short-circuit at the tenant check.

**If the count is not exactly 3, STOP and report.** Do not weaken any assertion.

- [ ] **Step 6: Run the full check**

```bash
go vet ./...
go test ./services/inventory/ -race -cover -count=1
grep -cF 'interceptor.RequirePermission(ctx' services/inventory/handler.go   # must be 6
git diff --stat gen/ migrations/   # must be empty for THIS task
```

The gate count goes from 3 to **6**.

- [ ] **Step 7: Commit**

```bash
git add services/inventory/handler.go services/inventory/handler_test.go
git commit -m "fix(inventory): gate the three read RPCs on inventory:read (#139)

GetAsset, ListAssets and ListCheckouts returned the tenant's asset register
and checkout history to any authenticated member.

Slice C deferred these because inventory:read was granted to
inventory_manager alone -- not even super_admin -- so gating them would have
locked out every role that can check an asset out. The previous commit
widened the grant to the four roles that already hold inventory:checkout,
which makes gating safe.

ListCheckouts gained its tenant block in #157; this adds the permission
check above the parse, matching the guard order in the rest of the file.

GetInventoryCategories is deliberately not gated -- vertical configuration,
not tenant data (decision D3).

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Whole-branch verification

Run after all three tasks, before opening the PR.

- [ ] **Step 1: Build and vet**

```bash
go vet ./...
go vet -tags=integration ./services/iam/db/
go build ./cmd/...
```

- [ ] **Step 2: Full suite**

```bash
go test ./... -short -count=1
go test ./services/expense/ ./services/inventory/ ./services/iam/ -race -count=1
```

- [ ] **Step 3: Confirm the end state**

```bash
grep -cF 'interceptor.RequirePermission(ctx' services/expense/handler.go     # 10
grep -cF 'interceptor.RequirePermission(ctx' services/inventory/handler.go   # 6
grep -cF 'expense:read' services/iam/service.go                              # 5
grep -cF 'inventory:read' services/iam/service.go                            # 5 (4 new + inventory_manager)
ls migrations/iam/020_seed_read_permissions.*.sql                            # both files
```

Note every one of these uses `grep -F` — the patterns contain `(` or `:`.

- [ ] **Step 4: Constraints**

```bash
git diff --stat e1871c5..HEAD -- gen/    # must be empty
git status --short                        # clean; services/billing/ must NOT appear
git diff --stat -- migrations/            # EXPECTED non-empty: the two 020 files
```

- [ ] **Step 5: Coverage**

```bash
go test ./services/expense/ ./services/inventory/ ./services/iam/ -cover -count=1
```

Compare each against its task baseline. `expense`/`inventory` floor is 75%; `iam` is 85%.

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin fix/expense-inventory-reads-139d
```

The PR body must state:

- Nine read RPCs gated; `expense:read` is new vocabulary and `inventory:read`'s grant set was wrong.
- **DEPLOY ORDERING: the migration must run BEFORE the new service code.** If the gates land first, every existing tenant loses expense and inventory read access until the migration runs. The reverse order is harmless — the permission exists but nothing checks it yet. **This is the first slice in #139 where deploy order matters; call it out prominently.**
- `member` deliberately excluded from `expense:read`, with the reason (`ListExpenses` has no `submitted_by` filter) and the acknowledged cost (a member cannot read back their own submission), plus the follow-up issue **#165**.
- The down migration excludes `inventory_manager` from the `inventory:read` removal.
- Flag for senior review — security change, needs 2 approvals.

- [ ] **Step 7: Confirm CI**

```bash
gh pr checks <number>
```

**Local green is not CI green.** Two jobs matter most here and neither can run locally: `Migration Validate (up + down)` exercises the new migration in both directions, and `Integration Tests (real Postgres)` runs the idempotency test. Do not declare the PR ready until both pass.
