# Shared grpc-gateway Helper (#60 Phase C1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the duplicated grpc-gateway REST-mux boilerplate into one `pkg/server` helper and migrate iam/budget/project onto it, byte-identical in behavior.

**Architecture:** A standalone `server.RunRESTGateway(ctx, GatewayConfig)` (built on a testable `buildGatewayHandler`) absorbs the shared mux/marshaler/CORS/dial code; the four per-service axes (endpoint, port, X-Project-Id, outer middleware) become config. Each `cmd/main.go` launches it with `go`, exactly as today.

**Tech Stack:** Go 1.25, grpc-gateway/v2, rs/cors, pkg/corsutil, httptest.

## Global Constraints

- **Behavior-preserving.** Same gateway ports (iam **9086**, budget **9081**, project **9080**), same gRPC endpoints (iam 8086, budget 8081, project 8090), same `JSONPb` marshaler (`UseProtoNames:true`, `EmitUnpopulated:true`, `DiscardUnknown:true`), same CORS (`corsutil.OriginFunc(ExtraOriginsFromEnv()...)`, methods GET/POST/PUT/PATCH/DELETE/OPTIONS, `AllowCredentials:true`), and **iam's rate-limit on `/api/v1/auth/*` preserved exactly** (`CORS(rateLimit(mux))`).
- **X-Project-Id** header matcher + CORS allowed-header on budget & project only; **not** iam.
- **Standalone helper**, `go`-launched goroutine — NOT integrated into `server.New`/`Run` (no graceful-HTTP-shutdown change).
- **No new REST surface** — no proto, `buf generate`, `kong.yml`, or web changes (that's Phase C2).
- **Prune now-unused imports** after each migration — `go build` fails on them; remove until it builds.
- **Commits:** Conventional Commits, scope `api`; end every message with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `pkg/server/gateway.go` | `GatewayConfig` + `buildGatewayHandler` + `RunRESTGateway` | 1 |
| `pkg/server/gateway_test.go` | httptest unit tests for the helper | 1 |
| `cmd/budget-planning/main.go` | replace hand-rolled block with helper call | 2 |
| `cmd/project-management/main.go` | replace hand-rolled block with helper call | 2 |
| `cmd/iam/main.go` | replace block; rate-limit via `Wrap` | 3 |

---

### Task 1: The gateway helper + unit tests

**Files:**
- Create: `pkg/server/gateway.go`
- Create: `pkg/server/gateway_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.GatewayConfig{ServiceName string; GRPCEndpoint string; HTTPPort int; Register func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error; ProjectHeader bool; Wrap func(http.Handler) http.Handler}`, `server.RunRESTGateway(ctx context.Context, cfg GatewayConfig) error`, and the unexported `buildGatewayHandler(ctx, cfg) (http.Handler, error)`.

- [ ] **Step 1: Write the helper**

Create `pkg/server/gateway.go`:
```go
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/wegofwd2020/thittam/pkg/corsutil"
)

// GatewayConfig configures a REST-over-gRPC gateway for one service.
type GatewayConfig struct {
	ServiceName  string // for log lines, e.g. "budget-planning"
	GRPCEndpoint string // the service's own gRPC listen address, e.g. "localhost:8081"
	HTTPPort     int    // the gateway's HTTP listen port, e.g. 9081

	// Register is the generated Register<Svc>ServiceHandlerFromEndpoint.
	Register func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error

	// ProjectHeader, when true, forwards the X-Project-Id request header into
	// gRPC metadata and adds it to the CORS allowed-headers. Identity always
	// comes from the verified token (#138); X-Project-Id selects a resource.
	ProjectHeader bool

	// Wrap, when non-nil, wraps the gateway mux with outer middleware (inside
	// CORS). iam uses it to rate-limit /api/v1/auth/*.
	Wrap func(http.Handler) http.Handler
}

// buildGatewayHandler assembles the gateway mux, optional outer middleware, and
// CORS. It does not listen. Register dials lazily (the gRPC endpoint is not
// contacted until the first RPC), so this is safe to call in tests.
func buildGatewayHandler(ctx context.Context, cfg GatewayConfig) (http.Handler, error) {
	var muxOpts []runtime.ServeMuxOption
	if cfg.ProjectHeader {
		matcher := func(key string) (string, bool) {
			if key == "X-Project-Id" {
				return key, true
			}
			return runtime.DefaultHeaderMatcher(key)
		}
		muxOpts = append(muxOpts, runtime.WithIncomingHeaderMatcher(matcher))
	}
	muxOpts = append(muxOpts, runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
		MarshalOptions: protojson.MarshalOptions{
			UseProtoNames:   true,
			EmitUnpopulated: true,
		},
		UnmarshalOptions: protojson.UnmarshalOptions{
			DiscardUnknown: true,
		},
	}))
	mux := runtime.NewServeMux(muxOpts...)

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := cfg.Register(ctx, mux, cfg.GRPCEndpoint, opts); err != nil {
		return nil, fmt.Errorf("%s: register gateway: %w", cfg.ServiceName, err)
	}

	var h http.Handler = mux
	if cfg.Wrap != nil {
		h = cfg.Wrap(h)
	}

	allowedHeaders := []string{"Content-Type", "Authorization", "Accept"}
	if cfg.ProjectHeader {
		allowedHeaders = append(allowedHeaders, "X-Project-Id")
	}
	corsHandler := cors.New(cors.Options{
		AllowOriginFunc: corsutil.OriginFunc(corsutil.ExtraOriginsFromEnv()...),
		AllowedMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		AllowedHeaders:   allowedHeaders,
		AllowCredentials: true,
	}).Handler(h)
	return corsHandler, nil
}

// RunRESTGateway builds the gateway handler and serves it on :HTTPPort. It
// blocks; callers launch it with `go`.
func RunRESTGateway(ctx context.Context, cfg GatewayConfig) error {
	handler, err := buildGatewayHandler(ctx, cfg)
	if err != nil {
		return err
	}
	log.Printf("%s REST gateway ready on :%d", cfg.ServiceName, cfg.HTTPPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", cfg.HTTPPort), handler)
}
```

- [ ] **Step 2: Write the unit tests**

Create `pkg/server/gateway_test.go`:
```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// noopRegister registers no handlers; the mux 404s but the CORS/Wrap chain
// still runs, which is all these tests exercise.
func noopRegister(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error {
	return nil
}

func TestBuildGatewayHandler_CORSPreflight(t *testing.T) {
	h, err := buildGatewayHandler(context.Background(), GatewayConfig{
		ServiceName: "test", GRPCEndpoint: "localhost:0", Register: noopRegister,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/anything", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, "http://localhost:3100", rec.Header().Get("Access-Control-Allow-Origin"))
}

func preflightAllowHeaders(t *testing.T, projectHeader bool) string {
	t.Helper()
	h, err := buildGatewayHandler(context.Background(), GatewayConfig{
		ServiceName: "test", GRPCEndpoint: "localhost:0",
		ProjectHeader: projectHeader, Register: noopRegister,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/anything", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "X-Project-Id")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Header().Get("Access-Control-Allow-Headers")
}

func TestBuildGatewayHandler_ProjectHeaderToggle(t *testing.T) {
	// rs/cors echoes a requested header in Access-Control-Allow-Headers only
	// when it is in AllowedHeaders. Compare case-insensitively (rs/cors
	// canonicalizes header names).
	on := strings.ToLower(preflightAllowHeaders(t, true))
	assert.Contains(t, on, "x-project-id", "ProjectHeader:true must allow X-Project-Id")

	off := strings.ToLower(preflightAllowHeaders(t, false))
	assert.NotContains(t, off, "x-project-id", "ProjectHeader:false must not allow X-Project-Id")
}

func TestBuildGatewayHandler_WrapInvoked(t *testing.T) {
	h, err := buildGatewayHandler(context.Background(), GatewayConfig{
		ServiceName: "test", GRPCEndpoint: "localhost:0", Register: noopRegister,
		Wrap: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Wrap-Marker", "1")
				next.ServeHTTP(w, r)
			})
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, "1", rec.Header().Get("X-Wrap-Marker"), "Wrap middleware must run inside CORS")
}
```

- [ ] **Step 3: Run the tests**

Run from repo root:
```bash
go test ./pkg/server/ -run TestBuildGatewayHandler -race -v
```
Expected: all three PASS. If `TestBuildGatewayHandler_ProjectHeaderToggle` fails because rs/cors formats the header differently than expected, inspect the actual `Access-Control-Allow-Headers` value printed and adjust the assertion to match rs/cors's real output (the toggle — present vs absent — must still be what's asserted; do not weaken it to always-pass).

- [ ] **Step 4: Build the whole tree**

```bash
go build ./... && go vet ./pkg/server/
```
Expected: both exit 0.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/gateway.go pkg/server/gateway_test.go
git commit -m "feat(api): add shared RunRESTGateway helper for grpc-gateway (#60 Phase C1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Migrate budget-planning and project-management

**Files:**
- Modify: `cmd/budget-planning/main.go` (replace the `// --- REST gateway ---` `go func(){…}()` block, ~lines 121-168)
- Modify: `cmd/project-management/main.go` (replace the equivalent block, ~lines 121-169)

**Interfaces:**
- Consumes: `server.RunRESTGateway` + `server.GatewayConfig` from Task 1. Both files already import `github.com/wegofwd2020/thittam/pkg/server` (they call `server.New`).

- [ ] **Step 1: Replace budget-planning's gateway block**

Delete the entire `// --- REST gateway (grpc-gateway …) ---` comment + `go func(){ … }()` block and replace with:
```go
	// --- REST gateway (grpc-gateway, #60) — via the shared helper. ---
	go func() {
		if err := server.RunRESTGateway(ctx, server.GatewayConfig{
			ServiceName:   "budget-planning",
			GRPCEndpoint:  "localhost:8081",
			HTTPPort:      9081,
			Register:      budgetv1.RegisterBudgetServiceHandlerFromEndpoint,
			ProjectHeader: true,
		}); err != nil {
			log.Fatalf("budget-planning: gateway: %v", err)
		}
	}()
```
(`budgetv1` is the existing import alias the deleted block used for `RegisterBudgetServiceHandlerFromEndpoint` — keep it.)

- [ ] **Step 2: Replace project-management's gateway block**

Same shape, with project's values (copy the endpoint/port and the exact `Register…FromEndpoint` name the deleted block used — for project it is `RegisterProjectServiceHandlerFromEndpoint` on the project's import alias):
```go
	// --- REST gateway (grpc-gateway, #60) — via the shared helper. ---
	go func() {
		if err := server.RunRESTGateway(ctx, server.GatewayConfig{
			ServiceName:   "project-management",
			GRPCEndpoint:  "localhost:8090",
			HTTPPort:      9080,
			Register:      projectv1.RegisterProjectServiceHandlerFromEndpoint,
			ProjectHeader: true,
		}); err != nil {
			log.Fatalf("project-management: gateway: %v", err)
		}
	}()
```
(Use whatever alias the file already imports the project gen package under — match the deleted block's `Register…` call verbatim.)

- [ ] **Step 3: Prune now-unused imports and build**

The deleted blocks were the only users of `runtime`, `protojson`, `cors`, `corsutil`, `insecure`, `grpc` (dial), and `net/http` in these two files. Run:
```bash
go build ./cmd/budget-planning/ ./cmd/project-management/
```
It will fail on each now-unused import. Remove exactly the imports it names from each file's import block, then rebuild until it passes. Do NOT remove imports still used elsewhere in the file (e.g. `log`, `context`, the gen alias, `server`).

- [ ] **Step 4: Build + vet the whole tree; re-run the helper test**

```bash
go build ./... && go vet ./... && go test ./pkg/server/ -run TestBuildGatewayHandler
```
Expected: all exit 0 / PASS. This confirms both migrated services compile against the helper and nothing else broke.

- [ ] **Step 5: Commit**

```bash
git add cmd/budget-planning/main.go cmd/project-management/main.go
git commit -m "refactor(api): migrate budget + project gateways onto RunRESTGateway (#60 Phase C1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Migrate iam (rate-limit via Wrap)

**Files:**
- Modify: `cmd/iam/main.go` (replace the `// --- REST gateway ---` block, ~lines 245-309)

**Interfaces:**
- Consumes: `server.RunRESTGateway` + `server.GatewayConfig` (Task 1). `cmd/iam/main.go` already imports `pkg/server`.

- [ ] **Step 1: Replace iam's gateway block**

iam's block builds the auth rate-limiter (needs `rdb` + `AUTH_RATE_LIMIT`/`AUTH_RATE_WINDOW` env) and routes `/api/v1/auth/*` through it. Keep the limiter construction in `main`; move only its wiring into `Wrap`. Replace the whole `// --- REST gateway … ---` comment + `go func(){ … }()` block with:
```go
	// --- REST gateway (grpc-gateway, #60) — via the shared helper. ---
	// The auth rate-limiter is built here (it needs rdb + AUTH_RATE_* env) and
	// wired into the gateway via GatewayConfig.Wrap, preserving the previous
	// CORS(rateLimit(mux)) chain: /api/v1/auth/* is rate-limited, everything
	// else goes straight through. iam does NOT forward X-Project-Id.
	authRateLimiter := ratelimit.Middleware(rdb, ratelimit.Config{
		Limit:     envInt("AUTH_RATE_LIMIT", 10),
		Window:    envDuration("AUTH_RATE_WINDOW", time.Minute),
		KeyPrefix: "iam:auth",
	})
	log.Printf("iam auth rate-limit: %d/%s", envInt("AUTH_RATE_LIMIT", 10), envDuration("AUTH_RATE_WINDOW", time.Minute))
	go func() {
		if err := server.RunRESTGateway(ctx, server.GatewayConfig{
			ServiceName:  "iam",
			GRPCEndpoint: "localhost:8086",
			HTTPPort:     9086,
			Register:     iamv1.RegisterIAMServiceHandlerFromEndpoint,
			Wrap: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
						authRateLimiter(next).ServeHTTP(w, r)
						return
					}
					next.ServeHTTP(w, r)
				})
			},
		}); err != nil {
			log.Fatalf("iam: gateway: %v", err)
		}
	}()
```
(`iamv1` is the existing alias the deleted block used for `RegisterIAMServiceHandlerFromEndpoint`. `http`, `strings`, `ratelimit`, `time`, `log`, `envInt`, `envDuration`, `rdb` are all still used here — keep them.)

- [ ] **Step 2: Prune now-unused imports and build**

The deleted block was the only user of `runtime`, `protojson`, `cors`, `corsutil`, `insecure`, and `grpc` (dial) in `cmd/iam/main.go` — but `http` and `strings` are STILL used (by the `Wrap` closure), so keep them. Run:
```bash
go build ./cmd/iam/
```
Remove exactly the imports it flags as unused, then rebuild until it passes.

- [ ] **Step 3: Verify the rate-limit wiring is preserved**

Confirm by inspection that the `Wrap` closure rate-limits `/api/v1/auth/*` and passes everything else through, and that CORS wraps it (the helper applies CORS outside `Wrap`) — i.e. the request path is `CORS( authWrap( mux ) )`, identical to the previous `CORS(routed)` where `routed` did the same prefix check. No behavior change.

- [ ] **Step 4: Build + vet the whole tree; re-run the helper test**

```bash
go build ./... && go vet ./... && go test ./pkg/server/ -run TestBuildGatewayHandler
```
Expected: all exit 0 / PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/iam/main.go
git commit -m "refactor(iam): migrate auth gateway onto RunRESTGateway, rate-limit via Wrap (#60 Phase C1)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Helper `GatewayConfig` + `buildGatewayHandler` + `RunRESTGateway`, shared marshaler/CORS, `ProjectHeader` toggle, `Wrap` → Task 1 ✅
- Migrate budget (9081, ProjectHeader) + project (9080, ProjectHeader) → Task 2 ✅
- Migrate iam (9086, no ProjectHeader, Wrap = rate-limit on /api/v1/auth/*) → Task 3 ✅
- Unit test: CORS preflight, ProjectHeader toggle observable, Wrap invoked → Task 1 ✅
- Behavior-preserving; no proto/kong/web change; standalone goroutine → Global Constraints ✅

**Placeholder scan:** none — full helper, full test, full replacement blocks. Import pruning is "remove exactly what `go build` flags" (concrete, compiler-driven), not a vague "clean up imports".

**Type consistency:** `GatewayConfig` field names/types match between the helper (Task 1) and all three call sites (Tasks 2-3). `Register`'s type matches the generated `Register…ServiceHandlerFromEndpoint` signature. `Wrap func(http.Handler) http.Handler` matches iam's closure.

**Ordering:** Task 1 (helper, standalone, tested) → Task 2 (two simple migrations) → Task 3 (iam, the rate-limit one). iam last as the highest-risk migration; each task builds the whole tree.
