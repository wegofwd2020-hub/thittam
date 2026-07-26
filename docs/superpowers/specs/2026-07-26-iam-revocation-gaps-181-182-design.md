# Session-revocation gaps: UpdateUser + SuspendTenant (#181 / #182) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issues:** #181 (UpdateUser leaves sessions alive on deactivation), #182 (SuspendTenant leaves every tenant user's sessions alive) — both #154 follow-ups
**Branch:** `fix/iam-revocation-gaps-181-182` off `main` (bundled — both close #154 revocation gaps)
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

Close the two session-revocation gaps the #154 review filed:
1. **#181** — `UpdateUser` can set `status` to a login-blocking value (`deactivated`/`invited`)
   but never revokes sessions, unlike `DeactivateUser`. Wire `RevokeAllForUser` into it.
2. **#182** — `SuspendTenant` blocks new logins but leaves every user in the tenant with a
   working refresh token for the full window. Add a per-**tenant** generation counter
   (`iam:tenantgen:<tid>`) mirroring the #154 per-user counter, checked in `Refresh`, and a
   `RevokeAllForTenant` primitive wired into `SuspendTenant`.

## Context (grounding facts, `main` @ c84a59f)

- **#154 machinery** (`pkg/auth/jwt.go`): `refreshPayload{UserID, TenantID, ..., Generation int64}`
  (line 47-60 — **`TenantID` already present**, so #182 needs no payload plumbing beyond a new
  gen field). `Issue` (135) reads `currentGeneration` then calls `issueWithGeneration(ctx, result, gen)`
  (148). `Refresh` (193) re-reads the gen, rejects with `ErrSessionRevoked` on `payload.Generation != gen`
  (`!=`, NOT `<` — a missing key reads 0; `<` would accept a stale token against a reset counter),
  then **carries the validated `gen` forward via `issueWithGeneration` — NOT `Issue`** (the #154 TOCTOU
  fix: `Issue` re-reads the counter, reopening the revoke-mid-refresh window). `RevokeAllForUser` (249)
  = `INCR iam:usergen:<uid>` + housekeeping `EXPIRE`. `currentGeneration` (374): `GET`, `redis.Nil`→0.
  Key prefix `usergenKeyPrefix = "iam:usergen:"` (jwt.go:32).
- **`JWTIssuer` holds only `redis.Cmdable`, no DB handle** (jwt.go:65-70; `pkg/auth` has zero DB imports).
  A tenant counter MUST be Redis, mirroring `usergen`.
- **`auth.TokenIssuer` interface** (`pkg/auth/token.go:31-48`): Issue/Refresh/Revoke/RevokeAllForUser/Validate.
  Depended on by `iam.Service.tokens` (service.go:135). **Four implementers** (adding a method breaks all):
  `*JWTIssuer` (jwt.go, has `var _` assert at :395), `mockTokenIssuer` (services/iam/service_test.go:338),
  `noopTokenIssuer` (tests/integration/iam_tenant_isolation_test.go:229, explicit `var _ auth.TokenIssuer`
  assert at :236), `stubTokenIssuer` (e2e/critical_path/helpers_test.go:86).
- **`DeactivateUser`** (service.go:333-343): repo write FIRST, then `s.tokens.RevokeAllForUser(ctx, id)`,
  revoke error wrapped + returned (never swallowed). Same shape in ChangePassword/RevokeRole. Both new
  wirings follow this ordering.
- **`UpdateUser`** (service.go:324-330): writes the user (incl. `status`) via `repo.UpdateUser`; no revoke.
  Repo SQL `UPDATE ... status = COALESCE(NULLIF($3,''), status)` — empty status is a no-op (prevents
  accidental reactivation, #139 slice B). Handler (handler.go:182-205) sets `Status: req.GetStatus()`.
  User statuses: `active, invited, deactivated` (models.go:17 + CHECK in migrations/iam/002).
  **`UpdateUser` is the ONLY reactivation path — no `ActivateUser`/`ReactivateUser` exists** (grep-confirmed),
  so #181 must NOT strip the status field; it revokes on a deactivating change instead.
- **`SuspendTenant`** (service.go:613-656): writes `status = TenantStatusSuspended` via
  `repo.UpdateTenantStatus`; no revoke. `pkg/auth/local.go:74-76` blocks new logins for a suspended tenant
  (`ErrTenantSuspended`) but the refresh path never re-reads tenant status. No `ResumeTenant` exists →
  no reactivation trap for #182. Sibling lifecycle methods (ClearLegalHold/SetRetention/Advance/purge) are
  either not status-changing or already-suspended → out of scope.
- **Issuer tests** (`pkg/auth/jwt_test.go`): miniredis via `testIssuer(t)` / `testIssuerWithClient(t)`.
  usergen suite: `_RejectsPriorRefreshToken` (498), `_NewTokenStillWorks` (513),
  `_CounterResetStillRejects` (528, uses `mr.Del`), `_ScopedToUser` (552), TOCTOU
  `TestJWTIssuer_Refresh_TOCTOU_RevokeMidRefresh` (136) via `midRefreshRevokeHook` redis.Hook (93-121).
- **iam revoke tests** (`services/iam/service_test.go`): `mockTokenIssuer{revokeAllForUserFn}` (338);
  3-part pattern per caller — `_RevokesAllSessions` (capture), `_RevokeFailure_IsReported` (revoke errors
  surface). E.g. `TestDeactivateUser_RevokesAllSessions` (1568).

## Design

### #181 — `UpdateUser` revokes on a deactivating status change

`services/iam/service.go` `UpdateUser` — after the repo write succeeds, revoke iff the caller set a
login-blocking status:
```go
func (s *Service) UpdateUser(ctx context.Context, user *User) (*User, error) {
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: update user %s: %w", user.ID, err)
	}
	// UpdateUser is a second path to set status (handler passes req.Status straight
	// through). A non-active status blocks new logins but, like DeactivateUser before
	// #154, leaves live refresh tokens working — revoke them. Empty status is a
	// no-op at the repo (leave-alone), and "active" is reactivation, so neither revokes.
	if user.Status != "" && user.Status != "active" {
		if err := s.tokens.RevokeAllForUser(ctx, user.ID); err != nil {
			return nil, fmt.Errorf("iam: update user %s: revoke sessions: %w", user.ID, err)
		}
	}
	return user, nil
}
```
No interface/handler/migration change. `RevokeAllForUser` already exists.

### #182 — per-tenant generation counter + `RevokeAllForTenant`

**1. Payload + key + helper** (`pkg/auth/jwt.go`):
- `refreshPayload` gains `TenantGeneration int64 \`json:"tenant_generation"\`` next to `Generation`.
- New const `tenantgenKeyPrefix = "iam:tenantgen:"`.
- `currentTenantGeneration(ctx, tenantID uuid.UUID) (int64, error)` — mirror `currentGeneration`
  (`GET tenantgenKeyPrefix+tid`, `redis.Nil`→0).

**2. Carry both generations (TOCTOU-safe)** — change the internal `issueWithGeneration` to take both
via a small struct (keeps the single validated read-set flowing forward; unexported, no interface ripple):
```go
// generations is the {user, tenant} token-generation pair embedded at issue time
// and re-validated on refresh. Carrying the already-validated pair forward (never
// re-reading in Refresh) is the #154 TOCTOU fix, now for both dimensions.
type generations struct {
	User   int64
	Tenant int64
}
```
- `issueWithGeneration(ctx, result, gens generations)` — embeds `Generation: gens.User`,
  `TenantGeneration: gens.Tenant` in the refresh payload.
- `Issue` reads both: `gens := generations{User: currentGeneration(...), Tenant: currentTenantGeneration(...)}`
  then `issueWithGeneration(ctx, result, gens)`.
- `Refresh` reads both live counters, rejects if **either** diverges, then carries the validated pair
  forward:
```go
	userGen, err := j.currentGeneration(ctx, payload.UserID)
	// ... err handling ...
	tenantGen, err := j.currentTenantGeneration(ctx, payload.TenantID)
	// ... err handling ...
	if payload.Generation != userGen || payload.TenantGeneration != tenantGen {
		return nil, ErrSessionRevoked
	}
	return j.issueWithGeneration(ctx, &AuthResult{...}, generations{User: userGen, Tenant: tenantGen})
```
Reuse `ErrSessionRevoked` for either divergence — the client re-authenticates; login then surfaces
`ErrTenantSuspended` if the tenant is actually suspended. (Refresh cannot know *why* a counter bumped.)

**3. `RevokeAllForTenant`** (`pkg/auth/jwt.go`) — mirror `RevokeAllForUser` on `tenantgenKeyPrefix+tid`
(one `INCR` + housekeeping `EXPIRE` in a pipeline). O(1) regardless of tenant size.

**4. Interface + 4 implementers** (`pkg/auth/token.go` + doubles): add
`RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error` to `auth.TokenIssuer`; implement on
`*JWTIssuer` (real) and add to the 3 test stubs (`mockTokenIssuer` with a `revokeAllForTenantFn` field,
`noopTokenIssuer`, `stubTokenIssuer`). **Whole-tree `go vet ./...`** proves all four updated
(the `var _ auth.TokenIssuer` asserts fail to build first otherwise).

**5. Wire into `SuspendTenant`** (`services/iam/service.go`) — after the `UpdateTenantStatus` write
succeeds (and before/after the audit log — after the status write, consistent with DeactivateUser
ordering; place it right after the write so a revoke failure aborts before the audit event):
```go
	if err := s.repo.UpdateTenantStatus(ctx, id, TenantStatusSuspended, holdUntil, freezeReason); err != nil {
		return nil, fmt.Errorf("iam: suspend tenant %s: %w", id, err)
	}
	if err := s.tokens.RevokeAllForTenant(ctx, id); err != nil {
		return nil, fmt.Errorf("iam: suspend tenant %s: revoke sessions: %w", id, err)
	}
```

## Testing

- **pkg/auth** (`jwt_test.go`): mirror the usergen suite for tenantgen — `RevokeAllForTenant`
  rejects a prior refresh token; a new token issued after revoke works; counter-reset
  (`mr.Del(tenantgenKeyPrefix+tid)`) still rejects (`!=` proof); revocation is scoped to the tenant
  (another tenant's session survives); a TOCTOU test with a redis.Hook keyed on `iam:tenantgen:<tid>`
  racing a `RevokeAllForTenant` against a `Refresh` (mirror `midRefreshRevokeHook`). Also: an existing
  Refresh/Issue round-trip still passes with the added tenantgen field (payload back-compat — a token
  minted before this change has `TenantGeneration: 0`, and a never-bumped tenant reads 0, so `0==0`
  passes; add a test asserting a pre-existing-shaped payload still refreshes).
- **services/iam** (`service_test.go`): add `revokeAllForTenantFn` to `mockTokenIssuer` + the method.
  `TestUpdateUser_RevokesSessionsOnDeactivate` (status `deactivated` → RevokeAllForUser called with the
  user ID); `TestUpdateUser_NoRevokeOnActiveOrEmpty` (status `active` and `""` → NOT called);
  `TestUpdateUser_RevokeFailure_IsReported`; `TestSuspendTenant_RevokesAllSessions` (RevokeAllForTenant
  called with the tenant ID); `TestSuspendTenant_RevokeFailure_IsReported`. Follow the existing 3-part
  `DeactivateUser` pattern.
- Gates: `go test ./pkg/auth/... ./services/iam/... -race`; **`go vet ./...` (whole tree)** — the
  interface change ripples to the 3 test doubles in other packages that `go build` skips;
  `go build ./...`; `gofmt -l` on touched Go files. No proto/sqlc/migration → no codegen gates.
  iam coverage floor ≥ 85% — keep it.

## Non-goals

- No `ActivateUser`/`ReactivateUser` RPC (out of scope; UpdateUser stays the reactivation path).
- No `ResumeTenant`/unsuspend (none exists; when built it should reset/rely on the counter, not un-bump).
- No revocation on the sibling tenant-lifecycle methods (ClearLegalHold/SetRetention/Advance/purge —
  already-suspended or non-status-changing). Tenant purge (#92) is a separate concern.
- No migration/proto/sqlc; no change to access-token TTL semantics (revocation bounds at the refresh
  boundary, as in #154).
- `UpdateUser` revokes on any non-active status set, NOT a diff against prior status (the service has no
  cheap prior-status read; revoking on an idempotent `deactivated`-set of an already-deactivated user is
  harmless).

## Review weight

`iam` + `pkg/auth` (session security) + a `TokenIssuer` interface change → security-sensitive; senior
engineer required per CLAUDE.md. Standard 2 approvals + senior. Whole-branch review on the most capable
model. Mind the #154 TOCTOU discipline (carry validated gens forward; never re-read in Refresh) and the
`!=`-not-`<` rationale — both apply to the new tenant dimension.
