# Expense + Inventory REST Gateways (#60 Phase C2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a grpc-gateway REST surface (via the shared `server.RunRESTGateway` helper) to expense-tracking and inventory-management for the RPCs the UI calls, and route them through Kong.

**Architecture:** Annotate the buildable RPCs with `google.api.http`, `buf generate` the gateway stubs, launch the gateway with the C1 helper in each cmd main, and add Kong routes (with a `/api/v1/config` longest-prefix split). No web, `pkg/server`, or migration changes.

**Tech Stack:** protobuf + buf 1.32.0 (grpc-gateway plugin), Go 1.25, Kong 3.6 (DB-less), `pkg/server.RunRESTGateway`.

## Global Constraints

- **Path-param placeholders are proto FIELD names**, not URL segments: `GetExpense`→`{id}`, `ApproveExpense`→`{expense_id}`, `GetAsset`→`{id}`, `CheckOutAsset`/`ListCheckouts`→`{asset_id}`.
- **Gateway ports:** expense **9082** (gRPC 8082), inventory **9084** (gRPC 8084). `ProjectHeader: true` for both; **no `Wrap`**.
- **Only the annotated RPCs get routes** (`generate_unbound_methods=false`). Adding annotations + the `google/api/annotations.proto` import is NOT a buf breaking change.
- **Scope each commit to its service:** stage only `proto/thittam/<svc>/` + `gen/<svc>/`. If `buf generate` also regenerates OTHER services' gen files (pre-existing comment drift), `git checkout --` those back and note it — do not commit unrelated churn.
- **Kong:** `strip_path: false` on every route; project keeps its broad `/api/v1/config` route (Kong longest-prefix match sends the specific config sub-paths to expense/inventory).
- **LOCAL DB SAFETY:** never `docker compose … -v`/`down`/`up` on `infra/local/`. Validate Kong with a throwaway `docker run --rm -e KONG_DATABASE=off … kong:3.6 kong config parse` (the `-e KONG_DATABASE=off` is required — a bare `kong:3.6` CLI defaults to the Postgres strategy and fails).
- **No web/client.ts, `pkg/server`, migration, or CI changes.**
- **Commits:** Conventional Commits, scope `expense`/`inventory`/`infra`; end every message with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `proto/thittam/expense/v1/expense.proto` | annotate 10 RPCs | 1 |
| `gen/expense/v1/*` | regenerated (`buf generate`) | 1 |
| `cmd/expense-tracking/main.go` | launch the gateway | 1 |
| `proto/thittam/inventory/v1/inventory.proto` | annotate 6 RPCs | 2 |
| `gen/inventory/v1/*` | regenerated | 2 |
| `cmd/inventory-management/main.go` | launch the gateway | 2 |
| `infra/local/kong.yml` | expense + inventory routes; config split | 3 |

---

### Task 1: Expense gateway (annotate → generate → wire)

**Files:**
- Modify: `proto/thittam/expense/v1/expense.proto`
- Modify (regenerated): `gen/expense/v1/*`
- Modify: `cmd/expense-tracking/main.go`

**Interfaces:**
- Consumes: `server.RunRESTGateway(ctx, server.GatewayConfig{...})` (already on `main`).
- Produces: REST routes on `:9082` for the 10 expense RPCs.

- [ ] **Step 1: Add the annotations import + `google.api.http` options**

In `proto/thittam/expense/v1/expense.proto`, add the import after the existing timestamp import:
```proto
import "google/protobuf/timestamp.proto";
import "google/api/annotations.proto";
```
Then give each of these ten RPCs an option block (leave `GetPurchaseOrder`/`GetPettyCashAdvance` unannotated — the UI doesn't call them). The service block becomes:
```proto
service ExpenseService {
  rpc CreatePurchaseOrder(CreatePurchaseOrderRequest) returns (PurchaseOrder) {
    option (google.api.http) = { post: "/api/v1/purchase-orders" body: "*" };
  }
  rpc GetPurchaseOrder(GetPurchaseOrderRequest) returns (PurchaseOrder);
  rpc ListPurchaseOrders(ListPurchaseOrdersRequest) returns (ListPurchaseOrdersResponse) {
    option (google.api.http) = { get: "/api/v1/purchase-orders" };
  }

  rpc SubmitExpense(SubmitExpenseRequest) returns (Expense) {
    option (google.api.http) = { post: "/api/v1/expenses" body: "*" };
  }
  rpc GetExpense(GetExpenseRequest) returns (Expense) {
    option (google.api.http) = { get: "/api/v1/expenses/{id}" };
  }
  rpc ListExpenses(ListExpensesRequest) returns (ListExpensesResponse) {
    option (google.api.http) = { get: "/api/v1/expenses" };
  }
  rpc ApproveExpense(ApproveExpenseRequest) returns (Expense) {
    option (google.api.http) = { post: "/api/v1/expenses/{expense_id}/approve" body: "*" };
  }

  rpc CreatePettyCashAdvance(CreatePettyCashAdvanceRequest) returns (PettyCashAdvance) {
    option (google.api.http) = { post: "/api/v1/petty-cash" body: "*" };
  }
  rpc GetPettyCashAdvance(GetPettyCashAdvanceRequest) returns (PettyCashAdvance);
  rpc ListPettyCashAdvances(ListPettyCashAdvancesRequest) returns (ListPettyCashAdvancesResponse) {
    option (google.api.http) = { get: "/api/v1/petty-cash" };
  }

  rpc GetExpenseCategories(GetExpenseCategoriesRequest) returns (GetExpenseCategoriesResponse) {
    option (google.api.http) = { get: "/api/v1/config/expense-categories" };
  }
  rpc GetApprovalLimits(GetApprovalLimitsRequest) returns (GetApprovalLimitsResponse) {
    option (google.api.http) = { get: "/api/v1/config/approval-workflow" };
  }
}
```

- [ ] **Step 2: Lint + regenerate**

Run from repo root:
```bash
buf lint
buf generate
```
Expected: both exit 0. If `buf generate` errors that `google/api/annotations.proto` can't be resolved, run `buf dep update` (the `buf.build/googleapis/googleapis` dep is already declared in `proto/buf.yaml`) and re-run `buf generate`.

- [ ] **Step 3: Scope the regen to expense; confirm the gateway file appeared**

Run:
```bash
git status --porcelain gen/ | sort
```
Expected: changes under `gen/expense/v1/` including a new/updated `expense.pb.gw.go`. If any OTHER service's gen files changed (pre-existing proto-comment drift, unrelated to this task), revert them so the commit stays scoped:
```bash
git checkout -- $(git status --porcelain gen/ | grep -v 'gen/expense/' | awk '{print $2}')
```
Then confirm the routes were generated:
```bash
grep -oE '/api/v1/(expenses|purchase-orders|petty-cash|config/(expense-categories|approval-workflow))[^"]*' gen/expense/v1/expense.pb.gw.go | sort -u
```
Expected: the paths `/api/v1/expenses`, `/api/v1/expenses/{expense_id}/approve` (grpc-gateway renders it with the field name), `/api/v1/purchase-orders`, `/api/v1/petty-cash`, `/api/v1/config/expense-categories`, `/api/v1/config/approval-workflow` appear.

- [ ] **Step 4: Wire the gateway in `cmd/expense-tracking/main.go`**

Insert, between the last `srv.RegisterHealthChecker(...)` line and `log.Printf("expense-tracking service ready …")`:
```go
	// --- REST gateway (grpc-gateway, #60 Phase C2) — via the shared helper. ---
	go func() {
		if err := server.RunRESTGateway(ctx, server.GatewayConfig{
			ServiceName:   "expense-tracking",
			GRPCEndpoint:  "localhost:8082",
			HTTPPort:      9082,
			Register:      expensev1.RegisterExpenseServiceHandlerFromEndpoint,
			ProjectHeader: true,
		}); err != nil {
			log.Fatalf("expense-tracking: gateway: %v", err)
		}
	}()
```
No new imports — `context` (`ctx`), `log`, `pkg/server`, and `expensev1` are already imported (`cmd/expense-tracking/main.go:10,11,20,26`). Use the `ctx` already in scope (the one passed to `auth.VerifierFromEnv(ctx)` etc.).

- [ ] **Step 5: Build + vet**

Run:
```bash
go build ./... && go vet ./cmd/expense-tracking/
```
Expected: both exit 0 (proves `RegisterExpenseServiceHandlerFromEndpoint` was generated and the wiring compiles).

- [ ] **Step 6: Commit**

```bash
git add proto/thittam/expense/v1/expense.proto gen/expense/v1 cmd/expense-tracking/main.go
git commit -m "feat(expense): grpc-gateway REST surface on :9082 (#60 Phase C2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Inventory gateway (annotate → generate → wire)

**Files:**
- Modify: `proto/thittam/inventory/v1/inventory.proto`
- Modify (regenerated): `gen/inventory/v1/*`
- Modify: `cmd/inventory-management/main.go`

**Interfaces:**
- Consumes: `server.RunRESTGateway`.
- Produces: REST routes on `:9084` for the 6 inventory RPCs.

- [ ] **Step 1: Add the annotations import + options (`CheckInAsset` left unannotated)**

In `proto/thittam/inventory/v1/inventory.proto` add `import "google/api/annotations.proto";` after the timestamp import, then annotate — leaving `CheckInAsset` bare (deferred, #192):
```proto
service InventoryService {
  rpc CreateAsset(CreateAssetRequest) returns (Asset) {
    option (google.api.http) = { post: "/api/v1/assets" body: "*" };
  }
  rpc GetAsset(GetAssetRequest) returns (Asset) {
    option (google.api.http) = { get: "/api/v1/assets/{id}" };
  }
  rpc ListAssets(ListAssetsRequest) returns (ListAssetsResponse) {
    option (google.api.http) = { get: "/api/v1/assets" };
  }

  rpc CheckOutAsset(CheckOutAssetRequest) returns (AssetCheckout) {
    option (google.api.http) = { post: "/api/v1/assets/{asset_id}/check-out" body: "*" };
  }
  rpc CheckInAsset(CheckInAssetRequest) returns (AssetCheckout);
  rpc ListCheckouts(ListCheckoutsRequest) returns (ListCheckoutsResponse) {
    option (google.api.http) = { get: "/api/v1/assets/{asset_id}/checkouts" };
  }

  rpc GetInventoryCategories(GetInventoryCategoriesRequest) returns (GetInventoryCategoriesResponse) {
    option (google.api.http) = { get: "/api/v1/config/inventory-categories" };
  }
}
```

- [ ] **Step 2: Lint + regenerate**

```bash
buf lint
buf generate
```
Expected: exit 0 (same `buf dep update` fallback as Task 1 if annotations don't resolve).

- [ ] **Step 3: Scope the regen to inventory; confirm routes**

```bash
git checkout -- $(git status --porcelain gen/ | grep -v 'gen/inventory/' | awk '{print $2}')  # revert any unrelated drift
grep -oE '/api/v1/(assets|config/inventory-categories)[^"]*' gen/inventory/v1/inventory.pb.gw.go | sort -u
```
Expected: `/api/v1/assets`, `/api/v1/assets/{id}`, `/api/v1/assets/{asset_id}/check-out`, `/api/v1/assets/{asset_id}/checkouts`, `/api/v1/config/inventory-categories` appear; **no** `check-in` route (CheckInAsset unannotated).

- [ ] **Step 4: Wire the gateway in `cmd/inventory-management/main.go`**

Insert between the last `srv.RegisterHealthChecker(...)` and the `log.Printf("inventory-management service ready …")`:
```go
	// --- REST gateway (grpc-gateway, #60 Phase C2) — via the shared helper. ---
	go func() {
		if err := server.RunRESTGateway(ctx, server.GatewayConfig{
			ServiceName:   "inventory-management",
			GRPCEndpoint:  "localhost:8084",
			HTTPPort:      9084,
			Register:      inventoryv1.RegisterInventoryServiceHandlerFromEndpoint,
			ProjectHeader: true,
		}); err != nil {
			log.Fatalf("inventory-management: gateway: %v", err)
		}
	}()
```
No new imports — `context`, `log`, `pkg/server`, `inventoryv1` already imported (`cmd/inventory-management/main.go:10,11,18,22`).

- [ ] **Step 5: Build + vet**

```bash
go build ./... && go vet ./cmd/inventory-management/
```
Expected: exit 0.

- [ ] **Step 6: Commit**

```bash
git add proto/thittam/inventory/v1/inventory.proto gen/inventory/v1 cmd/inventory-management/main.go
git commit -m "feat(inventory): grpc-gateway REST surface on :9084 (#60 Phase C2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Kong routes + config-prefix split

**Files:**
- Modify: `infra/local/kong.yml`

**Interfaces:**
- Consumes: Tasks 1–2 gateways on `:9082`/`:9084`.
- Produces: Kong routes so `:8500/api/v1/{expenses,purchase-orders,petty-cash,assets,config/…}` reach the right service.

- [ ] **Step 1: Add the expense + inventory services**

In `infra/local/kong.yml`, add two service blocks under the existing `services:` list (after the `project` block), matching the file's 2-space indent:
```yaml
  - name: expense
    url: http://host.docker.internal:9082
    routes:
      - name: expense
        paths:
          - /api/v1/expenses
          - /api/v1/purchase-orders
          - /api/v1/petty-cash
          - /api/v1/config/expense-categories
          - /api/v1/config/approval-workflow
        strip_path: false
  - name: inventory
    url: http://host.docker.internal:9084
    routes:
      - name: inventory
        paths:
          - /api/v1/assets
          - /api/v1/config/inventory-categories
        strip_path: false
```
Leave the `project` service's `/api/v1/config` route as-is — Kong routes the more specific config sub-paths (above) to expense/inventory by longest-prefix, and `entity-labels`/`phase-types` still fall to project.

- [ ] **Step 2: Validate the declarative config offline**

Run from repo root (throwaway container, no volumes, no compose up):
```bash
docker run --rm -e KONG_DATABASE=off -v "$PWD/infra/local/kong.yml:/kong.yml:ro" kong:3.6 kong config parse /kong.yml
```
Expected: `parse successful`. If it errors, fix the YAML and re-run. Do NOT run `docker compose up`/`down`/`-v`.

- [ ] **Step 3: Confirm all five services are present**

```bash
grep -E '^\s+- name:' infra/local/kong.yml
```
Expected: `iam`, `budget`, `project`, `expense`, `inventory` (service names) — five services now routed.

- [ ] **Step 4: Commit**

```bash
git add infra/local/kong.yml
git commit -m "feat(infra): route expense + inventory through Kong; split /api/v1/config (#60 Phase C2)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Expense: 10 annotations + gen + wiring (`:9082`, ProjectHeader) → Task 1 ✅
- Inventory: 6 annotations (CheckInAsset excluded) + gen + wiring (`:9084`) → Task 2 ✅
- Kong: expense + inventory services + config-prefix split (project keeps broad route) → Task 3 ✅
- Path placeholders = field names (`{expense_id}`, `{asset_id}`) → Tasks 1–2 exact annotations ✅
- No web/pkg-server/migration/CI change; reporting + unbacked endpoints deferred (#190/#191/#192) → Global Constraints + Non-goals ✅
- Testing: buf generate clean, build+vet, grep generated routes, kong config parse → each task's verify steps ✅

**Placeholder scan:** none — exact annotated service blocks, exact wiring blocks, exact Kong YAML, compiler/grep/parse-driven verification.

**Type consistency:** `server.GatewayConfig` fields (ServiceName/GRPCEndpoint/HTTPPort/Register/ProjectHeader) match the C1 helper on `main`. `Register` = the generated `Register<Svc>ServiceHandlerFromEndpoint` (produced by Task 1/2 Step 2). Ports 9082/9084 consistent between the wiring (Tasks 1-2) and Kong upstreams (Task 3).

**Ordering:** Tasks 1 & 2 are independent (different services); Task 3 (Kong) references both gateways' ports and should land after them. Each task builds the whole tree.
