# Transactional AcceptInvitation (#148) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run `Service.AcceptInvitation`'s three writes (create user, assign role, mark accepted) in one DB transaction so a failed role grant leaves no orphaned user row and a still-`pending` invitation retryable.

**Architecture:** Add a `WithTx(ctx, fn func(Repository) error)` primitive to `iam.Repository`; the Postgres impl threads a `DBTX` so its query methods ride the transaction; the two in-memory doubles run `fn` directly. `AcceptInvitation` composes `CreateUser`/`GetRoleByID`/`AssignRole`/`MarkInvitationAccepted` inside `WithTx`, with `tokens.Issue` (Redis) kept strictly after commit.

**Tech Stack:** Go 1.25, pgx/v5, sqlc (no query changes here), Postgres, `pkg/testdb`.

## Global Constraints

- **`tokens.Issue` stays AFTER the transaction commits** — it writes Redis; a session minted for a user the tx then rolls back would be a live session for a nonexistent user.
- **The interface widens across THREE implementers** — `*db.Postgres`, unit `mockRepo` (`services/iam/service_test.go`), e2e `iamRepo` (`e2e/critical_path/helpers_test.go`). Whole-tree `go vet ./...` is the completion gate (a focused build misses the e2e double).
- **In-memory doubles do NOT model rollback** — their `WithTx` runs `fn` directly; atomicity is a Postgres property, proven only by the integration test.
- **No Go-model, proto, or migration change.** No change to `Logout`, single `AssignRole`, or `CreateUser`'s external behavior (its body is refactored to share `prepareNewUser`).
- **`AssignRole`'s error keeps its `"assign role"` wrap** so the existing failure-source unit test still pins the step.
- **Local DB safety:** NEVER run `docker compose … -v/down/up` on `infra/local/`. Integration tests SKIP without `THITTAM_TEST_DSN`; CI's real-Postgres job is authoritative.
- **Commits:** Conventional Commits, scope `iam`; end every message with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `services/iam/repository.go` | Add `WithTx` to the `Repository` interface | 1 |
| `services/iam/db/postgres.go` | Thread `DBTX`; implement `WithTx`; switch `PurgeTenantSchemaAndTombstone` to `p.pool.Begin` | 1 |
| `services/iam/service_test.go` | `mockRepo.WithTx` pass-through | 1 |
| `e2e/critical_path/helpers_test.go` | `iamRepo.WithTx` pass-through | 1 |
| `services/iam/service.go` | `prepareNewUser` helper; `AcceptInvitation` wraps writes in `WithTx` | 2 |
| `services/iam/service_test.go` | Keep 7 existing accept tests green; add fail-closed (no token) test | 2 |
| `tests/integration/iam_accept_invitation_tx_test.go` | Real-Postgres rollback + same-token-retry proof | 3 |

---

### Task 1: Add the `WithTx` primitive to the repository

**Files:**
- Modify: `services/iam/repository.go:14` (interface) — add one method after the Invitations block (~line 117)
- Modify: `services/iam/db/postgres.go:19-30` (struct + constructor), add `WithTx`, change `p.db.Begin`→`p.pool.Begin` at `:1111`
- Modify: `services/iam/service_test.go` (`mockRepo`) — add `WithTx`
- Modify: `e2e/critical_path/helpers_test.go` (`iamRepo`) — add `WithTx`

**Interfaces:**
- Consumes: nothing.
- Produces: `Repository.WithTx(ctx context.Context, fn func(Repository) error) error`. When `fn` returns nil the tx commits; when it returns an error the tx rolls back and that error propagates. On `*db.Postgres`, the `Repository` handed to `fn` runs every write on one `pgx.Tx`; on the in-memory doubles `fn` runs against the same object (no real tx).

- [ ] **Step 1: Add the interface method**

In `services/iam/repository.go`, immediately after the Invitations block (after `MarkInvitationAccepted(...)` at line 117), add:
```go

	// WithTx runs fn against a repository bound to a single transaction. Every
	// write fn performs commits together when fn returns nil, or rolls back
	// together when fn returns an error. Implementations backed by an in-memory
	// store run fn directly and do not model rollback.
	WithTx(ctx context.Context, fn func(Repository) error) error
```

- [ ] **Step 2: Thread `DBTX` through the Postgres struct**

In `services/iam/db/postgres.go`, replace the struct + constructor (lines 19-30):
```go
// Postgres implements iam.Repository using sqlc-generated queries over a pgx/v5 pool.
type Postgres struct {
	q    *Queries
	db   DBTX          // pool normally; a pgx.Tx inside WithTx — all query methods use this
	pool *pgxpool.Pool // only for Begin; nil on a tx-bound instance
}

// NewPostgres creates a Postgres repository backed by the given pgx connection pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{
		q:    New(pool),
		db:   pool,
		pool: pool,
	}
}
```
`DBTX` is the generated interface in `db/db.go` (`Exec`/`Query`/`QueryRow`); both `*pgxpool.Pool` and `pgx.Tx` satisfy it. Every existing method already uses `p.q` or `p.db.{Exec,Query,QueryRow}` — only the field *type* of `db` changed.

- [ ] **Step 3: Switch the one pool-only call site**

`PurgeTenantSchemaAndTombstone` opens its own tx via `p.db.Begin` at `services/iam/db/postgres.go:1111`. `DBTX` has no `Begin`, so change that one line:
```go
	tx, err := p.pool.Begin(ctx)
```
(Leave the rest of that method unchanged — it already uses the local `tx`.)

- [ ] **Step 4: Implement `Postgres.WithTx`**

Add this method to `services/iam/db/postgres.go` (near the constructor). `errors` is already imported (line 5):
```go
// WithTx runs fn against a *Postgres bound to a single pgx.Tx. Commits when fn
// returns nil, rolls back otherwise. Not re-entrant: a tx-bound instance has a
// nil pool and refuses to open a nested transaction.
func (p *Postgres) WithTx(ctx context.Context, fn func(iam.Repository) error) error {
	if p.pool == nil {
		return errors.New("iam/db: WithTx cannot nest inside a transaction")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iam/db: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after a successful Commit
	txRepo := &Postgres{q: p.q.WithTx(tx), db: tx, pool: nil}
	if err := fn(txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 5: Add `mockRepo.WithTx` (unit double)**

`services/iam/service_test.go` is `package iam`. Add near the other `mockRepo` methods:
```go
func (m *mockRepo) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(m) // no real tx; atomicity is a Postgres property (see integration test)
}
```

- [ ] **Step 6: Add `iamRepo.WithTx` (e2e double)**

`e2e/critical_path/helpers_test.go` is `package critical_path`. Add after the last `iamRepo` method (~line 322):
```go
func (r *iamRepo) WithTx(ctx context.Context, fn func(iam.Repository) error) error {
	return fn(r)
}
```

- [ ] **Step 7: Build the whole tree and run the iam unit suite**

Run from repo root:
```bash
go build ./... && go vet ./... && go test ./services/iam/... -race
```
Expected: all succeed. `go vet ./...` is the gate proving all three implementers satisfy the widened interface (it compiles the e2e test package, which a plain `go build ./...` of non-test code would skip). The iam unit tests still pass unchanged — nothing calls `WithTx` yet.

- [ ] **Step 8: Commit**

```bash
git add services/iam/repository.go services/iam/db/postgres.go services/iam/service_test.go e2e/critical_path/helpers_test.go
git commit -m "feat(iam): add Repository.WithTx transaction primitive

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Wrap AcceptInvitation's writes in a transaction

**Files:**
- Modify: `services/iam/service.go:276-292` (`CreateUser` → extract `prepareNewUser`)
- Modify: `services/iam/service.go:727-780` (`AcceptInvitation`)
- Modify: `services/iam/service_test.go` (keep 7 accept tests green; add fail-closed test)

**Interfaces:**
- Consumes: `Repository.WithTx` from Task 1.
- Produces: unchanged public signature `AcceptInvitation(ctx, token, plainPassword) (*auth.TokenPair, error)`; new private `prepareNewUser(user *User, plainPassword string) error`.

- [ ] **Step 1: Extract `prepareNewUser` and rewire `CreateUser`**

In `services/iam/service.go`, replace `CreateUser` (lines 276-292) with:
```go
func (s *Service) CreateUser(ctx context.Context, user *User, plainPassword string) (*User, error) {
	if err := s.prepareNewUser(user, plainPassword); err != nil {
		return nil, err
	}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: create user: %w", err)
	}
	return user, nil
}

// prepareNewUser fills in defaults and hashes the password on a not-yet-persisted
// User. CPU-only (no I/O), so it is safe to call before opening a transaction.
func (s *Service) prepareNewUser(user *User, plainPassword string) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	if user.Status == "" {
		user.Status = "active"
	}
	hash, err := s.hasher.Hash(plainPassword)
	if err != nil {
		return fmt.Errorf("iam: hash password: %w", err)
	}
	user.PasswordHash = hash
	return nil
}
```

- [ ] **Step 2: Rewrite `AcceptInvitation` to use `WithTx`**

Replace the body from `user := &User{...}` through the `MarkInvitationAccepted` block (the lines between the expiry check and the `// Issue tokens directly` comment). Keep everything above (`GetInvitationByToken` + status/expiry checks) and below (`tokens.Issue`) unchanged. New middle section:
```go
	// Derive display name from the email local part until the user updates it.
	parts := strings.SplitN(inv.Email, "@", 2)
	user := &User{
		TenantID:    inv.TenantID,
		Email:       inv.Email,
		DisplayName: parts[0],
		Status:      "active",
	}
	if err := s.prepareNewUser(user, plainPassword); err != nil {
		return nil, fmt.Errorf("iam: accept invitation — prepare user: %w", err)
	}

	// All three writes commit together or roll back together (#148): a failed
	// role grant must leave no user row and a still-pending invitation. Route
	// through the tx-bound repo, not s.CreateUser/s.AssignRole (which use the
	// pool-bound s.repo). The role is re-validated inside the tx because the
	// invitation may be seven days old and its role since deleted; a role-less
	// user holding a valid token is worse than a rejected invitation.
	if err := s.repo.WithTx(ctx, func(tx Repository) error {
		if err := tx.CreateUser(ctx, user); err != nil {
			return fmt.Errorf("iam: create user: %w", err)
		}
		if inv.RoleID != nil {
			if _, err := tx.GetRoleByID(ctx, inv.TenantID, *inv.RoleID); err != nil {
				return err // ErrRoleNotFound for a role outside the invitation's tenant
			}
			ur := &UserRole{
				UserID:     user.ID,
				RoleID:     *inv.RoleID,
				AssignedBy: inv.InvitedBy,
				AssignedAt: time.Now().UTC(),
			}
			if err := tx.AssignRole(ctx, ur); err != nil {
				return fmt.Errorf("iam: assign role %s to user %s: %w", *inv.RoleID, user.ID, err)
			}
		}
		return tx.MarkInvitationAccepted(ctx, inv.ID)
	}); err != nil {
		return nil, err
	}

	// Issue tokens directly — user just proved they control the invited email.
	// AFTER commit: a session for a rolled-back user must never exist.
```
The `result := &auth.AuthResult{...}` block and the `s.tokens.Issue` call that follow stay exactly as they are.

- [ ] **Step 3: Run the existing accept tests (expect green)**

Run from repo root:
```bash
go test ./services/iam/ -run TestAcceptInvitation -race -v
```
Expected: all 7 existing `TestAcceptInvitation_*` pass. They build `mockRepo`, whose `WithTx` (Task 1) runs the callback in place, so `createUserFn`/`getRoleByIDFn`/`assignRoleFn`/`markInvitationFn` all still fire. If any fails on an error-message assertion, confirm the wrap text matches (the callback keeps `"iam: create user"`, `"iam: assign role %s to user %s"`, and `"iam: mark invitation accepted"` is now produced by the repo layer — see note below). Do NOT weaken an assertion to make it pass; if a message genuinely changed, update the test to the new exact text and note it in the report.

Note on `MarkInvitationAccepted`'s wrap: previously `AcceptInvitation` wrapped it as `"iam: mark invitation accepted: %w"`. In the new code `tx.MarkInvitationAccepted`'s error returns unwrapped from the callback. If `TestAcceptInvitation_*` asserts on that specific text, wrap it in the callback: `return fmt.Errorf("iam: mark invitation accepted: %w", err)` instead of the bare `return tx.MarkInvitationAccepted(...)`. Check `services/iam/service_test.go` for such an assertion and match it.

- [ ] **Step 4: Add the fail-closed unit test (no token on failed grant)**

Add to `services/iam/service_test.go`:
```go
func TestAcceptInvitation_FailedGrant_IssuesNoToken(t *testing.T) {
	roleID := uuid.New()
	repo := &mockRepo{
		getInvitationByTokenFn: func(_ context.Context, _ string) (*Invitation, error) {
			return &Invitation{
				ID: uuid.New(), TenantID: uuid.New(), Email: "invitee@example.com",
				Status: "pending", RoleID: &roleID, InvitedBy: uuid.New(),
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			}, nil
		},
		createUserFn: func(_ context.Context, _ *User) error { return nil },
		getRoleByIDFn: func(_ context.Context, _, _ uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound // the grant fails inside the tx
		},
	}
	issuer := &mockTokenIssuer{
		issueFn: func(_ context.Context, _ *auth.AuthResult) (*auth.TokenPair, error) {
			t.Fatal("tokens.Issue must not be called when the role grant fails")
			return nil, nil
		},
	}
	svc := NewService(repo, &mockAuthenticator{}, issuer, &mockHasher{}, &mockVerifier{})

	_, err := svc.AcceptInvitation(context.Background(), "tok", "pw")
	require.ErrorIs(t, err, ErrRoleNotFound)
}
```
(Field names — `getInvitationByTokenFn`, `createUserFn`, `getRoleByIDFn`, `issueFn` — are the existing `mockRepo`/`mockTokenIssuer` fields; confirm against the structs and adjust names if they differ. `mockAuthenticator`/`mockHasher`/`mockVerifier` are the existing doubles used by `TestAcceptInvitation_Success`.)

- [ ] **Step 5: Run the new test and the full iam suite**

Run from repo root:
```bash
go test ./services/iam/ -run TestAcceptInvitation_FailedGrant_IssuesNoToken -race -v
go test ./services/iam/... -race
```
Expected: the new test passes (and would fail — via the `t.Fatal` — if `tokens.Issue` were called before the grant error propagated); full suite green.

- [ ] **Step 6: Commit**

```bash
git add services/iam/service.go services/iam/service_test.go
git commit -m "fix(iam): make AcceptInvitation transactional (#148)

Create-user, assign-role and mark-accepted now commit or roll back together,
so a failed grant leaves no orphaned user row and a retryable pending
invitation. Token issuance stays after commit.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Real-Postgres rollback + same-token-retry proof

**Files:**
- Create: `tests/integration/iam_accept_invitation_tx_test.go`

**Interfaces:**
- Consumes: Tasks 1–2 (working `WithTx` + transactional `AcceptInvitation`). Reuses `seedIAMTenant`, `seedIAMUser`, `seedIAMRole`, `noopTokenIssuer` from the `integration` package (`iam_tenant_isolation_test.go`, `iam_invitation_roundtrip_test.go`).
- Produces: nothing downstream.

**FK-cleanup rule (learned in #185):** `invitations.invited_by` is `NOT NULL REFERENCES users(id)` with **no `ON DELETE`** (NO ACTION). Cleanup runs LIFO, and `seedIAMUser` registers a `DELETE FROM users WHERE id=<inviter>` cleanup. So this test MUST register its own `t.Cleanup` that deletes the invitation rows for `(tenant, email)` **after** the seed calls (so LIFO runs it first), or the inviter delete fails with an FK violation and marks the test failed. This is baked into the code below.

- [ ] **Step 1: Write the integration test**

Create `tests/integration/iam_accept_invitation_tx_test.go`:
```go
//go:build integration

// Real-Postgres proof that AcceptInvitation is transactional (#148): a role
// grant that fails inside the tx must leave NO user row and a still-pending
// invitation, and the same token must succeed once the invitation's role
// reference is repaired. Reuses the harness in iam_tenant_isolation_test.go /
// iam_invitation_roundtrip_test.go (seedIAMTenant, seedIAMUser, seedIAMRole,
// noopTokenIssuer).
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

func TestIAM_AcceptInvitation_RollsBackOnFailedGrant(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)
	hasher := auth.NewArgon2idHasher()
	svc := iam.NewService(repo, nil, noopTokenIssuer{}, hasher, auth.NewDualVerifier())

	tenantA := seedIAMTenant(t, pool)
	tenantB := seedIAMTenant(t, pool)
	inviterHash, err := hasher.Hash("inviter-pass")
	require.NoError(t, err)
	inviter := seedIAMUser(t, pool, tenantA, inviterHash)
	crossTenantRole := seedIAMRole(t, pool, tenantB, "cross") // in tenant B, not A
	validRole := seedIAMRole(t, pool, tenantA, "valid")       // in tenant A

	email := "invitee-" + uuid.NewString() + "@example.com"
	token := "tok-" + uuid.NewString()
	invID := seedIAMInvitationTx(t, pool, tenantA, email, crossTenantRole, inviter, token)

	// --- Failed grant: GetRoleByID(tenantA, crossTenantRole) fails inside the
	// tx, so CreateUser rolls back. ---
	_, err = svc.AcceptInvitation(ctx, token, "chosen-password")
	require.Error(t, err, "cross-tenant role grant must fail the acceptance")

	assert.Equal(t, uuid.Nil, readUserIDByEmail(t, pool, tenantA, email),
		"a rolled-back accept must leave NO user row")
	assert.Equal(t, "pending", readInvitationStatus(t, pool, invID),
		"a rolled-back accept must leave the invitation pending")

	// --- Repair the invitation's role to a valid tenant-A role, then retry the
	// SAME token: it must now succeed end to end. ---
	_, err = pool.Exec(ctx, `UPDATE invitations SET role_id = $1 WHERE id = $2`, validRole, invID)
	require.NoError(t, err, "repair invitation role")

	_, err = svc.AcceptInvitation(ctx, token, "chosen-password")
	require.NoError(t, err, "retry with the repaired role must succeed")

	newUserID := readUserIDByEmail(t, pool, tenantA, email)
	require.NotEqual(t, uuid.Nil, newUserID, "the invitee's user row must exist after success")
	assert.True(t, userHasRole(t, pool, newUserID, validRole), "the repaired role must be granted")
	assert.Equal(t, "accepted", readInvitationStatus(t, pool, invID), "the invitation must be accepted")
}

// seedIAMInvitationTx inserts a pending invitation carrying roleID and returns
// its id. Registered cleanup deletes the invitation row FIRST (LIFO) so the
// inviter user's own cleanup does not hit the invited_by FK (NO ACTION).
func seedIAMInvitationTx(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, email string, roleID, invitedBy uuid.UUID, token string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	invID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO invitations (id, tenant_id, email, invited_by, token, expires_at, role_id, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')`,
		invID, tenantID, email, invitedBy, token, time.Now().UTC().Add(7*24*time.Hour), roleID)
	require.NoError(t, err, "insert invitation")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM invitations WHERE id = $1`, invID)
	})
	return invID
}
```
`readUserIDByEmail`, `userHasRole`, `readInvitationStatus` already exist in `iam_invitation_roundtrip_test.go` (same package) — do not redefine them.

- [ ] **Step 2: Compile-check under the build tag**

Run from repo root:
```bash
go vet -tags=integration ./tests/integration/
```
Expected: exit 0. Catches helper-name collisions (only `seedIAMInvitationTx` is new here) and type mismatches. Do NOT connect to a database — the test SKIPs locally without `THITTAM_TEST_DSN`; CI's `Integration Tests (real Postgres)` job is authoritative.

- [ ] **Step 3: Commit**

```bash
git add tests/integration/iam_accept_invitation_tx_test.go
git commit -m "test(iam): prove AcceptInvitation rolls back a failed grant (#148)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- §1 `Repository.WithTx` → Task 1 ✅
- §2 `Postgres` threads `DBTX` + `WithTx` + `PurgeTenantSchemaAndTombstone`→`p.pool.Begin` → Task 1 ✅
- §3 in-memory doubles pass-through → Task 1 (mockRepo, iamRepo) ✅
- §4 `AcceptInvitation` rewrite + `prepareNewUser`, `tokens.Issue` after commit, dropped redundant `GetUser`, kept `"assign role"` wrap → Task 2 ✅
- §5 fail-closed preserved → Task 2's new test ✅
- Testing: unit (7 kept + fail-closed) → Task 2; integration rollback + same-token retry → Task 3 ✅
- Non-goals (no model/proto/migration change; no `Logout`/single-`AssignRole` change) → Global Constraints ✅

**Placeholder scan:** none — every step has concrete code. Field-name confirmations in Task 2 Step 4 name the exact structs to check, not vague "adjust as needed".

**Type consistency:** `WithTx(ctx, fn func(Repository) error) error` — interface uses unqualified `Repository` (package iam); `mockRepo` (package iam) matches; `*Postgres`/`iamRepo` (other packages) use `func(iam.Repository) error`, the same interface. `prepareNewUser(user *User, plainPassword string) error` defined and consumed in Task 2. `UserRole{UserID, RoleID, AssignedBy, AssignedAt}` matches the model used in the original `AssignRole`. Integration helpers (`readUserIDByEmail`/`userHasRole`/`readInvitationStatus`) are reused from Task 3's stated source file; only `seedIAMInvitationTx` is new.

**Ordering:** Task 1 (primitive, compiles standalone — nothing calls it yet) → Task 2 (uses it) → Task 3 (integration proof). Whole-tree `go vet ./...` lands in Task 1 where the interface widens.
