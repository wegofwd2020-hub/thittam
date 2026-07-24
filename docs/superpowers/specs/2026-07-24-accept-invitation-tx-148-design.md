# Transactional AcceptInvitation (#148) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issue:** #148 (AcceptInvitation is not transactional: a failed role grant leaves a committed user row)
**Branch:** `fix/accept-invitation-tx-148` off `main` (`10c9ea7`)
**Migration:** none

## Goal

Run the three writes of `Service.AcceptInvitation` — create user, assign the
invited role, mark the invitation accepted — in **one database transaction**, so
a failure at any step leaves no partial state. Today a failed `AssignRole`
leaves a committed user row and a still-`pending` invitation; the natural retry
then collides on the unique email and the invitee is permanently stuck.

## Context

`Service.AcceptInvitation` (`services/iam/service.go:727`) performs, in order:
`GetInvitationByToken` (read) → status/expiry checks → `s.CreateUser` (hashes
the password, inserts the user) → `s.AssignRole` (two tenant-scoped reads +
`user_roles` insert, only when the invitation carries a role) →
`s.repo.MarkInvitationAccepted` → `s.tokens.Issue`. Each DB write is its own
autocommit statement.

Since #146 (PR #147) propagates a failed grant's error instead of discarding it,
a failure after `CreateUser` now correctly rejects the acceptance — but the user
row from step 1 has already committed. The grant runs before
`MarkInvitationAccepted`, so the invitation stays `pending` (retry is the
intended recovery), yet `CreateUser` on retry hits the unique-email constraint.
Strictly better than the old silent under-privileged account, but a real
availability gap on a security-relevant path.

### Why #146 didn't fix it

`iam.Repository` exposes no transaction handle spanning these calls; every
`Service` method takes a bare `context.Context` and calls the repository
directly. Threading a transaction is a design change, out of scope for a
privilege-escalation patch.

### Grounding facts (measured on `main`)

- `Postgres` (`services/iam/db/postgres.go:18`) holds `q *Queries` **and**
  `db *pgxpool.Pool`; every method already routes through `p.q` (sqlc) or
  `p.db` (hand-written SQL). sqlc's `Queries.WithTx(tx pgx.Tx) *Queries` already
  exists in the generated `db/db.go` and is currently unused.
- **Three** `iam.Repository` implementers must widen together:
  `*db.Postgres` (production), `mockRepo` (`services/iam/service_test.go:32`,
  unit double), and the e2e `iamRepo` (`e2e/critical_path/helpers_test.go:106`,
  in-memory maps, no transaction concept). Whole-tree `go vet ./...` is the gate
  that catches the e2e double — a focused build misses it.
- `s.tokens.Issue` (`pkg/auth/jwt.go`) writes to **Redis**, not Postgres. It
  must stay **after** commit: a refresh token minted for a user the tx later
  rolls back would be a live session for a nonexistent user.
- `s.CreateUser` hashes the password (`s.hasher.Hash`, CPU-only, no I/O).
- `Service.AssignRole`'s pre-write validation is two tenant-scoped reads:
  `GetRoleByID(tenantID, roleID)` (role belongs to tenant → `ErrRoleNotFound`)
  and `GetUser(tenantID, userID)` (user belongs to tenant). In the accept path
  the second reads back the user just created one line earlier.
- `invitations.role_id` is `UUID REFERENCES roles(id) ON DELETE SET NULL`
  (`migrations/iam/009_create_invitations.up.sql:10`). The FK is global, not
  tenant-scoped: a tenant-A invitation may store a tenant-B role id. Deleting a
  role NULLs the reference (→ grant skipped, not failed).
- No NATS / outbox / audit write occurs anywhere in this path.

## Design

### 1. `iam.Repository` gains one method

In `services/iam/repository.go`:

```go
// WithTx runs fn against a repository bound to a single transaction. Every
// write fn performs commits together when fn returns nil, or rolls back
// together when fn returns an error. Implementations backed by an in-memory
// store run fn directly and do not model rollback.
WithTx(ctx context.Context, fn func(Repository) error) error
```

### 2. `Postgres` threads a `DBTX`

Change the struct so every query method rides whatever `DBTX` it holds — the
pool normally, a `pgx.Tx` inside `WithTx`:

```go
type Postgres struct {
	q    *Queries       // sqlc queries, bound to db
	db   DBTX           // pool OR tx — all hand-written SQL uses this
	pool *pgxpool.Pool  // only for Begin; nil on a tx-bound instance
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(pool), db: pool, pool: pool}
}
```

`DBTX` is the generated interface in `db/db.go` (`Exec`/`Query`/`QueryRow`);
both `*pgxpool.Pool` and `pgx.Tx` satisfy it. All existing methods keep using
`p.q` / `p.db` unchanged — only the field *type* of `db` changes (pool →
`DBTX`), which the pool still satisfies. No method reaches for a pool-only API
(`Begin`/`Acquire`) except `WithTx`.

```go
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
		return err // deferred Rollback fires
	}
	return tx.Commit(ctx)
}
```

The generated `db/db.go` is **not** edited — `Queries.WithTx` is reused as-is.

### 3. The two in-memory doubles

`mockRepo` and the e2e `iamRepo` each get a pass-through:

```go
func (m *mockRepo) WithTx(ctx context.Context, fn func(iam.Repository) error) error {
	return fn(m) // no real tx; atomicity is a Postgres property (see integration test)
}
```

This is deliberate and is what preserves the existing unit tests: their
`createUserFn` / `getRoleByIDFn` / `assignRoleFn` / `markInvitationFn` fields
still fire because the callback runs against the same mock. Rollback is not
modeled here; it is proven only against real Postgres (§5).

### 4. `Service.AcceptInvitation` rewrite

Move password prep ahead of the callback, wrap the writes in `WithTx`, keep
`tokens.Issue` after it:

```go
// ... GetInvitationByToken + status/expiry checks unchanged ...

user := &User{TenantID: inv.TenantID, Email: inv.Email, DisplayName: parts[0], Status: "active"}
if err := s.prepareNewUser(user, plainPassword); err != nil { // ID + status + hash
	return nil, fmt.Errorf("iam: accept invitation — prepare user: %w", err)
}

err = s.repo.WithTx(ctx, func(tx Repository) error {
	if err := tx.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("iam: create user: %w", err)
	}
	if inv.RoleID != nil {
		if _, err := tx.GetRoleByID(ctx, inv.TenantID, *inv.RoleID); err != nil {
			return err // ErrRoleNotFound for a role outside the invitation's tenant
		}
		ur := &UserRole{UserID: user.ID, RoleID: *inv.RoleID, AssignedBy: inv.InvitedBy, AssignedAt: time.Now().UTC()}
		if err := tx.AssignRole(ctx, ur); err != nil {
			return fmt.Errorf("iam: assign role %s to user %s: %w", *inv.RoleID, user.ID, err)
		}
	}
	return tx.MarkInvitationAccepted(ctx, inv.ID)
})
if err != nil {
	return nil, err
}

// After commit only — a session for a rolled-back user must never exist.
pair, err := s.tokens.Issue(ctx, &auth.AuthResult{ /* unchanged */ })
```

- `prepareNewUser(user, plainPassword) error` is a new private helper (ID
  default + status default + `s.hasher.Hash` → `user.PasswordHash`) extracted
  from the current `CreateUser` body; `CreateUser` calls it too, so no
  duplication and `CreateUser`'s external behavior is unchanged.
- The redundant read-back of the just-created user (`GetUser`) is dropped — the
  user is created in this same tx; its tenant is known. `GetRoleByID` is kept:
  it validates a possibly-stale or cross-tenant role and yields the
  `ErrRoleNotFound` the existing "role gone" test asserts.
- The `AssignRole` error keeps its `"assign role"` wrap so the existing
  failure-source unit test still pins the step.

### 5. Fail-closed preserved

On any callback error, `WithTx` returns it and `AcceptInvitation` returns before
`tokens.Issue` — a failed grant never yields a token, exactly the #146
guarantee. No new error type, so `grpcError` (`services/iam/handler.go`) is
unchanged; a begin/commit failure wraps to `codes.Internal`, which is correct
for an infrastructure fault.

## Testing

### Unit (`services/iam/service_test.go`)

- Add `mockRepo.WithTx` (pass-through). The 7 existing `AcceptInvitation` tests
  keep asserting on the same fn-fields with only wiring adjustments.
- Add a test that on a failed grant, `s.tokens.Issue` is **never** invoked
  (mock token issuer with a `t.Fatal` body) — pins fail-closed at the unit
  level, since the mock cannot model rollback.

### Integration (new: `tests/integration/iam_accept_invitation_tx_test.go`)

`//go:build integration`, `pkg/testdb.Open(t)` (SKIPs without `THITTAM_TEST_DSN`;
CI's real-Postgres job is authoritative). Reuses the harness in
`tests/integration/iam_tenant_isolation_test.go` — `seedIAMTenant` and the
`noopTokenIssuer` added in #183 — and builds `iam.NewService` over the real
`iamdb.NewPostgres(pool)`.

**Failure injection (deterministic, repairable, same token):** seed tenant A and
tenant B; seed a role `R` in **tenant B**; seed a `pending` invitation `I` in
**tenant A** carrying `role_id = R` and token `T` (the FK is global, so a
cross-tenant role id is storable). Accepting `T` inserts the user, then
`GetRoleByID(tenantA, R)` fails with `ErrRoleNotFound` inside the tx → rollback.

- **Rollback proof:** `AcceptInvitation(ctx, T, pw)` returns an error; assert
  the `users` table has **no** row for the invitation's email, and `I` is still
  `pending` (`accepted_at IS NULL`).
- **Retry-once-restored proof (same token):** `UPDATE invitations SET role_id =
  <a valid tenant-A role> WHERE id = I` (an admin fixing the invitation), then
  `AcceptInvitation(ctx, T, pw)` again → succeeds; assert the user now exists in
  tenant A with the granted role and `I` is `accepted`. Because the first
  attempt left no orphan row, the same email accepts cleanly.

This is the acceptance criteria made executable: a failing `AssignRole` leaves
no user row and a `pending` invitation, and the same token succeeds once the
role reference is repaired — with no change to the fail-closed behaviour.

### Completion gate

Whole-tree `go vet ./...` after the interface widening (catches the e2e double).
`go test ./services/iam/... -race` for the unit suite. Coverage floor
`services/iam` ≥ 85%.

## Non-goals

- **No migration, no schema change.**
- **No generic unit-of-work framework** beyond the single `WithTx` primitive —
  it exists because this is the one iam flow that needs multi-write atomicity.
- **No change to `Logout`, single `AssignRole`, or `CreateUser`'s external
  behavior** (only its body is refactored to share `prepareNewUser`).
- **No modelling of rollback in the in-memory doubles** — atomicity is verified
  against real Postgres only.
- No change to the invitation-issuance path (`InviteUser`) or to `tokens.Issue`.

## Review weight

Touches `iam` and an authentication/authorization path → senior engineer + 2
approvals per CLAUDE.md. Whole-branch review on the most capable model.
