# Tenant-Isolation Guard-by-Type (#139 slice H / #159) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert 7 tenant-isolation invariants held by caller discipline into compile-time guarantees by threading `tenantID` as a required parameter and enforcing it in SQL. Zero live defects exist; this hardens 7 sites each one refactor from becoming one.

**Architecture:** Three tasks by service (ledger / budget / iam). Each fix threads `tenantID` through `Repository` → `Service` → SQL as a **required** parameter (guard-by-type). Signature change + all call sites + all mock/double implementers land together per service; whole-tree `go vet ./...` is the completion gate. No migration.

**Tech Stack:** Go 1.25, pgx, sqlc (`queries.sql` + `sqlc generate`), `pkg/interceptor` (`CallerFromContext`, `TenantFromRequest`), `//go:build integration` tests against real Postgres.

## Global Constraints

- **No migration.** Every predicate uses an existing `tenant_id NOT NULL` column (`budgets`, `journal_entries`, `users`, `tenant_purge_requests`) or the parent's via JOIN (`journal_lines` has no `tenant_id`).
- **Not-found signal is per-query-kind, do not conflate:**
  - sqlc `:one` (`PostJournalEntry`, `VoidJournalEntry`, `UpdateBudgetStatus`, `Approve/CancelTenantPurgeRequest`, `GetUser`): 0 rows → `pgx.ErrNoRows` from Scan → existing sentinel. Keep the existing `errors.Is(err, pgx.ErrNoRows)` translation.
  - sqlc `:exec` (`UpdateUserPasswordHash`): 0 rows updates **silently, no error**. This MUST change to `:execrows` and check `RowsAffected()==0 → iam.ErrUserNotFound`, or the tenant guard is a silent no-op.
- **Cross-tenant id → the same NotFound as a missing row.** Never a distinct `PermissionDenied` (that leaks existence). The sentinel per method is named in its task.
- **Guard-by-type:** `tenantID uuid.UUID` is a required positional param on every changed signature. Signature change + ALL call sites + ALL mock/double implementers (`services/*` `mockRepo`, `tests/integration/vertical/mocks_test.go`, `e2e/critical_path/helpers_test.go`, `pkg/auth/local_test.go`) land in ONE commit per service. `go vet ./...` (whole tree) is the only reliable detector of the hidden e2e/integration doubles.
- **`GetUserByID` is on TWO interfaces** — `iam.Repository` AND `pkg/auth.UserStore` (`*db.Postgres` is passed as both at `cmd/iam/main.go:141,147`). Both declarations change to `(ctx, tenantID, userID)`. `auth.UserStore.GetUserByID` has no live caller in `pkg/auth` (only `GetUserByEmail` is used), so this is contained; its double `pkg/auth/local_test.go:28` `mockUserStore` changes in lockstep.
- **Testing:** cross-tenant regression tests assert the `tenantID` the repository actually received (or that a cross-tenant id → NotFound). A status-code-only assertion passes against vulnerable code because mock defaults return usable rows. The iam `mockRepo` unset defaults for `GetUser`/`GetRoleByID` echo the caller's own `tenantID` — a new `GetUserByID` default MUST NOT (`return &auth.UserRecord{ID: userID, PasswordHash: "hashed"}` with `TenantID` left `uuid.Nil`). A predicate-revert "teeth check" is worthless (handler/service tests are mock-driven); CI's real-Postgres job is the only proof of the SQL predicate.
- **DB safety:** never `docker compose … -v`/`down`/`up` on `infra/local/`. Use `pkg/testdb` (SKIPs without `THITTAM_TEST_DSN`) or a disposable uniquely-named throwaway container. CI's real-Postgres job is authoritative. Integration tests SKIP locally — expected.
- **Coverage floors:** iam/general-ledger ≥ 85%, budget ≥ 80%.
- Touches iam + general-ledger → senior review required.

---

## Task 1: ledger — ListJournalLines (JOIN) + UpdateJournalStatus (drop self-lookup)

**Files:**
- Modify: `services/ledger/db/queries.sql` (ListJournalLines JOIN)
- Regen: `services/ledger/db/queries.sql.go` (`sqlc generate`)
- Modify: `services/ledger/db/postgres.go` (`GetJournalEntry` call site :228; `UpdateJournalStatus` :265-296; delete `resolveTenantForJournal` :298-303)
- Modify: `services/ledger/repository.go:31` (UpdateJournalStatus signature)
- Modify: `services/ledger/service.go` (:220, :300 call sites)
- Modify: `services/ledger/service_test.go` (`mockRepo` :42 field, :127 method)
- Modify: `e2e/critical_path/helpers_test.go` (`ledgerRepo` :517 UpdateJournalStatus)
- Test: `tests/integration/` ledger isolation cases

**Interfaces:**
- Produces: `Repository.UpdateJournalStatus(ctx, tenantID, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error` (adds `tenantID` as the 2nd param). `ListJournalLines` stays internal to `Postgres` (not on the Repository interface).

### 1a — ListJournalLines JOIN (sqlc edit; no interface/double change)

- [ ] **Step 1: Edit the query to JOIN the parent for the tenant predicate**

`services/ledger/db/queries.sql` — replace the `ListJournalLines` query (currently `SELECT * FROM journal_lines WHERE journal_id = $1 ORDER BY id ASC`):

```sql
-- name: ListJournalLines :many
SELECT jl.id, jl.journal_id, jl.account_id, jl.debit_amount, jl.credit_amount, jl.currency, jl.description
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.journal_id
WHERE jl.journal_id = $1 AND je.tenant_id = $2
ORDER BY jl.id ASC;
```

(Explicit column list = the sqlc-expanded set `id, journal_id, account_id, debit_amount, credit_amount, currency, description`, qualified with `jl.` so the JOIN doesn't ambiguate. The follows the existing `GetAccountBalance` JOIN pattern in this file.)

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate` (from repo root). Expected: `ListJournalLines(ctx, journalID, tenantID uuid.UUID)` in `queries.sql.go`, return type `[]JournalLine` unchanged.

- [ ] **Step 3: Pass tenantID at the one call site**

`services/ledger/db/postgres.go:228`, inside `GetJournalEntry(ctx, tenantID, id)` — change:

```go
	lineRows, err := p.q.ListJournalLines(ctx, je.ID, tenantID)
```

(`tenantID` is already the function parameter; the header read `p.q.GetJournalEntry(GetJournalEntryParams{ID: id, TenantID: tenantID})` above already proved the entry is in-tenant — the JOIN now makes the child read structurally tenant-safe too.)

### 1b — UpdateJournalStatus: thread tenantID, delete the discarded-error self-lookup

- [ ] **Step 4: Widen the Repository signature**

`services/ledger/repository.go:31`:

```go
	UpdateJournalStatus(ctx context.Context, tenantID, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error
```

- [ ] **Step 5: Rewrite the Postgres impl to use the passed tenantID and delete `resolveTenantForJournal`**

`services/ledger/db/postgres.go` — change the signature and both param structs to use the passed `tenantID`; **delete** `resolveTenantForJournal` (`:298-303`, the `_ = p.db.QueryRow(...)` discarded-error self-lookup):

```go
func (p *Postgres) UpdateJournalStatus(ctx context.Context, tenantID, id uuid.UUID, status string, actorID uuid.UUID, _ time.Time) error {
	switch status {
	case "posted":
		_, err := p.q.PostJournalEntry(ctx, PostJournalEntryParams{
			ID:       id,
			TenantID: tenantID,
			PostedBy: pgtype.UUID{Bytes: actorID, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ledger.ErrJournalNotFound
			}
			return fmt.Errorf("ledger: post journal entry: %w", err)
		}
	case "void":
		_, err := p.q.VoidJournalEntry(ctx, VoidJournalEntryParams{
			ID:       id,
			TenantID: tenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ledger.ErrJournalNotFound
			}
			return fmt.Errorf("ledger: void journal entry: %w", err)
		}
	default:
		return fmt.Errorf("ledger: unsupported journal status %q", status)
	}
	return nil
}
```

(The `PostJournalEntry`/`VoidJournalEntry` sqlc `:one` queries already carry `WHERE id=$1 AND tenant_id=$2`; no `queries.sql` change. A cross-tenant `id` now matches 0 rows → `pgx.ErrNoRows` → `ErrJournalNotFound`, instead of the silent nil-tenant lookup.)

- [ ] **Step 6: Thread tenantID at the two service call sites**

`services/ledger/service.go:220` (`PostJournalEntry`, already holds `tenantID` from `GetJournalEntry(ctx, tenantID, id)` at :194):

```go
	if err := s.repo.UpdateJournalStatus(ctx, tenantID, id, "posted", postedBy, now); err != nil {
```

`services/ledger/service.go:300` (`VoidJournalEntry`, holds `tenantID` from :251):

```go
	if err := s.repo.UpdateJournalStatus(ctx, tenantID, id, "void", voidedBy, now); err != nil {
```

- [ ] **Step 7: Update both doubles**

`services/ledger/service_test.go` — `mockRepo.updateJournalStatusFn` field (:42) and method (:127):

```go
	updateJournalStatusFn func(ctx context.Context, tenantID, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error
```
```go
func (m *mockRepo) UpdateJournalStatus(ctx context.Context, tenantID, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error {
	if m.updateJournalStatusFn != nil {
		return m.updateJournalStatusFn(ctx, tenantID, id, status, actorID, at)
	}
	return nil
}
```

`e2e/critical_path/helpers_test.go:517` — `ledgerRepo.UpdateJournalStatus` gains the `tenantID` param (ignored in the in-memory double, keyed by id):

```go
func (r *ledgerRepo) UpdateJournalStatus(_ context.Context, _ , id uuid.UUID, status string, _ uuid.UUID, _ time.Time) error {
	if je, ok := r.entries[id]; ok {
		je.Status = status
		return nil
	}
	return ledger.ErrJournalNotFound
}
```

- [ ] **Step 8: Update existing unit-test call sites**

Any `service_test.go`/`handler_test.go` test calling `mockRepo.updateJournalStatusFn` or asserting on it must match the new arg list. (Grep `updateJournalStatusFn` and `UpdateJournalStatus(` in the two ledger test files; fix arg counts.)

- [ ] **Step 9: Write the failing integration test**

`tests/integration/` (follow the existing ledger integration harness, `pkg/testdb`, `//go:build integration`). Seed two tenants each with a posted-able draft journal entry. Assert:
- `PostJournalEntry` as tenant A on tenant B's entry id → `ErrJournalNotFound` (no state change to B's entry).
- `GetJournalEntry` as tenant A on B's id → `ErrJournalNotFound` (and never returns B's lines).

- [ ] **Step 10: Build, vet, test, commit**

Run: `sqlc generate && go build ./... && go vet ./... && go test ./services/ledger/... ./e2e/... && go vet -tags=integration ./tests/integration/`
Expected: all clean; integration SKIPs locally.
```bash
git add services/ledger cmd e2e/critical_path/helpers_test.go tests/integration
git commit -m "fix(ledger): thread tenantID through ListJournalLines + UpdateJournalStatus (#139 slice H)"
```

---

## Task 2: budget — UpdateBudgetStatus (drop tautological self-lookup)

**Files:**
- Modify: `services/budget/repository.go:16` (signature)
- Modify: `services/budget/db/postgres.go:77-109` (delete self-lookup :87, thread tenantID)
- Modify: `services/budget/service.go` (:67, :82 call sites)
- Modify: `services/budget/service_test.go` (`mockRepo` :21 field, :47 method)
- Modify: `tests/integration/vertical/mocks_test.go:47` (`budgetMock`)
- Modify: `e2e/critical_path/helpers_test.go:361` (`budgetRepo`)
- Test: `tests/integration/` budget isolation

**Interfaces:**
- Produces: `Repository.UpdateBudgetStatus(ctx, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error` (adds `tenantID` 2nd param). The sqlc `queries.sql` needs no edit — it already has `WHERE id=$1 AND tenant_id=$2`.

- [ ] **Step 1: Widen the Repository signature** — `services/budget/repository.go:16`:

```go
	UpdateBudgetStatus(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error
```

- [ ] **Step 2: Rewrite the Postgres impl to use the passed tenantID**

`services/budget/db/postgres.go:77-109` — **delete** the `SELECT tenant_id FROM budgets WHERE id = $1` self-lookup (:87) and pass the parameter straight in:

```go
func (p *Postgres) UpdateBudgetStatus(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error {
	var actor pgtype.UUID
	if approvedBy != nil {
		actor = pgtype.UUID{Bytes: *approvedBy, Valid: true}
	}
	_, err := p.q.UpdateBudgetStatus(ctx, UpdateBudgetStatusParams{
		ID:          id,
		TenantID:    tenantID,
		Status:      status,
		SubmittedBy: actor,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return budget.ErrBudgetNotFound
		}
		return fmt.Errorf("budget: update budget status: %w", err)
	}
	return nil
}
```

(`:one` query; 0 rows → `pgx.ErrNoRows` → `ErrBudgetNotFound`. A cross-tenant `id` now yields `ErrBudgetNotFound` instead of the tautological self-satisfied predicate.)

- [ ] **Step 3: Thread tenantID at the two service call sites**

`services/budget/service.go:67` (`SubmitBudget`, holds `tenantID` from `GetBudget(ctx, tenantID, id)` at :60):
```go
	return s.repo.UpdateBudgetStatus(ctx, tenantID, id, "submitted", nil)
```
`services/budget/service.go:82` (`ApproveBudget`, holds `tenantID` from :72):
```go
	if err := s.repo.UpdateBudgetStatus(ctx, tenantID, id, "approved", &approvedBy); err != nil {
```

- [ ] **Step 4: Update all three doubles**

`services/budget/service_test.go` — `mockRepo.updateBudgetStatusFn` field (:21) and method (:47):
```go
	updateBudgetStatusFn func(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error
```
```go
func (m *mockRepo) UpdateBudgetStatus(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error {
	if m.updateBudgetStatusFn != nil {
		return m.updateBudgetStatusFn(ctx, tenantID, id, status, approvedBy)
	}
	return nil
}
```
`tests/integration/vertical/mocks_test.go:47` — `budgetMock`:
```go
func (m *budgetMock) UpdateBudgetStatus(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error { return nil }
```
`e2e/critical_path/helpers_test.go:361` — `budgetRepo` (in-memory, keyed by id; ignore tenantID param):
```go
func (r *budgetRepo) UpdateBudgetStatus(_ context.Context, _ , id uuid.UUID, status string, approvedBy *uuid.UUID) error {
	if b, ok := r.budgets[id]; ok {
		b.Status = status
		b.ApprovedBy = approvedBy
		return nil
	}
	return budget.ErrBudgetNotFound
}
```

(`lineItemRecordingRepo` in `handler_test.go` embeds `mockRepo` and inherits the method — no separate edit, but confirm it still compiles.)

- [ ] **Step 5: Fix existing unit-test call sites** — grep `updateBudgetStatusFn` / `UpdateBudgetStatus(` in `services/budget/*_test.go`; correct arg counts.

- [ ] **Step 6: Write the failing integration test** — `tests/integration/` (vertical harness). Two tenants each with a draft budget; `SubmitBudget`/`ApproveBudget` as A on B's budget id → `ErrBudgetNotFound`, B's row unchanged.

- [ ] **Step 7: Build, vet, test, commit**

Run: `go build ./... && go vet ./... && go test ./services/budget/... ./e2e/... ./tests/integration/vertical/... && go vet -tags=integration ./tests/integration/`
```bash
git add services/budget tests/integration e2e/critical_path/helpers_test.go
git commit -m "fix(budget): thread tenantID through UpdateBudgetStatus, drop tautological lookup (#139 slice H)"
```

---

## Task 3: iam — ChangePassword path + purge pair

Two independent sub-units; commit each separately (3a user/password, 3b purge) for bisectability. Both land in this one task/review.

**Files:**
- Modify: `services/iam/repository.go` (:24 GetUserByID, :35 UpdatePasswordHash, :90 Approve, :91 Cancel)
- Modify: `pkg/auth/local.go` (`UserStore.GetUserByID` declaration)
- Modify: `services/iam/db/queries.sql` (GetUser :138, UpdateUserPasswordHash :153 → `:execrows`, ApproveTenantPurgeRequest :205, CancelTenantPurgeRequest :211) + `sqlc generate`
- Modify: `services/iam/db/postgres.go` (:103 GetUserByID, :285 UpdatePasswordHash + RowsAffected, :1104 Approve, :1121 Cancel)
- Modify: `services/iam/service.go` (:332 ChangePassword signature, :346 UpdatePasswordHash call, :230 rehashIfNeeded call), `services/iam/purge.go` (:56, :94 repo calls)
- Modify: `services/iam/handler.go:222` (ChangePassword — source tenantID from CallerFromContext)
- Modify doubles: `services/iam/service_test.go` (`mockRepo` :90/:132/:216/:222), `e2e/critical_path/helpers_test.go` (`iamRepo` :125/:158/:264/:267), `pkg/auth/local_test.go:28` (`mockUserStore` GetUserByID)
- Test: `tests/integration/` iam isolation

**Interfaces:**
- Produces:
  - `Repository.GetUserByID(ctx, tenantID, userID uuid.UUID) (*auth.UserRecord, error)` AND `auth.UserStore.GetUserByID(ctx, tenantID, userID uuid.UUID) (*auth.UserRecord, error)` (same signature — must stay identical).
  - `Repository.UpdatePasswordHash(ctx, tenantID, userID uuid.UUID, hash string) error`
  - `Repository.ApproveTenantPurgeRequest(ctx, tenantID, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error)`
  - `Repository.CancelTenantPurgeRequest(ctx, tenantID, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error)`
  - `Service.ChangePassword(ctx, tenantID, userID uuid.UUID, oldPassword, newPassword string) error` (adds `tenantID`).

### 3a — ChangePassword path (GetUserByID + UpdatePasswordHash)

- [ ] **Step 1: Edit both SQL queries + regenerate**

`services/iam/db/queries.sql` — GetUser (:138) add tenant predicate:
```sql
-- name: GetUser :one
SELECT * FROM users WHERE id = $1 AND tenant_id = $2;
```
UpdateUserPasswordHash (:153) add tenant predicate **and change to `:execrows`** so 0 rows is detectable:
```sql
-- name: UpdateUserPasswordHash :execrows
UPDATE users SET password_hash = $2 WHERE id = $1 AND tenant_id = $3;
```
Run: `sqlc generate`. Expected: `GetUserParams{ID, TenantID}`; `UpdateUserPasswordHash(ctx, arg) (int64, error)` (execrows returns affected count) with `UpdateUserPasswordHashParams{ID, PasswordHash, TenantID}`.

- [ ] **Step 2: Change both interface declarations for GetUserByID**

`services/iam/repository.go:24` and `pkg/auth/local.go` (`UserStore`):
```go
	GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error)  // repository.go (auth is the pkg's UserRecord type)
```
```go
	GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*UserRecord, error)       // pkg/auth/local.go UserStore
```

- [ ] **Step 3: Update the Postgres impls**

`services/iam/db/postgres.go:103` (GetUserByID):
```go
func (p *Postgres) GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error) {
	u, err := p.q.GetUser(ctx, GetUserParams{ID: userID, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrUserNotFound
		}
		return nil, fmt.Errorf("iam/db: get user by id: %w", err)
	}
	// ... unchanged loadRolesAndPerms + UserRecord build ...
}
```

`services/iam/db/postgres.go:285` (UpdatePasswordHash — thread tenantID, check RowsAffected):
```go
func (p *Postgres) UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, hash string) error {
	n, err := p.q.UpdateUserPasswordHash(ctx, UpdateUserPasswordHashParams{
		ID:           userID,
		PasswordHash: hash,
		TenantID:     tenantID,
	})
	if err != nil {
		return fmt.Errorf("iam/db: update password hash: %w", err)
	}
	if n == 0 {
		return iam.ErrUserNotFound
	}
	return nil
}
```

- [ ] **Step 4: Thread tenantID through Service.ChangePassword + rehashIfNeeded**

`services/iam/service.go:332` — add `tenantID` param and pass it to both repo calls:
```go
func (s *Service) ChangePassword(ctx context.Context, tenantID, userID uuid.UUID, oldPassword, newPassword string) error {
	record, err := s.repo.GetUserByID(ctx, tenantID, userID)
	// ... unchanged verify old password ...
	if err := s.repo.UpdatePasswordHash(ctx, tenantID, userID, hash); err != nil {  // :346
		return fmt.Errorf("iam: update password hash: %w", err)
	}
	// ...
}
```
`services/iam/service.go:230` (`rehashIfNeeded`, already holds `tenantID` param) — pass it:
```go
	_ = s.repo.UpdatePasswordHash(ctx, tenantID, record.ID, newHash)
```

- [ ] **Step 5: Source tenantID in the ChangePassword handler**

`services/iam/handler.go:222` — obtain `tenantID` from the verified caller and pass it:
```go
func (h *Handler) ChangePassword(ctx context.Context, req *iamv1.ChangePasswordRequest) (*iamv1.ChangePasswordResponse, error) {
	userID, err := interceptor.ActorFromRequest(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok || caller.TenantID == uuid.Nil {
		return nil, status.Error(codes.Unauthenticated, "caller tenant not present in context")
	}
	if err := h.svc.ChangePassword(ctx, caller.TenantID, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		// ... unchanged error mapping ...
	}
	// ...
}
```

- [ ] **Step 6: Update the three GetUserByID + two UpdatePasswordHash doubles**

`services/iam/service_test.go:90` (`mockRepo.GetUserByID`) — new signature; **default must NOT echo tenantID** (avoid the vacuous trap):
```go
func (m *mockRepo) GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, tenantID, userID)
	}
	return &auth.UserRecord{ID: userID, PasswordHash: "hashed"}, nil // TenantID left uuid.Nil deliberately
}
```
Field `getUserByIDFn` gains the `tenantID` param too. `mockRepo.UpdatePasswordHash` (:132) → `(ctx, tenantID, userID, hash)`.
`e2e/critical_path/helpers_test.go:125` (`iamRepo.GetUserByID`) and `:158` (`UpdatePasswordHash`) → new signatures.
`pkg/auth/local_test.go:28` (`mockUserStore.GetUserByID`) → `(ctx, tenantID, userID)`.

- [ ] **Step 7: Write the failing integration test (the silent-:exec case)**

`tests/integration/` — two tenants, each a user. As tenant A, `ChangePassword` for tenant B's userID:
- Assert it returns `NotFound`/`ErrUserNotFound` (NOT success). This is the test that would FAIL against the old `:exec` (which silently updated 0 rows and returned nil).
- Assert B's `password_hash` is unchanged.

### 3b — purge pair (Approve/Cancel)

- [ ] **Step 8: Edit both SQL queries + regenerate**

`services/iam/db/queries.sql` — ApproveTenantPurgeRequest (:205) and CancelTenantPurgeRequest (:211) add `AND tenant_id = $3`:
```sql
-- name: ApproveTenantPurgeRequest :one
UPDATE tenant_purge_requests SET status = 'approved', approved_by = $2, approved_at = now()
 WHERE id = $1 AND status = 'pending' AND tenant_id = $3
RETURNING *;
-- name: CancelTenantPurgeRequest :one
UPDATE tenant_purge_requests SET status = 'cancelled', cancelled_by = $2, cancelled_at = now()
 WHERE id = $1 AND status IN ('pending', 'approved') AND tenant_id = $3
RETURNING *;
```
Run: `sqlc generate`. Params structs gain `TenantID uuid.UUID`.

- [ ] **Step 9: Widen the interface + Postgres impls**

`services/iam/repository.go:90,91` — add `tenantID` as the 2nd param to both. `services/iam/db/postgres.go:1104,1121` — thread it into the `*Params`:
```go
func (p *Postgres) ApproveTenantPurgeRequest(ctx context.Context, tenantID, requestID, approverID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.ApproveTenantPurgeRequest(ctx, ApproveTenantPurgeRequestParams{
		ID: requestID, ApprovedBy: pgUUIDFromPtr(&approverID), TenantID: tenantID,
	})
	// ... unchanged pgx.ErrNoRows → iam.ErrPurgeRequestNotFound ...
}
```
(Same shape for `CancelTenantPurgeRequest`, `CancelledBy`.)

- [ ] **Step 10: Pass tenantID at the two service call sites**

`services/iam/purge.go:56` (`ApproveTenantPurge`, holds `tenantID`) and `:94` (`CancelTenantPurge`):
```go
	approved, err := s.repo.ApproveTenantPurgeRequest(ctx, tenantID, open.ID, actor.UserID)
	// ...
	cancelled, err := s.repo.CancelTenantPurgeRequest(ctx, tenantID, open.ID, actor.UserID)
```

- [ ] **Step 11: Update the four purge doubles**

`services/iam/service_test.go:216,222` (`mockRepo` Approve/Cancel) and `e2e/critical_path/helpers_test.go:264,267` (`iamRepo` Approve/Cancel) — add the `tenantID` 2nd param; keep their existing `ErrPurgeRequestNotFound` default bodies.

- [ ] **Step 12: Write the failing integration test**

`tests/integration/` — two tenants each with an open purge request. As tenant A, `ApproveTenantPurgeRequest`/`CancelTenantPurgeRequest` for B's request id → `ErrPurgeRequestNotFound`, B's request unchanged. (Assert the tenantID reaching the repo, per the vacuous-trap guidance.)

- [ ] **Step 13: Fix remaining call sites, build, vet, test, commit (both sub-units)**

Grep the iam + pkg/auth test trees for `ChangePassword(`, `getUserByIDFn`, `GetUserByID(`, `UpdatePasswordHash(`, `ApproveTenantPurgeRequest(`, `CancelTenantPurgeRequest(`; fix all arg counts.
Run: `sqlc generate && go build ./... && go vet ./... && go test ./services/iam/... ./pkg/auth/... ./e2e/... && go vet -tags=integration ./tests/integration/`
Expected: all clean; integration SKIPs locally.
```bash
# commit 3a
git add services/iam/db/queries.sql services/iam/db/queries.sql.go services/iam/db/postgres.go services/iam/repository.go services/iam/service.go services/iam/handler.go services/iam/service_test.go pkg/auth e2e/critical_path/helpers_test.go tests/integration
git commit -m "fix(iam): tenant-scope GetUserByID + UpdatePasswordHash (ChangePassword) (#139 slice H)"
# commit 3b
git add services/iam tests/integration e2e/critical_path/helpers_test.go
git commit -m "fix(iam): tenant-scope Approve/CancelTenantPurgeRequest (#139 slice H)"
```

---

## Self-Review

- **Spec coverage:** all 7 sites — ledger ListJournalLines (JOIN) + UpdateJournalStatus (Task 1) ✓; budget UpdateBudgetStatus (Task 2) ✓; iam GetUserByID, UpdatePasswordHash, Approve/CancelTenantPurgeRequest (Task 3) ✓. Discarded error at ledger `resolveTenantForJournal` deleted (Task 1 Step 5) ✓. Follow-ups #172/#173/#174 out of scope (Global Constraints) ✓.
- **Placeholder scan:** every SQL/Go step carries concrete code; "unchanged" bodies are explicitly marked where the surrounding method is quoted. The only deferred detail is exact existing-test-call-site arg fixes (grep-and-fix steps) — mechanical, bounded by the named identifiers.
- **Type consistency:** `tenantID` is the 2nd positional param uniformly (`UpdateJournalStatus`, `UpdateBudgetStatus`, `GetUserByID`, `UpdatePasswordHash`, `Approve/CancelTenantPurgeRequest`, `ChangePassword`). `GetUserByID` identical on both `iam.Repository` and `auth.UserStore`. Not-found sentinels named per method (`ErrJournalNotFound`, `ErrBudgetNotFound`, `ErrUserNotFound`, `ErrPurgeRequestNotFound`) and matched to the query kind (`:one`→`pgx.ErrNoRows`, `:execrows`→`RowsAffected==0`).
- **Query-kind trap:** `UpdateUserPasswordHash` explicitly converted `:exec`→`:execrows` (Step 1) so the tenant guard is not a silent no-op — the single most important correctness detail in this plan.
