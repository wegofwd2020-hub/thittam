# Thittam (திட்டம்)

**Thittam** — Tamil for "plan" — is a multi-tenant SaaS platform for
production management. One codebase, many verticals: film productions,
construction projects, software delivery, live events. Each tenant's industry
is captured declaratively in a YAML "vertical" file; services read the config
at request time and adapt entity names, phase graphs, budget categories, and
workflows to match the domain.

This repository contains the **application code** (Go microservices + a
Next.js frontend). Architecture docs, ADRs, and API specs live in the
companion docs repo:
[`github.com/wegofwd2020-hub/thittam_docs`](https://github.com/wegofwd2020-hub/thittam_docs).

**Company:** WeGoFwd2020

---

## Architecture

Nine Go microservices on gRPC (sync) + NATS JetStream (async), fronted by Kong
API Gateway for REST/JSON consumers. PostgreSQL for durable state; Redis for
caching and rate limiting; MinIO for object storage.

| Service                 | Port | Role                                            |
| ----------------------- | ---- | ----------------------------------------------- |
| project-management      | 8080 | Productions / projects, phases, crew, schedules |
| budget-planning         | 8081 | Budget versions, line items, approvals          |
| expense-tracking        | 8082 | POs, receipts, petty cash                       |
| general-ledger          | 8083 | Double-entry accounting                         |
| inventory-management    | 8084 | Equipment, props, locations                     |
| reporting-analytics     | 8085 | Cross-service reports (read-only)               |
| iam                     | 8086 | Identity, auth, RBAC, tenancy                   |
| notifications           | 8087 | Email, SMS, push, in-app                        |
| document                | 8088 | File storage, versioning, e-signatures          |

Data isolation is **tenant-per-schema**: each tenant's tables live in a
dedicated `tenant_<uuid>` PostgreSQL schema, routed via `SET search_path` on
the pooled connection (see [`pkg/tenantdb`](pkg/tenantdb/tenantdb.go)). The
full model — including the vertical plugin system, per-vertical UI
adaptation, and the runbook for adding new tenants — is documented in
[`docs/multi-tenancy.md`](docs/multi-tenancy.md).

## Repository layout

```
├── cmd/                      # main.go entry points per service
├── services/                 # service-local code (handlers, repos, tests)
│   ├── iam/  budget/  expense/  ledger/  project/  inventory/
│   ├── reporting/  notifications/  document/  billing/
├── proto/                    # protobuf + gRPC definitions
├── gen/                      # generated buf / grpc-gateway output
├── migrations/               # SQL migrations per service
├── pkg/                      # shared packages
│   ├── tenant/  tenantdb/    # tenant context + schema-routed connections
│   ├── vertical/             # vertical YAML loader, config types, interceptors
│   ├── secrets/              # Vault + file-based secret sources
│   └── ...
├── seeds/demo/               # per-tenant demo fixtures
│   ├── xyz-cba/              # XYZ_CBA Productions (movie vertical, INR) — seeded
│   └── xyz-construction/     # XYZ Construction LLC (construction, USD) — scaffolded
├── web/                      # Next.js frontend (port 3100)
├── e2e/critical_path/        # in-process Go E2E tests (no Docker)
├── load-tests/               # k6 scenarios + chaos tests
├── infra/local/              # docker-compose for Redis / NATS / MinIO
├── docs/                     # repo-local docs (multi-tenancy, operations, verticals)
└── Makefile                  # developer entry point — run `make help`
```

## Getting started

### Prerequisites

- Go 1.22+ (CI pins `1.25.9`)
- Node.js 20+ (for the `web/` frontend)
- PostgreSQL 16 on `localhost:5433`, or Docker (`make infra-up-full`)
- Docker + docker-compose (for Redis, NATS, MinIO)
- [`tmuxinator`](https://github.com/tmuxinator/tmuxinator) (optional, for `make run-all`)
- Tooling: `buf`, `sqlc`, `golangci-lint`, `migrate`, `govulncheck`,
  `gitleaks` — install per `tools/`

### First-run setup

```bash
# 1. Start Docker middleware (Redis, NATS, MinIO)
make infra-up

# 2. Create the local Postgres role + database + run every migration
make db-bootstrap WITH_SEED=1   # seeds XYZ_CBA demo tenant

# 3. Generate local JWT / OIDC keys (gitignored)
make dev-keys

# 4. Start all backend services (9 windows via tmuxinator)
make dev-start

# 5. In a separate terminal, start the frontend
make run-web                     # Next.js on :3100
```

Log in at <http://localhost:3100/login> as `rajesh.kumar@xyzcba.com` /
`demo1234`. See
[`seeds/demo/xyz-cba/README.md`](seeds/demo/xyz-cba/README.md) for the full
user roster and fixture details.

### Common commands

```bash
make help                     # show every Makefile target with a short description
make migrate-all              # run DB migrations
make seed                     # load XYZ_CBA demo data
make run-all                  # start all 9 services via tmuxinator
make test                     # unit tests (-short)
make test-race                # unit tests with -race
make test-integration         # integration tests against thittam_test DB
make test-e2e                 # Playwright UX suite (see web/tests/e2e/README.md)
make validate-verticals       # validate every vertical YAML
make coverage-check           # enforce per-package coverage thresholds (CI parity)
make lint                     # golangci-lint
make generate                 # buf + sqlc code generation
```

## Testing

- **Unit tests** — `go test ./... -short`. Coverage thresholds: `iam` /
  `general-ledger` ≥ 85%, `budget` / `expense` ≥ 80%, others ≥ 75%. Enforced
  in CI; run `make coverage-check` locally for parity.
- **Integration tests** — `go test ./... -tags=integration -race`. Uses a
  dedicated `thittam_test` database (`make db-test-bootstrap`).
- **E2E (Go)** — `e2e/critical_path/` runs in-process with stubbed deps, no
  Docker. Target: ≤ 5 minutes.
- **E2E (UI)** — Playwright in `web/tests/e2e/`. Five projects: `smoke`
  (unauthenticated), `setup` (logs in once), and `chromium` / `firefox` /
  `webkit` (reuse storageState). See
  [`web/tests/e2e/README.md`](web/tests/e2e/README.md).

## Verticals

Each vertical is a YAML file in
[`pkg/vertical/configs/`](pkg/vertical/configs/) that defines entity labels,
phase graphs, budget categories, expense types, and inventory buckets.
Shipped today:

- `movie-production` (in use by xyz-cba)
- `construction` (ready; xyz-construction demo seed pending)
- `software-development`
- `events-management`

Schema reference: [`docs/verticals/schema.md`](docs/verticals/schema.md).

## Conventions

- **Branching:** trunk-based; short-lived `feat/` / `fix/` / `chore/` /
  `docs/` / `hotfix/` branches with ticket IDs.
- **Commits:** Conventional Commits — `<type>(<scope>): <summary>`. Scopes
  include `iam`, `budget`, `expense`, `ledger`, `inventory`, `reporting`,
  `notifications`, `document`, `project`, `billing`, `proto`, `infra`, `api`,
  `ui`, `seed`.
- **Merge strategy:** squash-and-merge.
- **Review:** 2 approvals required; a senior engineer must review changes
  to `iam`, `general-ledger`, or anything security-sensitive.
- **Money:** `decimal.Decimal` (shopspring), `NUMERIC(14,2)`, never `float64`.
  JSON serialises as a 2-decimal-place string.
- **SQL:** parameterised via sqlc or pgx named params — no string
  interpolation, ever.
- **Secrets:** T1 (JWT keys, DB passwords) from Vault; T2 from env via K8s
  Secret; never hardcoded. Full tiering in
  [`~/coding-standards/CODING_RULES.md`](https://github.com/wegofwd2020-hub/coding-standards).
- **Logging:** structured via `slog`; no PII or secrets.

## Documentation

- **Architecture / ADRs / API specs:**
  [`wegofwd2020-hub/thittam_docs`](https://github.com/wegofwd2020-hub/thittam_docs)
- **Repo-local docs:** [`docs/`](docs/)
  - [`multi-tenancy.md`](docs/multi-tenancy.md) — isolation model and runbook
  - [`verticals/schema.md`](docs/verticals/schema.md) — vertical YAML schema
  - [`operations/nats-dlq.md`](docs/operations/nats-dlq.md) — NATS DLQ strategy
  - [`demo-xyz-construction-plan.md`](docs/demo-xyz-construction-plan.md) —
    rollout plan for the construction demo tenant
- **Frontend:** [`web/README.md`](web/README.md) and
  [`web/tests/e2e/README.md`](web/tests/e2e/README.md)
- **Load / chaos:** [`load-tests/README.md`](load-tests/README.md)

## License

Proprietary. © WeGoFwd2020. All rights reserved.
