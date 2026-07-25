# Expense + Inventory REST Gateways (#60 Phase C2) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-25
**Issue:** #60 (REST→gRPC bridge), **Phase C2** — gateway the buildable subset of the UI-facing services
**Branch:** `feat/gateway-expense-inventory-60c2` off `main` (`d9c38fe`)
**Migration:** none

## Goal

Give expense-tracking and inventory-management a grpc-gateway REST surface — via
the shared `server.RunRESTGateway` helper (#60 C1) — for exactly the RPCs the UI
calls that have a matching request contract, and route them through Kong. This
makes the expense and inventory pages reach live data through the single origin
`:8500`.

## Context

Phase A gave iam/budget/project a gateway; C1 extracted the shared helper; B put
Kong (`:8500`) in front. C2 was meant to gateway the three remaining UI-facing
services (expense, inventory, reporting). Grounding revealed the UI was built
**ahead of the backend**, so "annotate the RPCs the UI calls" mostly hits RPCs
that do not exist:

- **reporting-analytics is not buildable now.** Of six dashboard endpoints the
  UI calls, five (`portfolio`/`financial`/`approvals`/`team`/`compliance`) have
  **no gRPC RPC** — only unwired Go `Service` methods in
  `services/reporting/dashboard_service.go`. The sixth (`GetDashboardSummary`)
  name-matches but returns a flat proto (`tenant_id, project_count,
  total_budgeted, …`) where the UI expects a composite of the other five.
  Additionally `web/src/lib/api/dashboard.ts` uses `/v1/reports` (missing the
  `/api` prefix) and `reports.ts` is all-mock. Gatewaying reporting requires
  writing ~5 new RPCs + handlers + response messages — a feature effort, not a
  gateway. **Deferred to a follow-up.**
- **Several expense/inventory endpoints have no backing RPC** and are likewise
  deferred (see Non-goals).

So C2 ships the **cleanly-buildable subset**: 10 expense RPCs + 6 inventory RPCs.

### Grounding facts (measured on `main` `d9c38fe`)

- Neither proto imports `google/api/annotations.proto` or has any
  `google.api.http` today. buf 1.32.0 is installed; `buf.gen.yaml` already has
  the grpc-gateway plugin with `generate_unbound_methods=false` (only annotated
  RPCs get routes) — no buf config change needed.
- Path-param placeholders bind by **proto field name**, not URL segment:
  `GetExpenseRequest.id`, `ApproveExpenseRequest.expense_id`,
  `GetAssetRequest.id`, `CheckOutAssetRequest.asset_id`,
  `ListCheckoutsRequest.asset_id` — confirmed.
- gRPC ports / gateway ports (`Port+1000`, matching the convention): expense
  8082→**9082**, inventory 8084→**9084**. cmd insertion point is after the last
  `RegisterHealthChecker`, before `srv.Run()`. gen aliases: `expensev1`,
  `inventoryv1`.
- `NEXT_PUBLIC_API_URL=http://localhost:8500` is the committed dev default
  (`web/.env.development`), so `client.ts`'s `resolveBaseUrl` returns Kong for
  every path — the per-prefix table is dead code under Kong. **No web change is
  needed**: once Kong routes expenses/assets, the pages work.

## Design

### 1. Annotate the expense RPCs (`proto/thittam/expense/v1/expense.proto`)

Add `import "google/api/annotations.proto";`, then an `option (google.api.http)`
on each of the ten:

| RPC | annotation |
|---|---|
| `ListExpenses` | `get: "/api/v1/expenses"` |
| `GetExpense` | `get: "/api/v1/expenses/{id}"` |
| `SubmitExpense` | `post: "/api/v1/expenses" body: "*"` |
| `ApproveExpense` | `post: "/api/v1/expenses/{expense_id}/approve" body: "*"` |
| `ListPurchaseOrders` | `get: "/api/v1/purchase-orders"` |
| `CreatePurchaseOrder` | `post: "/api/v1/purchase-orders" body: "*"` |
| `ListPettyCashAdvances` | `get: "/api/v1/petty-cash"` |
| `CreatePettyCashAdvance` | `post: "/api/v1/petty-cash" body: "*"` |
| `GetExpenseCategories` | `get: "/api/v1/config/expense-categories"` |
| `GetApprovalLimits` | `get: "/api/v1/config/approval-workflow"` |

`GetPurchaseOrder`/`GetPettyCashAdvance` exist but the UI never calls them —
left unannotated (no route generated).

### 2. Annotate the inventory RPCs (`proto/thittam/inventory/v1/inventory.proto`)

Add the annotations import, then:

| RPC | annotation |
|---|---|
| `ListAssets` | `get: "/api/v1/assets"` |
| `GetAsset` | `get: "/api/v1/assets/{id}"` |
| `CreateAsset` | `post: "/api/v1/assets" body: "*"` |
| `CheckOutAsset` | `post: "/api/v1/assets/{asset_id}/check-out" body: "*"` |
| `ListCheckouts` | `get: "/api/v1/assets/{asset_id}/checkouts"` |
| `GetInventoryCategories` | `get: "/api/v1/config/inventory-categories"` |

The UI's extra `category_id`/`search` (on `ListAssets`) and `limit`/`after` (on
`ListCheckouts`) query params have no matching request fields — grpc-gateway
silently ignores unknown query params, so the endpoint works but returns
unfiltered/unpaginated results. That filter gap is a follow-up (Non-goals), not
a C2 blocker. `CheckInAsset` is **excluded** — its request requires a
`checkout_id` the UI never has (see Non-goals).

### 3. Regenerate

`buf generate` produces `gen/expense/v1/expense.pb.gw.go` and
`gen/inventory/v1/inventory.pb.gw.go` (and refreshes the `.pb.go` for the new
options). Adding annotations is not a buf **breaking** change (the `FILE`
category flags removals/renames, not added options), so the
`Protobuf Breaking Change Detection` CI job stays green. Generated files are
committed; a missing registrar would fail the `Build` job.

### 4. cmd wiring (`cmd/expense-tracking/main.go`, `cmd/inventory-management/main.go`)

After the last `RegisterHealthChecker`, before `srv.Run()`, launch the shared
helper:

```go
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

inventory is identical with `"inventory-management"` / `localhost:8084` / `9084`
/ `inventoryv1.RegisterInventoryServiceHandlerFromEndpoint`. **`ProjectHeader:
true`** for both — expenses and assets are project-scoped domains (like
budget/project), so forwarding `X-Project-Id` keeps project-scoped RBAC working;
it is harmless if a handler ignores it. No `Wrap` (no rate-limiting).

### 5. Kong routes + config-prefix split (`infra/local/kong.yml`)

Add two services with `strip_path: false`:

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

The project service keeps its broad `/api/v1/config` route (backing
`entity-labels`/`phase-types`). Kong resolves overlapping prefixes by
**longest-prefix match**, so the new specific `/api/v1/config/expense-categories`
etc. entries win for their sub-paths while `/api/v1/config/*` otherwise still
falls to project. Validate the config offline with `docker run --rm kong:3.6
kong config parse` (as in Phase B) — never `docker compose up`.

### 6. No web change

`client.ts` already routes every `/api/v1/*` call to Kong under the committed
`NEXT_PUBLIC_API_URL=:8500`; adding Kong routes is all that's needed for the
expense/inventory pages to reach live data. The dead per-prefix fallback table is
left untouched (out of scope), and the reporting `/v1/reports` prefix bug is
deferred with reporting.

## Testing

Dev-tooling + proto, not a new CI job. Verification:

- `buf generate` runs clean; `git status` shows only the expected
  `gen/{expense,inventory}/v1/*` changes.
- `go build ./...` + `go vet ./...` pass (proves the generated registrars exist
  and the cmd wiring compiles).
- **Route assertion:** grep each generated `*.pb.gw.go` for the expected path
  patterns (e.g. `/api/v1/expenses`, `/api/v1/expenses/{expense_id}/approve`,
  `/api/v1/assets/{asset_id}/check-out`) — confirms the annotations produced the
  intended routes.
- `kong config parse` accepts the updated `kong.yml`.
- **Manual (human):** with the stack up (`infra-up-full` incl. Kong +
  `dev-start`), `curl :8500/api/v1/config/expense-categories` and
  `curl :8500/api/v1/assets` return data through Kong; the expense/inventory UI
  pages load live data.

Existing gRPC integration/e2e tests are untouched (gRPC handlers unchanged).

## Non-goals — deferred, follow-up issues filed

- **reporting-analytics gateway** — needs ~5 new dashboard RPCs + handlers +
  response messages, and a `GetDashboardSummary` shape reconciliation; plus the
  `dashboard.ts` `/v1/reports`→`/api/v1/reports` prefix fix. (**#190**)
- **expense `RejectExpense` / `ApprovePurchaseOrder` / `SettlePettyCash`** — the
  UI calls these but no RPC exists. (**#191**)
- **inventory `CheckInAsset` contract** — the request requires `checkout_id`
  (the UI only has `asset_id`) and lacks the UI's damage fields; plus
  `ListAssets`/`ListCheckouts` missing filter/pagination fields. (**#192**)
- **No web/client.ts change**, no `pkg/server` change, no migration, no new CI
  job. ledger/notifications/document/billing gateways remain deferred (no UI
  consumer).

## Review weight

Touches proto + generated code + two cmd mains + Kong config, no
iam/general-ledger/security core → standard 2 approvals. Whole-branch review at
the end.
