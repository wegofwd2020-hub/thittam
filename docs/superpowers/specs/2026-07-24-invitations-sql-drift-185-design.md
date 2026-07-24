# Fix Invitations SQL Drift (#185) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issue:** #185 (invitations SQL queries reference `accepted_at` + a nonexistent `ON CONFLICT` target)
**Branch:** `fix/invitations-sql-drift-185` off `main` (`10c9ea7`)
**Migration:** one (iam `024`)
**Prerequisite for:** #148 (transactional AcceptInvitation)

## Goal

Make the invitations SQL layer actually work against real Postgres — the three
queries in `services/iam/db/queries.sql` reference columns/constraints the
migrated schema does not have — and add the first real-Postgres integration test
covering the `InviteUser → GetInvitationByToken → AcceptInvitation` round-trip.

## Context

`services/iam/db/queries.sql` has drifted from
`migrations/iam/009_create_invitations.up.sql`. Three breakages, each fatal at
runtime against real Postgres:

| query | references | schema (migration 009) actually has |
|---|---|---|
| `GetInvitationByToken` | `WHERE accepted_at IS NULL` | `status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','expired'))` — **no `accepted_at`** |
| `AcceptInvitation` | `SET accepted_at = now()` | same |
| `CreateInvitation` | `ON CONFLICT (tenant_id, email)` | only `token` is `UNIQUE`; `(tenant_id, email)` is a **non-unique** index |

Against real Postgres these raise `column "accepted_at" does not exist` and
`no unique or exclusion constraint matching the ON CONFLICT specification`.

**Why sqlc didn't catch it:** sqlc validates `SELECT *` expansion (the generated
`GetInvitationByToken` correctly lists nine columns, none named `accepted_at`)
but not bare column references in `WHERE` / `SET` / `ON CONFLICT`. So
`sqlc generate` is clean and Codegen Freshness stays green while the queries are
runtime-broken.

**Why it was never caught at runtime:** unit tests use `mockRepo` (no SQL); the
e2e suite uses an in-memory `iamRepo` and does not exercise `AcceptInvitation` at
all; there is **zero** real-Postgres integration coverage of invitations. So
`InviteUser`/`AcceptInvitation` are broken in any real deployment — consistent
with the product being pre-launch.

**A second drift bug, same query:** `CreateInvitation`'s `INSERT` omits `role_id`
(columns are `id, tenant_id, email, invited_by, token, expires_at`).
`Service.InviteUser` validates `inv.RoleID` but the value is never persisted, so
`role_id` is always NULL and `AcceptInvitation` always sees `inv.RoleID == nil`
— the invited-role feature is dead. This must be fixed here, or #148's
transactional role grant tests a path that never carries a role.

### Grounding facts (measured on `main`)

- Highest iam migration is `023`; this adds `024`.
- `Invitation.Status` (`services/iam/models.go:145`) is the mapped Go field;
  `GetInvitationByToken` already maps `row.Status` → `inv.Status` and
  `row.RoleID.Valid` → `inv.RoleID` (`services/iam/db/postgres.go:714`).
- `Service.InviteUser` (`services/iam/service.go`) validates the role, mints a
  token, sets `inv.Status = "pending"` and a 7-day expiry, then calls
  `repo.CreateInvitation`. `Service.AcceptInvitation` checks `inv.Status ==
  "accepted"` and expiry, then creates the user and (when `inv.RoleID != nil`)
  grants the role.
- `pgtype` is already imported in `db/postgres.go`.
- `users` already has `UNIQUE (tenant_id, email)`
  (`migrations/iam/002_create_users.up.sql:15`) — the constraint #148's stuck
  state collides on; invitations has no equivalent.

### The authoritative side

The schema's `status` enum is the model the Go layer already speaks
(`Invitation.Status`, the service's `== "accepted"` check). The queries drifted
to `accepted_at`. Align the **queries to the `status` schema** — no new column,
no Go-model churn. (Adding an `accepted_at` column instead would duplicate state
with `status` and force Go-model changes; rejected.)

## Design

### 1. Migration `024` — the unique constraint the upsert needs

`migrations/iam/024_invitations_tenant_email_unique.up.sql`:

```sql
-- CreateInvitation upserts on (tenant_id, email) so a re-invite refreshes the
-- token; that ON CONFLICT target requires a UNIQUE constraint, which the
-- original table lacked (only token was UNIQUE). Replace the non-unique lookup
-- index with the unique constraint (its index serves the same lookups).
DROP INDEX IF EXISTS idx_invitations_email;
ALTER TABLE invitations
    ADD CONSTRAINT invitations_tenant_email_unique UNIQUE (tenant_id, email);
```

`024_invitations_tenant_email_unique.down.sql`:

```sql
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_tenant_email_unique;
CREATE INDEX idx_invitations_email ON invitations (tenant_id, email);
```

The `Migration Validate (up + down)` CI job gates this.

### 2. Align the three queries to `status` (`services/iam/db/queries.sql`)

```sql
-- name: CreateInvitation :one
INSERT INTO invitations (id, tenant_id, email, invited_by, token, expires_at, role_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, email) DO UPDATE
    SET token = EXCLUDED.token, expires_at = EXCLUDED.expires_at,
        status = 'pending', role_id = EXCLUDED.role_id
RETURNING *;

-- name: GetInvitationByToken :one
SELECT * FROM invitations
WHERE token = $1 AND status = 'pending' AND expires_at > now();

-- name: AcceptInvitation :exec
UPDATE invitations SET status = 'accepted' WHERE id = $1;
```

`status` is only ever set to values inside the CHECK set (`'pending'` on
insert/conflict via default and the explicit reset, `'accepted'` on accept), so
the constraint is respected. `GetInvitationByToken`'s `status = 'pending' AND
expires_at > now()` mirrors the prior intent of `accepted_at IS NULL AND
expires_at > now()` — behavior preserved, so the service's redundant
status/expiry checks stay as harmless defense (not refactored here).

Then **`sqlc generate`** regenerates `services/iam/db/queries.sql.go`;
`CreateInvitationParams` gains a `RoleID` field (nullable `uuid` →
`pgtype.UUID`). Generated files are never hand-edited; Codegen Freshness gates
this.

### 3. `Postgres.CreateInvitation` — persist the role

`services/iam/db/postgres.go`: populate the new `RoleID` param from
`inv.RoleID`, as a nullable `pgtype.UUID` (mirror of the reverse mapping already
in `GetInvitationByToken`):

```go
params := CreateInvitationParams{
    ID:        inv.ID,
    TenantID:  inv.TenantID,
    Email:     inv.Email,
    InvitedBy: inv.InvitedBy,
    Token:     inv.Token,
    ExpiresAt: inv.ExpiresAt,
    RoleID:    roleIDToPg(inv.RoleID), // pgtype.UUID{Valid:false} when nil
}
```

`roleIDToPg(*uuid.UUID) pgtype.UUID` is a small local helper (Valid=false when
nil, else Bytes set + Valid=true). The exact generated field type
(`pgtype.UUID`) is confirmed against the regenerated code during
implementation.

### 4. No Go-model or service change

`Invitation.Status` is already the field, `InviteUser`/`AcceptInvitation` already
use it. Nothing outside the db layer changes.

## Testing

### Integration (new: `tests/integration/iam_invitation_roundtrip_test.go`)

`//go:build integration`, `pkg/testdb.Open(t)` (SKIPs without `THITTAM_TEST_DSN`;
CI's real-Postgres job is authoritative). Reuses the harness in
`tests/integration/iam_tenant_isolation_test.go` — `seedIAMTenant`, the
`noopTokenIssuer` (#183) — and builds `iam.NewService` over the real
`iamdb.NewPostgres(pool)`. Adds local helpers to seed an inviter user and a role,
and to read invitation `status` / `user_roles`.

**Round-trip (the core, first-ever real-DB exercise):**
- Seed tenant A, an inviter user, and a role `R` in tenant A.
- `InviteUser(&Invitation{TenantID: A, Email: e, RoleID: &R, InvitedBy: inviter})`
  → no error; returns a token. Proves the `CreateInvitation` INSERT (incl.
  `role_id`) and the `ON CONFLICT` target both work.
- `repo.GetInvitationByToken(token)` → returns the invitation with `RoleID == R`
  and `Status == "pending"`. Proves the SELECT works and `role_id` persisted.
- `svc.AcceptInvitation(ctx, token, pw)` → succeeds; assert (a) a `users` row
  exists for `e` in tenant A, (b) a `user_roles` row grants `R` to that user
  (the invited role is actually applied), (c) the invitation is now
  `status = 'accepted'`. Proves the UPDATE works and the full role round-trip.

**Re-invite upsert:**
- `InviteUser` again for the same `(A, e)` → no error; assert exactly **one**
  invitation row for `(A, e)` and that its token changed. Proves the upsert
  against the new `UNIQUE (tenant_id, email)` constraint.

### Gates

`Migration Validate (up + down)`, `Codegen Freshness (sqlc)`, whole-tree
`go build ./...`, and `go test ./services/iam/... -race` (the existing unit
tests keep passing — they use `mockRepo` and are unaffected by SQL changes).

## Non-goals

- **No #148 work** — transactionality is the follow-up PR; this only makes the
  path correct against real Postgres.
- **No new invitation feature**, no revoke/list, no expiry sweeper.
- **No Go-model change** and no reviving the service's redundant status/expiry
  checks — behavior is preserved, only the SQL is corrected.
- **No `accepted_at` column** — the `status` enum is authoritative.

## Review weight

Touches `iam`, a schema migration, and an auth-adjacent path (invitations) →
senior engineer + 2 approvals per CLAUDE.md. Whole-branch review on the most
capable model.
