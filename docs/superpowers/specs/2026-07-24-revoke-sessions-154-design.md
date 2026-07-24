# Revoke Sessions on Password Change, Deactivation and Role Revocation (#154) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issue:** #154 (changing a password revokes no sessions)
**Branch:** `fix/revoke-sessions-154` off `main` (`becfb2b`)
**Migration:** none

## Goal

Give the platform a revoke-all-sessions primitive and call it from the three
actions that are supposed to end someone's access: `ChangePassword`,
`DeactivateUser`, and `RevokeRole`.

## Context

Nothing in the codebase revokes a user's sessions. `Service.Logout` revokes the
**one** refresh token presented; `ChangePassword` revokes nothing. So:

1. An attacker obtains a refresh token.
2. The victim notices and changes their password.
3. **The attacker's refresh token keeps working** — for the full refresh window.

`JWTIssuer.Refresh` re-issues purely from the Redis payload:

```go
payload, err := j.consumeRefreshToken(ctx, refreshToken)
// ...
return j.Issue(ctx, &AuthResult{
    UserID: payload.UserID, TenantID: payload.TenantID,
    Roles: payload.Roles, Permissions: payload.Permissions, // ...
})
```

It never re-reads the user record, so neither the new password hash nor any role
change is consulted. Changing a password is the one action a user takes
*specifically* to end someone else's access, and today it does not.

### Why the issue's proposed fix is the wrong shape here

#154 offers two designs: a per-user token index, or a generation counter on the
**user record** compared at refresh. Grounding rules the second one out on
layering grounds:

```go
type JWTIssuer struct {
	privateKey *rsa.PrivateKey
	rdb        redis.Cmdable
	accessTTL  time.Duration
	refreshTTL time.Duration
}
```

`pkg/auth` holds **no database handle at all** — deliberately. Putting the
counter in Postgres would force a DB dependency into a package that has none and
add a DB round trip to every refresh. And refresh tokens are stored one key per
token (`iam:refresh:<token>`), with no per-user index, so enumeration would mean
either a `SCAN` across every token in the system or a new secondary structure to
maintain and garbage-collect.

The counter idea is right; its *location* was wrong. Redis is already this
package's dependency.

There is a tempting-looking hook: `auth.UserStore.GetUserByID` is documented
"used for refresh token validation" — but it has **zero call sites inside
`pkg/auth`**. It was declared for exactly this job and never wired. Using it now
is precisely what would drag the DB dependency in, so it stays unused; the
counter design deliberately does not touch it.

## Design

### 1. The primitive: a per-user generation counter in Redis

Key `iam:usergen:<user_id>` holds an integer.

- **`Issue`** reads the current generation (missing ⇒ `0`) and stores it in the
  refresh payload alongside the existing fields.
- **`Refresh`** re-reads the current generation and compares it with the payload's.
- **`RevokeAllForUser(ctx, userID)`** is a single `INCR`. O(1) regardless of how
  many sessions the user has.

`TokenIssuer` gains one method:

```go
// RevokeAllForUser invalidates every outstanding refresh token for a user.
// Access tokens already issued remain valid until they expire (15 min) —
// revocation bounds the session at the refresh boundary, not instantly.
RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
```

Single-session `Revoke` is unchanged and still works by deleting the token's key;
the two mechanisms are independent and compose.

### 2. The comparison must be `!=`, not `<`

This is the one subtle part. Compare payload generation to current generation
with **inequality**, rejecting on any divergence — not `payload < current`.

Reason: the counter key can disappear (TTL expiry, a Redis flush, a failover to a
cold replica). A missing key reads as `0`. Under `payload < current`, a stale
token carrying generation `5` against a reset counter of `0` computes `5 < 0` ⇒
**false** ⇒ *accepted* — the revocation silently un-does itself precisely when the
store is least healthy. Under `!=`, `5 != 0` ⇒ rejected. Every divergence fails
closed, and a Redis flush logs everyone out rather than resurrecting revoked
sessions.

The key is written with a TTL comfortably longer than the refresh window (and
re-set on each `INCR`) so counters do not accumulate forever. That TTL is a
housekeeping choice, not a correctness one — correctness comes from `!=`.

### 3. Callers

All three call the same primitive:

| caller | why |
|---|---|
| `Service.ChangePassword` | the action a user takes to end someone else's access |
| `Service.DeactivateUser` | a deactivated account keeping live sessions for a week defeats deactivation |
| `Service.RevokeRole` | `Refresh` re-issues roles from the stored payload, so a revoked role otherwise survives past the 15-minute access TTL |

**`ChangePassword` revokes every session including the caller's own.** No
exceptions and nothing to reason about: after the change, every pre-existing
refresh token is rejected. The user re-authenticates on the device they just
used, which is standard and is what someone changing a password under threat
expects.

### 4. What this does and does not bound

Revocation takes effect at the **refresh** boundary. An access token already
issued stays valid until it expires (15 minutes) — the JWT is verified in-process
against the public key (#138) with no store lookup, by design. So the guarantee
is: *a compromised session dies within the access-token TTL, instead of surviving
for the full refresh window.* Worth stating plainly in the spec so nobody reads
"revoke" as "instant".

Closing the 15-minute gap would mean a store lookup on every request, which
#138's fail-closed in-process verification deliberately avoids. Out of scope.

## Testing

- **The issue's stated test:** a refresh token issued before a password change is
  rejected after it. Same for deactivation and role revocation.
- **The reset case** — the reason `!=` was chosen: with a payload generation of
  `N` and the counter key **deleted**, refresh must be rejected. A test written
  against `<` passes while the system is exploitable, so this case is the one
  that actually pins the design.
- `Revoke` (single session) still works and does not disturb other sessions.
- A token issued *after* the revocation works normally — proving revoke-all
  doesn't wedge the user out permanently.
- Unit tests at the `pkg/auth` level (miniredis or the existing Redis test
  harness) plus service-level tests that each of the three callers invokes the
  primitive.

`TokenIssuer` gains a method, so **every implementer and test double must be
updated in the same commit** — there are exactly three, confirmed by whole-tree
grep: `JWTIssuer` (`pkg/auth/jwt.go`, with the compile-time assertion at `:316`),
`mockTokenIssuer` (`services/iam/service_test.go`), and the e2e `stubTokenIssuer`
(`e2e/critical_path/helpers_test.go`). Whole-tree `go vet ./...` is the gate that
catches the e2e one.

`Refresh`'s signature is unchanged, so the existing delegation tests
(`TestRefreshToken_Delegates`, `TestLogout_RevokesToken`) keep working as written
— the new behaviour is additive.

## Non-goals

- **No migration, no DB dependency in `pkg/auth`.**
- **No instant access-token revocation** (see §4) — that is a different design
  with a per-request cost #138 rejected.
- **No "active sessions" UI / selective session revocation.** The per-user index
  design would enable it; the counter does not. If that product need appears,
  it's an additive change on top, not a conflict.
- No change to `Logout`'s single-token behaviour.

## Review weight

Touches `iam` and `pkg/auth` (authentication) → senior engineer + 2 approvals per
CLAUDE.md. Whole-branch review on the most capable model.
