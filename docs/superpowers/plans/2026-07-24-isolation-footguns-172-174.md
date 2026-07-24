# Close the #159 Audit Footguns (#172, #173, #174) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete six dead unscoped sqlc queries, and tenant-scope the three latent-but-intended methods (`IncrementRetryCount`, `ListDunningAttempts`, `RecordDunningAttempt`) plus `InitiateUpload`'s missing folder-ownership check.

**Architecture:** Three independent tasks, one per service (document, notifications, billing). No task depends on another. No migration.

**Tech Stack:** Go 1.25, pgx, sqlc v1.26.0, `//go:build integration` tests against real Postgres.

## Global Constraints

- **Delete vs scope — do not confuse them.** The six sqlc queries are *pure redundancy* (live raw-SQL replacements exist) → delete. `IncrementRetryCount` and the dunning pair are *unbuilt features* with no replacement → scope, keep.
- **`sqlc generate` after every `queries.sql` edit.** Never hand-edit `queries.sql.go`/`models.go`. CI's **Codegen Freshness (sqlc)** job (#160) fails on any drift. Verify on a COMMITTED tree: `git status` clean → `sqlc generate` → `git status` must still be clean. (Running `git add -A && git diff --cached` mid-work compares staged-vs-HEAD and always reports your own edits — that is not a freshness check.)
- **document and notifications STAY in `sqlc.yaml`** — their other generated queries are live. Only billing was removed (#160).
- **Predicate vs parent-JOIN is decided by the schema:** `notification_log` HAS `tenant_id` → predicate. `dunning_attempts` has NO `tenant_id` (only `invoice_id` FK → `invoices`) → JOIN the parent for reads, scoped `GetInvoice` for the insert. Same shape as `journal_lines` in slice H.
- **A tenant predicate on a statement whose row count is ignored is a silent no-op.** `IncrementRetryCount` is a raw `Exec` discarding its tag; adding a predicate without a `RowsAffected()==0` check makes a cross-tenant call return `nil` while changing nothing (the `UpdatePasswordHash` trap from slice I).
- **Cross-tenant → the same not-found as a missing row.** `ErrNotificationNotFound`, `ErrFolderNotFound`, `ErrInvoiceNotFound`. Never a distinct permission error (that leaks existence).
- **Whole-tree `go vet ./...`** is the completion gate wherever an interface changes — it catches the `e2e/critical_path` doubles.
- **DB safety:** never `docker compose … -v`/`down`/`up` on `infra/local/`. Use `pkg/testdb` (SKIPs without `THITTAM_TEST_DSN`). Integration tests skipping locally is expected.
- Coverage floor for all three services: ≥ 75%.

---

## Task 1: document — delete 4 dead queries + fix InitiateUpload (#172, #174)

**Files:**
- Modify: `services/document/db/queries.sql` (delete 4 queries), regenerate `queries.sql.go`
- Modify: `services/document/service.go:106-149` (`InitiateUpload`)
- Create: `tests/integration/document_tenant_isolation_test.go`

**Interfaces:** no signature changes. `InitiateUpload` already has the tenant via `req.TenantID`.

- [ ] **Step 1: Write the failing integration test**

Create `tests/integration/document_tenant_isolation_test.go` (`//go:build integration`, package `integration`). Follow the harness in `tests/integration/notifications_authz_test.go` (uses `pkg/testdb`, seeds tenants directly via the pool). Seed tenant A and tenant B, and a folder owned by **B**. Then, as tenant A, call `Service.InitiateUpload` with `FolderID` pointing at B's folder and assert:
- the error is `document.ErrFolderNotFound`
- **no `documents` row was created for tenant A** (query the pool directly)

Seeding note: a `tenants` INSERT needs `country_code` and `primary_currency_code` (NOT NULL since migration 014) and a **unique** name (`tenants_name_ci_unique`) — mirror `notifications_authz_test.go`'s seed exactly.

- [ ] **Step 2: Verify it fails (compile + intent)**

Run: `go vet -tags=integration ./tests/integration/`
Expected: compiles. It SKIPs locally without `THITTAM_TEST_DSN`; CI's real-Postgres job is the authoritative gate. The pre-fix code would create the document with a foreign `folder_id`.

- [ ] **Step 3: Add the folder-ownership check**

`services/document/service.go` — in `InitiateUpload`, before building the `Document` literal (currently line 128), mirror `MoveDocument:279-281`. **`req.FolderID` is `*uuid.UUID` and optional, so guard on nil:**

```go
	// A folder from another tenant must not become this document's parent.
	// MoveDocument already enforces this; InitiateUpload did not (#174).
	if req.FolderID != nil {
		if _, err := s.repo.GetFolder(ctx, req.TenantID, *req.FolderID); err != nil {
			return nil, fmt.Errorf("document: initiate upload — %w", ErrFolderNotFound)
		}
	}
```

Place it **before** `s.store.PresignedPutURL` (line 123) so a rejected upload does not mint a presigned URL it will never use. The `Document` literal at :132 keeps `FolderID: req.FolderID` unchanged.

- [ ] **Step 4: Delete the 4 dead queries**

`services/document/db/queries.sql` — delete these four entries **in full** (comment line + SQL):
- `ListFolders` (:11-17)
- `ListDocuments` (:27-34)
- `UpdateDocumentSize` (:36-37)
- `IncrementDocumentVersion` (:39-40)

**Do NOT touch** the live ones: `CreateFolder`, `GetFolder`, `CreateDocument`, `GetDocument`, `SoftDeleteDocument`, `CreateDocumentVersion`, `GetDocumentVersion`, `ListDocumentVersions` (all called via `p.q.X` in `postgres.go`).

Confirmed zero callers: nothing invokes `q.ListFolders(`, `q.ListDocuments(`, `q.UpdateDocumentSize(`, or `q.IncrementDocumentVersion(` anywhere. `Postgres.ListFolders`/`ListDocuments` are separate hand-written raw-SQL methods and are unaffected.

- [ ] **Step 5: Regenerate**

Run: `sqlc generate`
Expected: `services/document/db/queries.sql.go` loses exactly those four methods and their `*Params` types. `models.go` should not change (no schema change).

- [ ] **Step 6: Run the gate**

Run: `go build ./... && go vet ./... && go test ./services/document/... && go vet -tags=integration ./tests/integration/`
Expected: all clean. If any document unit test asserted on the deleted generated methods, that test was testing dead code — remove it and say so in the report.

- [ ] **Step 7: Commit**

```bash
git add services/document tests/integration/document_tenant_isolation_test.go
git commit -m "fix(document): verify folder ownership on InitiateUpload; drop dead sqlc queries (#174, #172)

InitiateUpload assigned req.FolderID without checking the folder belongs to
the caller's tenant, so a document's folder_id FK could point into another
tenant. MoveDocument already did this check; now both do.

Also deletes four sqlc queries with zero callers, superseded by hand-written
raw SQL in postgres.go. Two of them (ListFolders, ListDocuments) carried a
dead nullable filter: the repo overrides db_type uuid to a non-nullable
uuid.UUID, so their (\$2::uuid IS NULL OR ...) branch could never fire."
```

---

## Task 2: notifications — delete 2 dead queries + scope IncrementRetryCount (#172)

**Files:**
- Modify: `services/notifications/db/queries.sql` (delete 2 queries), regenerate
- Modify: `services/notifications/repository.go:24`, `services/notifications/db/postgres.go:233-239`
- Modify doubles: `services/notifications/service_test.go:100-104`, `cmd/notifications/dispatcher_test.go:61-63`, `e2e/critical_path/notifications_test.go:147`
- Modify: `tests/integration/notifications_authz_test.go` (extend)

**Interfaces:**
- Produces: `Repository.IncrementRetryCount(ctx context.Context, tenantID, id uuid.UUID) error` — `tenantID` added as the 2nd param. All three doubles change with it.

- [ ] **Step 1: Write the failing integration test**

Extend `tests/integration/notifications_authz_test.go` (do NOT overwrite — it holds slice G's self-scope and slice H-era tests). Add a case: seed a `notification_log` row for tenant A, then call `repo.IncrementRetryCount(ctx, tenantB, rowID)` and assert:
- it returns `notifications.ErrNotificationNotFound` (**not** `nil`)
- A's `retry_count` is **unchanged** (read it back via the pool)

This is the test that fails against the pre-fix `Exec`, which returns `nil` and increments nothing.

- [ ] **Step 2: Verify it fails to compile**

Run: `go vet -tags=integration ./tests/integration/`
Expected: compile error — `IncrementRetryCount` currently takes 2 args, not 3.

- [ ] **Step 3: Widen the interface**

`services/notifications/repository.go:24`:

```go
	IncrementRetryCount(ctx context.Context, tenantID, id uuid.UUID) error
```

- [ ] **Step 4: Scope the implementation and check RowsAffected**

`services/notifications/db/postgres.go:233-239` — replace in full:

```go
func (p *Postgres) IncrementRetryCount(ctx context.Context, tenantID, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx,
		"UPDATE notification_log SET retry_count = retry_count + 1 WHERE id = $1 AND tenant_id = $2",
		id, tenantID)
	if err != nil {
		return fmt.Errorf("notifications: increment retry count: %w", err)
	}
	// Without this the tenant predicate would be a silent no-op: a cross-tenant
	// id matches zero rows and Exec still returns nil.
	if tag.RowsAffected() == 0 {
		return notifications.ErrNotificationNotFound
	}
	return nil
}
```

(`notification_log` has `tenant_id NOT NULL`, so this is a predicate, not a parent read. `ErrNotificationNotFound` is `services/notifications/errors.go:10`, already used by `GetNotification` in this file.)

- [ ] **Step 5: Update all three doubles**

`services/notifications/service_test.go:100-104` — `mockRepo` (widen the `incrementRetryCountFn` field too):
```go
func (m *mockRepo) IncrementRetryCount(ctx context.Context, tenantID, id uuid.UUID) error {
	if m.incrementRetryCountFn != nil {
		return m.incrementRetryCountFn(ctx, tenantID, id)
	}
	return nil
}
```
`cmd/notifications/dispatcher_test.go:61-63` — `mockTemplateRepo`:
```go
func (r *mockTemplateRepo) IncrementRetryCount(_ context.Context, _, _ uuid.UUID) error {
	return nil
}
```
`e2e/critical_path/notifications_test.go:147` — `notifRepo`:
```go
func (r *notifRepo) IncrementRetryCount(_ context.Context, _, id uuid.UUID) error { return nil }
```

- [ ] **Step 6: Delete the 2 dead queries**

`services/notifications/db/queries.sql` — delete in full:
- `ListPendingNotifications` (:41-45) — `WHERE status='pending'`, **no tenant predicate at all**
- `ListNotificationLog` (:47-51) — superseded by the hand-written `Postgres.ListNotifications`, which slice G scoped to tenant **and** recipient

**Do NOT touch** the live ones: `UpsertTemplate`, `GetTemplate`, `ListTemplates`, `CreateNotificationLog`, `UpdateNotificationLogSent`, `UpdateNotificationLogFailed`.

- [ ] **Step 7: Regenerate and run the gate**

Run: `sqlc generate && go build ./... && go vet ./... && go test ./services/notifications/... ./cmd/notifications/... ./e2e/... && go vet -tags=integration ./tests/integration/`
Expected: all clean. `go vet ./...` is what proves all three doubles were updated.

- [ ] **Step 8: Commit**

```bash
git add services/notifications cmd/notifications e2e/critical_path/notifications_test.go tests/integration/notifications_authz_test.go
git commit -m "fix(notifications): tenant-scope IncrementRetryCount; drop dead sqlc queries (#172)

IncrementRetryCount had no tenant predicate and no production caller — a
retry worker wired to it would have incremented any tenant's row by id. It
now takes tenantID and checks RowsAffected: adding a predicate to an Exec
whose tag is discarded would otherwise return nil while changing nothing.

Also deletes ListPendingNotifications (no tenant predicate at all) and
ListNotificationLog (superseded by the recipient-scoped hand-written path),
both with zero callers."
```

---

## Task 3: billing — scope the dunning pair (#173)

`dunning_attempts` has **no `tenant_id`** (`migrations/billing/001:84-95`) — only `invoice_id` FK → `invoices`. Isolation is inherited from the parent.

**Files:**
- Modify: `services/billing/repository.go:42-43`, `services/billing/service.go:373-387`, `services/billing/db/postgres.go:545-586`
- Modify doubles: `services/billing/service_test.go:149-160`, `e2e/critical_path/billing_test.go:200-207`
- Modify tests: `services/billing/service_test.go:801-814`, `:929-944`, `:946-953`
- Create: `services/billing/db/dunning_integration_test.go`

**Interfaces:**
- Produces: `Repository.CreateDunningAttempt(ctx, tenantID uuid.UUID, d *DunningAttempt) error` and `Repository.ListDunningAttempts(ctx, tenantID, invoiceID uuid.UUID) ([]DunningAttempt, error)`. Service methods gain `tenantID` as their first id param.
- Consumes: `Postgres.GetInvoice(ctx, tenantID, id)` (`postgres.go:343`) — already tenant-scoped, returns `billing.ErrInvoiceNotFound`.

- [ ] **Step 1: Write the failing integration test**

Create `services/billing/db/dunning_integration_test.go`. **Follow the existing per-service convention** — `package db_test`, `//go:build integration`, `pkg/testdb.Open(t)` + `billingdb.NewPostgres(pool)`, mirroring `services/billing/db/subscription_integration_test.go`. (Billing has no file in `tests/integration/`; do not create one.)

Seed tenant A with an invoice and a dunning attempt. Assert:
- `ListDunningAttempts(ctx, tenantB, invoiceA)` returns **zero rows** (not A's attempts)
- `ListDunningAttempts(ctx, tenantA, invoiceA)` still returns them (regression guard on the JOIN)
- `CreateDunningAttempt(ctx, tenantB, {InvoiceID: invoiceA})` returns `billing.ErrInvoiceNotFound` and **inserts nothing** (count rows via the pool)

- [ ] **Step 2: Verify it fails to compile**

Run: `go vet -tags=integration ./services/billing/db/`
Expected: compile error — both methods currently take no `tenantID`.

- [ ] **Step 3: Widen the Repository interface**

`services/billing/repository.go:42-43`:

```go
	CreateDunningAttempt(ctx context.Context, tenantID uuid.UUID, d *DunningAttempt) error
	ListDunningAttempts(ctx context.Context, tenantID, invoiceID uuid.UUID) ([]DunningAttempt, error)
```

- [ ] **Step 4: Rewrite the read as a parent JOIN**

`services/billing/db/postgres.go:560-586` — the query becomes a JOIN. **Preserve the exact 6-column output list and its order** (`id, invoice_id, attempt_number, outcome, attempted_at, next_retry_at`) so the existing `rows.Scan` block is untouched — note DB column `outcome` scans into Go field `d.Result`:

```go
func (p *Postgres) ListDunningAttempts(ctx context.Context, tenantID, invoiceID uuid.UUID) ([]billing.DunningAttempt, error) {
	rows, err := p.db.Query(ctx, `
		SELECT da.id, da.invoice_id, da.attempt_number, da.outcome, da.attempted_at, da.next_retry_at
		FROM dunning_attempts da
		JOIN invoices i ON i.id = da.invoice_id
		WHERE da.invoice_id = $1 AND i.tenant_id = $2
		ORDER BY da.attempt_number ASC`, invoiceID, tenantID)
	// ... existing rows.Close / Scan loop / return unchanged ...
}
```

(`dunning_attempts` has no `tenant_id`, so the tenant lives on the parent — the same fix `ListJournalLines` got in slice H. A cross-tenant `invoiceID` yields zero rows, which is the correct answer for a list.)

- [ ] **Step 5: Scope the insert via a parent read**

`services/billing/db/postgres.go:545-558` — an INSERT cannot JOIN, so verify the parent first:

```go
func (p *Postgres) CreateDunningAttempt(ctx context.Context, tenantID uuid.UUID, d *billing.DunningAttempt) error {
	// dunning_attempts carries no tenant_id; the invoice is the tenant boundary.
	// A cross-tenant invoice_id must not be insertable.
	if _, err := p.GetInvoice(ctx, tenantID, d.InvoiceID); err != nil {
		return err // billing.ErrInvoiceNotFound for a foreign or missing invoice
	}
	_, err := p.db.Exec(ctx, `
		INSERT INTO dunning_attempts
		  (id, invoice_id, attempt_number, outcome, attempted_at, next_retry_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO NOTHING`,
		d.ID, d.InvoiceID, d.AttemptNumber, d.Result,
		d.AttemptedAt, d.NextRetryAt,
	)
	if err != nil {
		return fmt.Errorf("billing: create dunning attempt: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Thread tenantID through the Service**

`services/billing/service.go:373-387`:

```go
// RecordDunningAttempt appends a retry record for an overdue invoice.
func (s *Service) RecordDunningAttempt(ctx context.Context, tenantID uuid.UUID, d *DunningAttempt) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	d.AttemptedAt = time.Now().UTC()
	return s.repo.CreateDunningAttempt(ctx, tenantID, d)
}

// ListDunningAttempts returns all retry records for an invoice.
func (s *Service) ListDunningAttempts(ctx context.Context, tenantID, invoiceID uuid.UUID) ([]DunningAttempt, error) {
	return s.repo.ListDunningAttempts(ctx, tenantID, invoiceID)
}
```

- [ ] **Step 7: Update both doubles and the three tests**

`services/billing/service_test.go:149-160` — `mockRepo`, widening both fn fields and methods to take `tenantID`.
`e2e/critical_path/billing_test.go:200-207` — `billingRepo`: add the `tenantID` param to both (the in-memory double may ignore it).
Then fix the three call sites: `TestRecordDunningAttempt_GeneratesID` (:801-814), `TestListDunningAttempts_ReturnsAttempts` (:929-944), `TestListDunningAttempts_Empty` (:946-953) — pass a tenant id.

- [ ] **Step 8: Run the gate**

Run: `go build ./... && go vet ./... && go test ./services/billing/... ./e2e/... && go vet -tags=integration ./services/billing/db/`
Expected: all clean. No `sqlc generate` needed — billing has no generated layer since #160 (its `Postgres` struct has only a `db` pool).

- [ ] **Step 9: Commit**

```bash
git add services/billing e2e/critical_path/billing_test.go
git commit -m "fix(billing): tenant-scope the dunning attempt methods (#173)

ListDunningAttempts and RecordDunningAttempt took no tenantID and applied no
tenant predicate. dunning_attempts has no tenant_id column, so isolation is
inherited from the parent invoice: the read now JOINs invoices and filters
i.tenant_id, and the insert verifies the invoice via a scoped GetInvoice
first. Same shape as ListJournalLines in #175.

Not live today — no RPC reaches them — but a dunning worker wired to the old
signatures would have leaked another tenant's payment-retry history for any
guessed invoice UUID."
```

---

## Self-Review

- **Spec coverage:** 6 dead queries deleted (T1 S4, T2 S6) ✓; `IncrementRetryCount` scoped with the RowsAffected guard (T2 S4) ✓; dunning JOIN + scoped insert (T3 S4/S5) ✓; `InitiateUpload` folder check (T1 S3) ✓; document/notifications stay in `sqlc.yaml` (Global Constraints) ✓; no migration ✓; integration tests per service (T1 S1, T2 S1, T3 S1) ✓.
- **Placeholder scan:** every deletion names exact line ranges from the grounding scan; every code change is written out. The one "unchanged" elision (T3 S4's scan loop) is explicit about what must not move and why.
- **Type consistency:** `tenantID` is the first id parameter in every widened signature. `req.FolderID` is `*uuid.UUID` so the check is nil-guarded and dereferenced — using it as a value would not compile. Billing's `Result` field ↔ DB `outcome` column mapping is preserved by keeping the SELECT list identical.
- **Schema drives the fix:** predicate for `notification_log` (has `tenant_id`), parent-JOIN/parent-read for `dunning_attempts` (does not). Stated per task so an implementer cannot pick the wrong one.
- **Ordering note:** T1 Step 3 places the folder check *before* the presigned-URL call, so a rejected upload never mints a URL — a detail the issue did not specify but that falls out of reading the function.
