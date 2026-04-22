---
description: Scaffold a new Thittam Go service with the 6-file layout
---

Scaffold a new service named **$ARGUMENTS** under `services/$ARGUMENTS/`.

## Before scaffolding

1. Verify the name does not already exist (`ls services/`).
2. Confirm with me:
   - Is this service **universal** (unchanged per industry) or **vertical-aware** (loads tenant config via gRPC interceptor)?
   - Port assignment (next free after 8088).
   - Whether it will expose gRPC, REST via grpc-gateway, or both.

## Files to create

Mirror the existing `services/budget/` layout exactly:

| File | Content |
|---|---|
| `models.go` | Domain structs with `json` tags. Monetary fields `decimal.Decimal` (Rule #1). IDs `uuid.UUID`. Timestamps `time.Time`. |
| `errors.go` | Sentinel errors: `var ErrNotFound = errors.New("$ARGUMENTS: not found")`, etc. |
| `repository.go` | `Repository` interface — pure data access contract; no business logic. |
| `service.go` | Business logic. Depends on `Repository` interface. For vertical-aware services, read config via `vertical.FromContext(ctx)`. |
| `service_test.go` | Table-driven tests with `t.Parallel()`, hand-written mock `Repository`, deterministic UUIDs. |
| `handler.go` | gRPC handler wrapping `Service`. Validate input at this boundary only (Rule #11). |
| `handler_test.go` | gRPC handler tests using a mock service. |

If SQL queries are needed, add a `db/` subdirectory with sqlc query files — mirror `services/budget/db/`.

## Cross-cutting

- Register in `cmd/<service>/main.go` with the standard observability stack (`/healthz`, `/readyz`, `/metrics`).
- Add to the service table in root `CLAUDE.md` and to the service table in `thittam_docs/docs/developers/services/`.
- Add proto under `proto/thittam/$ARGUMENTS/v1/` — then suggest running `/new-proto $ARGUMENTS` next.

## Output

After writing the files, show:
1. A tree of what was created
2. The next recommended command (likely `/new-proto $ARGUMENTS` or a DB migration)
3. Any TODO markers I need to fill in (e.g., port number if you weren't told)

Do not commit — let me review the diff first.
