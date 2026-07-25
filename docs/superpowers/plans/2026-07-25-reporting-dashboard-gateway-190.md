# Reporting Dashboard API + Gateway (#190) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the five existing reporting dashboard `Service` methods as gRPC RPCs with proto messages + grpc-gateway REST routes, front them with Kong, and fix the `dashboard.ts` `/api`-prefix bug.

**Architecture:** Add 5 RPCs + response messages to `reporting.proto` (mirroring the Go structs), `buf generate`, add 5 handler methods (copying `GetDashboardSummary`), wire the gateway with the shared helper, add the Kong route, and fix two UI lines. No new business logic — service + repo methods already exist.

**Tech Stack:** protobuf + buf 1.32.0 (grpc-gateway plugin), Go 1.25, `pkg/server.RunRESTGateway`, Kong 3.6, Next.js.

## Global Constraints

- **Go→proto conversions:** `decimal.Decimal`→`string` via `.StringFixed(2)`; `uuid.UUID`→`string` via `.String()`; `int`→`int32`; `time.Time`→`google.protobuf.Timestamp` via `timestamppb.New(v)`; `*string`→`string` (`""` when nil).
- **Age-band proto field names must be valid identifiers** (no leading digit): `under_24h`, `days_1_to_3`, `days_3_to_7`, `over_7_days`. The gateway emits proto field names (`UseProtoNames:true`), so the UI's `ApprovalAgeBands` interface is reconciled to match.
- **All 5 handlers gate identically:** `tenant.IDFromContext` (Unauthenticated if absent) → `interceptor.RequirePermission(ctx, h.perm, "report:read")` → `h.svc.GetX(ctx, tenantID)` → map → `grpcErr(err)`.
- **Gateway:** reporting HTTP port **9085** (gRPC 8085), `ProjectHeader: false`, no `Wrap`.
- **buf:** run with a `proto` target (no root `buf.work.yaml`); if annotations don't resolve, `buf dep update` once then revert any `buf.lock` side-effect. Scope each commit to `gen/reporting/v1/` — revert unrelated cross-service regen drift (`git checkout -- $(git status --porcelain gen/ | grep -v 'gen/reporting/' | awk '{print $2}')`). Adding annotations is NOT a buf breaking change.
- **Kong:** `strip_path: false`; validate with throwaway `docker run --rm -e KONG_DATABASE=off … kong:3.6 kong config parse`. NEVER `docker compose -v`/`down`/`up` on `infra/local/`.
- **Web changes pass `Web Lint & Build` (#179):** `npm run lint -- --max-warnings 0` + `npm run build`.
- No composite `/summary`, no `page.tsx` rewrite, no migration, no `pkg/server` change.
- **Commits:** Conventional Commits, scope `reporting`/`infra`/`web`; end every message with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `proto/thittam/reporting/v1/reporting.proto` | 5 RPCs + messages + annotations | 1 |
| `gen/reporting/v1/*` | regenerated | 1 |
| `services/reporting/handler.go` | 5 handler methods + conversion helpers | 2 |
| `services/reporting/handler_test.go` | 5 × (_Success/_Denied/_NoTenant) | 2 |
| `cmd/reporting-analytics/main.go` | gateway launch | 3 |
| `infra/local/kong.yml` | reporting route | 3 |
| `web/src/lib/api/dashboard.ts` | `/api` prefix + age-band keys | 3 |

---

### Task 1: Proto — RPCs, messages, annotations

**Files:**
- Modify: `proto/thittam/reporting/v1/reporting.proto`
- Modify (regenerated): `gen/reporting/v1/*`

**Interfaces:**
- Produces: RPCs `GetPortfolioOverview`/`GetFinancialSummary`/`GetApprovalPipeline`/`GetTeamUtilization`/`GetComplianceStatus`, the generated `RegisterReportingServiceHandlerFromEndpoint`, and the proto message types the handlers (Task 2) populate.

- [ ] **Step 1: Add the annotations import + 5 RPCs**

Add `import "google/api/annotations.proto";` after the existing timestamp import. Add these RPCs to the `ReportingService` block (after `GetDashboardSummary`):
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

- [ ] **Step 2: Add the request + response + nested messages**

Append to `reporting.proto` (mirroring `services/reporting/dashboard_models.go`):
```proto
message GetPortfolioOverviewRequest {}
message GetFinancialSummaryRequest {}
message GetApprovalPipelineRequest {}
message GetTeamUtilizationRequest {}
message GetComplianceStatusRequest {}

message PortfolioOverview {
  string tenant_id = 1;
  string project_label = 2;
  int32 total_active = 3;
  string total_budget = 4;
  string total_actual = 5;
  BudgetHealthCount health_counts = 6;
  repeated ProjectSummary projects = 7;
}
message BudgetHealthCount {
  int32 on_track = 1;
  int32 at_risk = 2;
  int32 over_budget = 3;
}
message ProjectSummary {
  string id = 1;
  string title = 2;
  string status = 3;
  string current_phase = 4;
  string budget_total = 5;
  string actual_spend = 6;
  string budget_used_pct = 7;
  string health = 8;
  string start_date = 9;
}

message FinancialSummary {
  string tenant_id = 1;
  string total_budgeted = 2;
  string total_actual = 3;
  string total_committed = 4;
  string total_available = 5;
  string burn_rate_per_month = 6;
  repeated CategorySpend by_category = 7;
  repeated MonthlySpend monthly_trend = 8;
}
message CategorySpend {
  string category_id = 1;
  string category_name = 2;
  string budgeted = 3;
  string actual = 4;
  string percentage = 5;
}
message MonthlySpend {
  string month = 1;
  string actual = 2;
  string budget = 3;
}

message ApprovalPipeline {
  string tenant_id = 1;
  int32 total_pending = 2;
  string total_amount = 3;
  ApprovalAgeBands by_age = 4;
  repeated PendingApproval pending_items = 5;
}
message ApprovalAgeBands {
  int32 under_24h = 1;
  int32 days_1_to_3 = 2;
  int32 days_3_to_7 = 3;
  int32 over_7_days = 4;
}
message PendingApproval {
  string id = 1;
  string type = 2;
  string description = 3;
  string amount = 4;
  string submitted_by = 5;
  google.protobuf.Timestamp submitted_at = 6;
  string project_title = 7;
  int32 age_hours = 8;
}

message TeamUtilization {
  string tenant_id = 1;
  string team_member_label = 2;
  int32 total_members = 3;
  int32 assigned = 4;
  int32 available = 5;
  string utilization_pct = 6;
  repeated ProjectTeam by_project = 7;
}
message ProjectTeam {
  string project_id = 1;
  string project_title = 2;
  int32 member_count = 3;
}

message ComplianceStatus {
  string tenant_id = 1;
  int32 overdue_approvals = 2;
  int32 policy_violations = 3;
  int32 audit_flags = 4;
  repeated ComplianceItem recent_violations = 5;
}
message ComplianceItem {
  string id = 1;
  string type = 2;
  string description = 3;
  string severity = 4;
  google.protobuf.Timestamp detected_at = 5;
  string project_title = 6;
}
```

- [ ] **Step 3: Lint + regenerate + scope**

Run (buf target `proto`; `buf dep update` fallback per Global Constraints):
```bash
buf lint proto
buf generate
git checkout -- $(git status --porcelain gen/ | grep -v 'gen/reporting/' | awk '{print $2}')  # revert unrelated drift
```

- [ ] **Step 4: Confirm the routes generated + it builds**

```bash
grep -oE '/api/v1/reports/dashboard/(portfolio|financial|approvals|team|compliance)' gen/reporting/v1/reporting.pb.gw.go | sort -u
go build ./...
```
Expected: all five paths listed; `RegisterReportingServiceHandlerFromEndpoint` present in the generated file; `go build` exits 0.

- [ ] **Step 5: Commit**

```bash
git add proto/thittam/reporting/v1/reporting.proto gen/reporting/v1
git commit -m "feat(reporting): add 5 dashboard RPCs + REST annotations (#190)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Handlers + tests

**Files:**
- Modify: `services/reporting/handler.go` (5 methods + conversion helpers)
- Modify: `services/reporting/handler_test.go` (5 × trio)

**Interfaces:**
- Consumes: the proto types from Task 1; `Service.GetPortfolioOverview(ctx, uuid.UUID) (*PortfolioOverview, error)` … (all five, in `dashboard_service.go`); the existing `Handler`, `grpcErr`, `tenant.IDFromContext`, `interceptor.RequirePermission`.
- Produces: `Handler.GetPortfolioOverview` … `GetComplianceStatus` gRPC methods.

`handler.go` already imports `context`, `uuid`, `reportingv1`, `interceptor`, `tenant`, `codes`, `status`, `timestamppb` — no new imports needed.

- [ ] **Step 1: Add the conversion helpers**

Append to `services/reporting/handler.go`'s conversion-helper section (after `budgetFactToProto`):
```go
func portfolioToProto(p *PortfolioOverview) *reportingv1.PortfolioOverview {
	out := &reportingv1.PortfolioOverview{
		TenantId:     p.TenantID.String(),
		ProjectLabel: p.ProjectLabel,
		TotalActive:  int32(p.TotalActive),
		TotalBudget:  p.TotalBudget.StringFixed(2),
		TotalActual:  p.TotalActual.StringFixed(2),
		HealthCounts: &reportingv1.BudgetHealthCount{
			OnTrack:    int32(p.HealthCounts.OnTrack),
			AtRisk:     int32(p.HealthCounts.AtRisk),
			OverBudget: int32(p.HealthCounts.OverBudget),
		},
	}
	for _, ps := range p.Projects {
		item := &reportingv1.ProjectSummary{
			Id:            ps.ID.String(),
			Title:         ps.Title,
			Status:        ps.Status,
			CurrentPhase:  ps.CurrentPhase,
			BudgetTotal:   ps.BudgetTotal.StringFixed(2),
			ActualSpend:   ps.ActualSpend.StringFixed(2),
			BudgetUsedPct: ps.BudgetUsedPct.StringFixed(2),
			Health:        ps.Health,
		}
		if ps.StartDate != nil {
			item.StartDate = *ps.StartDate
		}
		out.Projects = append(out.Projects, item)
	}
	return out
}

func financialToProto(f *FinancialSummary) *reportingv1.FinancialSummary {
	out := &reportingv1.FinancialSummary{
		TenantId:         f.TenantID.String(),
		TotalBudgeted:    f.TotalBudgeted.StringFixed(2),
		TotalActual:      f.TotalActual.StringFixed(2),
		TotalCommitted:   f.TotalCommitted.StringFixed(2),
		TotalAvailable:   f.TotalAvailable.StringFixed(2),
		BurnRatePerMonth: f.BurnRate.StringFixed(2),
	}
	for _, c := range f.ByCategory {
		out.ByCategory = append(out.ByCategory, &reportingv1.CategorySpend{
			CategoryId:   c.CategoryID,
			CategoryName: c.CategoryName,
			Budgeted:     c.Budgeted.StringFixed(2),
			Actual:       c.Actual.StringFixed(2),
			Percentage:   c.Percentage.StringFixed(2),
		})
	}
	for _, m := range f.MonthlyTrend {
		out.MonthlyTrend = append(out.MonthlyTrend, &reportingv1.MonthlySpend{
			Month:  m.Month,
			Actual: m.Actual.StringFixed(2),
			Budget: m.Budget.StringFixed(2),
		})
	}
	return out
}

func approvalToProto(a *ApprovalPipeline) *reportingv1.ApprovalPipeline {
	out := &reportingv1.ApprovalPipeline{
		TenantId:     a.TenantID.String(),
		TotalPending: int32(a.TotalPending),
		TotalAmount:  a.TotalAmount.StringFixed(2),
		ByAge: &reportingv1.ApprovalAgeBands{
			Under_24H:  int32(a.ByAge.Under24h),
			Days_1To_3: int32(a.ByAge.OneToThreeDays),
			Days_3To_7: int32(a.ByAge.ThreeToSevenDays),
			Over_7Days: int32(a.ByAge.OverSevenDays),
		},
	}
	for _, p := range a.PendingItems {
		out.PendingItems = append(out.PendingItems, &reportingv1.PendingApproval{
			Id:           p.ID.String(),
			Type:         p.Type,
			Description:  p.Description,
			Amount:       p.Amount.StringFixed(2),
			SubmittedBy:  p.SubmittedBy,
			SubmittedAt:  timestamppb.New(p.SubmittedAt),
			ProjectTitle: p.ProjectTitle,
			AgeHours:     int32(p.AgeHours),
		})
	}
	return out
}

func teamToProto(t *TeamUtilization) *reportingv1.TeamUtilization {
	out := &reportingv1.TeamUtilization{
		TenantId:        t.TenantID.String(),
		TeamMemberLabel: t.TeamMemberLabel,
		TotalMembers:    int32(t.TotalMembers),
		Assigned:        int32(t.Assigned),
		Available:       int32(t.Available),
		UtilizationPct:  t.UtilizationPct.StringFixed(2),
	}
	for _, pt := range t.ByProject {
		out.ByProject = append(out.ByProject, &reportingv1.ProjectTeam{
			ProjectId:    pt.ProjectID.String(),
			ProjectTitle: pt.ProjectTitle,
			MemberCount:  int32(pt.MemberCount),
		})
	}
	return out
}

func complianceToProto(c *ComplianceStatus) *reportingv1.ComplianceStatus {
	out := &reportingv1.ComplianceStatus{
		TenantId:         c.TenantID.String(),
		OverdueApprovals: int32(c.OverdueApprovals),
		PolicyViolations: int32(c.PolicyViolations),
		AuditFlags:       int32(c.AuditFlags),
	}
	for _, v := range c.RecentViolations {
		out.RecentViolations = append(out.RecentViolations, &reportingv1.ComplianceItem{
			Id:           v.ID.String(),
			Type:         v.Type,
			Description:  v.Description,
			Severity:     v.Severity,
			DetectedAt:   timestamppb.New(v.DetectedAt),
			ProjectTitle: v.ProjectTitle,
		})
	}
	return out
}
```
NOTE: the generated Go field names for the age bands are protoc's camelCase of `under_24h`/`days_1_to_3`/`days_3_to_7`/`over_7_days` — verify the exact generated identifiers in `gen/reporting/v1/reporting.pb.go` (protoc renders `under_24h`→`Under_24H`, `days_1_to_3`→`Days_1To_3`, etc.) and match them; `go build` will fail on a wrong name. Adjust the helper to the generated names.

- [ ] **Step 2: Add the 5 handler methods**

Append to `handler.go` (after `GetDashboardSummary`):
```go
func (h *Handler) GetPortfolioOverview(ctx context.Context, _ *reportingv1.GetPortfolioOverviewRequest) (*reportingv1.PortfolioOverview, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetPortfolioOverview(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return portfolioToProto(res), nil
}

func (h *Handler) GetFinancialSummary(ctx context.Context, _ *reportingv1.GetFinancialSummaryRequest) (*reportingv1.FinancialSummary, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetFinancialSummary(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return financialToProto(res), nil
}

func (h *Handler) GetApprovalPipeline(ctx context.Context, _ *reportingv1.GetApprovalPipelineRequest) (*reportingv1.ApprovalPipeline, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetApprovalPipeline(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return approvalToProto(res), nil
}

func (h *Handler) GetTeamUtilization(ctx context.Context, _ *reportingv1.GetTeamUtilizationRequest) (*reportingv1.TeamUtilization, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetTeamUtilization(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return teamToProto(res), nil
}

func (h *Handler) GetComplianceStatus(ctx context.Context, _ *reportingv1.GetComplianceStatusRequest) (*reportingv1.ComplianceStatus, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
	res, err := h.svc.GetComplianceStatus(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return complianceToProto(res), nil
}
```

- [ ] **Step 3: Build**

```bash
go build ./services/reporting/
```
Expected: exit 0 (this is where a wrong generated age-band field name surfaces — fix the helper to match `reporting.pb.go` and rebuild).

- [ ] **Step 4: Add the test trio — template (portfolio), then the other four**

Append to `handler_test.go`. The `mockRepo` (in `service_test.go`) already has `getPortfolioOverviewFn` … `getComplianceStatusFn`; `ctxWithCaller`, `denyPerm`, `allowAllPerm`, `ctxWithVertical` already exist. Full template for portfolio:
```go
func TestHandler_GetPortfolioOverview_Success(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPortfolioOverviewFn: func(_ context.Context, tid uuid.UUID) (*PortfolioOverview, error) {
			return &PortfolioOverview{
				TenantID:     tid,
				ProjectLabel: "Productions",
				TotalActive:  3,
				TotalBudget:  decimal.RequireFromString("100.00"),
				TotalActual:  decimal.RequireFromString("40.00"),
				HealthCounts: BudgetHealthCount{OnTrack: 2, AtRisk: 1},
				Projects:     []ProjectSummary{{ID: uuid.New(), Title: "P1", BudgetTotal: decimal.RequireFromString("50.00")}},
			}, nil
		},
	}), allowAllPerm{})

	resp, err := h.GetPortfolioOverview(ctxWithCaller(tenantID), &reportingv1.GetPortfolioOverviewRequest{})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.TenantId)
	assert.Equal(t, int32(3), resp.TotalActive)
	assert.Equal(t, "100.00", resp.TotalBudget)          // decimal → StringFixed(2)
	assert.Equal(t, int32(2), resp.HealthCounts.OnTrack) // nested message
	require.Len(t, resp.Projects, 1)
	assert.Equal(t, "50.00", resp.Projects[0].BudgetTotal)
}

func TestHandler_GetPortfolioOverview_Denied(t *testing.T) {
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPortfolioOverviewFn: func(context.Context, uuid.UUID) (*PortfolioOverview, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetPortfolioOverview(ctxWithCaller(tenantID), &reportingv1.GetPortfolioOverviewRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetPortfolioOverview_NoTenant(t *testing.T) {
	_, err := newHandler().GetPortfolioOverview(ctxWithVertical(), &reportingv1.GetPortfolioOverviewRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
```
`decimal` needs importing in the test file: add `"github.com/shopspring/decimal"` to `handler_test.go`'s import block.

For the other four RPCs, write the identical trio, swapping only these (structure — guard order, `ctxWithCaller`/`denyPerm`/`ctxWithVertical`, the `codes` asserted — is unchanged):

| RPC | mock fn field | return type | one nested + one decimal assert |
|---|---|---|---|
| `GetFinancialSummary` | `getFinancialSummaryFn` | `*FinancialSummary` | set `TotalBudgeted: decimal.RequireFromString("200.00")`, `ByCategory: []CategorySpend{{CategoryName:"cam", Actual: decimal.RequireFromString("10.00")}}`; assert `resp.TotalBudgeted=="200.00"`, `resp.ByCategory[0].Actual=="10.00"` |
| `GetApprovalPipeline` | `getApprovalPipelineFn` | `*ApprovalPipeline` | set `TotalPending: 4`, `ByAge: ApprovalAgeBands{Under24h:1, OneToThreeDays:2}`, `PendingItems: []PendingApproval{{Amount: decimal.RequireFromString("5.00"), SubmittedAt: time.Now()}}`; assert `resp.TotalPending==int32(4)`, `resp.ByAge.Days_1To_3==int32(2)`, `resp.PendingItems[0].Amount=="5.00"`, `resp.PendingItems[0].SubmittedAt != nil` (add `"time"` import) |
| `GetTeamUtilization` | `getTeamUtilizationFn` | `*TeamUtilization` | set `TotalMembers: 5`, `UtilizationPct: decimal.RequireFromString("60.00")`, `ByProject: []ProjectTeam{{ProjectTitle:"P1", MemberCount:2}}`; assert `resp.TotalMembers==int32(5)`, `resp.UtilizationPct=="60.00"`, `resp.ByProject[0].MemberCount==int32(2)` |
| `GetComplianceStatus` | `getComplianceStatusFn` | `*ComplianceStatus` | set `OverdueApprovals: 1`, `RecentViolations: []ComplianceItem{{Severity:"high", DetectedAt: time.Now()}}`; assert `resp.OverdueApprovals==int32(1)`, `resp.RecentViolations[0].Severity=="high"`, `resp.RecentViolations[0].DetectedAt != nil` |

(The `_Denied` and `_NoTenant` bodies are byte-identical to portfolio's except the handler method name and the mock fn field set to `t.Fatal`.)

- [ ] **Step 5: Run the reporting suite**

```bash
go test ./services/reporting/... -race
```
Expected: all pass, including the 15 new tests. If an age-band assertion fails on the generated field name, align it with `reporting.pb.go`.

- [ ] **Step 6: Commit**

```bash
git add services/reporting/handler.go services/reporting/handler_test.go
git commit -m "feat(reporting): wire 5 dashboard RPC handlers + tests (#190)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Gateway wiring + Kong + web fix

**Files:**
- Modify: `cmd/reporting-analytics/main.go`
- Modify: `infra/local/kong.yml`
- Modify: `web/src/lib/api/dashboard.ts`

**Interfaces:**
- Consumes: Task 1's `reportingv1.RegisterReportingServiceHandlerFromEndpoint`; `server.RunRESTGateway` (on `main`).

- [ ] **Step 1: Launch the gateway**

In `cmd/reporting-analytics/main.go`, insert between the last `srv.RegisterHealthChecker("projection-lag", …)` and `log.Printf("reporting-analytics service ready …")`:
```go
	// --- REST gateway (grpc-gateway, #60 / #190) — via the shared helper. ---
	go func() {
		if err := server.RunRESTGateway(ctx, server.GatewayConfig{
			ServiceName:   "reporting-analytics",
			GRPCEndpoint:  "localhost:8085",
			HTTPPort:      9085,
			Register:      reportingv1.RegisterReportingServiceHandlerFromEndpoint,
			ProjectHeader: false,
		}); err != nil {
			log.Fatalf("reporting-analytics: gateway: %v", err)
		}
	}()
```
If `pkg/server` or `context` aren't already imported in this file, add them (the file already imports `reportingv1` and calls `server.New`, so `pkg/server` is present; confirm and add only what `go build` flags as missing).

- [ ] **Step 2: Build + vet**

```bash
go build ./... && go vet ./cmd/reporting-analytics/
```
Expected: exit 0.

- [ ] **Step 3: Add the Kong route**

In `infra/local/kong.yml`, append under `services:` (after `inventory`), 2-space indent:
```yaml
  - name: reporting
    url: http://host.docker.internal:9085
    routes:
      - name: reporting
        paths:
          - /api/v1/reports
        strip_path: false
```
Also update the stale header comment (line 1-2, "three services") to reflect the actual count (now six).

- [ ] **Step 4: Validate Kong**

```bash
docker run --rm -e KONG_DATABASE=off -v "$PWD/infra/local/kong.yml:/kong.yml:ro" kong:3.6 kong config parse /kong.yml
grep -E '^\s+- name:' infra/local/kong.yml
```
Expected: `parse successful`; service names include `reporting` (six total: iam, budget, project, expense, inventory, reporting). No `docker compose` up/down/-v.

- [ ] **Step 5: Fix the UI path + age-band keys**

In `web/src/lib/api/dashboard.ts`:
- Change `const BASE = "/v1/reports/dashboard";` → `const BASE = "/api/v1/reports/dashboard";`.
- In the `ApprovalAgeBands` interface (lines 63-68), rename the two quoted numeric keys to the proto field names:
```ts
export interface ApprovalAgeBands {
  under_24h: number;
  days_1_to_3: number;
  days_3_to_7: number;
  over_7_days: number;
}
```

- [ ] **Step 6: Web lint + build gate**

```bash
cd web && npm run lint -- --max-warnings 0; echo "LINT: $?"
npm run build; echo "BUILD: $?"
```
Expected: `LINT: 0`, `BUILD: 0`. (If a consumer references the old `"1_to_3_days"` key, `tsc`/build will flag it — but per grounding no page consumes these types yet, so a clean build confirms the rename is self-contained.)

- [ ] **Step 7: Commit**

```bash
git add cmd/reporting-analytics/main.go infra/local/kong.yml web/src/lib/api/dashboard.ts
git commit -m "feat(reporting): gateway :9085 + Kong route + fix dashboard.ts /api prefix (#190)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- 5 RPCs + messages + annotations + gen → Task 1 ✅
- 5 handlers (tenant→report:read→svc→map) + conversion helpers + 15 tests → Task 2 ✅
- gateway :9085 ProjectHeader:false → Task 3 ✅; Kong reporting route → Task 3 ✅; dashboard.ts /api prefix + age-band reconcile → Task 3 ✅
- Conversions (decimal StringFixed(2), uuid String, int32, time Timestamp, *string) → Global Constraints + Task 2 helpers ✅
- Deferred (composite /summary, page rewrite) → not in any task ✅

**Placeholder scan:** none — full proto, full handlers/helpers, one full test trio + a concrete per-RPC table (mock fn + fixture + exact asserts). The only "verify the generated name" note (age-band Go identifiers) is a real, compiler-checked step, not a placeholder.

**Type consistency:** proto message/field names in Task 1 match the conversion helpers and asserts in Task 2 (`HealthCounts`, `ByAge.Days_1To_3`, `SubmittedAt`, etc.). `RegisterReportingServiceHandlerFromEndpoint` (Task 1) = the `Register` in Task 3's wiring. Handler method names match `dashboard_service.go`'s `Service.GetX`.

**Ordering:** Task 1 (proto+gen, the contract) → Task 2 (handlers against the generated types) → Task 3 (wiring+kong+web). Tasks 2–3 depend on Task 1's generated code.
