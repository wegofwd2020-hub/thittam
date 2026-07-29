# Banked #138 follow-ups (#141) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Discharge the six minor findings banked during #138 in one branch — registry injection, a JWT nil-guard, a dead config key, a drifted-test fix, a gofmt, and bufconn deadlines.

**Architecture:** Five independent tasks, one per file-cluster. Task 1 (Prometheus registry injection) is the only design-bearing one and is behavior-preserving when the new field is nil. The rest are mechanical.

**Tech Stack:** Go 1.25, `github.com/prometheus/client_golang`, `github.com/golang-jwt/jwt/v5`, gRPC bufconn, testify.

## Global Constraints

- Module path: `github.com/wegofwd2020/thittam`.
- No proto / sqlc / migration in this branch.
- Every touched Go file must be `gofmt`-clean after the task (`gofmt -l` prints nothing).
- Structured `slog`, no PII/secrets. Monetary rule N/A (no money touched).
- CI Lint = golangci-lint v2 `latest`, NO config file → default linters only (errcheck, govet, ineffassign, staticcheck, unused). gofmt does NOT gate CI (it is a separate v2 `formatters` section, unenabled) — §5 is hygiene, not a CI gate.
- Commits end with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

### Task 1: Inject the Prometheus registry into `pkg/server`

**Files:**
- Modify: `pkg/observability/metrics.go` (`NewMetrics` signature + 8 `promauto.New*` calls)
- Modify: `pkg/server/server.go` (`Config` struct + the `NewMetrics` call at :79 + a prometheus import)
- Test: `pkg/server/server_test.go` (add a duplicate-name / distinct-registry test)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `observability.NewMetrics(serviceName string, reg prometheus.Registerer) *Metrics`; `server.Config.Registry prometheus.Registerer` (nil = global default). No other task depends on these.

**Context:** `NewMetrics` today takes only `serviceName` and calls package-level `promauto.New*`, all of which register on the global `prometheus.DefaultRegisterer`. Two `server.New` calls with the same `Config.Name` panic on duplicate registration — which is why `pkg/server` is barely unit-tested. The sole `NewMetrics` caller is `server.go:79`; the 10 `cmd/*` callers of `server.New` must stay behavior-identical (they leave `Registry` nil).

- [ ] **Step 1: Write the failing test** in `pkg/server/server_test.go`

```go
func TestNew_InjectedRegistry_NoDuplicatePanic(t *testing.T) {
	// Two servers with the SAME Name but distinct registries must both
	// construct without the global-registry duplicate-registration panic.
	reg1 := prometheus.NewRegistry()
	reg2 := prometheus.NewRegistry()
	_ = New(Config{Name: "dup-svc", Registry: reg1}, nil)
	_ = New(Config{Name: "dup-svc", Registry: reg2}, nil)

	// The injected registry actually received this server's collectors.
	mfs, err := reg1.Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, mfs, "expected collectors registered on the injected registry")
}
```
Add imports to the test file as needed: `"github.com/prometheus/client_golang/prometheus"`, `"github.com/stretchr/testify/assert"`, `"github.com/stretchr/testify/require"` (check which are already present).

- [ ] **Step 2: Run it — expect FAIL (panic today)**

Run: `go test ./pkg/server/ -run TestNew_InjectedRegistry_NoDuplicatePanic -v`
Expected: FAIL — `New` does not yet accept `Registry`; compile error (unknown field) OR, once the field exists but is unused, the second `New` panics "duplicate metrics collector registration".

- [ ] **Step 3: Change `NewMetrics` in `pkg/observability/metrics.go`**

Signature and body head:
```go
// NewMetrics creates all metric collectors for a service, registered with reg.
// A nil reg means the global prometheus.DefaultRegisterer (production default);
// tests pass a fresh prometheus.NewRegistry() so repeated construction with the
// same service name does not panic on duplicate registration.
func NewMetrics(serviceName string, reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	f := promauto.With(reg)
	return &Metrics{
		RequestDuration: f.NewHistogramVec(/* unchanged opts + labels */),
		RequestCounter:  f.NewCounterVec(/* unchanged */),
		ActiveRequests:  f.NewGauge(/* unchanged */),
		TenantRequests:  f.NewCounterVec(/* unchanged */),
		CacheOperations: f.NewCounterVec(/* unchanged */),
		DBActiveConns:   f.NewGauge(/* unchanged */),
		DBIdleConns:     f.NewGauge(/* unchanged */),
		RedisConnected:  f.NewGauge(/* unchanged */),
	}
}
```
Change ONLY `promauto.NewX(` → `f.NewX(` on all 8 collectors; leave every `prometheus.*Opts{...}` and label slice byte-identical. `promauto` and `prometheus` are already imported in this file.

- [ ] **Step 4: Add the `Registry` field to `Config` and thread it in `pkg/server/server.go`**

In the `Config` struct (after `EnableReflection`):
```go
	// Registry receives this server's Prometheus collectors. Nil means the
	// global default registry (production default). Inject a fresh
	// prometheus.NewRegistry() in tests to build the server more than once in
	// one process without a duplicate-registration panic.
	Registry prometheus.Registerer
```
At the `NewMetrics` call (server.go:79):
```go
	metrics := observability.NewMetrics(sanitizeName(cfg.Name), cfg.Registry)
```
Add `"github.com/prometheus/client_golang/prometheus"` to `server.go` imports.

- [ ] **Step 5: Run the new test + the existing server tests — expect PASS**

Run: `go test ./pkg/server/ ./pkg/observability/ -race -v`
Expected: PASS — new test green; `TestNew_ReflectionOffByDefault` / `TestNew_ReflectionOnViaEnv` still green (they leave `Registry` nil → global default, unchanged).

- [ ] **Step 6: Confirm the 10 cmd callers still build unchanged**

Run: `go build ./...`
Expected: clean — `server.New` signature is unchanged, so no `cmd/*/main.go` needs editing.

- [ ] **Step 7: gofmt + commit**

```bash
gofmt -l pkg/observability/metrics.go pkg/server/server.go pkg/server/server_test.go   # must print nothing
git add pkg/observability/metrics.go pkg/server/server.go pkg/server/server_test.go
git commit -m "refactor(server): inject Prometheus registry so pkg/server is unit-testable (#141)

NewMetrics gains a prometheus.Registerer; Config.Registry (nil = global
default) threads it through New. Production behavior is unchanged (all 10
cmd callers leave Registry nil); tests inject a fresh registry to build a
server twice with the same name without the duplicate-registration panic.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Guard `JWTIssuer.Validate` against nil dates (`pkg/auth`)

**Files:**
- Modify: `pkg/auth/jwt.go` (`Validate`, lines ~347-356)
- Test: `pkg/auth/jwt_test.go` (add a missing-`iat` test)

**Interfaces:**
- Consumes: nothing. Produces: nothing new (internal fix).

**Context:** `Validate` parses `sub`/`tid` then builds `*Claims` with unguarded `c.IssuedAt.Time` / `c.ExpiresAt.Time` on `*jwt.NumericDate`. `iat` is optional (only `exp` is parser-required), so a signed token omitting `iat` panics. `pkg/auth/verifier.go:104-127` already guards both — mirror it. The test is `package auth`, so it can sign with `issuer.privateKey` directly. Claims type is `jwtClaims`; `Subject` and `TenantID` must be valid UUIDs or `Validate` errors before reaching the date code.

- [ ] **Step 1: Write the failing test** in `pkg/auth/jwt_test.go`

```go
func TestJWTIssuer_Validate_MissingIat_NoPanic(t *testing.T) {
	issuer, _ := testIssuer(t)

	// A validly-signed token that omits iat (only exp is parser-required).
	claims := &jwtClaims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   fixtureUserID.String(),
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
			// IssuedAt deliberately omitted.
		},
		TenantID: fixtureTenantID.String(),
	}
	tokenStr, err := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims).SignedString(issuer.privateKey)
	require.NoError(t, err)

	got, err := issuer.Validate(context.Background(), tokenStr)
	require.NoError(t, err)
	assert.True(t, got.IssuedAt.IsZero(), "missing iat must map to zero time, not panic")
	assert.False(t, got.ExpiresAt.IsZero())
}
```

- [ ] **Step 2: Run it — expect FAIL (panic)**

Run: `go test ./pkg/auth/ -run TestJWTIssuer_Validate_MissingIat_NoPanic -v`
Expected: FAIL — panic `runtime error: invalid memory address or nil pointer dereference` at the `c.IssuedAt.Time` line.

- [ ] **Step 3: Guard the dates in `jwt.go` `Validate`**

Replace the direct return with nil-checked locals (mirror verifier.go):
```go
	var issuedAt, expiresAt time.Time
	if c.IssuedAt != nil {
		issuedAt = c.IssuedAt.Time
	}
	if c.ExpiresAt != nil {
		expiresAt = c.ExpiresAt.Time
	}

	return &Claims{
		Subject:     userID,
		TenantID:    tenantID,
		Email:       c.Email,
		Roles:       c.Roles,
		Permissions: c.Permissions,
		AuthMethod:  c.AuthMethod,
		IssuedAt:    issuedAt,
		ExpiresAt:   expiresAt,
	}, nil
```

- [ ] **Step 4: Run the auth suite — expect PASS**

Run: `go test ./pkg/auth/ -race`
Expected: PASS — new test green; `TestJWTIssuer_Validate_ValidToken` and the rest unaffected.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -l pkg/auth/jwt.go pkg/auth/jwt_test.go   # must print nothing
git add pkg/auth/jwt.go pkg/auth/jwt_test.go
git commit -m "fix(iam): guard JWTIssuer.Validate against a missing iat claim (#141)

Validate dereferenced *jwt.NumericDate for iat/exp unguarded; a
validly-signed token omitting the optional iat panicked in a security
boundary. Mirror the nil guard verifier.go already has.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Fix the two drifted `_NoTenant` expense tests (`services/expense`)

**Files:**
- Modify: `services/expense/handler_test.go` (new helper + two tests: `TestHandler_SubmitExpense_NoTenant` ~296, `TestHandler_CreatePettyCashAdvance_NoTenant` ~661)

**Interfaces:**
- Consumes: nothing. Produces: nothing (test-only).

**Context:** Both tests call the handler with `ctxWithVertical()` (no caller, no tenant). Post-#138 `RequirePermission` checks the caller first, so both satisfy `Unauthenticated` via the caller-missing path and never reach the tenant lookup their names claim. Give them a caller (no tenant) so they hit — and assert — the tenant branch. Existing helper patterns: `ctxWithCaller` (handler_test.go:43-44) uses `interceptor.WithCaller(ctx, interceptor.CallerInfo{...})`; `newHandler()` wires `allowAllPerm{}` so `RequirePermission` passes once a caller is present. The tenant-missing error message is `"tenant ID not found in context"` (handler.go).

- [ ] **Step 1: Add the caller-no-tenant helper** near the other ctx helpers in `handler_test.go`

```go
// ctxCallerNoTenant carries vertical config + a caller identity but NO tenant,
// so a handler passes RequirePermission (allowAllPerm) and reaches the tenant
// lookup — exercising the tenant-missing branch the _NoTenant tests claim.
func ctxCallerNoTenant() context.Context {
	return interceptor.WithCaller(ctxWithVertical(), interceptor.CallerInfo{UserID: uuid.New()})
}
```

- [ ] **Step 2: Point the two tests at the helper + assert the tenant branch**

`TestHandler_SubmitExpense_NoTenant`:
```go
func TestHandler_SubmitExpense_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SubmitExpense(ctxCallerNoTenant(), &expensev1.SubmitExpenseRequest{
		ProductionId: uuid.New().String(),
		Amount:       "100.00",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "tenant",
		"must fail on the tenant-missing branch, not the caller branch")
}
```
`TestHandler_CreatePettyCashAdvance_NoTenant`: same edit — swap `ctxWithVertical()` → `ctxCallerNoTenant()` and add the same `assert.Contains(..., "tenant")` line, keeping its existing request fields (`ProductionId`, `IssuedTo`, `Amount`).

- [ ] **Step 3: Run the two tests — expect PASS on the tenant branch**

Run: `go test ./services/expense/ -run 'TestHandler_(SubmitExpense|CreatePettyCashAdvance)_NoTenant' -v`
Expected: PASS — both now return `Unauthenticated` with a message containing "tenant". (Sanity: temporarily revert one to `ctxWithVertical()` → the `Contains("tenant")` assert fails because the message is the caller-branch text; then restore.)

- [ ] **Step 4: Run the whole expense suite**

Run: `go test ./services/expense/ -race`
Expected: PASS.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -l services/expense/handler_test.go   # must print nothing
git add services/expense/handler_test.go
git commit -m "test(expense): make the two _NoTenant tests exercise the tenant branch (#141)

Post-#138 RequirePermission checks the caller first, so these satisfied
Unauthenticated via the caller-missing path, not the tenant-missing path
their names claim. Give them a caller (no tenant) and assert the tenant
message so the branch they name is actually covered.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Add per-call deadlines to the bufconn harness (`pkg/server`)

**Files:**
- Modify: `pkg/server/integration_test.go` (`bearer` helper + 5 RPC call sites)

**Interfaces:**
- Consumes: nothing. Produces: nothing (test-only). Independent of Task 1 — the harness stays on `grpc.NewServer` (rewriting it onto `server.New` is a NON-GOAL).

**Context:** Every RPC in this file rides an undeadlined `context.Background()` (directly at :102/:110/:121/:144 and via the `bearer` helper at :95, used by the tests at :135/:154). A hung bufconn pipe would block to the test-binary timeout instead of failing fast.

- [ ] **Step 1: Add the deadline helper** in `integration_test.go`

```go
// callCtx returns a short-deadline context for a bufconn RPC so a hung pipe
// fails the test fast instead of blocking to the test-binary timeout.
func callCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
```
Ensure `"time"` is imported.

- [ ] **Step 2: Thread `t` through `bearer` and switch all RPC sites to `callCtx(t)`**

- Change `bearer` from `bearer(t, key)`... confirm its current signature; make its base context `callCtx(t)` instead of `context.Background()` (it already takes `t` per the grounding — if not, add `t *testing.T` as the first param and update its two callers at ~:135/:154).
- Replace `context.Background()` with `callCtx(t)` at the four direct RPC sites (`client.Login` ~:102, `client.ListRoles` ~:110, the `metadata.NewOutgoingContext(context.Background(), ...)` base ~:121, `client.CheckPermission` ~:144).

- [ ] **Step 3: Run the harness — expect PASS**

Run: `go test ./pkg/server/ -run TestChain -race -v`
Expected: PASS — all six `TestChain_*` tests green under the deadlined contexts.

- [ ] **Step 4: gofmt + commit**

```bash
gofmt -l pkg/server/integration_test.go   # must print nothing
git add pkg/server/integration_test.go
git commit -m "test(server): give the bufconn harness per-call deadlines (#141)

RPCs rode an undeadlined context.Background(); a hung pipe would block to
the test-binary timeout. Add a 5s callCtx helper and use it everywhere.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: Delete the dead `IAM_ADDR` key + gofmt reporting main (config/hygiene)

**Files:**
- Modify: `infra/k8s/config/configmap.yaml` (remove `IAM_ADDR` + fix its comment)
- Modify: `cmd/reporting-analytics/main.go` (gofmt: import reorder + trailing blank line)

**Interfaces:**
- Consumes: nothing. Produces: nothing. Pure cleanup, no test.

**Context:** `IAM_ADDR` (configmap.yaml:79) is read by zero Go code; the live key is `IAM_SERVICE_ADDR` (:85, read by `pkg/iamclient/dial.go`). `scripts/verify-project-rbac.sh` uses `IAM_ADDR` only as a self-contained local shell variable — leave that script alone. `gofmt -l cmd/reporting-analytics/main.go` is dirty (reorder `pgxpool`/`nats`, strip trailing blank line); does not gate CI but is worth cleaning while here.

- [ ] **Step 1: Remove the dead key from `configmap.yaml`**

Delete the `IAM_ADDR: iam.thittam.svc.cluster.local:8086` line. Rewrite the surrounding comment so it documents `IAM_SERVICE_ADDR` WITHOUT referencing the deleted key (drop the "not IAM_ADDR above" phrasing; keep the "without this key permission checks fail closed" explanation). Leave the `IAM_SERVICE_ADDR:` line intact.

- [ ] **Step 2: gofmt the reporting main**

Run: `gofmt -w cmd/reporting-analytics/main.go`
Then confirm: `gofmt -l cmd/reporting-analytics/main.go` prints nothing.

- [ ] **Step 3: Verify build + no IAM_ADDR env readers exist**

Run: `go build ./... && grep -rn 'os.Getenv("IAM_ADDR")\|"IAM_ADDR"' --include='*.go' . || echo "no Go reader of IAM_ADDR (expected)"`
Expected: build clean; no Go code reads `IAM_ADDR`.

- [ ] **Step 4: Commit**

```bash
git add infra/k8s/config/configmap.yaml cmd/reporting-analytics/main.go
git commit -m "chore(infra): drop dead IAM_ADDR configmap key; gofmt reporting main (#141)

Nothing reads IAM_ADDR as an env var (the live key is IAM_SERVICE_ADDR);
a config key that looks load-bearing and isn't misleads. Also gofmt the
reporting-analytics main (import order + trailing blank line).

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Final gate (after all five tasks)

Run before opening the PR:
```bash
go build ./...
go vet ./...
go test ./pkg/observability/ ./pkg/server/ ./pkg/auth/ ./services/expense/ -race
gofmt -l pkg/observability/metrics.go pkg/server/server.go pkg/server/server_test.go \
  pkg/server/integration_test.go pkg/auth/jwt.go pkg/auth/jwt_test.go \
  services/expense/handler_test.go cmd/reporting-analytics/main.go   # must print nothing
```
Then whole-branch review on the most capable model (attention on Task 1's nil-default equivalence and Task 2's security-boundary guard), then PR, then poll `gh pr checks`.

## Self-Review

- **Spec coverage:** §1→Task 1, §2→Task 2, §4→Task 3, §6→Task 4, §3+§5→Task 5. All six covered.
- **Placeholder scan:** collector opts in Task 1 Step 3 say "unchanged" deliberately (copy the existing `prometheus.*Opts{}` verbatim) — not a placeholder, an instruction to preserve. No TBD/TODO.
- **Type consistency:** `interceptor.CallerInfo` (not `Caller`) — matches auth.go:35. `jwtClaims` + `jwtlib` alias — matches jwt_test.go. `prometheus.Registerer` interface used consistently in Task 1. `NewMetrics(serviceName, reg)` call in server.go matches the new signature.
