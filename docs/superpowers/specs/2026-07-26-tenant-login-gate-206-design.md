# Gate login to active tenants only (#206) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #206 (tenant login gate blocks only `suspended`; grace/deactivated/purge_eligible reopen logins) — filed from #182's review
**Branch:** `fix/tenant-login-gate-206` off `main`
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

The login gate rejects only `status == "suspended"`, so a tenant swept through the
retention lifecycle (`suspended → grace → deactivated → purge_eligible`) reopens logins and
mints fresh sessions — undoing #182's tenant revocation the moment the sweeper advances a
tenant out of `suspended`. Gate login to **active tenants only**: every non-active lifecycle
state blocks. This closes #206 entirely in `pkg/auth`, with no lifecycle/sweeper change.

## Context (grounding facts, `main` @ dae4f65)

- **Login gate** — `pkg/auth/local.go:68-76` (`LocalProvider.Authenticate`) and
  `pkg/auth/oidc.go:74-82` (`OIDCProvider.Authenticate`), identical:
  ```go
  status, err := p.tenants.GetTenantStatus(ctx, req.TenantID)
  if err != nil {
      return nil, fmt.Errorf("auth: check tenant: %w", err)
  }
  if status == "suspended" {
      return nil, ErrTenantSuspended
  }
  ```
  `GetTenantStatus` is a **live DB read** per login (`services/iam/db/postgres.go:174-181`) —
  the gate sees the current status; it just compares wrong. These two are the ONLY tenant-status
  login gates in the codebase.
- **Tenant statuses** (`services/iam/lifecycle.go:37-45` consts; CHECK in migration 016):
  `active, suspended, grace, deactivated, purge_eligible`. Only `suspended` is currently gated.
- **`grace` is documented "read-only" accessible** (doc §1.3a "Grace (read-only)",
  `lifecycle.go:19`, migration 016 comment) — **but no read-only enforcement exists anywhere**
  (grep found only the comments; no interceptor/handler restricts a grace tenant's writes). So a
  grace login today grants FULL read/write, not read-only. `deactivated` is documented "access
  revoked"; `purge_eligible` is strictly further. Decision (approved): **fail closed — block grace
  too** until a real read-only layer is built (blocking is safer than today's full access; grace can
  be carved back out of the gate when that layer exists).
- **No re-revoke / sweeper change needed:** `SuspendTenant` already revokes (#182), and once this
  gate lands, no login succeeds past `suspended` → no fresh sessions are ever minted in
  grace/deactivated/purge_eligible → no live sessions exist there to revoke. (`AdvanceTenantLifecycle`
  never touches `s.tokens`, and `cmd/retention-sweeper/main.go:80` builds `NewService(repo, nil, ...)`
  with `tokens=nil` — untouched, so no nil-panic risk introduced.)
- **Errors** (`pkg/auth/errors.go`): `ErrTenantSuspended = "auth: tenant is suspended"`. `grpcError`
  (`services/iam/handler.go:828-830`) maps `ErrTenantSuspended`/`ErrAccountDeactivated` →
  `codes.PermissionDenied`.
- **Existing tests:** `pkg/auth/local_test.go:181` `TestLocalProvider_TenantSuspended` (mocks
  `GetTenantStatus`→`"suspended"`, asserts `ErrTenantSuspended`); `pkg/auth/oidc_test.go:222` the
  OIDC equivalent.

## Design

### 1. Shared helper (`pkg/auth`)

Both providers duplicate the status check today; add one helper (new file `pkg/auth/tenant_status.go`
or into an existing auth file — implementer's choice, keep it in `package auth`):
```go
// tenantStatusError returns the login-blocking error for a tenant status, or nil if
// the tenant may authenticate. Only "active" tenants may log in; every lifecycle state
// past active (suspended/grace/deactivated/purge_eligible, or any unknown value) blocks.
// grace's documented read-only access is unenforced, so it fails closed until built (#206).
func tenantStatusError(status string) error {
	switch status {
	case "active":
		return nil
	case "suspended":
		return ErrTenantSuspended
	default: // grace, deactivated, purge_eligible, or any unknown status
		return ErrTenantInactive
	}
}
```
Keeps `ErrTenantSuspended` for `suspended` (preserves the existing error, message, and test); the
newly-blocked states get a distinct `ErrTenantInactive`. Status literals (not the
`services/iam` `TenantStatus*` consts) because `pkg/auth` is a lower layer and must not import
`services/iam` — consistent with the current literal `"suspended"`.

### 2. New error (`pkg/auth/errors.go`)

Add after `ErrTenantSuspended`:
```go
	// ErrTenantInactive is returned when the tenant is past 'active' in the retention
	// lifecycle (grace/deactivated/purge_eligible) and may not authenticate (#206).
	ErrTenantInactive = errors.New("auth: tenant is not active")
```

### 3. Wire both providers

In `pkg/auth/local.go` and `pkg/auth/oidc.go`, replace the `if status == "suspended" { return nil, ErrTenantSuspended }` block with:
```go
	if err := tenantStatusError(status); err != nil {
		return nil, err
	}
```
(Keep the preceding `GetTenantStatus` read + its error wrap unchanged.)

### 4. gRPC mapping (`services/iam/handler.go`)

Add `ErrTenantInactive` to the existing `PermissionDenied` arm (`:828-830`):
```go
	case errors.Is(err, auth.ErrTenantSuspended),
		errors.Is(err, auth.ErrTenantInactive),
		errors.Is(err, auth.ErrAccountDeactivated):
		return status.Error(codes.PermissionDenied, err.Error())
```

## Testing

- **`pkg/auth/local_test.go` + `oidc_test.go`**: keep `suspended` → `ErrTenantSuspended`; add
  `grace`, `deactivated`, `purge_eligible` → `ErrTenantInactive` (table-driven over the blocked
  statuses is cleanest); confirm `active` still authenticates (an existing success test already
  covers active — verify it passes unchanged). Assert via `errors.Is`/`ErrorIs`.
- **Helper unit test** (optional but cheap): `tenantStatusError` returns nil for `active`,
  `ErrTenantSuspended` for `suspended`, `ErrTenantInactive` for grace/deactivated/purge_eligible/unknown.
- **Handler** (`services/iam/handler_test.go` if it has a grpcError-style test): `ErrTenantInactive`
  → `codes.PermissionDenied`.
- Gates: `go test ./pkg/auth/... ./services/iam/... -race`; `go vet ./...`; `go build ./...`;
  `gofmt -l` on touched Go files. No proto/sqlc/migration → no codegen gates.

## Non-goals

- No read-only enforcement layer for `grace` (separate feature; once built, `grace` can be removed
  from the blocked set so read-only access resumes — the `default` arm makes that a one-line change).
- No re-revoke on lifecycle transitions and no `cmd/retention-sweeper` rewiring (unnecessary — no live
  sessions exist past `suspended` once this lands; keeping the sweeper's `tokens=nil` avoids a panic).
- No reactivation RPC (`ReactivateTenant` is an unimplemented port; out of scope).
- No migration/proto/sqlc; no change to #182's counter mechanism.

## Review weight

`iam` + `pkg/auth` (login authorization) → security-sensitive; senior engineer required per CLAUDE.md.
Standard 2 approvals + senior. Small, contained diff; whole-branch review on the most capable model.
