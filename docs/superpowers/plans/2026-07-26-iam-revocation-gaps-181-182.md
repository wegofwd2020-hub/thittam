# iam session-revocation gaps (#181 / #182) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close two #154 session-revocation gaps — `UpdateUser` must revoke sessions when it deactivates a user (#181), and `SuspendTenant` must revoke every session in the tenant via a new per-tenant generation counter (#182).

**Architecture:** #181 is a small wiring into `UpdateUser` reusing the existing `RevokeAllForUser`. #182 mirrors #154's per-user Redis counter with a per-tenant one (`iam:tenantgen:<tid>`) embedded in the refresh payload and checked in `Refresh`, plus a `RevokeAllForTenant` primitive added to the `auth.TokenIssuer` interface (breaks 4 implementers) and wired into `SuspendTenant`. Three tasks, each builds tree-wide.

**Tech Stack:** Go 1.25, `pkg/auth` (JWTIssuer over `redis.Cmdable`, miniredis in tests), `services/iam`, testify.

## Global Constraints

- **Revocation ordering:** repo write FIRST, then `s.tokens.Revoke*`, revoke error **wrapped and returned** (never swallowed) — exactly as `DeactivateUser` (service.go:333-343).
- **#154 discipline (applies to the new tenant dimension too):** `Refresh` compares `!=` NOT `<` (a missing counter key reads 0; `<` would accept a stale token against a reset counter — fail closed on any divergence). `Refresh` must carry the **already-validated** generations forward via `issueWithGeneration`, NEVER re-read via `Issue` (the #154 TOCTOU fix). Do not "simplify" Refresh back to `Issue`.
- **`pkg/auth` has NO database handle** — the tenant counter lives in Redis, mirroring `iam:usergen:`. Never add a DB dependency to the issuer.
- **Adding `RevokeAllForTenant` to `auth.TokenIssuer` breaks 4 implementers** — `*JWTIssuer` (real) + 3 test stubs: `mockTokenIssuer` (services/iam/service_test.go:338), `noopTokenIssuer` (tests/integration/iam_tenant_isolation_test.go:229, has a `var _ auth.TokenIssuer` assert), `stubTokenIssuer` (e2e/critical_path/helpers_test.go:86). `go build ./...` skips other packages' `_test.go`; only **whole-tree `go vet ./...`** catches the stubs. All four must be updated in the SAME commit as the interface change (Task 2).
- **Payload back-compat:** existing refresh tokens in Redis (minted before this change) deserialize with `TenantGeneration: 0`; a never-bumped tenant's counter also reads 0, so `0 == 0` passes — no forced logout on deploy. A Task 2 test must prove this.
- **#181 revoke rule:** revoke iff `user.Status != "" && user.Status != "active"` (deactivating to `deactivated`/`invited`). Empty status is a repo no-op; `active` is reactivation — neither revokes.
- **iam coverage floor ≥ 85%** — keep it. No migration/proto/sqlc → no codegen gates. `gofmt -l` on touched Go files in every gate.
- **Security-sensitive** (iam + auth + interface change) → senior review per CLAUDE.md.
- Commits Conventional-Commits (scope `iam`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `services/iam/service.go` | `UpdateUser` revoke (#181); `SuspendTenant` revoke (#182) | 1,3 |
| `services/iam/service_test.go` | UpdateUser + SuspendTenant revoke tests; mock `RevokeAllForTenant` | 1,2,3 |
| `pkg/auth/jwt.go` | tenantgen counter, payload field, Issue/Refresh, `RevokeAllForTenant` | 2 |
| `pkg/auth/token.go` | `TokenIssuer.RevokeAllForTenant` | 2 |
| `pkg/auth/jwt_test.go` | tenantgen suite + TOCTOU + back-compat | 2 |
| `tests/integration/iam_tenant_isolation_test.go` | `noopTokenIssuer.RevokeAllForTenant` | 2 |
| `e2e/critical_path/helpers_test.go` | `stubTokenIssuer.RevokeAllForTenant` | 2 |

---

### Task 1: #181 — UpdateUser revokes sessions on a deactivating status change

**Files:** Modify `services/iam/service.go`, `service_test.go`

**Interfaces:**
- Consumes: existing `TokenIssuer.RevokeAllForUser`, `mockTokenIssuer.revokeAllForUserFn`.

- [ ] **Step 1: Write failing service tests**

Add to `services/iam/service_test.go` (mirror `TestDeactivateUser_RevokesAllSessions` at :1568 — same `mockTokenIssuer{revokeAllForUserFn}` + `mockRepo{updateUserFn}` + `NewService(...)` construction; source the exact `NewService` arg list and `fixedTenantID`/`fixedUserID` from that test):
```go
func TestUpdateUser_RevokesSessionsOnDeactivate(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	called := false
	tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, id uuid.UUID) error {
		called = true
		revokedFor = id
		return nil
	}}
	repo := &mockRepo{updateUserFn: func(_ context.Context, _ *User) error { return nil }}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})

	_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID, Status: "deactivated"})
	require.NoError(t, err)
	assert.True(t, called, "deactivating via UpdateUser must revoke sessions")
	assert.Equal(t, fixedUserID, revokedFor)
}

func TestUpdateUser_NoRevokeOnActiveOrEmpty(t *testing.T) {
	t.Parallel()
	for _, st := range []string{"active", ""} {
		st := st
		t.Run("status="+st, func(t *testing.T) {
			t.Parallel()
			called := false
			tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, _ uuid.UUID) error {
				called = true
				return nil
			}}
			repo := &mockRepo{updateUserFn: func(_ context.Context, _ *User) error { return nil }}
			svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
			_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID, Status: st})
			require.NoError(t, err)
			assert.False(t, called, "profile edit / reactivation must NOT revoke")
		})
	}
}

func TestUpdateUser_RevokeFailure_IsReported(t *testing.T) {
	t.Parallel()
	tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, _ uuid.UUID) error {
		return errors.New("redis down")
	}}
	repo := &mockRepo{updateUserFn: func(_ context.Context, _ *User) error { return nil }}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID, Status: "deactivated"})
	require.Error(t, err)
}
```
(Confirm `mockRepo`'s update-user field name — the mock method is `UpdateUser`; the configurable field is likely `updateUserFn`. Confirm `NewService`'s exact parameter order from `TestDeactivateUser_RevokesAllSessions`. Adjust to compile; keep the assertions.)

- [ ] **Step 2: Run — expect FAIL** (`UpdateUser` doesn't revoke yet): `go test ./services/iam/ -run TestUpdateUser_`

- [ ] **Step 3: Implement**

In `services/iam/service.go` `UpdateUser` (:324-330), after the repo write:
```go
func (s *Service) UpdateUser(ctx context.Context, user *User) (*User, error) {
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: update user %s: %w", user.ID, err)
	}
	// UpdateUser is a second path that sets status (handler passes req.Status straight
	// through). A non-active status blocks new logins but, like DeactivateUser before
	// #154, leaves live refresh tokens working — revoke them. Empty status is a repo
	// no-op (leave-alone) and "active" is reactivation, so neither revokes. (#181)
	if user.Status != "" && user.Status != "active" {
		if err := s.tokens.RevokeAllForUser(ctx, user.ID); err != nil {
			return nil, fmt.Errorf("iam: update user %s: revoke sessions: %w", user.ID, err)
		}
	}
	return user, nil
}
```

- [ ] **Step 4: Run — expect PASS + gate**
```bash
go test ./services/iam/ -race && go vet ./... && go build ./...
gofmt -l services/iam/service.go services/iam/service_test.go
```

- [ ] **Step 5: Commit**
```bash
git add services/iam/service.go services/iam/service_test.go
git commit -m "fix(iam): revoke sessions when UpdateUser deactivates a user (#181)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: #182 mechanism — per-tenant generation counter + RevokeAllForTenant

**Files:** Modify `pkg/auth/jwt.go`, `token.go`, `jwt_test.go`, `services/iam/service_test.go` (mock), `tests/integration/iam_tenant_isolation_test.go`, `e2e/critical_path/helpers_test.go`

**Interfaces:**
- Produces: `TokenIssuer.RevokeAllForTenant(ctx, tenantID uuid.UUID) error`; `refreshPayload.TenantGeneration`; `mockTokenIssuer.revokeAllForTenantFn` (consumed by Task 3).

- [ ] **Step 1: Write failing pkg/auth tests**

Add to `pkg/auth/jwt_test.go`, mirroring the usergen suite (read `TestJWTIssuer_RevokeAllForUser_RejectsPriorRefreshToken` @:498, `_CounterResetStillRejects` @:528 with its `mr.Del`, `_ScopedToUser` @:552, and the TOCTOU `TestJWTIssuer_Refresh_TOCTOU_RevokeMidRefresh` @:136 with `midRefreshRevokeHook` @:93 — copy their exact `testIssuer`/`testIssuerWithClient` setup and `AuthResult` fixture, adding a distinct `TenantID`). Cover:
```go
// RevokeAllForTenant rejects a refresh token minted before the bump.
func TestJWTIssuer_RevokeAllForTenant_RejectsPriorRefreshToken(t *testing.T) { /* issue → RevokeAllForTenant(tid) → Refresh(old) == ErrSessionRevoked */ }

// A token issued AFTER the tenant revoke still refreshes.
func TestJWTIssuer_RevokeAllForTenant_NewTokenStillWorks(t *testing.T) { /* revoke → Issue → Refresh ok */ }

// `!=` (not `<`): after two revokes, deleting the tenantgen key still rejects the old token.
func TestJWTIssuer_RevokeAllForTenant_CounterResetStillRejects(t *testing.T) { /* revoke ×2, mr.Del(tenantgenKeyPrefix+tid), Refresh(old) == ErrSessionRevoked */ }

// Revocation is scoped: another tenant's session survives.
func TestJWTIssuer_RevokeAllForTenant_ScopedToTenant(t *testing.T) { /* two tenants, revoke A, B's token still refreshes */ }

// TOCTOU: a RevokeAllForTenant landing mid-Refresh is not baked into the new token.
func TestJWTIssuer_Refresh_TOCTOU_RevokeTenantMidRefresh(t *testing.T) { /* redis.Hook keyed on tenantgenKeyPrefix+tid, mirror midRefreshRevokeHook */ }

// Back-compat: a payload missing tenant_generation (0) refreshes against a never-bumped tenant (0).
func TestJWTIssuer_Refresh_PreTenantgenPayloadStillWorks(t *testing.T) { /* store a refreshPayload with TenantGeneration unset, Refresh ok */ }
```
Fill each body concretely by copying the matching usergen test and swapping user→tenant. Assertions must use `ErrSessionRevoked` via `ErrorIs`.

- [ ] **Step 2: Run — expect FAIL** (`RevokeAllForTenant`, `tenantgenKeyPrefix`, `TenantGeneration` undefined): `go test ./pkg/auth/ -run RevokeAllForTenant`

- [ ] **Step 3: Implement in `pkg/auth/jwt.go`**

Add the key prefix next to `usergenKeyPrefix` (jwt.go:32):
```go
	// tenantgenKeyPrefix namespaces the per-tenant token generation counter.
	// Bumping a tenant's generation invalidates every refresh token issued to any
	// of its users before the bump (#182).
	tenantgenKeyPrefix = "iam:tenantgen:"
```
Add `TenantGeneration` to `refreshPayload` (after `Generation`, jwt.go:59):
```go
	// TenantGeneration is the tenant's token generation at issue time; Refresh
	// rejects on ANY difference, revoking every session in a suspended tenant (#182).
	TenantGeneration int64 `json:"tenant_generation"`
```
Add the `generations` pair type and `currentTenantGeneration` helper (mirror `currentGeneration`):
```go
// generations is the {user, tenant} token-generation pair embedded at issue time
// and re-validated on refresh. Refresh carries the already-validated pair forward
// (never re-reading) — the #154 TOCTOU fix, now across both dimensions.
type generations struct {
	User   int64
	Tenant int64
}

// currentTenantGeneration returns the tenant's token generation, 0 if never bumped.
func (j *JWTIssuer) currentTenantGeneration(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	gen, err := j.rdb.Get(ctx, tenantgenKeyPrefix+tenantID.String()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("auth: jwt: read tenant generation: %w", err)
	}
	return gen, nil
}
```
Change `issueWithGeneration`'s signature to take the pair and embed both:
```go
func (j *JWTIssuer) issueWithGeneration(ctx context.Context, result *AuthResult, gens generations) (*TokenPair, error) {
	// ... unchanged claims/access-token building ...
	refreshToken, err := j.issueRefreshToken(ctx, &refreshPayload{
		UserID:           result.UserID,
		TenantID:         result.TenantID,
		Email:            result.Email,
		Roles:            result.Roles,
		Permissions:      result.Permissions,
		AuthMethod:       result.AuthMethod,
		Generation:       gens.User,
		TenantGeneration: gens.Tenant,
	})
	// ... unchanged ...
}
```
Update `Issue` to read both:
```go
func (j *JWTIssuer) Issue(ctx context.Context, result *AuthResult) (*TokenPair, error) {
	userGen, err := j.currentGeneration(ctx, result.UserID)
	if err != nil {
		return nil, err
	}
	tenantGen, err := j.currentTenantGeneration(ctx, result.TenantID)
	if err != nil {
		return nil, err
	}
	return j.issueWithGeneration(ctx, result, generations{User: userGen, Tenant: tenantGen})
}
```
Update `Refresh`'s check + carry-forward (replace the single-generation block at jwt.go:201-228):
```go
	userGen, err := j.currentGeneration(ctx, payload.UserID)
	if err != nil {
		return nil, err
	}
	tenantGen, err := j.currentTenantGeneration(ctx, payload.TenantID)
	if err != nil {
		return nil, err
	}
	// `!=`, not `<`, on BOTH dimensions (see #154): a missing key reads 0; `<` would
	// accept a stale token against a reset counter. Any divergence fails closed.
	if payload.Generation != userGen || payload.TenantGeneration != tenantGen {
		return nil, ErrSessionRevoked
	}
	// Carry the already-validated pair forward via issueWithGeneration, NOT Issue —
	// Issue re-reads the counters, reopening the revoke-mid-refresh TOCTOU window (#154).
	return j.issueWithGeneration(ctx, &AuthResult{
		UserID:          payload.UserID,
		TenantID:        payload.TenantID,
		Email:           payload.Email,
		Roles:           payload.Roles,
		Permissions:     payload.Permissions,
		AuthMethod:      payload.AuthMethod,
		AuthenticatedAt: time.Now().UTC(),
	}, generations{User: userGen, Tenant: tenantGen})
```
Add `RevokeAllForTenant` (mirror `RevokeAllForUser` @:249):
```go
// RevokeAllForTenant invalidates every outstanding refresh token for every user in
// a tenant by bumping the tenant generation counter. O(1) regardless of user count
// (suspension, #182). Access tokens remain valid until they expire (accessTTL).
func (j *JWTIssuer) RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error {
	key := tenantgenKeyPrefix + tenantID.String()
	if _, err := j.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, j.refreshTTL*2)
		return nil
	}); err != nil {
		return fmt.Errorf("auth: jwt: revoke all tenant sessions: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Interface + 4 implementers**

`pkg/auth/token.go` — add to `TokenIssuer` (after `RevokeAllForUser`, :44):
```go
	// RevokeAllForTenant invalidates every outstanding refresh token for every user
	// in a tenant (tenant suspension — #182).
	RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error
```
`services/iam/service_test.go` `mockTokenIssuer` — add field + method:
```go
	revokeAllForTenantFn func(ctx context.Context, tenantID uuid.UUID) error
```
```go
func (m *mockTokenIssuer) RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error {
	if m.revokeAllForTenantFn != nil {
		return m.revokeAllForTenantFn(ctx, tenantID)
	}
	return nil
}
```
`tests/integration/iam_tenant_isolation_test.go` `noopTokenIssuer` — add:
```go
func (*noopTokenIssuer) RevokeAllForTenant(context.Context, uuid.UUID) error { return nil }
```
`e2e/critical_path/helpers_test.go` `stubTokenIssuer` — add the same no-op method (match that file's receiver/param style).

- [ ] **Step 5: Run — expect PASS + whole-tree vet**
```bash
go test ./pkg/auth/ -race && go vet ./... && go build ./...
gofmt -l pkg/auth/jwt.go pkg/auth/token.go pkg/auth/jwt_test.go services/iam/service_test.go tests/integration/iam_tenant_isolation_test.go e2e/critical_path/helpers_test.go
```
`go vet ./...` (WHOLE TREE) MUST pass — it proves all 4 implementers (incl. the two out-of-package stubs with `var _ auth.TokenIssuer` asserts) satisfy the widened interface. Also run `go test ./services/iam/ -race` to confirm the mock change didn't break iam tests.

- [ ] **Step 6: Commit**
```bash
git add pkg/auth/jwt.go pkg/auth/token.go pkg/auth/jwt_test.go services/iam/service_test.go tests/integration/iam_tenant_isolation_test.go e2e/critical_path/helpers_test.go
git commit -m "feat(iam): per-tenant token generation counter + RevokeAllForTenant (#182)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: #182 wiring — SuspendTenant revokes all tenant sessions

**Files:** Modify `services/iam/service.go`, `service_test.go`

**Interfaces:**
- Consumes: Task 2's `TokenIssuer.RevokeAllForTenant`, `mockTokenIssuer.revokeAllForTenantFn`.

- [ ] **Step 1: Write failing service tests**

Add to `services/iam/service_test.go` (mirror the `SuspendTenant` test setup — source the exact `mockRepo` fields it uses, e.g. `updateTenantStatusFn`/`getTenantFn`, from an existing `TestSuspendTenant*` test; `SuspendTenant(ctx, id, holdUntil, freezeReason)`):
```go
func TestSuspendTenant_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	called := false
	tokens := &mockTokenIssuer{revokeAllForTenantFn: func(_ context.Context, tid uuid.UUID) error {
		called = true
		revokedFor = tid
		return nil
	}}
	repo := &mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ TenantStatus, _ *time.Time, _ *string) error { return nil },
		getTenantFn:          func(_ context.Context, id uuid.UUID) (*Tenant, error) { return &Tenant{ID: id}, nil },
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.NoError(t, err)
	assert.True(t, called, "suspending a tenant must revoke every session in it")
	assert.Equal(t, fixedTenantID, revokedFor)
}

func TestSuspendTenant_RevokeFailure_IsReported(t *testing.T) {
	t.Parallel()
	tokens := &mockTokenIssuer{revokeAllForTenantFn: func(_ context.Context, _ uuid.UUID) error {
		return errors.New("redis down")
	}}
	repo := &mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ TenantStatus, _ *time.Time, _ *string) error { return nil },
		getTenantFn:          func(_ context.Context, id uuid.UUID) (*Tenant, error) { return &Tenant{ID: id}, nil },
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.Error(t, err)
}
```
(Confirm the `mockRepo` field names + `TenantStatus`/`UpdateTenantStatus` signature + `TenantStatusSuspended` from the real source and an existing SuspendTenant test; adjust to compile.)

- [ ] **Step 2: Run — expect FAIL** (`SuspendTenant` doesn't revoke): `go test ./services/iam/ -run TestSuspendTenant_Revoke`

- [ ] **Step 3: Wire the revoke**

In `services/iam/service.go` `SuspendTenant`, immediately after the `UpdateTenantStatus` write succeeds (:630-632) and before the `GetTenant`/audit block, add:
```go
	// A suspended tenant whose users keep refreshing tokens for the full window
	// defeats suspension. One INCR revokes every session in the tenant (#182).
	if err := s.tokens.RevokeAllForTenant(ctx, id); err != nil {
		return nil, fmt.Errorf("iam: suspend tenant %s: revoke sessions: %w", id, err)
	}
```

- [ ] **Step 4: Run — expect PASS + gate**
```bash
go test ./services/iam/ -race && go vet ./... && go build ./...
gofmt -l services/iam/service.go services/iam/service_test.go
```

- [ ] **Step 5: Commit**
```bash
git add services/iam/service.go services/iam/service_test.go
git commit -m "feat(iam): revoke all tenant sessions on SuspendTenant (#182)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- #181 UpdateUser revoke on non-active status → Task 1 ✅
- #182 tenantgen counter (payload/key/helper/generations) + Issue/Refresh dual-check TOCTOU-safe → Task 2 ✅
- #182 RevokeAllForTenant on interface + all 4 implementers → Task 2 ✅
- #182 SuspendTenant wiring → Task 3 ✅
- Tests: UpdateUser deactivate/no-revoke/failure; tenantgen suite + TOCTOU + back-compat; SuspendTenant revoke/failure → Tasks 1,2,3 ✅
- Non-goals honored (no ActivateUser, no ResumeTenant, no sibling-lifecycle revoke, no migration/proto/sqlc) ✅

**Placeholder scan:** the pkg/auth test bodies in Task 2 Step 1 are described by-behavior with the exact sibling test to copy (fixtures live in the file the implementer reads) — acceptable, not TODOs; all production code and iam tests are full. The "confirm mock field/NewService arg order" notes are compiler-checked.

**Type consistency:** `RevokeAllForTenant(ctx, tenantID uuid.UUID) error` identical across interface (token.go), `*JWTIssuer`, `mockTokenIssuer`, `noopTokenIssuer`, `stubTokenIssuer` (all Task 2), and the SuspendTenant call site (Task 3). `generations{User, Tenant int64}` used in `Issue`/`Refresh`/`issueWithGeneration` (Task 2). `refreshPayload.TenantGeneration int64` embedded (Task 2) and compared (Task 2 Refresh). `mockTokenIssuer.revokeAllForTenantFn` added Task 2, used Task 3.

**Ordering:** Task 1 (#181, isolated, no interface change) → Task 2 (#182 mechanism; interface + all 4 impls in one commit so whole-tree vet is green; SuspendTenant not yet calling it — tree builds with an unused method) → Task 3 (SuspendTenant wiring; needs the interface method + mock from Task 2). Every commit builds tree-wide and passes its gate.
