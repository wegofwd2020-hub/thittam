# Thittam — Multi-Industry Production SaaS Platform

> **Thittam** (திட்டம்) means *"plan"* in Tamil — a fitting name for a platform that brings
> financial discipline and operational clarity to production management.

## Overview

Thittam is a cloud-native, multi-tenant SaaS platform for production management. It
consolidates project management, budgeting, expense tracking, inventory, general ledger
accounting, and document management into a single cohesive system.

The platform is **industry-agnostic** via a Vertical Plugin System that configures entity
labels, phase types, budget categories, and workflows per industry.

| Vertical | Status |
|---|---|
| Movie & Film Production | GA |
| Software Development Services | GA |
| Construction & Civil Engineering | GA |
| Events & Experiential Management | GA |

> **Full documentation** lives in a separate repository:
> [`github.com/wegofwd2020-hub/thittam_docs`](https://github.com/wegofwd2020-hub/thittam_docs)

## Architecture

9 Go microservices communicating via gRPC (sync) and NATS JetStream (async), fronted by
Kong API Gateway for REST/JSON.

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

**Data isolation:** Tenant-per-schema PostgreSQL — each tenant gets schema `tenant_<uuid>`.

## Code Structure

```
thittam/
├── pkg/
│   ├── vertical/         ← Core vertical config types, loader, gRPC interceptors, validator
│   └── tenant/           ← Tenant context helpers (shared across packages)
├── cmd/                  ← Service entrypoints and CLI (planned)
├── services/             ← Per-service business logic (planned)
├── proto/                ← Protocol Buffer definitions (planned)
└── infra/                ← Docker Compose, Helm charts (planned)
```

## Quick Start

```bash
# Prerequisites: Go 1.22+, Docker, buf, sqlc, golang-migrate

# Clone and build
git clone https://github.com/wegofwd2020-hub/thittam.git
cd thittam
go build ./...

# Run tests
go test ./... -race

# Full local setup (requires infrastructure)
# See: https://github.com/wegofwd2020-hub/thittam_docs/blob/main/docs/operations/local-dev-setup.md
```

## Tech Stack

| Layer | Choice |
|---|---|
| Language | Go 1.22+ |
| Service communication | gRPC + protobuf |
| REST API | HTTP/JSON via Kong Gateway |
| Database | PostgreSQL 16 (tenant-per-schema) |
| Message broker | NATS JetStream |
| Cache | Redis 7 |
| Object storage | MinIO / S3-compatible |
| Container runtime | Docker + Kubernetes |
| Service mesh | Istio |
| Observability | OpenTelemetry → Grafana stack |
| CI/CD | GitHub Actions + ArgoCD |

## Company

**WeGoFwd2020** — wegofwd2020@gmail.com

## Licence

Proprietary — © WeGoFwd2020. All rights reserved.
