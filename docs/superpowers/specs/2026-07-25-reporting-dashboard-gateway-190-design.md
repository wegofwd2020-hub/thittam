# Reporting Dashboard API + Gateway (#190 / #60) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-25
**Issue:** #190 (reporting dashboard RPCs + gateway), the last UI-facing piece of #60
**Branch:** `feat/reporting-dashboard-gateway-190` off `main` (`b3d70d7`)
**Migration:** none

## Goal

Expose the five reporting dashboard `Service` methods that already exist
(`GetPortfolioOverview`, `GetFinancialSummary`, `GetApprovalPipeline`,
`GetTeamUtilization`, `GetComplianceStatus`) as gRPC RPCs with proto response
messages and grpc-gateway REST routes, front them with Kong (`:9085`), and fix
the `dashboard.ts` `/api`-prefix bug so the five existing `use-dashboard` hooks
resolve. No new business logic — the service and repository methods are done;
this is the RPC/proto/gateway wiring plus one UI-path fix.

## Context

`services/reporting/dashboard_service.go` already implements all five methods
(`func (s *Service) GetX(ctx context.Context, tenantID uuid.UUID) (*X, error)`),
backed by `Repository` methods (the mock in `service_test.go` already has all
five fn-fields). But `reporting.proto` has **no** annotations and only one
dashboard RPC (`GetDashboardSummary`, a flat rollup) is wired; the five section
methods reach no gRPC surface. The UI (`web/src/lib/api/dashboard.ts` +
`web/src/lib/hooks/use-dashboard.ts`) is built against REST paths for the five
sections but uses `BASE = "/v1/reports/dashboard"` — missing the `/api` prefix —
so every call falls through `client.ts`'s `resolveBaseUrl` to the iam gateway and
404s. `reporting-analytics` has no `RunRESTGateway` call and no Kong route.

**Deferred (confirmed out of scope):**
- The composite `/summary` — the UI's `getDashboardSummary()` expects
  `{portfolio, financial, approvals, team, compliance}`, but no hook or page
  consumes it and it clashes with the existing flat `DashboardSummary` proto. Not
  built here; the flat RPC stays gRPC-only.
- The dashboard **page** — `app/(dashboard)/page.tsx` is 100% mock and consumes
  none of these hooks. Its mock→live rewrite is a separate frontend slice. This
  spec delivers a curl-verifiable API + resolving hooks.

### Grounding facts (`main` `b3d70d7`)

- The handler pattern (`handler.go` `GetDashboardSummary`, :107): `tenantID, ok
  := tenant.IDFromContext(ctx)` → `interceptor.RequirePermission(ctx, h.perm,
  "report:read")` → `h.svc.GetX(ctx, tenantID)` → explicit struct→proto mapping →
  `grpcErr(err)`. `decimal.Decimal`→`string` via `.StringFixed(2)`;
  `uuid.UUID`→`string` via `.String()`.
- The mock harness (`service_test.go` `mockRepo`) already has
  `getPortfolioOverviewFn` … `getComplianceStatusFn`; the handler test template
  (`handler_test.go`, `GetDashboardSummary` trio) is `_Success` / `_Denied`
  (`denyPerm` + a repo fn that `t.Fatal`s if reached) / `_NoTenant`
  (`codes.Unauthenticated`).
- Reporting gRPC port 8085; gateway HTTP port **9085** is free (`Port+1000`;
  9095 is metrics). gen alias `reportingv1`. cmd insertion point: after the last
  `RegisterHealthChecker`, before `srv.Run()`.
- **`time.Time`** fields (`PendingApproval.SubmittedAt`,
  `ComplianceItem.DetectedAt`) → the UI expects RFC3339 strings; `protojson`
  renders `google.protobuf.Timestamp` as RFC3339, so those become Timestamp
  fields.
- **The age-band naming conflict:** `ApprovalAgeBands`'s UI keys are `under_24h,
  "1_to_3_days", "3_to_7_days", over_7_days`. Proto field names can't start with a
  digit, and the gateway marshaler emits **proto field names** (`UseProtoNames:
  true`). So the proto uses `under_24h, days_1_to_3, days_3_to_7, over_7_days`,
  and the UI's (unused) `ApprovalAgeBands` interface is reconciled to those
  names. The Go struct's `json` tags are irrelevant on the gateway path (the
  handler maps field-by-field), so no Go-struct change.

## Design

### 1. Proto (`proto/thittam/reporting/v1/reporting.proto`)

Add `import "google/api/annotations.proto";`, five empty request messages, five
response messages mirroring the Go structs, and the eight nested messages. RPCs:

```proto
  rpc GetPortfolioOverview(GetPortfolioOverviewRequest) returns (PortfolioOverview) {
    option (google.api.http) = { get: "/api/v1/reports/dashboard/portfolio" };
  }
  rpc GetFinancialSummary(GetFinancialSummaryRequest) returns (FinancialSummary) {
    option (google.api.http) = { get: "/api/v1/reports/dashboard/financial" };
  }
  rpc GetApprovalPipeline(GetApprovalPipelineRequest) returns (ApprovalPipeline) {
    option (google.api.http) = { get: "/api/v1/reports/dashboard/approvals" };
  }
  rpc GetTeamUtilization(GetTeamUtilizationRequest) returns (TeamUtilization) {
    option (google.api.http) = { get: "/api/v1/reports/dashboard/team" };
  }
  rpc GetComplianceStatus(GetComplianceStatusRequest) returns (ComplianceStatus) {
    option (google.api.http) = { get: "/api/v1/reports/dashboard/compliance" };
  }
```

Message field mapping (Go type → proto type): `decimal.Decimal`→`string`,
`uuid.UUID`→`string`, `int`→`int32`, `time.Time`→`google.protobuf.Timestamp`,
`*string`→`string` (empty when nil). The eight nested messages —
`BudgetHealthCount`, `ProjectSummary`, `CategorySpend`, `MonthlySpend`,
`ApprovalAgeBands` (valid-identifier fields per above), `PendingApproval`
(`submitted_at` Timestamp), `ProjectTeam`, `ComplianceItem` (`detected_at`
Timestamp) — mirror `dashboard_models.go` field-for-field. Add `import
"google/protobuf/timestamp.proto";` is already present.

Adding annotations + messages is **not** a buf breaking change (the `FILE`
category flags removals/renames), so `Protobuf Breaking Change Detection` stays
green.

### 2. `buf generate`

Regenerates `gen/reporting/v1/{reporting.pb.go, reporting.pb.gw.go}`. Scope the
commit to `gen/reporting/v1/` (buf regenerates the whole tree; revert unrelated
cross-service drift, as in Phase C2). Run buf with a `proto` target (no root
`buf.work.yaml`).

### 3. Handlers (`services/reporting/handler.go`)

Five methods copying `GetDashboardSummary` verbatim in structure: tenant guard →
`RequirePermission("report:read")` → `svc.GetX` → build the proto message. The
mapping is mechanical but nested — e.g. `GetPortfolioOverview` maps the top
scalars, `HealthCounts` into a `*BudgetHealthCount`, and loops `Projects` into
`[]*ProjectSummary` (each with `.StringFixed(2)` decimals and `start_date` `""`
when the pointer is nil). Time fields use `timestamppb.New(v)`. Errors flow
through the existing `grpcErr` (dashboard methods raise no special sentinels, so
they map to `Internal` — matching current behavior).

### 4. Handler tests (`services/reporting/handler_test.go`)

For each of the five RPCs, the existing trio:
- `_Success`: a `mockRepo` fn returning a populated struct; assert the mapped
  proto fields (incl. a nested value and a decimal rendered as its `StringFixed`
  string).
- `_Denied`: `NewHandler(NewService(&mockRepo{...}), denyPerm{})` with the repo
  fn set to `t.Fatal` if reached; assert `codes.PermissionDenied`.
- `_NoTenant`: no tenant in context; assert `codes.Unauthenticated`.

Keeps `services/reporting` above its 75% coverage floor.

### 5. cmd wiring (`cmd/reporting-analytics/main.go`)

After the last `RegisterHealthChecker`, before `srv.Run()`:
```go
go func() {
	if err := server.RunRESTGateway(ctx, server.GatewayConfig{
		ServiceName:  "reporting-analytics",
		GRPCEndpoint: "localhost:8085",
		HTTPPort:     9085,
		Register:     reportingv1.RegisterReportingServiceHandlerFromEndpoint,
		ProjectHeader: false,
	}); err != nil {
		log.Fatalf("reporting-analytics: gateway: %v", err)
	}
}()
```
`ProjectHeader: false` — the dashboard is tenant-level (methods take `tenantID`,
not `productionID`); no `X-Project-Id`. No `Wrap`.

### 6. Kong (`infra/local/kong.yml`)

Add a `reporting` service (`http://host.docker.internal:9085`) with route
`/api/v1/reports`, `strip_path: false`. Validate with the throwaway `docker run
--rm -e KONG_DATABASE=off … kong:3.6 kong config parse`. Also refresh the stale
header comment ("three services" → the actual count) while here.

### 7. Web fix (`web/src/lib/api/dashboard.ts`)

Two edits, both gated by `Web Lint & Build` (#179):
- `BASE = "/v1/reports/dashboard"` → `"/api/v1/reports/dashboard"` (the missing
  `/api` bug), so the five hooks route through Kong.
- Reconcile the `ApprovalAgeBands` interface keys to the proto field names:
  `"1_to_3_days"` → `days_1_to_3`, `"3_to_7_days"` → `days_3_to_7` (the gateway
  emits proto names; these keys are unused today).

## Testing

- Handler unit tests (5 × trio, above): `go test ./services/reporting/... -race`.
- `buf generate` clean; `go build ./...` + `go vet ./...` pass; grep
  `gen/reporting/v1/reporting.pb.gw.go` for the five
  `/api/v1/reports/dashboard/*` route patterns.
- `kong config parse` accepts the updated `kong.yml`.
- `cd web && npm run lint -- --max-warnings 0 && npm run build` pass.
- **Manual (human):** stack up, `curl :8500/api/v1/reports/dashboard/portfolio`
  (with a seed token) returns the populated JSON; the five hooks resolve.

Existing gRPC/integration tests are untouched; the flat `GetDashboardSummary` RPC
is unchanged.

## Non-goals

- **No composite `/summary` RPC**, no change to the flat `DashboardSummary`.
- **No `app/(dashboard)/page.tsx` rewrite** (mock→live) — separate frontend slice.
- No new business logic, no migration, no `pkg/server` change, no new CI job.
- The other #60 services (ledger/notifications/document/billing) remain deferred
  (no UI consumer).

## Review weight

Touches `reporting`, proto/generated code, and `web` — no
iam/general-ledger/security core → standard 2 approvals. Whole-branch review on
the most capable model.
