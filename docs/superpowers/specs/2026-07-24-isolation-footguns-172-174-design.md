# Close the #159 Audit Footguns (#172, #173, #174) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issues:** #172 (dead unscoped sqlc queries), #173 (billing dunning has no tenant scoping), #174 (document InitiateUpload skips folder-ownership)
**Branch:** `fix/isolation-footguns-172-174` off `main` (`108649a`)
**Migration:** none

## Goal

Close the three latent findings the #159 tenant-isolation audit deferred. None is a live defect — all three are unreachable today — but each becomes a real cross-tenant leak the moment someone wires it up, and each currently reads as safe.

## Context

Slice H (#175) fixed every *live* caller-discipline site and filed the rest. #160 then resolved billing's half of #172 by deleting its never-called sqlc layer outright. What remains:

| issue | what | why it isn't live |
|---|---|---|
| **#172** | 6 dead sqlc queries: document's `ListFolders`, `ListDocuments`, `UpdateDocumentSize`, `IncrementDocumentVersion`; notifications' `ListPendingNotifications`, `ListNotificationLog`. Plus notifications' `IncrementRetryCount`. | Zero callers — the live paths are hand-written raw SQL in each service's `postgres.go`. `IncrementRetryCount` is on the `Repository` interface with an implementation and three doubles, but no production caller: there is no retry worker. |
| **#173** | billing `Service.RecordDunningAttempt` / `ListDunningAttempts` take no `tenantID` and apply no tenant predicate. | No RPC exposes them. |
| **#174** | `Service.InitiateUpload` sets `doc.FolderID = req.FolderID` without verifying the folder belongs to the caller's tenant. | Reads still hard-filter `documents.tenant_id`, so no data leaks — but the FK can point at another tenant's folder. |

### Two different kinds of dead code, handled differently

#160 deleted billing's generated layer because it was **pure redundancy** — a duplicate of hand-written code with a live replacement. The six queries here are the same: delete them.

`IncrementRetryCount` and the dunning pair are **not** that. They are unbuilt features (notification retry, payment retry) with real design intent and, for dunning, passing tests. There is no replacement to fall back on. Deleting them discards the design; leaving them unscoped is the footgun. So they get **scoped**, not deleted — whoever wires a retry worker later inherits correct isolation instead of a latent leak.

## Design

Three tasks, one per service. Each is self-contained.

### Task 1 — document: delete 4 dead queries, fix `InitiateUpload` (#172 + #174)

**Dead queries** — remove from `services/document/db/queries.sql` and regenerate:
- `UpdateDocumentSize`, `IncrementDocumentVersion` — unscoped `UPDATE documents WHERE id = $1`, superseded by `Postgres.UpdateDocument`'s `WHERE id=$1 AND tenant_id=$2`.
- `ListFolders`, `ListDocuments` — superseded by raw-SQL equivalents, and carrying a **dead nullable filter**: `sqlc.yaml` overrides `db_type: uuid → uuid.UUID` (non-nullable), so the generated `($2::uuid IS NULL OR …)` branch can never fire. Deleting them removes that trap rather than documenting it.

Document stays in `sqlc.yaml` — its other generated queries are live.

**`InitiateUpload` (#174)** — mirror `MoveDocument` (`service.go:279`), which already does this correctly:

```go
if _, err := s.repo.GetFolder(ctx, tenantID, folderID); err != nil {
    return nil, fmt.Errorf("document: move — %w", ErrFolderNotFound)
}
```

`req.FolderID` is optional (`*uuid.UUID`), so the check applies only when it is set. A folder in another tenant yields `ErrFolderNotFound` — the same answer as a genuinely missing folder, so nothing confirms the other tenant's folder exists.

### Task 2 — notifications: delete 2 dead queries, scope `IncrementRetryCount` (#172)

**Dead queries** — remove `ListPendingNotifications` (`WHERE status='pending'`, **no tenant predicate at all**) and `ListNotificationLog` (unused; the live path is the hand-written `Postgres.ListNotifications`, which slice G scoped to tenant + recipient).

**`IncrementRetryCount`** — `notification_log` **has** `tenant_id`, so this takes a predicate rather than a parent read. Thread `tenantID` as a required parameter and add the predicate. Critically, the current implementation is a raw `Exec` that discards the result:

```go
_, err := p.db.Exec(ctx, "UPDATE notification_log SET retry_count = retry_count + 1 WHERE id = $1", id)
```

Adding a tenant predicate to a statement whose row count is ignored makes a cross-tenant call **silently update zero rows and return nil** — the exact trap slice I hit with `UpdatePasswordHash`'s `:exec`. So the fix must also check `RowsAffected() == 0` and return `ErrNotificationNotFound`.

Interface, implementation, and all three doubles (`mockRepo`, `mockTemplateRepo`, the e2e `notifRepo`) change together; whole-tree `go vet ./...` is the gate.

### Task 3 — billing: scope the dunning pair (#173)

`dunning_attempts` has **no `tenant_id`** — only `invoice_id` FK → `invoices`. So isolation is inherited from the parent, exactly like `journal_lines`, and gets the same treatment slice H used for `ListJournalLines`:

- **`ListDunningAttempts`** — thread `tenantID` and **JOIN the parent**, so the predicate lives in the same statement as the read:

  ```sql
  SELECT da.<cols>
    FROM dunning_attempts da
    JOIN invoices i ON i.id = da.invoice_id
   WHERE da.invoice_id = $1 AND i.tenant_id = $2
  ```

- **`RecordDunningAttempt`** — an INSERT cannot JOIN, so it takes `tenantID` and does a scoped `GetInvoice(ctx, tenantID, d.InvoiceID)` first, returning on error before inserting.

Signature changes propagate through `Repository`, `Service`, `Postgres`, `mockRepo`, and the e2e `billingRepo`; the two existing tests are updated to pass a tenant.

## Testing

- **Integration** (`//go:build integration`, real-Postgres CI job) per service: a cross-tenant id returns NotFound / yields no rows, **and the victim row is unchanged** — a status-code-only assertion passes against unfixed code because mock defaults return usable rows.
- `IncrementRetryCount` specifically needs the "zero rows ≠ success" case: cross-tenant call must return `ErrNotificationNotFound`, not nil. That test fails against the pre-fix `Exec`.
- `InitiateUpload` gets a case where `folder_id` names another tenant's folder → `ErrFolderNotFound`, and no document row is created.
- Unit tests updated for the new signatures; existing dunning tests keep their coverage with a tenant threaded through.
- **`sqlc generate` must leave the tree clean** after the query deletions — CI's Codegen Freshness job (#160) enforces it.
- Whole-tree `go vet ./...` for the widened interfaces.

Coverage floors: budget/expense ≥ 80%, others ≥ 75% (document, notifications, billing are all in the 75% tier).

## Non-goals

- **No migration.** No `tenant_id` column is added to `dunning_attempts` — inheriting from `invoices` is correct for a child table, and matches how `journal_lines` and `document_versions` are handled.
- **No removal of document or notifications from `sqlc.yaml`** — unlike billing (#160), their remaining generated queries are live.
- **No wiring of the retry features.** Dunning and notification retry stay unbuilt; this only makes them safe to build.
- No change to the live paths slice G/H already scoped.

## Review weight

Touches billing, document, notifications — none is an iam/ledger senior-required service, so normal 2 approvals apply. The diff is small but changes three `Repository` interfaces, so the whole-branch review still runs on the most capable model.
