# Tenant login-gate hardening (#206) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate login to `active` tenants only — every non-active retention-lifecycle state (`suspended`/`grace`/`deactivated`/`purge_eligible`) blocks — so a tenant swept past `suspended` can't reopen logins and regenerate sessions (closes #206, the durability gap in #182's tenant revocation).

**Architecture:** `pkg/auth`-only. A `tenantStatusError` helper returns the blocking error for a status; both `Authenticate` providers call it. Keeps `ErrTenantSuspended` for `suspended`; a new `ErrTenantInactive` covers the other blocked states, mapped to `PermissionDenied`. No lifecycle/sweeper/re-revoke change — no live sessions exist past suspension once this lands.

**Tech Stack:** Go 1.25, `pkg/auth`, `services/iam`, testify.

## Global Constraints

- **Only `active` tenants may authenticate.** `suspended` → `ErrTenantSuspended` (unchanged); `grace`/`deactivated`/`purge_eligible`/any unknown → new `ErrTenantInactive`. Fail closed on unknown status.
- **`pkg/auth` must NOT import `services/iam`** (layering — iam imports auth, not the reverse). Use status string literals (`"active"`, `"suspended"`, …), consistent with the current literal `"suspended"`.
- No re-revoke, no `cmd/retention-sweeper` change, no lifecycle change (unnecessary + the sweeper's `tokens=nil` must stay untouched to avoid a panic).
- No migration/proto/sqlc → no codegen gates. Preserve the existing `suspended` behavior/message/test exactly.
- Gate: `go test ./pkg/auth/... ./services/iam/... -race && go vet ./... && go build ./...` + `gofmt -l <touched .go files>` (empty).
- **Security-sensitive** (iam + auth login authorization) → senior review per CLAUDE.md.
- Commit Conventional-Commits (scope `iam`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility |
|---|---|
| `pkg/auth/tenant_status.go` (new) | `tenantStatusError` helper |
| `pkg/auth/errors.go` | `ErrTenantInactive` |
| `pkg/auth/local.go` | call the helper in `Authenticate` |
| `pkg/auth/oidc.go` | call the helper in `Authenticate` |
| `services/iam/handler.go` | map `ErrTenantInactive` → `PermissionDenied` |
| `pkg/auth/local_test.go`, `oidc_test.go`, `tenant_status_test.go` | tests |

---

### Task 1: Gate login to active tenants only

**Files:** Create `pkg/auth/tenant_status.go`, `pkg/auth/tenant_status_test.go`; Modify `pkg/auth/errors.go`, `local.go`, `oidc.go`, `services/iam/handler.go`, `pkg/auth/local_test.go`, `pkg/auth/oidc_test.go`

**Interfaces:**
- Produces: `func tenantStatusError(status string) error`; `ErrTenantInactive`.

- [ ] **Step 1: Write failing tests**

Add `pkg/auth/tenant_status_test.go`:
```go
package auth

import (
	"errors"
	"testing"
)

func TestTenantStatusError(t *testing.T) {
	cases := []struct {
		status string
		want   error
	}{
		{"active", nil},
		{"suspended", ErrTenantSuspended},
		{"grace", ErrTenantInactive},
		{"deactivated", ErrTenantInactive},
		{"purge_eligible", ErrTenantInactive},
		{"something_unknown", ErrTenantInactive},
	}
	for _, c := range cases {
		if got := tenantStatusError(c.status); !errors.Is(got, c.want) {
			t.Errorf("tenantStatusError(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}
```
In `pkg/auth/local_test.go`, mirror `TestLocalProvider_TenantSuspended` (:181 — read it for the exact provider/mock construction, `AuthRequest` fixture, and how `GetTenantStatus` is stubbed) and add blocked-state coverage. Keep the existing suspended test. Add:
```go
func TestLocalProvider_TenantInactiveStatuses(t *testing.T) {
	for _, st := range []string{"grace", "deactivated", "purge_eligible"} {
		st := st
		t.Run(st, func(t *testing.T) {
			// Build the provider exactly as TestLocalProvider_TenantSuspended does,
			// but stub GetTenantStatus to return st.
			p := /* same setup, tenants mock returns st */ nil
			_, err := p.Authenticate(context.Background(), /* same AuthRequest fixture */)
			assert.ErrorIs(t, err, ErrTenantInactive)
		})
	}
}
```
Do the same in `pkg/auth/oidc_test.go` (mirror the suspended test at :222). Fill the provider/mock/fixture setup by copying the sibling suspended test in each file (do NOT invent new mocks).

- [ ] **Step 2: Run — expect FAIL** (`tenantStatusError`/`ErrTenantInactive` undefined; grace/deactivated/purge_eligible currently authenticate): `go test ./pkg/auth/ -run 'TenantStatusError|TenantInactive'`

- [ ] **Step 3: Add the error**

In `pkg/auth/errors.go`, after `ErrTenantSuspended`:
```go
	// ErrTenantInactive is returned when the tenant is past 'active' in the retention
	// lifecycle (grace/deactivated/purge_eligible) and may not authenticate (#206).
	ErrTenantInactive = errors.New("auth: tenant is not active")
```

- [ ] **Step 4: Add the helper**

Create `pkg/auth/tenant_status.go`:
```go
package auth

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

- [ ] **Step 5: Wire both providers**

In `pkg/auth/local.go` (`Authenticate`, ~:74) replace:
```go
	if status == "suspended" {
		return nil, ErrTenantSuspended
	}
```
with:
```go
	if err := tenantStatusError(status); err != nil {
		return nil, err
	}
```
Apply the identical replacement in `pkg/auth/oidc.go` (`Authenticate`, ~:80). Leave the preceding `GetTenantStatus` read + error wrap unchanged.

- [ ] **Step 6: Map the new error in grpcError**

In `services/iam/handler.go` (~:828), add `ErrTenantInactive` to the existing `PermissionDenied` arm:
```go
	case errors.Is(err, auth.ErrTenantSuspended),
		errors.Is(err, auth.ErrTenantInactive),
		errors.Is(err, auth.ErrAccountDeactivated):
		return status.Error(codes.PermissionDenied, err.Error())
```

- [ ] **Step 7: Run — expect PASS + gate**
```bash
go test ./pkg/auth/... ./services/iam/... -race && go vet ./... && go build ./...
gofmt -l pkg/auth/tenant_status.go pkg/auth/tenant_status_test.go pkg/auth/errors.go pkg/auth/local.go pkg/auth/oidc.go pkg/auth/local_test.go pkg/auth/oidc_test.go services/iam/handler.go
```
All green; `gofmt -l` prints nothing. The existing `suspended` tests (both providers) must still pass with `ErrTenantSuspended`, and any existing `active`-tenant success test must still authenticate.

- [ ] **Step 8: Commit**
```bash
git add pkg/auth/tenant_status.go pkg/auth/tenant_status_test.go pkg/auth/errors.go pkg/auth/local.go pkg/auth/oidc.go pkg/auth/local_test.go pkg/auth/oidc_test.go services/iam/handler.go
git commit -m "fix(iam): gate login to active tenants only (#206)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- `tenantStatusError` helper (active→nil, suspended→ErrTenantSuspended, else→ErrTenantInactive) → Step 4 ✅
- `ErrTenantInactive` → Step 3 ✅
- Both providers call the helper → Step 5 ✅
- grpcError maps ErrTenantInactive → PermissionDenied → Step 6 ✅
- Tests: helper table; local+oidc grace/deactivated/purge_eligible→ErrTenantInactive; suspended kept; active still works → Steps 1,7 ✅
- Non-goals honored (no read-only layer, no re-revoke/sweeper change, no reactivation, no migration/proto/sqlc) ✅

**Placeholder scan:** production code (helper, error, both wirings, grpc arm) is fully given. The local/oidc test bodies say "build the provider exactly as the sibling suspended test does" — the mock/fixture setup lives in the file the implementer reads; this is a copy-the-sibling instruction, not a TODO. The helper test is complete.

**Type consistency:** `tenantStatusError(status string) error` (Step 4) called identically in local.go + oidc.go (Step 5) and tested (Step 1). `ErrTenantInactive` (Step 3) returned by the helper, mapped in handler.go (Step 6), asserted in tests. `pkg/auth` uses string literals only — no `services/iam` import (layering preserved).

**Single task:** one coherent, small, security-scoped change (helper + error + two wirings + one grpc arm + tests) a reviewer gates as a unit; no meaningful split point.
