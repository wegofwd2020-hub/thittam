# Shared grpc-gateway Helper (#60 Phase C1) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-25
**Issue:** #60 (REST→gRPC bridge), **Phase C1** — extract the gateway boilerplate + migrate the 3 existing services
**Branch:** `refactor/gateway-helper-60c1` off `main` (`1555367`)
**Migration:** none

## Goal

Extract the grpc-gateway REST-mux boilerplate — duplicated near-verbatim in
`cmd/iam`, `cmd/budget-planning`, `cmd/project-management` — into one shared
`pkg/server` helper, and migrate all three services onto it. Behavior-preserving:
each service's REST gateway must serve byte-identically after. This gives Phase
C2 a single, tested way to add a gateway to a new service instead of copying ~45
lines a fourth through sixth time.

## Context

Phase A shipped a REST gateway for three services; each `cmd/<svc>/main.go` runs
its own `go func(){ … http.ListenAndServe(":90xx", …) }()` block. The three
blocks are identical except in four spots (measured):

| axis | iam | budget | project |
|---|---|---|---|
| gRPC endpoint dialed | `localhost:8086` | `localhost:8081` | `localhost:8090` |
| gateway HTTP port | `:9086` | `:9081` | `:9080` |
| `X-Project-Id` incoming-header matcher + CORS allowed-header | **no** | yes | yes |
| outer middleware | rate-limit on `/api/v1/auth/*` | none | none |

Everything else is byte-identical: the `runtime.NewServeMux` `JSONPb` marshaler
options (`UseProtoNames:true`, `EmitUnpopulated:true`, `DiscardUnknown:true`), the
`grpc.WithTransportCredentials(insecure.NewCredentials())` dial option, and the
CORS config (`corsutil.OriginFunc(corsutil.ExtraOriginsFromEnv()...)`,
`AllowedMethods` = GET/POST/PUT/PATCH/DELETE/OPTIONS, `AllowCredentials:true`).

`pkg/server.Config` has **no** gateway field today; the gateway is a
fire-and-forget goroutine each `main.go` launches after `RegisterHealthChecker`,
before `srv.Run()`. `pkg/server` is otherwise gRPC-only.

### Design choice: standalone helper, not a `server.Run` integration

The helper is a standalone `server.RunRESTGateway(ctx, GatewayConfig)` launched
by `go` from each `main.go` — preserving today's fire-and-forget goroutine model.
Folding the gateway into `server.New`/`Run` (so `Run` serves both gRPC and HTTP
with graceful HTTP shutdown) is a larger architectural change than "extract the
boilerplate" warrants and would alter the shutdown semantics of a working auth
gateway. Out of scope for C1; revisit only if a concrete need appears.

## Design

### The helper — `pkg/server/gateway.go`

```go
// GatewayConfig configures a REST-over-gRPC gateway for one service.
type GatewayConfig struct {
	ServiceName  string // for log lines, e.g. "budget-planning"
	GRPCEndpoint string // the service's own gRPC listen address, e.g. "localhost:8081"
	HTTPPort     int    // the gateway's HTTP listen port, e.g. 9081

	// Register is the generated Register<Svc>ServiceHandlerFromEndpoint.
	Register func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error

	// ProjectHeader, when true, forwards the X-Project-Id request header into
	// gRPC metadata and adds it to the CORS allowed-headers.
	ProjectHeader bool

	// Wrap, when non-nil, wraps the gateway mux with outer middleware (inside
	// CORS). iam uses it to rate-limit /api/v1/auth/*; nil means no wrapper.
	Wrap func(http.Handler) http.Handler
}
```

Two functions — construction split from serving so the handler is unit-testable
without binding a port:

```go
// buildGatewayHandler assembles the mux + optional wrap + CORS. It does not
// listen. Register dials lazily (grpc endpoint not contacted until first RPC),
// so this is safe to call in tests with any endpoint.
func buildGatewayHandler(ctx context.Context, cfg GatewayConfig) (http.Handler, error)

// RunRESTGateway builds the handler and serves it on :HTTPPort (blocking).
// Callers launch it with `go`.
func RunRESTGateway(ctx context.Context, cfg GatewayConfig) error
```

`buildGatewayHandler` does exactly what the three blocks do:
1. mux options: the shared `JSONPb` marshaler; **if `ProjectHeader`**, prepend a
   `runtime.WithIncomingHeaderMatcher` that passes `X-Project-Id` through and
   defers to `runtime.DefaultHeaderMatcher` otherwise.
2. `cfg.Register(ctx, mux, cfg.GRPCEndpoint, []grpc.DialOption{insecure})`; wrap
   the returned error as `"<ServiceName>: register gateway: %w"`.
3. `var h http.Handler = mux; if cfg.Wrap != nil { h = cfg.Wrap(h) }`.
4. CORS: `AllowedHeaders` = `[Content-Type, Authorization, Accept]`, **plus
   `X-Project-Id` when `ProjectHeader`**; other CORS options exactly as today;
   `.Handler(h)`.

`RunRESTGateway` logs `"<ServiceName> REST gateway ready on :<HTTPPort>"` then
`http.ListenAndServe(":<HTTPPort>", handler)`.

### The three migrations

Each `cmd/<svc>/main.go` replaces its ~45-line block with an ~8-line launch. The
generated `Register…FromEndpoint` already matches `GatewayConfig.Register`'s
signature, so it is passed directly.

- **budget-planning** — `ServiceName:"budget-planning"`, `GRPCEndpoint:"localhost:8081"`, `HTTPPort:9081`, `Register: budgetv1.RegisterBudgetServiceHandlerFromEndpoint`, `ProjectHeader:true`, `Wrap:nil`.
- **project-management** — same shape, `"localhost:8090"` / `9080` / `RegisterProjectServiceHandlerFromEndpoint`, `ProjectHeader:true`.
- **iam** — `"localhost:8086"` / `9086` / `RegisterIAMServiceHandlerFromEndpoint`, `ProjectHeader:false`, and `Wrap` = a closure built in `main` that carries the existing `ratelimit.Middleware(rdb, …)` and routes `/api/v1/auth/*` through it, else passes to the next handler — reproducing today's `CORS(rateLimit(mux))` exactly. The rate limiter is still constructed in `cmd/iam/main.go` (it needs `rdb` + the `AUTH_RATE_LIMIT`/`AUTH_RATE_WINDOW` env); only its *wiring into the handler chain* moves behind `Wrap`.

The iam `Wrap` closure, verbatim behavior of the current `routed` HandlerFunc:
```go
Wrap: func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			authRateLimiter(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
},
```

## Testing

- **Helper unit test** (`pkg/server/gateway_test.go`), via `httptest` against
  `buildGatewayHandler` (no real port, `Register` = a no-op that registers
  nothing so the mux 404s but the wrapper chain still runs):
  - a CORS **preflight** (`OPTIONS` with `Origin: http://localhost:3100`) returns
    `Access-Control-Allow-Origin: http://localhost:3100`.
  - **`ProjectHeader` toggle is observable**: with `ProjectHeader:true` the
    preflight's `Access-Control-Allow-Headers` includes `X-Project-Id`; with
    `false` it does not.
  - **`Wrap` is invoked**: a `Wrap` that sets a marker response header is present
    on a request that passes through, proving the outer middleware runs inside
    CORS.
- **Migrations are behavior-preserving and verified by `go build ./...` +
  `go vet ./...`** plus the helper test staying green. The three cmd gateways
  have no existing automated test harness (they're `main`-package goroutines), so
  there is no unit/integration test to assert their runtime behavior; the
  whole-branch review confirms the four per-service axes are wired correctly, and
  a manual `curl` through Kong (Phase B) is the optional real-world check. This
  limitation is stated plainly rather than papered over with a vacuous test.
- Existing gRPC integration/e2e tests are untouched — gRPC ports and handlers do
  not change.

## Non-goals

- **No new REST surface** — no proto changes, no `buf generate`, no `kong.yml`
  or web changes. Those are Phase C2 (expense/inventory/reporting).
- **No `server.New`/`Run` lifecycle integration / graceful HTTP shutdown** — the
  goroutine model is preserved.
- **No behavior change** to any of the three gateways — same ports, same JSON
  options, same CORS, same iam rate-limiting on `/api/v1/auth/*`.
- Ledger/notifications/document/billing gateways — deferred (no UI consumer).

## Review weight

Touches `pkg/server` (shared infrastructure) and iam's auth gateway (rate
limiting) → senior engineer + 2 approvals. Whole-branch review on the most
capable model.
