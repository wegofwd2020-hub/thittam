# billing Service Authorization Implementation Plan (#139 slice F)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the payment-method cross-tenant defect cluster in `billing`, wire fail-closed authorization, and gate its 13 tenant-bounded RPCs on two new permissions (`billing:read`/`billing:manage`) with a migration and seed updates.

**Architecture:** Four tasks. Task 1 creates the vocabulary (migration 022 + `systemRoles` + both seed fixtures) and **must land first**. Task 2 closes the #157-class payment-method cross-tenant cluster (three repo primitives, two live RPCs) by threading `tenantID` — no gates yet. Task 3 wires the fail-closed `perm` field into both `billing` constructors + `cmd/billing`, and gates all 13 RPCs. Task 4 is the migration integration test and the `systemRoles` grant test.

**Tech Stack:** Go 1.25, golang-migrate, pgx/v5, grpc-go, testify.

**Spec:** `docs/superpowers/specs/2026-07-23-billing-authz-139f-design.md` (committed on this branch at `a78aa38`). Read §4 (the cross-tenant cluster — the sharpest content), §2.2 (the deliberate `DownloadInvoice` narrowing), and §3 (two constructors).

## Global Constraints

- **Branch:** `fix/billing-authz-139f`, already created off `main` (`0a78bd5`, includes slice E's migration 021). This branch adds migration **022**.
- **NO Docker, NO database.** NEVER run `docker compose` with `-v`, `down`, or `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. Use `pkg/testdb` (skips without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. **CI's `Migration Validate (up + down)` and `Integration Tests (real Postgres)` jobs are the authoritative gates.**
- **`Migration Validate` runs against a freshly created empty database** — it validates 022's syntax only, not the grant matrix or idempotency. Those are proven solely by Task 4's integration test.
- **Do NOT run `make generate-sqlc`.** billing's generated layer is unused and stale (#160); regenerating would dirty it and drag #160 into this diff. `gen/` and `services/billing/db/queries.sql.go` must both stay unchanged. The payment-method fixes edit raw inline `p.db.Exec`/`QueryRow` statements, not `queries.sql`.
- **NEVER `git add -A`.** Use the scoped `git add` in each task's commit step.
- **Whole-tree `go vet ./...` is the completion gate for every task.** `billing.Repository` has two implementers — `db.Postgres` and `mockRepo` (`services/billing/service_test.go`). No hidden e2e/integration double exists for it, but `go vet ./...` is still the gate.
- **Guard order: tenant (`TenantFromRequest`) → permission (`RequirePermission`) → `uuid.Parse` → service call.** The gate goes after the tenant block and before any parse. `interceptor` is already imported in `handler.go`.
- Permission strings are **inline literals** (`"billing:read"`, `"billing:manage"`). No constants.
- **`billing` MUST use `interceptor.RequirePermission`** — it dials iam over gRPC (the reporting/ledger/document pattern). Not iam's in-process helper.
- Coverage floors per CLAUDE.md:77 — iam/general-ledger ≥ 85%, budget/expense ≥ 80%, others ≥ 75%. `billing` is `others`. `iam` is 87.3%.
- **Verification commands containing `$`, `(`, `)`, `:` or quotes MUST use `grep -F`.** Count failures with `go test ./... 2>&1 | grep -- '--- FAIL'`, NEVER `grep -E '^\s+--- FAIL'` (matches nothing; silently reports zero).

### The three-halves rule (learned #168, reinforced by D/E)

Adding a permission string touches **three** places: the migration (existing tenants), `systemRoles` in `services/iam/service.go` (new tenants), and **both seed fixtures** — `seeds/demo/xyz-cba/007_iam_roles.sql`, `seeds/template/new-tenant/001_tenant.sql` — which hardcode the seven roles and would silently revert the migration under `make db-reset`. Task 1 has `grep -cF` count checks.

### The grant matrix (spec §2.1) — every surface must match

| role | `billing:read` | `billing:manage` |
|---|:-:|:-:|
| super_admin | ✓ | ✓ |
| manager | ✓ | ✓ |
| accountant | ✓ | — |
| coordinator | — | — |
| member | — | — |
| inventory_manager | — | — |
| project_supervisor | — | — |

read → 3 roles, manage → 2 roles. `accountant` reads billing but does not manage it.

### The RPC → permission mapping (spec §2)

- `billing:read`: `GetSubscription`, `ListInvoices`, `GetInvoice`, `DownloadInvoice`, `ListPaymentMethods`, `GetUsageSummary`, `CheckPlanLimit`
- `billing:manage`: `CreateSubscription`, `UpgradeSubscription`, `CancelSubscription`, `AddPaymentMethod`, `RemovePaymentMethod`, `SetDefaultPaymentMethod`
- **`HandlePaymentWebhook` stays ungated** (unrouted; needs HMAC not JWT). Do not gate it.

---

### Task 1: Vocabulary — migration 022, systemRoles, seeds

Creates `billing:read` and `billing:manage`. **Nothing is gated here.**

**Files:**
- Create: `migrations/iam/022_seed_billing_permissions.up.sql`
- Create: `migrations/iam/022_seed_billing_permissions.down.sql`
- Modify: `services/iam/service.go` (the `systemRoles` var)
- Modify: `seeds/demo/xyz-cba/007_iam_roles.sql`
- Modify: `seeds/template/new-tenant/001_tenant.sql`

**Interfaces:**
- Produces: `"billing:read"` and `"billing:manage"` present on the seeded system roles per the matrix. Task 3 gates on those literals.

- [ ] **Step 1: Record the iam coverage baseline**

```bash
go test ./services/iam/ -cover -count=1 2>&1 | tail -1
```

Record it (expected `87.3%`); compare at Step 6.

- [ ] **Step 2: Write the up migration**

Create `migrations/iam/022_seed_billing_permissions.up.sql`:

```sql
-- 022_seed_billing_permissions.up.sql
-- #139 slice F: grant the two billing permissions to existing tenants.
--
-- systemRoles (services/iam/service.go) is edited in the same change for new
-- tenants; both seed fixtures too. All three halves required (see #168).
--
-- Idempotent by necessity: migrations/iam runs against the public schema via
-- `make migrate-all` AND against every new tenant_<uuid> at CreateTenant.
-- is_system = true only.

UPDATE roles SET permissions = array_append(permissions, 'billing:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'accountant')
  AND NOT ('billing:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'billing:manage')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('billing:manage' = ANY (permissions));
```

- [ ] **Step 3: Write the down migration**

Create `migrations/iam/022_seed_billing_permissions.down.sql`:

```sql
-- 022_seed_billing_permissions.down.sql
-- Reverse of 022. Both strings are new to every role this migration touches,
-- so removing each unconditionally across is_system roles is correct -- no
-- pre-existing grant to preserve (unlike slice D's inventory:read).

UPDATE roles SET permissions = array_remove(permissions, 'billing:read')   WHERE is_system = true;
UPDATE roles SET permissions = array_remove(permissions, 'billing:manage') WHERE is_system = true;
```

- [ ] **Step 4: Edit `systemRoles`**

In `services/iam/service.go`, read the current block (slices D and E edited it too). Add:

- `super_admin`: add `"billing:read", "billing:manage"`
- `manager`: add `"billing:read", "billing:manage"`
- `accountant`: add `"billing:read"` only

`coordinator`, `member`, `inventory_manager`, `project_supervisor` get neither. Place each string next to that role's existing entries; do not reorder or reformat the rest.

- [ ] **Step 5: Update both seed fixtures**

Both hardcode the seven roles. Read each file first and match its array-literal style exactly (they differ). Add per the matrix: `super_admin`/`manager` get both strings, `accountant` gets `billing:read` only, the other four get neither. Sanity-check:

```bash
grep -cF 'billing:read' seeds/demo/xyz-cba/007_iam_roles.sql       # expect 3
grep -cF 'billing:manage' seeds/demo/xyz-cba/007_iam_roles.sql     # expect 2
grep -cF 'billing:read' seeds/template/new-tenant/001_tenant.sql   # expect 3
grep -cF 'billing:manage' seeds/template/new-tenant/001_tenant.sql # expect 2
```

If any count is off, a role was missed or double-added — fix before committing.

- [ ] **Step 6: Verify and reconcile flips**

```bash
go vet ./...
go test ./services/iam/ -count=1 2>&1 | grep -- '--- FAIL' | sort
git diff --stat gen/          # must be empty
```

**Flip prediction: exactly 2.** `services/iam/service_test.go` has whole-list `assert.ElementsMatch` pins for two roles that this task changes — but wait: this task grants to `super_admin`, `manager`, `accountant`, none of which is `inventory_manager` or `project_supervisor` (the two roles slices D/E's whole-list tests pin). **So predict the count by reading the tests, not by assuming.** The whole-list pins are `TestSystemRoles_InventoryManagerPermissions` and `TestSystemRoles_ProjectSupervisorPermissions` — neither `inventory_manager` nor `project_supervisor` gains a billing permission, so **those two do NOT flip**. The namespace-filtered tests (`LedgerGrants`, `ReadGrants`, and slice E's `DocumentGrants`) collect only their own prefix, so billing is invisible to them.

**Therefore predict: exactly 0 flips.** Run the command above; if anything flips, STOP and report — it means a test pins a role this task changed (super_admin/manager/accountant) and the reading was wrong. Slice D predicted zero and got one; reconcile against the actual run and repair any flipped whole-list `ElementsMatch` by extending its literal, never weakening it.

Coverage ≥ the Step 1 baseline.

- [ ] **Step 7: Commit**

```bash
git add migrations/iam/022_seed_billing_permissions.up.sql \
        migrations/iam/022_seed_billing_permissions.down.sql \
        services/iam/service.go \
        seeds/demo/xyz-cba/007_iam_roles.sql \
        seeds/template/new-tenant/001_tenant.sql
git commit -m "feat(iam): grant billing:read/billing:manage to system roles (#139)

Creates the vocabulary slice F's gates need. Nothing is gated in this
commit -- gating a permission nobody holds locks everyone out.

billing:read -> super_admin, manager, accountant (finance roles).
billing:manage -> super_admin, manager. accountant reads billing but does
not change subscriptions or payment methods.

All three halves: migration for existing tenants, systemRoles for new ones,
both seed fixtures which would otherwise revert the migration under
make db-reset.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

If Step 6 turned up a flipped `service_test.go` test, add `services/iam/service_test.go` to the `git add`.

---

### Task 2: Close the payment-method cross-tenant cluster (#157-class)

Three repo primitives operate on a bare `id`, feeding two live cross-tenant RPCs (spec §4). **No gates in this task** — it closes the tenant holes; Task 3 gates. Doing it first means Task 3's `RemovePaymentMethod` gate lands on a handler that already has its tenant block.

**Files:**
- Modify: `services/billing/repository.go` (widen 2 method signatures)
- Modify: `services/billing/db/postgres.go` (`GetPaymentMethod`, `UpdatePaymentMethod`, `DeletePaymentMethod`)
- Modify: `services/billing/service.go` (`RemovePaymentMethod`, `SetDefaultPaymentMethod`)
- Modify: `services/billing/handler.go` (`RemovePaymentMethod` — add tenant block)
- Modify: `services/billing/service_test.go` (`mockRepo` — the two widened methods)
- Test: `services/billing/handler_test.go`

**Interfaces:**
- Produces: `Repository.GetPaymentMethod(ctx, tenantID, id uuid.UUID) (*PaymentMethod, error)`, `Repository.DeletePaymentMethod(ctx, tenantID, id uuid.UUID) error`, `Service.RemovePaymentMethod(ctx, tenantID, id uuid.UUID) error`. `UpdatePaymentMethod(ctx, pm *PaymentMethod)` keeps its signature (scoped via `pm.TenantID`).

- [ ] **Step 1: Record the billing coverage baseline**

```bash
go test ./services/billing/ -cover -count=1 2>&1 | tail -1
```

- [ ] **Step 2: Write the failing cross-tenant tests**

Add to `services/billing/handler_test.go`. `billing` has a `callerCtx(tid)` helper (supplies caller + tenant). Both tests assert the repo received the **caller's** tenant, not a request value — a status-code-only assertion could pass against the vulnerable code because the mock returns a usable row by default.

```go
func TestHandler_RemovePaymentMethod_PassesCallerTenantToRepo(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	var gotGetTenant, gotDelTenant uuid.UUID
	delCalled := false
	repo := &mockRepo{
		getPaymentMethodFn: func(_ context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
			gotGetTenant = tenantID
			return &PaymentMethod{ID: id, TenantID: tenantID}, nil
		},
		deletePaymentMethodFn: func(_ context.Context, tenantID, id uuid.UUID) error {
			gotDelTenant = tenantID
			delCalled = true
			return nil
		},
	}
	h := newHandlerWithRepo(repo)

	_, err := h.RemovePaymentMethod(callerCtx(callerTenant), &billingv1.RemovePaymentMethodRequest{
		PaymentMethodId: uuid.New().String(),
	})

	require.NoError(t, err)
	require.True(t, delCalled)
	require.Equal(t, callerTenant, gotGetTenant, "GetPaymentMethod must receive the caller's tenant")
	require.Equal(t, callerTenant, gotDelTenant, "the DELETE must be scoped to the caller's tenant")
}

func TestHandler_SetDefaultPaymentMethod_PassesCallerTenantToGet(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	var gotGetTenant uuid.UUID
	repo := &mockRepo{
		getPaymentMethodFn: func(_ context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
			gotGetTenant = tenantID
			return &PaymentMethod{ID: id, TenantID: tenantID}, nil
		},
		clearDefaultPaymentMethodsFn: func(_ context.Context, _ uuid.UUID) error { return nil },
		updatePaymentMethodFn:        func(_ context.Context, _ *PaymentMethod) error { return nil },
		listPaymentMethodsFn:         func(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) { return nil, nil },
	}
	h := newHandlerWithRepo(repo)

	_, _ = h.SetDefaultPaymentMethod(callerCtx(callerTenant), &billingv1.SetDefaultPaymentMethodRequest{
		PaymentMethodId: uuid.New().String(),
	})

	require.Equal(t, callerTenant, gotGetTenant,
		"SetDefaultPaymentMethod must fetch the payment method scoped to the caller's tenant, not a bare id")
}
```

**Confirm the exact `mockRepo` fn-field names and the `clearDefaultPaymentMethodsFn`/`listPaymentMethodsFn` names against `services/billing/service_test.go` before writing** — adapt and report if they differ.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./services/billing/ -run 'PassesCallerTenant' -v`
Expected: compile failure — the `getPaymentMethodFn`/`deletePaymentMethodFn` mock fields still have the old 2-arg signature. That compile failure is the correct starting state; fixing the signatures (next steps) is what makes the tests reachable, and they will then FAIL until the service/handler thread the tenant.

- [ ] **Step 4: Widen the Repository interface**

In `services/billing/repository.go`:

```go
	GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error)
	...
	DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error
```

`UpdatePaymentMethod(ctx, pm *PaymentMethod)` is unchanged — it scopes on `pm.TenantID`.

- [ ] **Step 5: Fix the Postgres implementation**

In `services/billing/db/postgres.go`:

```go
func (p *Postgres) GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*billing.PaymentMethod, error) {
	row := p.db.QueryRow(ctx, `
		SELECT id, tenant_id, type, display_name, is_default,
		       razorpay_token, stripe_pm_id, expires_at, created_at
		FROM payment_methods WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return scanPaymentMethod(row)
}

func (p *Postgres) UpdatePaymentMethod(ctx context.Context, pm *billing.PaymentMethod) error {
	tag, err := p.db.Exec(ctx, `
		UPDATE payment_methods
		SET is_default = $2, expires_at = $3
		WHERE id = $1 AND tenant_id = $4`, pm.ID, pm.IsDefault, pm.ExpiresAt, pm.TenantID)
	if err != nil {
		return fmt.Errorf("billing: update payment method: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return billing.ErrPaymentMethodNotFound
	}
	return nil
}

func (p *Postgres) DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM payment_methods WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("billing: delete payment method: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return billing.ErrPaymentMethodNotFound
	}
	return nil
}
```

`billing.ErrPaymentMethodNotFound` exists (`services/billing/errors.go:9`) and is already mapped to `codes.NotFound` in `grpcErr` (`handler.go:383`), so returning it needs no handler change. Confirm `scanPaymentMethod` maps `pgx.ErrNoRows` — read it; if `GetPaymentMethod` on an empty tenant-scoped result would return a raw scan error rather than `ErrPaymentMethodNotFound`, map it, and report.

- [ ] **Step 6: Fix the service layer**

In `services/billing/service.go`:

```go
func (s *Service) RemovePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	if _, err := s.repo.GetPaymentMethod(ctx, tenantID, id); err != nil {
		return fmt.Errorf("get payment method: %w", err)
	}
	return s.repo.DeletePaymentMethod(ctx, tenantID, id)
}

func (s *Service) SetDefaultPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	pm, err := s.repo.GetPaymentMethod(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("get payment method: %w", err)
	}
	if err := s.repo.ClearDefaultPaymentMethods(ctx, tenantID); err != nil {
		return fmt.Errorf("clear defaults: %w", err)
	}
	pm.IsDefault = true
	return s.repo.UpdatePaymentMethod(ctx, pm)
}
```

`pm.TenantID` now equals the caller's tenant (the scoped `GetPaymentMethod` guarantees it), so `UpdatePaymentMethod`'s `WHERE ... AND tenant_id = pm.TenantID` is safe.

- [ ] **Step 7: Add the tenant block to `RemovePaymentMethod`'s handler**

In `services/billing/handler.go`, `RemovePaymentMethod` currently has NO tenant block. Add it, and pass the tenant to the service. **No gate yet** — Task 3 adds that.

```go
func (h *Handler) RemovePaymentMethod(ctx context.Context, req *billingv1.RemovePaymentMethodRequest) (*emptypb.Empty, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.PaymentMethodId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid payment_method_id: %v", err)
	}
	if err := h.svc.RemovePaymentMethod(ctx, tenantID, id); err != nil {
		return nil, grpcErr(err)
	}
	return &emptypb.Empty{}, nil
}
```

`RemovePaymentMethodRequest` has a `TenantId` field (verified `gen/billing/v1/billing.pb.go:1169`), so `TenantFromRequest` works exactly as shown.

- [ ] **Step 8: Update the mockRepo**

In `services/billing/service_test.go`, widen the two fn-fields and their methods:

```go
	getPaymentMethodFn    func(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error)
	deletePaymentMethodFn func(ctx context.Context, tenantID, id uuid.UUID) error
```

```go
func (m *mockRepo) GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
	if m.getPaymentMethodFn != nil {
		return m.getPaymentMethodFn(ctx, tenantID, id)
	}
	return &PaymentMethod{ID: id, TenantID: tenantID}, nil
}
func (m *mockRepo) DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	if m.deletePaymentMethodFn != nil {
		return m.deletePaymentMethodFn(ctx, tenantID, id)
	}
	return nil
}
```

- [ ] **Step 9: Repair existing tests that called the old signatures**

Any existing test that stubs `getPaymentMethodFn`/`deletePaymentMethodFn` or calls `Service.RemovePaymentMethod(ctx, id)` now fails to compile. Find them:

```bash
go build ./services/billing/... 2>&1 | grep -F 'PaymentMethod' | head
```

Update each to the new arity. **Do not change any assertion** — only thread the tenant argument. Then:

```bash
go test ./services/billing/ -run 'PassesCallerTenant' -v   # now PASS
go test ./services/billing/ -count=1 2>&1 | grep -- '--- FAIL' | sort   # reconcile any other flips
```

- [ ] **Step 10: Full check**

```bash
go vet ./...
go test ./services/billing/ -race -cover -count=1
grep -cF 'WHERE id = $1 AND tenant_id' services/billing/db/postgres.go   # >= 3 (the 3 payment-method statements)
git diff --stat gen/ migrations/   # both empty this task
```

Coverage ≥ the Step 1 baseline.

- [ ] **Step 11: Commit**

```bash
git add services/billing/repository.go services/billing/db/postgres.go \
        services/billing/service.go services/billing/handler.go \
        services/billing/service_test.go services/billing/handler_test.go
git commit -m "fix(billing): close the payment-method cross-tenant cluster (#139)

Three repo primitives operated on a bare id: GetPaymentMethod,
UpdatePaymentMethod, DeletePaymentMethod all WHERE id = \$1. Two live
cross-tenant RPCs fed them.

RemovePaymentMethod had no tenant block at all -- any authenticated user
deleted any tenant's payment method by UUID. SetDefaultPaymentMethod
resolved tenantID but used it only for ClearDefaultPaymentMethods, then
fetched and updated a bare id -- flipping is_default on another tenant's
card (the inventory.CheckInAsset dropped-tenant shape from #157).

Threads tenantID through the three primitives (SQL gains AND tenant_id, with
RowsAffected -> NotFound so a cross-tenant id can't silently hit a foreign
row) and through Service.RemovePaymentMethod. GetPaymentMethod being scoped
means pm.TenantID equals the caller's tenant, so UpdatePaymentMethod's
predicate is safe. payment_methods already has tenant_id NOT NULL -- no
migration. Invoices and subscriptions were already tenant-scoped.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Wire fail-closed authorization and gate the 13 RPCs

Mirrors `document` (slice E): a required `perm` on **both** constructors, an IAM dial in `cmd/billing`, and 13 gates. The signature change and `cmd/billing` wiring land in one commit.

**Files:**
- Modify: `services/billing/handler.go` (struct, both `NewHandler`s, the 13 RPCs)
- Modify: `cmd/billing/main.go` (IAM dial + `NewHandlerWithDeps` call)
- Modify: `services/billing/handler_test.go` (construction helpers + denial tests)

**Interfaces:**
- Consumes: the two `"billing:*"` strings (Task 1); the tenant-safe `RemovePaymentMethod` handler (Task 2).
- Produces: `NewHandler(svc, perm)` and `NewHandlerWithDeps(svc, perm, docClient)`.

- [ ] **Step 1: Add the `perm` field and require it in both constructors**

In `services/billing/handler.go`:

```go
type Handler struct {
	billingv1.UnimplementedBillingServiceServer
	svc       *Service
	docClient documentv1.DocumentServiceClient
	perm      interceptor.PermissionChecker
}

// NewHandler creates a billing handler. perm is required; cmd/billing refuses
// to start when the checker is nil. billing dials iam over gRPC, so it uses
// interceptor.RequirePermission -- not iam's in-process helper.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}

// NewHandlerWithDeps adds the downstream document client. perm is required.
func NewHandlerWithDeps(svc *Service, perm interceptor.PermissionChecker, docClient documentv1.DocumentServiceClient) *Handler {
	return &Handler{svc: svc, perm: perm, docClient: docClient}
}
```

`go build ./...` now fails at every construction site — that is the point.

- [ ] **Step 2: Wire `cmd/billing/main.go`**

Add the IAM dial above the handler construction (around line 105), mirroring `cmd/document/main.go` / `cmd/reporting-analytics/main.go:92-101`, and change the `NewHandlerWithDeps` call to pass `iamPerm` second:

```go
	iamPerm, closeIAM, err := iamclient.DialFromEnv("billing")
	if err != nil {
		log.Fatalf("billing: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()
	if iamPerm == nil {
		log.Fatalf("billing: startup: %s is not set; billing cannot authorize without a permission checker", iamclient.EnvAddr)
	}
	handler := billing.NewHandlerWithDeps(svc, iamPerm, docClient)
```

Add the import `"github.com/wegofwd2020/thittam/pkg/iamclient"` (read `cmd/document/main.go`'s import block for the exact path). `iamPerm` is the concrete `*iamclient.PermissionChecker` at the nil check — plain pointer comparison, not the typed-nil trap.

- [ ] **Step 3: Fix the test construction helpers and add the permission doubles**

`services/billing/handler_test.go` has `newHandlerWithRepo(r Repository)` (~:88) and `newHandlerWithDocClient(r, doc)` (~:575), and NO `allowAllPerm`/`denyPerm`. Add both doubles (mirroring `services/project/handler_test.go`):

```go
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}
```

Thread `allowAllPerm{}` through the helpers:

```go
func newHandlerWithRepo(r Repository) *Handler {
	return NewHandler(NewService(r), allowAllPerm{})   // match the exact service constructor this helper uses
}
func newHandlerWithDocClient(r Repository, doc documentv1.DocumentServiceClient) *Handler {
	return NewHandlerWithDeps(NewService(r), allowAllPerm{}, doc)
}
```

**Read the two helpers first** — they may construct the service differently (`NewService(r)` vs a passed service). Match what they do, only inserting the checker argument. Add a `newHandlerWithRepoDeny` that threads `denyPerm{}` for the denial tests:

```go
func newHandlerWithRepoDeny(r Repository) *Handler {
	return NewHandler(NewService(r), denyPerm{})
}
```

- [ ] **Step 4: Write the 13 denial tests**

One per gated RPC. Each installs `t.Fatal` on the first repo/service fn its happy path reaches — trace by reading each handler and its service method (billing's mock fn-fields do not all match RPC names). Template (`GetSubscription`):

```go
func TestHandler_GetSubscription_Denied(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			t.Fatal("repository reached: GetSubscription must deny before querying")
			return nil, nil
		},
	}
	h := newHandlerWithRepoDeny(repo)

	_, err := h.GetSubscription(callerCtx(uuid.New()), &billingv1.GetSubscriptionRequest{})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Write the same shape for the other 12: `ListInvoices`, `GetInvoice`, `DownloadInvoice`, `ListPaymentMethods`, `GetUsageSummary`, `CheckPlanLimit` (all `billing:read`), and `CreateSubscription`, `UpgradeSubscription`, `CancelSubscription`, `AddPaymentMethod`, `RemovePaymentMethod`, `SetDefaultPaymentMethod` (all `billing:manage`). **Read each handler + service method to find the correct tripwire fn before writing** — a test faulting on a fn the RPC never reaches passes vacuously. Note `DownloadInvoice`'s first repo call is `GetInvoice` (its tripwire is the invoice fetch, not the doc client — the gate fires before the doc client is touched).

- [ ] **Step 5: Run the denial tests to verify they fail against the ungated handlers**

```bash
go test ./services/billing/ -run Denied -v
```

Expected: all 13 FAIL (handlers reach the repo, `t.Fatal` fires). Teeth check — record it.

- [ ] **Step 6: Insert the 13 gates**

In `services/billing/handler.go`, add to each RPC after the `TenantFromRequest` block and before the parse, with the permission from the mapping. `GetSubscription` becomes:

```go
func (h *Handler) GetSubscription(ctx context.Context, req *billingv1.GetSubscriptionRequest) (*billingv1.Subscription, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.TenantId)
	if err != nil {
		return nil, err
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "billing:read"); err != nil {
		return nil, err
	}
	...
```

Apply `billing:read` to the 7 reads and `billing:manage` to the 6 manage RPCs per the mapping. **Do NOT gate `HandlePaymentWebhook`.** `RemovePaymentMethod` already has its tenant block from Task 2 — insert the `billing:manage` gate between the tenant block and the parse.

- [ ] **Step 7: Run denial tests + reconcile flips**

```bash
go test ./services/billing/ -run Denied -v          # all 13 now PASS
go test ./services/billing/ -count=1 2>&1 | grep -- '--- FAIL' | sort
```

**Predict the flip count by reading the tests before this step.** Existing tests that build a handler via `newHandlerWithRepo`/`newHandlerWithDocClient` (now carrying `allowAllPerm{}`) pass the gate. Any test that constructed a handler another way, or asserted a specific non-denied outcome without a checker, flips. Enumerate them in the report; **if the observed count differs from your prediction, STOP and report.**

- [ ] **Step 8: Full check**

```bash
go vet ./...
go build ./cmd/...
go test ./services/billing/ -race -cover -count=1
grep -cF 'interceptor.RequirePermission(ctx' services/billing/handler.go   # must be 13
grep -cF 'iamclient.DialFromEnv' cmd/billing/main.go                       # 1
git diff --stat gen/ migrations/   # both empty this task
```

Coverage ≥ Task 2's figure; `billing` floor is 75%.

- [ ] **Step 9: Commit**

```bash
git add services/billing/handler.go cmd/billing/main.go services/billing/handler_test.go
git commit -m "fix(billing): wire fail-closed authz and gate 13 RPCs (#139)

billing had 13 ungated RPCs and no permission checker. Adds the
reporting/document pattern: perm required on both NewHandler and
NewHandlerWithDeps, and cmd/billing refuses to start without
IAM_SERVICE_ADDR -- the fourth fail-closed service.

billing:read on the 7 reads, billing:manage on the 6 manage RPCs.
HandlePaymentWebhook stays ungated (unrouted; needs HMAC not JWT).

Gating DownloadInvoice on billing:read deliberately narrows invoice
download to the finance roles -- slice E kept document:read wide to avoid
regressing while document was unprotected; this sets the actual policy.

The signature change and cmd/billing land in one commit (the #149 T3
lesson).

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Migration and grant tests

**Files:**
- Create: `services/iam/db/billing_permission_backfill_integration_test.go`
- Modify: `services/iam/service_test.go` (add `TestSystemRoles_BillingGrants`)

**Interfaces:**
- Consumes: migration 022 and the `systemRoles` edit (Task 1).

- [ ] **Step 1: Write the `systemRoles` grant test**

In `services/iam/service_test.go`, add `TestSystemRoles_BillingGrants`, mirroring slice E's `TestSystemRoles_DocumentGrants` (want-map + inline "nothing outside expected in the namespace" scan). Read that test first.

```go
func TestSystemRoles_BillingGrants(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"super_admin": {"billing:read", "billing:manage"},
		"manager":     {"billing:read", "billing:manage"},
		"accountant":  {"billing:read"},
	}
	byName := map[string][]string{}
	for _, r := range systemRoles {
		byName[r.name] = r.permissions
	}
	for role, expected := range want {
		perms := byName[role]
		for _, p := range expected {
			assert.Contains(t, perms, p, "%s must hold %s", role, p)
		}
	}
	// No role outside want holds any billing: permission; and within want,
	// nothing beyond expected.
	for _, r := range systemRoles {
		for _, p := range r.permissions {
			if len(p) >= 8 && p[:8] == "billing:" {
				assert.Contains(t, want[r.name], p, "%s holds unexpected %s", r.name, p)
			}
		}
	}
}
```

This fails if `billing:manage` is added to `accountant`, or any billing permission to `coordinator`/`member`/`inventory_manager`/`project_supervisor`.

- [ ] **Step 2: Run the grant test**

```bash
go test ./services/iam/ -run TestSystemRoles_BillingGrants -v
```

Expected: PASS (Task 1 edited `systemRoles`). If it FAILS, Task 1's edit and the matrix disagree — fix `service.go`, not the test.

- [ ] **Step 3: Write the migration integration test**

Create `services/iam/db/billing_permission_backfill_integration_test.go`. **First line the build tag, then blank line** — copy the header and `testdb` setup from `services/iam/db/document_permission_backfill_integration_test.go` (slice E's, on `main`). Copy the exact `UPDATE` statements from `022_seed_billing_permissions.up.sql` into consts with a keep-in-sync comment.

Cover: (1) `billing:read` idempotency (apply twice, count == 1); (2) `billing:manage` NOT granted to `accountant` (out-of-list enforcement — `accountant` is in the read list but not the manage list).

- [ ] **Step 4: Verify the tagged file compiles and check local skip**

```bash
go vet -tags=integration ./services/iam/db/
go test -tags=integration ./services/iam/db/ -run Billing -v
```

First MUST be clean (only local signal the file builds). Second SKIPs without `THITTAM_TEST_DSN` — expected, report as a SKIP not a pass. Do not stand up a database.

- [ ] **Step 5: Full check**

```bash
go vet ./...
go test ./services/iam/ -race -cover -count=1
git diff --stat gen/   # empty
```

`iam` coverage ≥ 87.3% (floor 85%).

- [ ] **Step 6: Commit**

```bash
git add services/iam/db/billing_permission_backfill_integration_test.go services/iam/service_test.go
git commit -m "test(iam): cover migration 022 idempotency and the billing grant matrix (#139)

Migration Validate runs against an empty DB, so it proves only 022's
syntax. The integration test is the sole check that grants land
idempotently and that billing:manage is not granted to accountant.

TestSystemRoles_BillingGrants pins the matrix on the code side: adding
billing:manage to accountant now fails a test.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Whole-branch verification

Run after all four tasks, before opening the PR.

- [ ] **Step 1: Build and vet**

```bash
go vet ./...
go vet -tags=integration ./services/iam/db/
go build ./cmd/...
```

- [ ] **Step 2: Suites**

```bash
go test ./... -short -count=1
go test ./services/billing/ ./services/iam/ -race -count=1
```

- [ ] **Step 3: End state**

```bash
grep -cF 'interceptor.RequirePermission(ctx' services/billing/handler.go   # 13
grep -cF 'billing:read' services/iam/service.go                            # 3
grep -cF 'billing:manage' services/iam/service.go                          # 2
grep -cF 'iamclient.DialFromEnv' cmd/billing/main.go                       # 1
grep -cF 'WHERE id = $1 AND tenant_id' services/billing/db/postgres.go     # >= 3
ls migrations/iam/022_seed_billing_permissions.*.sql                       # both files
```

- [ ] **Step 4: Constraints**

```bash
git diff --stat main..HEAD -- gen/                     # empty
git diff --stat main..HEAD -- services/billing/db/queries.sql.go   # empty (no generate-sqlc)
git status --short                                     # clean
git diff --stat main..HEAD -- migrations/              # the two 022 files
```

- [ ] **Step 5: Coverage**

```bash
go test ./services/billing/ ./services/iam/ -cover -count=1
```

`billing` ≥ 75%, `iam` ≥ 85%.

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin fix/billing-authz-139f
```

PR body must state:

- 13 RPCs gated; `billing` becomes the fourth fail-closed service (ledger, reporting, document, billing).
- **The payment-method cross-tenant cluster fix (§4)** — two live cross-tenant RPCs (`RemovePaymentMethod` delete, `SetDefaultPaymentMethod` write) closed by threading `tenantID`. This is #157-class and is arguably more important than the gates. Found by a design-time scan, not in the original slice scope.
- **`DownloadInvoice` narrows deliberately** — from all-callers to the finance roles (`billing:read`). Slice E kept `document:read` wide to avoid regressing; slice F sets the actual policy. Real behaviour change: coordinator/member/inventory_manager/project_supervisor lose invoice download.
- **DEPLOY ORDERING: migration 022 before the new billing code**, or existing tenants get hard `PermissionDenied` on all gated billing RPCs (billing is newly fail-closed). #166 tracks that nothing enforces it.
- `Migration Validate` is a syntax gate (empty DB); the grant matrix and idempotency are proven by the Task 4 integration test in CI's real-Postgres job.
- **#160 (billing sqlc drift) is NOT touched** — billing calls its generated layer zero times, so this slice never runs `generate-sqlc`.
- Flag for senior review — security change, 2 approvals.

- [ ] **Step 7: Confirm CI**

```bash
gh pr checks <number>
```

**Local green is not CI green.** `Migration Validate (up + down)` and `Integration Tests (real Postgres)` cannot run locally and are the only checks that exercise migration 022 and its idempotency test. Do not declare ready until both pass.
