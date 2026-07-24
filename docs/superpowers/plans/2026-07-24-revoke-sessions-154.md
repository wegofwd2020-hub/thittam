# Revoke Sessions on Password Change, Deactivation and Role Revocation (#154) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a revoke-all-sessions primitive backed by a per-user generation counter in Redis, and call it from `ChangePassword`, `DeactivateUser`, and `RevokeRole`.

**Architecture:** Two tasks. Task 1 builds the primitive inside `pkg/auth` (the interface method, the counter, the refresh-time check, and all three implementers — they must land together or the build breaks). Task 2 wires the three `services/iam` callers. No migration, no new dependency.

**Tech Stack:** Go 1.25, Redis (`redis.Cmdable`), JWT (RS256), miniredis for tests.

## Global Constraints

- **`pkg/auth` must stay database-free.** `JWTIssuer` holds only `privateKey`, `rdb`, `accessTTL`, `refreshTTL`. Do NOT add a repository/DB handle. `auth.UserStore.GetUserByID` exists and is documented "used for refresh token validation" but has **zero callers** — leave it that way; wiring it is exactly the dependency this design avoids.
- **The generation comparison is `!=`, never `<`.** The counter key can vanish (TTL, flush, cold-replica failover) and a missing key reads as `0`. Under `<`, a stale token with generation `5` against a reset counter of `0` gives `5 < 0` → false → **accepted**, un-revoking sessions exactly when the store is least healthy. Under `!=` every divergence fails closed. This is the single most important line in the change.
- **`Refresh`'s signature does not change** — it stays `Refresh(ctx, refreshToken)`. The existing delegation tests (`TestRefreshToken_Delegates`, `TestLogout_RevokesToken`) must keep passing untouched.
- **Three implementers of `TokenIssuer`, all updated in Task 1's commit** (whole-tree grep confirmed exactly these): `JWTIssuer` (`pkg/auth/jwt.go`, compile-time assertion at `:316`), `mockTokenIssuer` (`services/iam/service_test.go:335`), `stubTokenIssuer` (`e2e/critical_path/helpers_test.go:86`). `go vet ./...` is the gate that catches the e2e one.
- **Revocation binds at the refresh boundary, not instantly.** Access tokens already issued stay valid for their 15-minute TTL — #138 verifies them in-process with no store lookup, deliberately. Do not add a per-request lookup.
- **No migration.** The counter lives in Redis; `users` is untouched.
- **DB safety:** never `docker compose … -v`/`down`/`up` on `infra/local/`. This slice needs no Postgres at all — `pkg/auth` tests use miniredis.
- Coverage floor: iam ≥ 85%.

---

## Task 1: the primitive in `pkg/auth`

**Files:**
- Modify: `pkg/auth/token.go:31-44` (interface), `pkg/auth/jwt.go` (constants, `refreshPayload`, `Issue`, `Refresh`, new `RevokeAllForUser`)
- Modify: `services/iam/service_test.go:335-364` (`mockTokenIssuer`)
- Modify: `e2e/critical_path/helpers_test.go:86-97` (`stubTokenIssuer`)
- Modify: `pkg/auth/jwt_test.go` (new tests)

**Interfaces:**
- Produces: `TokenIssuer.RevokeAllForUser(ctx context.Context, userID uuid.UUID) error`.
- `refreshPayload` gains `Generation int64 \`json:"generation"\``.

- [ ] **Step 1: Write the failing tests**

In `pkg/auth/jwt_test.go`, using the existing `testIssuer(t)` helper (returns `*JWTIssuer` and the `*miniredis.Miniredis` handle):

```go
func TestJWTIssuer_RevokeAllForUser_RejectsPriorRefreshToken(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)

	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err, "a refresh token issued before revoke-all must be rejected")
}

// A token issued AFTER the revocation must work — revoke-all must not wedge
// the user out permanently.
func TestJWTIssuer_RevokeAllForUser_NewTokenStillWorks(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)
	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
}

// THE test that pins `!=` over `<`. With the counter key deleted (TTL expiry,
// flush, cold-replica failover) a stale token must still be rejected. Written
// against `<` this fails, because 1 < 0 is false and the token is accepted.
func TestJWTIssuer_RevokeAllForUser_CounterResetStillRejects(t *testing.T) {
	iss, mr := testIssuer(t)
	ctx := context.Background()

	// Baseline bump FIRST, so the issued token carries a NONZERO generation.
	// Without this the payload would carry 0, a missing key also reads 0, and
	// both `0 != 0` and `0 < 0` are false — the test would fail even against a
	// correct implementation and would not discriminate between the operators.
	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)
	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	// Simulate the counter disappearing while the refresh token is still alive.
	mr.Del(usergenKeyPrefix + fixtureUserID.String())

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err, "a reset counter must not resurrect a revoked session")
}

// Revoking one user must not touch another's sessions.
func TestJWTIssuer_RevokeAllForUser_ScopedToUser(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()
	other := uuid.MustParse("a1000000-0000-0000-0000-0000000000ff")

	pair, err := iss.Issue(ctx, &AuthResult{UserID: other, TenantID: fixtureTenantID, Email: "o@example.com"})
	require.NoError(t, err)

	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err, "revoking one user must not revoke another's sessions")
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./pkg/auth/ -run RevokeAllForUser -v`
Expected: compile failure — `RevokeAllForUser` and `usergenKeyPrefix` do not exist yet.

- [ ] **Step 3: Add the constant, the payload field, and a new sentinel**

`pkg/auth/jwt.go` — beside `refreshKeyPrefix` (`:27`):

```go
	// usergenKeyPrefix namespaces the per-user token generation counter.
	// Bumping a user's generation invalidates every refresh token issued
	// before the bump (#154).
	usergenKeyPrefix = "iam:usergen:"
```

Extend `refreshPayload` (`:44-51`) with:

```go
	// Generation is the user's token generation at issue time. Refresh compares
	// it against the live counter and rejects on ANY difference (#154).
	Generation int64 `json:"generation"`
```

Add a sentinel beside the existing `ErrRefreshTokenNotFound`:

```go
	// ErrSessionRevoked is returned when a refresh token was issued before a
	// revoke-all (password change, deactivation, role revocation).
	ErrSessionRevoked = errors.New("auth: session revoked")
```

- [ ] **Step 4: Read the generation in `Issue`**

`pkg/auth/jwt.go` `Issue` (`:126-168`) — before the `issueRefreshToken` call, fetch the current generation and thread it into the payload:

```go
	gen, err := j.currentGeneration(ctx, result.UserID)
	if err != nil {
		return nil, err
	}

	refreshToken, err := j.issueRefreshToken(ctx, &refreshPayload{
		UserID:      result.UserID,
		TenantID:    result.TenantID,
		Email:       result.Email,
		Roles:       result.Roles,
		Permissions: result.Permissions,
		AuthMethod:  result.AuthMethod,
		Generation:  gen,
	})
```

Add the helper (missing key ⇒ generation `0`):

```go
// currentGeneration returns the user's token generation, 0 if never bumped.
func (j *JWTIssuer) currentGeneration(ctx context.Context, userID uuid.UUID) (int64, error) {
	gen, err := j.rdb.Get(ctx, usergenKeyPrefix+userID.String()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("auth: jwt: read token generation: %w", err)
	}
	return gen, nil
}
```

- [ ] **Step 5: Check the generation in `Refresh`**

`pkg/auth/jwt.go` `Refresh` (`:172-187`) — after `consumeRefreshToken`, before re-issuing:

```go
	gen, err := j.currentGeneration(ctx, payload.UserID)
	if err != nil {
		return nil, err
	}
	// Deliberately `!=`, not `<`. The counter key can vanish (TTL, flush,
	// failover) and a missing key reads as 0; under `<` a stale token with a
	// higher generation would be ACCEPTED, un-revoking sessions exactly when
	// the store is least healthy. Any divergence fails closed.
	if payload.Generation != gen {
		return nil, ErrSessionRevoked
	}
```

The token has already been consumed (deleted) at this point, which is correct — a rejected refresh burns the token.

- [ ] **Step 6: Add `RevokeAllForUser`**

`pkg/auth/jwt.go`, beside `Revoke`:

```go
// RevokeAllForUser invalidates every outstanding refresh token for a user by
// bumping their generation counter. O(1) regardless of session count.
//
// Access tokens already issued remain valid until they expire (accessTTL) —
// this bounds a compromised session at the refresh boundary, not instantly.
func (j *JWTIssuer) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	key := usergenKeyPrefix + userID.String()
	// EXPIRE alongside INCR so counters do not accumulate forever. The window
	// is longer than refreshTTL so a counter outlives every token it governs;
	// correctness does not depend on it (see the `!=` comparison in Refresh).
	if _, err := j.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, j.refreshTTL*2)
		return nil
	}); err != nil {
		return fmt.Errorf("auth: jwt: revoke all sessions: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: Add the method to the interface and both doubles**

`pkg/auth/token.go` `TokenIssuer` (`:31-44`):

```go
	// RevokeAllForUser invalidates every outstanding refresh token for a user
	// (password change, deactivation, role revocation — #154).
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
```

`services/iam/service_test.go` `mockTokenIssuer` (`:335-364`) — add the fn field and method, matching the file's existing style:

```go
	revokeAllForUserFn func(ctx context.Context, userID uuid.UUID) error
```
```go
func (m *mockTokenIssuer) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	if m.revokeAllForUserFn != nil {
		return m.revokeAllForUserFn(ctx, userID)
	}
	return nil
}
```

`e2e/critical_path/helpers_test.go` `stubTokenIssuer` (`:86-97`):

```go
func (stubTokenIssuer) RevokeAllForUser(_ context.Context, _ uuid.UUID) error { return nil }
```

(Check `token.go` imports `uuid`; add it if the interface file does not already.)

- [ ] **Step 8: Run the gate**

Run: `go build ./... && go vet ./... && go test ./pkg/auth/... ./services/iam/... ./e2e/...`
Expected: all pass, including the four new tests. `go vet ./...` proves the e2e double was updated.

- [ ] **Step 9: Commit**

```bash
git add pkg/auth services/iam/service_test.go e2e/critical_path/helpers_test.go
git commit -m "feat(auth): add RevokeAllForUser via a per-user generation counter (#154)

Refresh re-issued purely from the Redis payload, so no action could end a
user's other sessions: a stolen refresh token survived a password change for
the full 7-day refresh window.

Adds iam:usergen:<uid>. Issue embeds the current generation in the refresh
payload; Refresh compares it against the live counter; RevokeAllForUser is a
single INCR, O(1) in session count.

The comparison is != rather than <, deliberately. The counter key can vanish
(TTL, flush, cold-replica failover) and a missing key reads as 0; under < a
stale token carrying a higher generation would be accepted, un-revoking
sessions exactly when the store is least healthy. Any divergence now fails
closed, and a Redis flush logs everyone out rather than resurrecting revoked
sessions.

The counter lives in Redis, not on the user row, because pkg/auth holds no
database handle by design and every refresh would otherwise gain a DB read."
```

---

## Task 2: wire the three callers in `services/iam`

**Files:**
- Modify: `services/iam/service.go` — `ChangePassword` (`:332-350`), `DeactivateUser` (`:324-329`), `RevokeRole` (`:378-389`)
- Modify: `services/iam/service_test.go` — new assertions

**Interfaces:**
- Consumes: `s.tokens.RevokeAllForUser(ctx, userID)` from Task 1.

- [ ] **Step 1: Write the failing tests**

In `services/iam/service_test.go`, add one per caller. They assert the primitive is invoked with the right user id — a test that only checks the happy path would pass without the wiring:

```go
func TestChangePassword_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	tokens := &mockTokenIssuer{
		revokeAllForUserFn: func(_ context.Context, userID uuid.UUID) error {
			revokedFor = userID
			return nil
		},
	}
	// ... build the service with a repo whose GetUserByID returns a record whose
	// PasswordHash the verifier accepts, and a working hasher + UpdatePasswordHash ...

	require.NoError(t, svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "old", "new"))
	assert.Equal(t, fixedUserID, revokedFor, "changing a password must revoke every session")
}
```

Mirror it for `TestDeactivateUser_RevokesAllSessions` and `TestRevokeRole_RevokesAllSessions`.

Add one failure-path test proving the error is surfaced, not swallowed:

```go
func TestChangePassword_RevokeFailure_IsReported(t *testing.T) {
	t.Parallel()
	tokens := &mockTokenIssuer{
		revokeAllForUserFn: func(context.Context, uuid.UUID) error { return errors.New("redis down") },
	}
	// ... same service setup ...
	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "old", "new")
	require.Error(t, err, "a failed revocation must not be silently swallowed")
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./services/iam/ -run 'RevokesAllSessions|RevokeFailure' -v`
Expected: FAIL — none of the three services calls the primitive yet, so `revokedFor` stays `uuid.Nil`.

- [ ] **Step 3: Wire `ChangePassword`**

`services/iam/service.go` — after the successful `UpdatePasswordHash`, before returning:

```go
	// Changing a password is the action a user takes to end someone else's
	// access, so it must end every session — including this caller's. The
	// user re-authenticates on the device they just used.
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		// Reported, not swallowed. Note this failure is self-limiting: Refresh
		// also needs Redis, so if the counter bump failed because Redis is
		// unreachable, no refresh can succeed either.
		return fmt.Errorf("iam: change password: revoke sessions: %w", err)
	}
	return nil
```

Order matters: revoke **after** the hash is persisted, so a failed password update does not log the user out.

- [ ] **Step 4: Wire `DeactivateUser`**

```go
func (s *Service) DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if err := s.repo.DeactivateUser(ctx, tenantID, id); err != nil {
		return fmt.Errorf("iam: deactivate user %s: %w", id, err)
	}
	// A deactivated account holding live sessions for up to the refresh window
	// defeats deactivation (#154).
	if err := s.tokens.RevokeAllForUser(ctx, id); err != nil {
		return fmt.Errorf("iam: deactivate user %s: revoke sessions: %w", id, err)
	}
	return nil
}
```

- [ ] **Step 5: Wire `RevokeRole`**

```go
	if err := s.repo.RevokeRole(ctx, userID, roleID); err != nil {
		return fmt.Errorf("iam: revoke role %s from user %s: %w", roleID, userID, err)
	}
	// Refresh re-issues roles from the stored payload, so without this the
	// revoked role survives to the refresh window rather than the 15-minute
	// access TTL (#154).
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("iam: revoke role %s from user %s: revoke sessions: %w", roleID, userID, err)
	}
	return nil
```

- [ ] **Step 6: Run the gate**

Run: `go build ./... && go vet ./... && go test ./services/iam/... ./pkg/auth/... ./e2e/...`
Expected: all pass. Existing `TestChangePassword_Success`, `TestDeactivateUser_Success`, `TestRevokeRole_*` must still pass — `mockTokenIssuer`'s default `RevokeAllForUser` returns nil, so they are unaffected.

- [ ] **Step 7: Commit**

```bash
git add services/iam
git commit -m "fix(iam): revoke all sessions on password change, deactivation, role revocation (#154)

Each of these is supposed to end access and none of them did. A stolen refresh
token survived a password change for the full refresh window; a deactivated
user kept working sessions for a week; a revoked role stayed live past the
15-minute access TTL because Refresh re-issues roles from the stored payload.

All three now call TokenIssuer.RevokeAllForUser. ChangePassword revokes every
session including the caller's own — no exceptions to reason about, and the
user re-authenticates on the device they just used.

Revocation failures are returned, not swallowed. The failure is self-limiting:
Refresh also requires Redis, so a counter bump that failed because Redis is
unreachable leaves no refresh path working either.

Closes #154."
```

---

## Self-Review

- **Spec coverage:** generation counter in Redis (T1 S3-S6) ✓; `!=` not `<` with the reset case as a required test (T1 S1/S5) ✓; `RevokeAllForUser` on the interface + all three implementers in one commit (T1 S7) ✓; all three callers wired (T2 S3-S5) ✓; `ChangePassword` revokes the caller's own session too (T2 S3) ✓; no migration, no DB in `pkg/auth` (Global Constraints) ✓; the issue's stated test — a token issued before the change is rejected after (T1 S1) ✓.
- **Placeholder scan:** every step carries real code. Task 2 Step 1's service-construction lines are elided as `// ...` because the existing `TestChangePassword_Success` at `service_test.go:574-602` already shows the exact fixture wiring to copy — the assertion, which is the point, is written out in full.
- **Type consistency:** `Generation` is `int64` throughout (Redis `INCR` returns `int64`); `RevokeAllForUser(ctx, userID uuid.UUID) error` is identical on the interface, `JWTIssuer`, `mockTokenIssuer`, and `stubTokenIssuer`. `Refresh`'s signature is untouched, so the existing delegation tests keep compiling.
- **Ordering:** the interface change and all three implementers are in Task 1's single commit — splitting them would leave a non-building commit. Revocation happens *after* the state change in every caller, so a failed write never logs anyone out.
