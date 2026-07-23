# billing service authorization — full wiring, new vocabulary, and a cross-tenant DELETE — design

**Issue:** #139, slice F. **Branch:** `fix/billing-authz-139f`, base `0a78bd5` (`main`, includes slice E's migration 021).
**Follows:** #138, #144, #146, #149, slice A (#155), slice C (#158), #157 (#161), slice B (#164), slice D (#168), slice E (#169).
**Policy table:** `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`.

## 1. What this slice is

`billing` has **fourteen RPCs, thirteen ungated** and one already-authenticated-but-unrouted. It is the second-to-last zero-authorization service (F billing, G notifications). Like `document` (slice E) it needs the full fail-closed wiring *and* new vocabulary + migration + seeds. It also has something slice E did not: a **cluster of live cross-tenant payment-method defects** — a DELETE and a WRITE, both reaching other tenants' cards — that a permission gate cannot fix (§4).

Every gated RPC is already tenant-bounded via `TenantFromRequest`, and `billing` already authenticates (`cmd/billing/main.go:117` installs `UnaryAuthInterceptor`), so the caller is in context. This slice adds authorization on top of authentication that already works — plus one tenant-scoping fix.

Two things simplify this slice relative to E:

- **`#160` (billing's stale sqlc) is not in scope.** `billing` calls its generated query layer **zero** times — every data access is hand-written SQL (`grep -c 'p\.q\.' services/billing/db/postgres.go` → 0). The gates are handler-only, and this slice does **not** run `make generate-sqlc`, so the drift is neither touched nor fixed here. #160 remains its own issue.
- **Nothing cross-service dials billing.** `NewBillingServiceClient` has zero non-generated callers tree-wide, so gating billing breaks no forwarding path — unlike `document`, which `billing` itself dials.

## 2. Vocabulary and grants — two strings

| permission | RPCs |
|---|---|
| `billing:read` | `GetSubscription`, `ListInvoices`, `GetInvoice`, `DownloadInvoice`, `ListPaymentMethods`, `GetUsageSummary`, `CheckPlanLimit` |
| `billing:manage` | `CreateSubscription`, `UpgradeSubscription`, `CancelSubscription`, `AddPaymentMethod`, `RemovePaymentMethod`, `SetDefaultPaymentMethod` |

Read/write split, matching `budget`/`expense`. Subscription changes and payment-method changes are the same "manage billing" privilege — no product distinction today warrants a third string (YAGNI).

**`CheckPlanLimit` is `billing:read`** — this resolves open decision **D6**. It is a pre-flight capacity check ("is there room for another production?") that reads plan limits and current usage. Nothing cross-service dials it (billing has no in-tree gRPC callers), so it is invoked only by authenticated clients through the gateway; `billing:read` is the right gate.

**`HandlePaymentWebhook` stays ungated.** It is not on `PublicMethods` (so it already requires a token) and carries no `google.api.http` annotation (so nothing routes to it). When webhooks go live they need **gateway HMAC signature verification, not a JWT permission gate** — a different mechanism entirely. Gating it on a billing permission would be theatre on a dead RPC. Out of scope, per the policy table §6.

### 2.1 Grant matrix

| role | `billing:read` | `billing:manage` |
|---|:-:|:-:|
| super_admin | ✓ | ✓ |
| manager | ✓ | ✓ |
| accountant | ✓ | — |
| coordinator | — | — |
| member | — | — |
| inventory_manager | — | — |
| project_supervisor | — | — |

Billing is financial data — invoices, subscriptions, payment instruments. Read goes to the three finance-touching roles: `super_admin`, `manager`, `accountant` (who reads invoices and usage as their job). Manage is narrower — `super_admin` and `manager` only; an `accountant` reads billing but does not change subscriptions or remove payment methods.

### 2.2 The `DownloadInvoice` behaviour change — deliberate

Slice E granted `document:read` to **all seven roles** specifically so invoice download would not regress: `billing.DownloadInvoice` was ungated and forwarded the caller's token to `document.GetDownloadURL`, so slice E kept the downstream door wide to avoid breaking the upstream.

Slice F now gates `DownloadInvoice` on `billing:read` (three roles). **This narrows invoice download to the finance roles** — `coordinator`, `member`, `inventory_manager` and `project_supervisor` can call it today and will get `PermissionDenied` after this slice. That is the intended policy: invoices are financial data, and the billing gate is where that policy belongs. `document:read`'s width is now irrelevant to this path — the billing gate refuses the call before it ever reaches document. **This must be called out in the PR body** as a real behaviour change, not a silent one.

The two slices are consistent: E was "don't regress while document is unprotected," F is "now set the actual policy."

## 3. Wiring — mirror document/reporting, but note two constructors

`billing` gets the fail-closed startup #149 built for ledger and slices C/E replicated. **`billing` has two constructors** — `NewHandler(svc)` and `NewHandlerWithDeps(svc, docClient)` (the latter is what `cmd/billing` uses) — and both must gain the required `perm`:

```go
type Handler struct {
	billingv1.UnimplementedBillingServiceServer
	svc       *Service
	docClient documentv1.DocumentServiceClient
	perm      interceptor.PermissionChecker
}

// NewHandler creates a billing handler. perm is required.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}

// NewHandlerWithDeps adds the downstream document client. perm is required.
func NewHandlerWithDeps(svc *Service, perm interceptor.PermissionChecker, docClient documentv1.DocumentServiceClient) *Handler {
	return &Handler{svc: svc, perm: perm, docClient: docClient}
}
```

`cmd/billing/main.go` gains the IAM dial and refuse-to-start, mirroring `cmd/reporting-analytics/main.go` / `cmd/document/main.go`, and calls `NewHandlerWithDeps(svc, iamPerm, docClient)`. This makes `billing` the **fourth fail-closed service** (ledger, reporting, document, billing); project/budget/expense/inventory remain log-and-proceed (#167).

**Compile-break shape:** the required-param change breaks both constructors, every test construction site, and `cmd/billing/main.go`. All land in the same commit as the signature change, or that commit does not build (#149 Task 3 lesson).

`IAM_SERVICE_ADDR` already reaches the billing pod via the shared `thittam-common` ConfigMap; confirm during implementation.

## 4. The payment-method cross-tenant cluster — #157-class, TWO live RPCs

**A scan of billing during design expanded this from one defect to a cluster.** Invoices and subscriptions are clean — `GetInvoice(ctx, tenantID, id)` and `GetSubscriptionByTenant(ctx, tenantID)` are tenant-scoped. The defect is confined to **payment methods**: three repository primitives operate on a bare `id`, feeding two live cross-tenant RPCs. A permission gate cannot fix any of it — a caller holding `billing:manage` in their own tenant would still reach another tenant's card.

**The three unsafe repo primitives** (`services/billing/db/postgres.go`):

| primitive | SQL | line |
|---|---|---|
| `GetPaymentMethod(ctx, id)` | `SELECT ... FROM payment_methods WHERE id = $1` | :435 |
| `UpdatePaymentMethod(ctx, pm)` | `UPDATE payment_methods SET is_default=$2, expires_at=$3 WHERE id = $1` | :465 |
| `RemovePaymentMethod(ctx, id)` | `DELETE FROM payment_methods WHERE id = $1` | :477 |

**The two live RPC-level defects:**

1. **`RemovePaymentMethod` — cross-tenant DELETE.** The handler has **no tenant block at all**; it parses `payment_method_id` and calls `Service.RemovePaymentMethod(ctx, id)` → the bare `DELETE`. Any authenticated user deletes any tenant's payment method by UUID.

2. **`SetDefaultPaymentMethod` — cross-tenant WRITE, the #157 "dropped tenant" shape.** The handler resolves `tenantID` correctly, but `Service.SetDefaultPaymentMethod(ctx, tenantID, id)` uses it only for `ClearDefaultPaymentMethods(ctx, tenantID)` and then calls `GetPaymentMethod(ctx, id)` + `UpdatePaymentMethod(pm)` on a bare, caller-supplied `id`. A caller in tenant A flips `is_default` on tenant B's card. The re-fetch via `ListPaymentMethods(tenantA)` returns `NotFound` — *after* the cross-tenant write already landed. This is exactly `inventory.CheckInAsset` from #157: the tenant is in hand and discarded.

### 4.1 The fix — thread `tenantID` through the three primitives

`payment_methods` already declares `tenant_id UUID NOT NULL` (`migrations/billing/001_create_tables.up.sql:54`), so this is #157-shaped and needs **no migration**:

- **Repo:** `GetPaymentMethod(ctx, tenantID, id)`, `UpdatePaymentMethod` scoped by tenant, `RemovePaymentMethod(ctx, tenantID, id)` — each SQL gains `AND tenant_id = $N`. The DELETE and the by-id read/update get a `RowsAffected()==0`/`ErrNoRows → ErrPaymentMethodNotFound` mapping so a cross-tenant id returns `NotFound`, not a silent no-op or a foreign row.
- **Service:** `RemovePaymentMethod(ctx, tenantID, id)` gains the tenant; `SetDefaultPaymentMethod` passes its already-held `tenantID` into `GetPaymentMethod`.
- **Handler:** `RemovePaymentMethod` gains the `TenantFromRequest` block it lacks. `SetDefaultPaymentMethod`'s handler already resolves the tenant — no handler change beyond the gate.

Guard-by-type applies: once `GetPaymentMethod`/`RemovePaymentMethod` require a `tenantID` parameter, neither RPC can compile without supplying it, and `SetDefaultPaymentMethod`'s drop becomes impossible to reintroduce.

This is done here, not deferred: it is a live cross-tenant read/write/delete cluster in the exact files this slice already rewrites, and the same class #157 fixed for project/budget/inventory. **It is the sharpest content in the slice — arguably more important than the gates**, since the gates only stop unauthorized *callers* while this stops authorized callers reaching *other tenants*.

### 4.2 No further billing defects

The scan is complete for billing: no other handler parses an id without a tenant block, and no other tenant-scoped table's by-id SQL omits the predicate. The `event_outbox`/`event_outbox_dead` statements that use `WHERE id = $1` are the dispatcher's internal outbox processing (keyed by outbox row id, not tenant data — the same correct-unscoped class as reporting's projection watermarks), not caller-reachable cross-tenant paths.

## 5. Design — the gates

Thirteen gate insertions, guard order **tenant → permission → parse → service**, after the existing tenant block and before any parse, matching every prior slice. Permission strings are inline literals. `RemovePaymentMethod` additionally gains the tenant block it lacks (§4).

No proto change. The only repository/service signature change is `RemovePaymentMethod` gaining `tenantID` (§4).

## 6. The migration and seeds — the three-halves pattern

`migrations/iam/022_seed_billing_permissions.{up,down}.sql`, idempotent, `is_system = true` only:

```sql
-- up
UPDATE roles SET permissions = array_append(permissions, 'billing:read')
WHERE is_system = true AND name IN ('super_admin','manager','accountant')
  AND NOT ('billing:read' = ANY (permissions));

UPDATE roles SET permissions = array_append(permissions, 'billing:manage')
WHERE is_system = true AND name IN ('super_admin','manager')
  AND NOT ('billing:manage' = ANY (permissions));
```

The down migration removes both unconditionally across `is_system` roles — both strings are new to every role, so no exclusion (contrast slice D's `inventory_manager`).

**Three halves** (the #168 lesson, reinforced by E): the migration (existing tenants), the `systemRoles` edit in `services/iam/service.go` (new tenants), and **both seed fixtures** — `seeds/demo/xyz-cba/007_iam_roles.sql`, `seeds/template/new-tenant/001_tenant.sql`. The plan's count checks are `billing:read` = 3 per surface, `billing:manage` = 2.

## 7. Testing

**Denial tests** — one per gated RPC, `t.Fatal` on the first repository/service fn its happy path reaches, traced by reading the handler (billing's mock fn-fields may not match RPC names). `billing`'s test doubles are checked for `allowAllPerm`/`denyPerm`; both added if missing.

**The payment-method cross-tenant tests** — one per live defect (§4). For `RemovePaymentMethod`: a caller in tenant A cannot delete tenant B's card — assert the repo received the caller's tenant and a cross-tenant id yields `NotFound` with no delete on the wrong row. For `SetDefaultPaymentMethod`: a caller in tenant A cannot flip `is_default` on tenant B's card — assert `GetPaymentMethod`/`UpdatePaymentMethod` receive the caller's tenant. Both assert on the tenant the repo *received*, not just a status code (the #157 denial-test rule). These are the tests that prove the §4 cluster fix and matter more than the gate denial tests.

**Flip predictions, measured before implementation.** Two shapes:
1. The `NewHandler`/`NewHandlerWithDeps` required-param change is a **compile break** across every construction site — enumerated in the plan, fixed by threading a checker, no runtime flip.
2. Any existing test that passes a valid tenant but no permission-granting checker flips once gated. Predict the exact count by reading the tests; if the actual differs, stop and report (the discipline that caught mispredictions in B and D).

**The migration integration test** — idempotency for both strings (apply twice, count == 1), and an out-of-list assertion (`billing:manage` not granted to `accountant`). `//go:build integration`; `go vet -tags=integration ./services/iam/db/` is the only local compile signal; SKIPs without `THITTAM_TEST_DSN`; CI's real-Postgres job is authoritative.

**`TestSystemRoles_BillingGrants`** — pins the matrix on the code side (want-map + none-list, or the namespace-scan shape slice E used), so adding `billing:manage` to `accountant` on the `systemRoles` side fails a test.

**The billing → document e2e gap (from slice E's review).** `services/billing/handler_test.go` uses a `mockDocClient` gRPC double, so `DownloadInvoice`'s test never crosses into document's real gate. Slice E's whole-branch review flagged this. This slice gates `DownloadInvoice` at the billing layer, which the billing unit test *does* exercise (the gate is in billing's handler, above the doc client) — so the billing gate is unit-covered even though the downstream document gate still is not. Note in the PR whether an e2e test for the full billing→document path is added here or deferred; adding one is in scope if cheap, but the billing-layer gate is the load-bearing new control.

**Coverage:** `billing` is in the `others ≥ 75%` tier; record the baseline. `iam` is 87.3%, floor 85%.

## 8. Constraints

- Security change. Senior review; 2 approvals.
- **No Docker, no database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/`. Use `pkg/testdb` (skips without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. CI's `Migration Validate` and `Integration Tests (real Postgres)` are the authoritative gates. **Binds delegated subagents.**
- **`Migration Validate` runs against an empty database** — 022's grant matrix and idempotency are proven only by the integration test.
- **Do NOT run `make generate-sqlc`.** billing's generated layer is unused and stale (#160); regenerating would dirty it and drag #160 into this diff. Gates are handler-only; the `RemovePaymentMethod` fix edits a raw inline statement, not `queries.sql`. `gen/` and `services/billing/db/queries.sql.go` must both be unchanged.
- `migrations/` WILL show a diff (the two 022 files). `gen/` empty.
- Whole-tree `go vet ./...` is the gate. Both constructors and every call site land in one commit.
- **Verification commands with `$`, `(`, `)`, `:` or quotes use `grep -F`.** Count flips with `grep -- '--- FAIL'`, never `grep -E '^\s+--- FAIL'`.
- Coverage floors per CLAUDE.md:77 — iam/general-ledger ≥ 85%, budget/expense ≥ 80%, others ≥ 75% (billing is `others`).
- **Deploy ordering: migration 022 before the new billing code**, or existing tenants lose all gated billing RPCs (hard `PermissionDenied`, because billing is also newly fail-closed). #166 tracks that nothing enforces it. State in the PR body.
- `gh pr checks` before declaring the PR ready.

## 9. Out of scope

- **`HandlePaymentWebhook` HMAC verification** — a different mechanism; belongs with the webhook-go-live work.
- **#160 billing sqlc drift** — untouched here; its own issue.
- **Converting the four log-and-proceed services to fail-closed** — #167.
- **`notifications` (slice G), the #159 audit (slice H), machine tokens (slice I).**
- **Per-invoice / per-payment-method ownership beyond the tenant boundary.** This slice closes the cross-tenant hole and adds role gates; it does not add "only the user who added a card may remove it." A `billing:manage` holder can manage any payment method in their tenant. Finer scoping is a separate design.
