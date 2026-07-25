# Kong Single-Origin Local Proxy (#60 Phase B) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-25
**Issue:** #60 (REST→gRPC bridge), **Phase B only** — Kong in local docker-compose
**Branch:** `feat/kong-local-60b` off `main` (`ecd218e`)
**Migration:** none

## Goal

Put a Kong gateway in front of the local stack on a single origin
(`http://localhost:8500`) so the Next.js UI reaches the backend through one URL
instead of per-service grpc-gateway ports. Route `/api/v1/*` paths to the three
services that already have a REST gateway; make Kong part of the standard local
stack and the UI's default API origin.

## Context

`#60` Phase A (grpc-gateway) is **already merged** for three services — each
exposes a REST surface on its own HTTP port via a hand-rolled
`runtime.NewServeMux` in `cmd/<svc>/main.go`:

| service | gRPC | grpc-gateway HTTP | `/api/v1` paths (from proto annotations) |
|---|---|---|---|
| iam | 8086 | **9086** | `/api/v1/auth/{login,refresh,logout,me}`, `/api/v1/invitations/{token}/accept` |
| budget-planning | 8081 | **9081** | `/api/v1/budgets`, `/api/v1/budgets/{id}/...` |
| project-management | 8090 | **9080** | `/api/v1/productions/...`, `/api/v1/phases/...`, `/api/v1/crew/...`, `/api/v1/config/...` |

The UI (`web/src/env.ts`, `client.ts`, `auth.ts`) is already built against these
REST paths, currently routing per-service by port, with an **unused escape
hatch**: `NEXT_PUBLIC_API_URL` (env.ts comment literally anticipates
`http://localhost:8000` + Kong). This phase fills that in.

**What is missing (this phase supplies):** there is **no Kong** anywhere in
local — no service in either `infra/local/docker-compose{,.infra}.yml`, no
`kong.yml`, no Makefile/dev-start integration. (K8s has only Kong *plugin* CRDs
and uses a different, Kong-native transcoding approach — out of scope.)

### The one hard constraint: host-native services

`scripts/dev-start.sh` runs the Go services **natively on the host** (`go run`),
not in the compose network. So the Kong *container* must reach host ports
`:9086/:9081/:9080`. This design uses **`host.docker.internal` via
`extra_hosts: ["host.docker.internal:host-gateway"]`** — portable across
Linux/macOS and compose-idiomatic — rather than `network_mode: host`
(Linux-only, and it disables port publishing).

### Port choice

`:8500` (chosen). `:8000` (Kong's default, and what env.ts's comment names) is
free too, but the user selected `8500`; `:9000` is **taken by MinIO**. `8500`
collides with nothing (gRPC ports stop at 8090; the 90xx range holds gateways +
health + prometheus). Kong's proxy is explicitly set to `0.0.0.0:8500`.

## Design

### 1. `infra/local/kong.yml` — DB-less declarative config

`strip_path: false` so the full `/api/v1/...` path reaches each gateway
unchanged. Routes only the three services that have gateways today:

```yaml
_format_version: "3.0"

services:
  - name: iam
    url: http://host.docker.internal:9086
    routes:
      - name: iam
        paths: ["/api/v1/auth", "/api/v1/invitations"]
        strip_path: false
  - name: budget
    url: http://host.docker.internal:9081
    routes:
      - name: budget
        paths: ["/api/v1/budgets"]
        strip_path: false
  - name: project
    url: http://host.docker.internal:9080
    routes:
      - name: project
        paths: ["/api/v1/productions", "/api/v1/phases", "/api/v1/crew", "/api/v1/config"]
        strip_path: false
```

Paths for the other seven services deliberately have **no route** — a request to
`/api/v1/expenses` returns Kong's 404, which fails fast (better than today's UI
fallthrough that silently hits iam:9086). Those routes get added as Phase C
gives each service a gateway.

### 2. Kong service in both compose files

MinIO/Redis/NATS already appear in **both** `docker-compose.yml` (full, with
Postgres — used by `infra-up-full`) and `docker-compose.infra.yml`
(middleware-only — used by `infra-up`); Kong goes in both, so either startup
path brings it up:

```yaml
kong:
  image: kong:3.6
  restart: unless-stopped
  environment:
    KONG_DATABASE: "off"
    KONG_DECLARATIVE_CONFIG: /kong/kong.yml
    KONG_PROXY_LISTEN: "0.0.0.0:8500"
    KONG_ADMIN_LISTEN: "off"
  extra_hosts:
    - "host.docker.internal:host-gateway"
  volumes:
    - ./kong.yml:/kong/kong.yml:ro
  ports:
    - "8500:8500"
  healthcheck:
    test: ["CMD", "kong", "health"]
    interval: 10s
    timeout: 5s
    retries: 5
```

Admin API is `off` (config is loaded from the mounted file at boot; nothing needs
the admin port locally). Kong DB-less lazy-connects upstreams, so it can start
before the host services without a boot-time dependency.

### 3. CORS — no Kong plugin

The service gateways already wrap responses in CORS (`pkg/corsutil`) and Kong
forwards the browser's `Origin: http://localhost:3100`, so the upstream's
`Access-Control-Allow-Origin` still resolves correctly through `:8500`. Adding a
Kong CORS plugin would double the headers — so this phase adds none. (Verified as
part of acceptance.)

### 4. UI default → single origin

- New committed `web/.env.development` with
  `NEXT_PUBLIC_API_URL=http://localhost:8500`. `client.ts` uses `env.apiUrl`
  whenever set, so every `/api/v1/*` call routes through Kong. Kong is now part
  of the standard stack (`infra-up`), so this is the default dev path; the
  per-service-port fallback remains for anyone running without Kong.
- `web/src/env.ts` comment updated: the `8000` reference → `8500`.
- `web/src/app/(auth)/login/page.tsx` SSO fallback `http://localhost:8086` →
  `http://localhost:8500` (stale bare-gRPC-port default the map flagged).

These web changes are covered by the **`Web Lint & Build`** CI gate (#179);
`.env.development` isn't linted, the comment/string edits are trivial.

### 5. Dev-tooling wiring

- `scripts/dev-start.sh`: add the `:8500` single origin to the printed summary;
  fix its stale "Quick login" example (`curl :8086/auth/login` →
  `curl :8500/api/v1/auth/login` with the seed cred + correct path).
- `scripts/verify-kong.sh` (new): `POST http://localhost:8500/api/v1/auth/login`
  with the seed super-admin cred and assert an `access_token` in the flat
  snake_case TokenPair response; non-zero exit on failure. Documented as the
  scripted half of acceptance (requires the stack up).
- `Makefile`: a `kong-check` target running `scripts/verify-kong.sh` (optional
  convenience; the target simply calls the script).

## Testing

This is **dev-infra, not CI-gated** — there is no CI job for local docker-compose
or Kong, and no Go code changes, so the gRPC integration tests are untouched.
Acceptance is scripted + manual against a running stack:

1. `make infra-up-full` (brings Postgres + middleware **incl. Kong**) →
   `make db-bootstrap WITH_SEED=1` → `make dev-start`.
2. **Scripted:** `scripts/verify-kong.sh` — `curl -X POST
   localhost:8500/api/v1/auth/login` with seed creds returns a JWT pair
   (`access_token` present). Exit 0.
3. **Manual:** the UI login form (`web/src/app/(auth)/login/page.tsx`) at
   `localhost:3100`, with `NEXT_PUBLIC_API_URL=http://localhost:8500`, logs in
   through Kong and lands on the dashboard (proves end-to-end + CORS through
   Kong).
4. `curl localhost:8500/api/v1/budgets` and a `/api/v1/productions` path route to
   budget/project respectively (proves multi-service routing); an unrouted path
   (`/api/v1/expenses`) returns Kong 404 (proves fail-fast, not silent
   fallthrough).

The `docker compose config` parse (both files) and Kong booting with the
declarative config (validated by Kong at startup) are the structural checks the
plan will run without a full stack.

## Non-goals

- **The remaining seven service gateways** (expense, ledger, inventory,
  reporting, notifications, document, billing) and the shared `pkg/server`
  gateway helper — Phase C, separate spec.
- **Kong JWT / rate-limiting / tenant-header plugins locally** — those live in
  the K8s manifests; not needed for local end-to-end.
- **Production K8s Kong wiring** — separate ticket; note the K8s approach
  (Kong-native gRPC transcoding to the bare gRPC port) differs from this local
  grpc-gateway-per-service model and is not unified here.
- No Go service changes; no proto changes; no new CI job.

## Review weight

Touches `infra/local` and `web` (dev tooling + UI config), no
`iam`/`general-ledger`/security core → standard 2 approvals. Whole-branch review
at the end.
