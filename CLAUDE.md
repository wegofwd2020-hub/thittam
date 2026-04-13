# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is the **application code repository** for Thittam (திட்டம் — "plan" in Tamil), a multi-tenant SaaS platform for production management. Documentation lives in a separate repo: [`github.com/wegofwd2020-hub/thittam_docs`](https://github.com/wegofwd2020-hub/thittam_docs).

**Company:** WeGoFwd2020

## Architecture

Go 1.22+ microservices platform with 9 services communicating via gRPC (sync) and NATS JetStream (async), fronted by Kong API Gateway for REST/JSON.

| Service | Port | Role |
|---|---|---|
| project-management | 8080 | Productions, phases, crew, schedules |
| budget-planning | 8081 | Budget versions, line items, approvals |
| expense-tracking | 8082 | POs, receipts, petty cash |
| general-ledger | 8083 | Double-entry accounting |
| inventory-management | 8084 | Equipment, props, locations |
| reporting-analytics | 8085 | Cross-service reports (read-only) |
| iam | 8086 | Identity, auth, RBAC, tenancy |
| notifications | 8087 | Email, SMS, push, in-app |
| document | 8088 | File storage, versioning, e-signatures |

**Data isolation:** Tenant-per-schema PostgreSQL — each tenant gets schema `tenant_<uuid>`, set via `X-Tenant-ID` header on every request.

### Vertical Plugin System

The platform is industry-agnostic via YAML-based vertical definitions (`pkg/vertical/`). Services are split into:
- **Universal** (unchanged per industry): general-ledger, iam, notifications, document
- **Vertical-aware** (load tenant config via gRPC interceptor): project-mgmt, budget-planning, expense-tracking, inventory-mgmt, reporting-analytics

## Key Code Locations

- `pkg/vertical/` — Core vertical config types, Redis-cached loader, gRPC interceptors, YAML validator
- `pkg/tenant/` — Tenant context helpers (shared across packages)

## Development Commands

```bash
# Build & run
make run-all                    # All services via tmuxinator
make migrate-all                # Run DB migrations
make seed                       # Load fixture data

# Testing
go test ./... -short            # Unit tests only
go test ./... -tags=integration # Integration tests (requires Docker)
go test -race ./...             # With race detector
go test -coverprofile=coverage.out ./...  # Coverage

# Linting & security
golangci-lint run ./...         # Linting
buf lint                        # Protobuf validation
buf breaking proto --against '.git#branch=main,subdir=proto'  # Breaking change detection
govulncheck ./...               # Vulnerability scanning
gitleaks protect --staged       # Secret detection

# Code generation
buf generate                    # Protobuf/gRPC codegen
sqlc generate                   # SQL query codegen
```

## Conventions

- **Branching:** Trunk-based — `feat/`, `fix/`, `chore/`, `docs/`, `hotfix/` prefixes with ticket IDs
- **Commits:** Conventional Commits — `<type>(<scope>): <summary>`. Scopes: `iam`, `budget`, `expense`, `ledger`, `inventory`, `reporting`, `notifications`, `document`, `project`, `billing`, `proto`, `infra`, `api`
- **Merge strategy:** Squash & merge for features
- **PR reviews:** 2 approvals required; senior engineer required for `iam`/`general-ledger`/security changes
- **Monetary values:** Always `NUMERIC(14,2)` / `decimal.Decimal`, never `float64`
- **SQL:** All queries parameterized via sqlc or pgx named params, no string interpolation
- **Logging:** Structured via `slog`, no PII or secrets
- **Coverage thresholds:** iam/general-ledger ≥ 85%, budget/expense ≥ 80%, others ≥ 75%
