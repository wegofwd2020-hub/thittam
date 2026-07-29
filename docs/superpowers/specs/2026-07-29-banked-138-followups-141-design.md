# Banked #138 follow-ups (#141) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-29
**Issue:** #141 (chore: follow-ups banked during #138 fail-closed auth) — six minor findings from PR #140 reviews
**Branch:** `chore/141-banked-138-followups` off `main` (390b3e1)
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

Discharge the six minor findings banked during #138. Each is small; grouped into one branch/PR as filed.
The only non-trivial one is §1 (Prometheus registry injection), which unlocks real `pkg/server` unit
testing. The rest are a nil-deref guard, a dead config key, a test-naming fix, a gofmt, and per-call
deadlines in a test harness.

## Context (grounding facts, `main` @ 390b3e1)

- **Go module:** `github.com/wegofwd2020/thittam`.
- **§1:** `pkg/observability/metrics.go` `NewMetrics(serviceName string) *Metrics` uses package-level
  `promauto.New*` (8 collectors, lines 35/46/56/65/75/85/94/103) → all register on the global
  `prometheus.DefaultRegisterer`. Sole caller: `pkg/server/server.go:79`
  `metrics := observability.NewMetrics(sanitizeName(cfg.Name))`. `Config` (server.go:26-43) has NO
  registry field. `server.New(cfg, logger)` is called from 10 `cmd/*/main.go` sites (iam, document,
  project-management, notifications, budget-planning, general-ledger, expense-tracking, billing,
  reporting-analytics, inventory-management). `pkg/server/server_test.go` already documents the hazard
  (lines 14-17): two `New()` with the same `Name` panic "duplicate metrics collector registration". The
  reflection tests dodge it via distinct names. `pkg/server` imports `pkg/observability` (server.go:17).
- **§2:** `pkg/auth/jwt.go` `Validate` (lines 314-357) returns `&Claims{... IssuedAt: c.IssuedAt.Time,
  ExpiresAt: c.ExpiresAt.Time ...}` (lines 347-356) — unguarded `.Time` on `*jwt.NumericDate`. Parser
  uses `jwt.WithExpirationRequired()` so `exp` is present, but `iat` is optional → a signed token
  omitting `iat` panics. `pkg/auth/verifier.go` (lines 104-127) already guards both with nil-checked
  locals — the exact pattern to mirror. Not attacker-reachable (needs iam's signing key; iam always
  mints `iat`), but a panic in a security boundary is bad to leave loaded.
- **§3:** `infra/k8s/config/configmap.yaml:79` `IAM_ADDR: iam.thittam.svc.cluster.local:8086` is read
  by zero Go code. The live key is `IAM_SERVICE_ADDR` (:85, read by `pkg/iamclient/dial.go:17`
  `const EnvAddr = "IAM_SERVICE_ADDR"`). `scripts/verify-project-rbac.sh:42,231` uses `IAM_ADDR` only as
  a self-contained local shell variable (assign-then-echo) — NOT an env read; leave it alone. The
  configmap comment (lines 78-84) already explains IAM_SERVICE_ADDR and references the dead key.
- **§4:** `services/expense/handler_test.go` `TestHandler_SubmitExpense_NoTenant` (296-303) and
  `TestHandler_CreatePettyCashAdvance_NoTenant` (661-669) call the handler with `ctxWithVertical()` — a
  context with NEITHER caller NOR tenant. Post-#138 every handler runs `RequirePermission` first, and
  `RequirePermission` (`pkg/interceptor/permission.go:62-66`) checks caller identity BEFORE the checker:
  no caller → `codes.Unauthenticated "caller identity not present in context"`. So both tests satisfy
  their `Unauthenticated` assertion via the caller-missing path and never reach the tenant lookup their
  names claim. Existing helpers: `ctxWithVertical()` (service_test.go:149-151, neither), `ctxWithTenant`
  (handler_test.go:37-39, BOTH tenant+caller), `ctxWithCaller` (43-44, BOTH). NO helper builds
  caller-present-tenant-absent. `newHandler()` (52-54) wires `allowAllPerm{}`. All 13 `_NoTenant` tests
  share this drift; only the two named here are in scope.
- **§5:** `gofmt -l cmd/reporting-analytics/main.go` → dirty. Two changes: reorder the third-party
  import group so `github.com/jackc/pgx/v5/pgxpool` sorts before `nats "github.com/nats-io/nats.go"`;
  strip one trailing blank line at EOF. CI's `golangci-lint` (ci.yml:97-103, action v8 / v2.x `latest`,
  NO config file) runs only default linters (errcheck, govet, ineffassign, staticcheck, unused) —
  formatting is a separate v2 `formatters` section, not enabled — so this does NOT gate CI. Hygiene only.
- **§6:** `pkg/server/integration_test.go` `startServer` helper (55-72) dials a bufconn via
  `grpc.NewClient` (no context on the dial). Five RPC call sites ride undeadlined `context.Background()`:
  the `bearer(t, key)` helper (:95, feeds tests at :135/:154), and direct calls at :102, :110, :121/:126,
  :144. A hung pipe would block to the test-binary timeout instead of failing fast. The harness builds
  its server with `grpc.NewServer` directly (not `server.New`) — because #138 could not inject a
  registry; §1 removes that blocker but rewriting the harness onto `server.New` is a NON-GOAL here.

## Design

### §1 — inject the Prometheus registry (`pkg/observability`, `pkg/server`)

`pkg/observability/metrics.go`:
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
		RequestDuration: f.NewHistogramVec(...),
		RequestCounter:  f.NewCounterVec(...),
		// ... all 8 collectors switched from promauto.NewX to f.NewX
	}
}
```
`pkg/server/server.go` — add to `Config` (a `prometheus.Registerer`, the interface, so both
`DefaultRegisterer` and a test `*Registry` satisfy it):
```go
	// Registry receives this server's Prometheus collectors. Nil means the
	// global default registry (production default). Inject a fresh
	// prometheus.NewRegistry() in tests to build the server more than once in
	// one process without a duplicate-registration panic.
	Registry prometheus.Registerer
```
and thread it (server.go:79):
```go
	metrics := observability.NewMetrics(sanitizeName(cfg.Name), cfg.Registry)
```
The 10 `cmd/*` call sites are UNCHANGED — they leave `Registry` zero (nil) → global default → identical
production behavior. `pkg/server` gains a `github.com/prometheus/client_golang/prometheus` import.

### §2 — guard `Validate` against nil dates (`pkg/auth/jwt.go`)

Mirror `verifier.go:104-127`. Replace the direct `.Time` reads with nil-checked locals:
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

### §3 — delete the dead `IAM_ADDR` key (`infra/k8s/config/configmap.yaml`)

Remove the `IAM_ADDR:` line and rewrite the surrounding comment so it documents `IAM_SERVICE_ADDR`
without referencing the deleted key. `scripts/verify-project-rbac.sh` unchanged.

### §4 — fix the two drifted expense tests (`services/expense/handler_test.go`)

Add a helper that puts a caller in context but NO tenant, then point the two named tests at it so they
reach — and assert — the tenant-missing branch:
```go
// ctxCallerNoTenant carries vertical config + a caller identity but no tenant,
// so a handler passes RequirePermission and reaches the tenant lookup.
func ctxCallerNoTenant() context.Context {
	return interceptor.WithCaller(ctxWithVertical(), interceptor.Caller{UserID: uuid.New()})
}
```
(Exact `interceptor.WithCaller` / `Caller` construction to match `ctxWithCaller` at handler_test.go:43-44.)
Each of the two tests uses `ctxCallerNoTenant()` and, beyond the `codes.Unauthenticated` code, asserts
the message identifies the tenant branch (e.g. `assert.Contains(t, status.Convert(err).Message(),
"tenant")`) so a future reordering that sends them back through the caller branch fails the test.

### §5 — gofmt `cmd/reporting-analytics/main.go`

Run `gofmt -w`. Only the import reorder + trailing-blank-line strip.

### §6 — per-call deadlines in the bufconn harness (`pkg/server/integration_test.go`)

Add a helper and use it at all five RPC sites so a hang fails fast rather than blocking to the binary
timeout:
```go
// callCtx returns a short-deadline context for a bufconn RPC; a hung pipe then
// fails the test fast instead of blocking to the test-binary timeout.
func callCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
```
Replace each `context.Background()` used for an RPC (including the base inside `bearer`) with
`callCtx(t)`. `bearer` gains a `t *testing.T` parameter (its two call sites at :135/:154 pass `t`).

## Testing

- **§1:** new `pkg/server/server_test.go` test — build two servers with the SAME `Name` but each with
  its own `Registry: prometheus.NewRegistry()`; assert neither construction panics (today the second
  panics on duplicate registration). Optionally assert a known collector landed in an injected registry
  via `reg.Gather()`. Existing reflection tests keep passing (they leave `Registry` nil).
- **§2:** a test that signs a token with the issuer's key but omits `iat`, then calls `Validate` and
  asserts no panic + `IssuedAt.IsZero()` + `exp` preserved. Mirror any equivalent verifier.go test.
- **§4:** the two revised tests assert `codes.Unauthenticated` AND the tenant-branch message.
- **§6:** existing six `TestChain_*` tests keep passing under the deadlined contexts.
- **Gates:** `go test ./pkg/observability/... ./pkg/server/... ./pkg/auth/... ./services/expense/...
  -race`; `go vet ./...`; `go build ./...`; `gofmt -l` on every touched Go file (must be empty).
  YAML validity of configmap.yaml. No proto/sqlc/migration.

## Non-goals

- Rewriting the bufconn harness (`integration_test.go`) onto `server.New` — §1 unblocks it, but the
  rewrite is out of scope; the harness stays on `grpc.NewServer`.
- Fixing the other 11 `_NoTenant` expense tests (same drift; not in the issue's named scope).
- Any schema.json / role-enum / config change beyond deleting `IAM_ADDR`.
- Changing production metrics behavior — §1 is behavior-preserving when `Registry` is nil.

## Review weight

Mostly mechanical. §1 touches a shared package (`pkg/server`, `pkg/observability`) used by all 10
services — the behavior-preserving-when-nil property is the thing to verify. §2 is a security-boundary
nil guard. No senior-required domain (not iam/general-ledger business logic, not money). Whole-branch
review on a capable model, attention on §1's nil-default equivalence.
