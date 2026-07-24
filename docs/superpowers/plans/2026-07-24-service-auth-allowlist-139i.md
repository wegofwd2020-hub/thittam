# Service-to-Service Auth: close the PublicMethods allowlist (#139 slice I) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove `CheckPermission` and `ValidateToken` from `interceptor.PublicMethods`, scope `CheckPermission` to the authenticated caller, and retire the dead `ValidateToken` RPC — closing #139 §4.

**Architecture:** Two independent tasks. Task 1 tightens `CheckPermission` (security change: closes the permission-enumeration oracle). Task 2 retires `ValidateToken` (dead-code removal). Each drops its own `PublicMethods` entry and updates the map's deliberate size assertion. No migration, no new infrastructure, no proto breaking change.

**Tech Stack:** Go 1.25, gRPC, `pkg/interceptor` (`PublicMethods`, `UnaryAuthInterceptor`, `CallerFromContext`), bufconn integration test in `pkg/server`.

## Global Constraints

- **No new infrastructure.** Machine tokens / service JWTs / mTLS are explicitly deferred (spec Non-goals). The caller's bearer token already reaches iam via `pkg/iamclient/dial.go:42` → `ForwardAuthUnaryClientInterceptor`.
- **Never delete an RPC.** `proto/buf.yaml` enables the `FILE` breaking category and CI runs `buf breaking … --against main`; removing an RPC fails CI. Retirement = gut the implementation, return `codes.Unimplemented`, deprecate by comment (the D8 precedent).
- **Proto edits are comment-only** → no `buf generate` required. It cannot run locally anyway (`google/api/annotations.proto` unresolvable without BSR); `gen/` comment drift is pre-existing and accepted.
- **Guard order in `CheckPermission`** (house rule): all UUID parses — *including the optional `project_id`* — happen BEFORE the caller gate, so a malformed id yields `InvalidArgument` and is never masked as `PermissionDenied`. In production the auth interceptor rejects tokenless calls before the handler runs; the handler's own caller check is defense-in-depth for direct-handler tests.
- **Cross-caller mismatch returns `PermissionDenied` and must not echo either id** — confirming the other user exists is an oracle.
- **`PublicMethods` size is asserted deliberately:** `pkg/interceptor/authjwt_test.go:260` asserts `assert.Len(t, PublicMethods, 7, "adding a public method is a security decision — update this count deliberately")`. Task 1 takes it to **6**, Task 2 to **5**. Update it in the task that changes it — never leave it stale.
- **Blast radius is a regression risk, not a vulnerability:** nine services depend on `CheckPermission` succeeding. Every one dials via `iamclient.DialFromEnv`, which forwards the token — but any test or path calling it without a caller now fails closed. That is the intended signal; fix the call site, never re-add the allowlist entry.
- **DB safety:** never `docker compose … -v`/`down`/`up` on `infra/local/`. This slice needs no database.
- Coverage floor: iam ≥ 85%. Touches `iam` + `pkg/interceptor` → senior review.

---

## File Structure

- `pkg/interceptor/public.go` — remove both entries; document why they no longer need to be public.
- `pkg/interceptor/authjwt_test.go:260` — the deliberate map-size assertion (7 → 6 → 5).
- `services/iam/handler.go` — `CheckPermission` (:335) gains the caller guard; `ValidateToken` (:69) gutted; `claimsToProto` (:812) removed when orphaned.
- `services/iam/handler_test.go` — 4 `CheckPermission` tests (:632, :648, :654, :676) + 1 `ValidateToken` test (:117).
- `pkg/server/integration_test.go` — `stubIAM` gains `CheckPermission`; new chain tests.
- `proto/thittam/iam/v1/iam.proto:28` — `ValidateToken` deprecation comment.

---

## Task 1: Scope CheckPermission to the caller + drop its allowlist entry

**Files:**
- Modify: `pkg/interceptor/public.go` (remove the `CheckPermission` entry)
- Modify: `pkg/interceptor/authjwt_test.go:260` (7 → 6)
- Modify: `services/iam/handler.go:335-353` (`CheckPermission`)
- Modify: `services/iam/handler_test.go:632-684` (4 tests)
- Modify: `pkg/server/integration_test.go` (`stubIAM` + 2 new chain tests)

**Interfaces:**
- Consumes: `interceptor.CallerFromContext(ctx) (CallerInfo, bool)` — `CallerInfo.UserID` is `uuid.UUID` (`pkg/interceptor/auth.go:35-43`). Test helper `memberCtxAs(tid, uid uuid.UUID) context.Context` (`services/iam/handler_test.go:54-61`) is the only helper that lets a test control the caller's `UserID`.
- Produces: no signature changes. `CheckPermission` gains `Unauthenticated` and `PermissionDenied` failure modes.

- [ ] **Step 1: Write the failing unit tests**

In `services/iam/handler_test.go`, add:

```go
func TestHandler_CheckPermission_RejectsOtherUser(t *testing.T) {
	t.Parallel()
	tid, caller := uuid.New(), uuid.New()
	victim := uuid.New()
	require.NotEqual(t, caller, victim)

	h := newHandlerWithRepo(&mockRepo{
		getUserPermissionsFn: func(context.Context, uuid.UUID) ([]string, error) {
			t.Fatal("service must not be reached for a cross-user permission check")
			return nil, nil
		},
	})

	_, err := h.CheckPermission(memberCtxAs(tid, caller), &iamv1.CheckPermissionRequest{
		UserId: victim.String(), Permission: "budget:read",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CheckPermission_RejectsTokenlessCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckPermission(context.Background(), &iamv1.CheckPermissionRequest{
		UserId: uuid.New().String(), Permission: "budget:read",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
```

Note the `t.Fatal` tripwire: iam's `mockRepo` unset defaults return usable rows, so a status-code-only assertion can pass vacuously (see the iam mock trap). Use whichever repo fn `Service.CheckPermission` actually calls — read `services/iam/service.go:410` and name that fn field; if the mock has no settable fn for it, assert instead that the returned `allowed` is never consulted by checking the error is `PermissionDenied` *and* add the tripwire on the nearest settable fn.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./services/iam/ -run 'CheckPermission_(RejectsOtherUser|RejectsTokenlessCaller)' -v`
Expected: FAIL — the current handler ignores the caller entirely, so both return `OK`/`InvalidArgument` rather than `PermissionDenied`/`Unauthenticated`.

- [ ] **Step 3: Add the caller guard to the handler**

`services/iam/handler.go` — replace the body of `CheckPermission` (:335-353). Parses first (house rule), then the gate:

```go
func (h *Handler) CheckPermission(ctx context.Context, req *iamv1.CheckPermissionRequest) (*iamv1.CheckPermissionResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	var projectID *uuid.UUID
	if pid := req.GetProjectId(); pid != "" {
		parsed, err := uuid.Parse(pid)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid project_id")
		}
		projectID = &parsed
	}

	// A caller may only ask about itself. Every in-tree caller does exactly
	// that (interceptor.RequirePermission passes caller.UserID), and allowing
	// an arbitrary user_id made this an enumeration oracle for anyone who
	// could reach iam's port (#139 §4).
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok || caller.UserID == uuid.Nil {
		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if userID != caller.UserID {
		// Deliberately echoes neither id: confirming the other exists is an oracle.
		return nil, status.Error(codes.PermissionDenied, "user_id does not match the authenticated caller")
	}

	allowed, err := h.svc.CheckPermission(ctx, userID, req.GetPermission(), projectID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.CheckPermissionResponse{Allowed: allowed}, nil
}
```

- [ ] **Step 4: Fix the two success-path tests to carry a matching caller**

`services/iam/handler_test.go:632` (`TestHandler_CheckPermission_Success`) and `:654`
(`TestHandler_CheckPermission_WithProjectID`) currently pass `context.Background()` and a
random `UserId`. Give each a caller whose `UserID` equals the requested `user_id`:

```go
	tid, uid := uuid.New(), uuid.New()
	// ... existing handler/mock setup ...
	resp, err := h.CheckPermission(memberCtxAs(tid, uid), &iamv1.CheckPermissionRequest{
		UserId:     uid.String(),
		Permission: "budget:read",
	})
```

(`WithProjectID` keeps its `ProjectId` field; only the context and `UserId` change.)

The two `Invalid*` tests (`:648` `InvalidUserID`, `:676` `InvalidProjectID`) need **no change** — parses run before the caller gate, so a bare context still yields `InvalidArgument`. Run them to confirm; if either now returns `Unauthenticated`, the guard order in Step 3 is wrong.

- [ ] **Step 5: Remove the CheckPermission allowlist entry**

`pkg/interceptor/public.go` — delete the `iamv1.IAMService_CheckPermission_FullMethodName` line. Move its rationale into the "Deliberately ABSENT, and why" doc block above the map:

```go
//	CheckPermission      — service-to-service, but pkg/iamclient dials iam with
//	                       ForwardAuthUnaryClientInterceptor, so the caller's own
//	                       bearer token already arrives. It additionally refuses a
//	                       user_id that is not the caller's, so it is no longer an
//	                       enumeration oracle (#139 §4).
```

Leave the `ValidateToken` entry and the "Service-to-service" comment block for Task 2 (trim the block's wording to cover only `ValidateToken`).

- [ ] **Step 6: Update the deliberate map-size assertion**

`pkg/interceptor/authjwt_test.go:260` — `assert.Len(t, PublicMethods, 7, …)` → `6`. Keep the message verbatim.

- [ ] **Step 7: Add the interceptor-chain proof**

`pkg/server/integration_test.go` is the only test that installs the real chain
(`startServer` at :48-65 wires `interceptor.UnaryAuthInterceptor(v, interceptor.PublicMethods)`).
`stubIAM` (:31-46) does not implement `CheckPermission`, so add it:

```go
func (s *stubIAM) CheckPermission(ctx context.Context, _ *iamv1.CheckPermissionRequest) (*iamv1.CheckPermissionResponse, error) {
	if _, ok := interceptor.CallerFromContext(ctx); !ok {
		return nil, status.Error(codes.Internal, "handler reached without a verified caller")
	}
	return &iamv1.CheckPermissionResponse{Allowed: true}, nil
}
```

then two cases, mirroring `TestChain_PrivateMethodRejectsTokenlessCall` (:99-106) and
`TestChain_ValidTokenReachesHandlerWithVerifiedIdentity` (:124-132):

```go
func TestChain_CheckPermissionRejectsTokenlessCall(t *testing.T) {
	key, v := keyAndVerifier(t)
	_ = key
	client := startServer(t, v)
	_, err := client.CheckPermission(context.Background(), &iamv1.CheckPermissionRequest{
		UserId: uuid.New().String(), Permission: "budget:read",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err),
		"CheckPermission must not be reachable without a token now that it is off PublicMethods")
}

func TestChain_CheckPermissionAcceptsForwardedToken(t *testing.T) {
	key, v := keyAndVerifier(t)
	client := startServer(t, v)
	resp, err := client.CheckPermission(bearer(t, key), &iamv1.CheckPermissionRequest{
		UserId: uuid.New().String(), Permission: "budget:read",
	})
	require.NoError(t, err)
	assert.True(t, resp.GetAllowed())
}
```

(`bearer(t, key)` at :78-89 mints a real RS256 JWT and attaches it as outgoing metadata — the same shape `iamclient`'s forwarding interceptor produces. The stub does not enforce user_id matching; this case proves the *chain* accepts a forwarded token, while Step 1's unit tests prove the handler's user_id guard.)

- [ ] **Step 8: Run the full gate**

Run: `go build ./... && go vet ./... && go test ./services/iam/... ./pkg/interceptor/... ./pkg/server/... ./pkg/iamclient/...`
Expected: all pass. If any other test fails calling `CheckPermission`, fix the call site — do not re-add the allowlist entry.

- [ ] **Step 9: Commit**

```bash
git add pkg/interceptor/public.go pkg/interceptor/authjwt_test.go services/iam/handler.go services/iam/handler_test.go pkg/server/integration_test.go
git commit -m "fix(iam): scope CheckPermission to the caller, drop its PublicMethods entry (#139 slice I)"
```

---

## Task 2: Retire ValidateToken + drop its allowlist entry

**Files:**
- Modify: `services/iam/handler.go:69-75` (`ValidateToken`), remove `claimsToProto` (:812-823)
- Modify: `proto/thittam/iam/v1/iam.proto:28` (deprecation comment)
- Modify: `pkg/interceptor/public.go` (remove the `ValidateToken` entry + the now-empty service-to-service block)
- Modify: `pkg/interceptor/authjwt_test.go:260` (6 → 5)
- Modify: `services/iam/handler_test.go:117-122` (`TestHandler_ValidateToken_Success`)

**Interfaces:**
- Produces: `ValidateToken` returns `codes.Unimplemented` for all inputs. The RPC remains declared in the proto (never deleted).
- Unaffected: `h.svc.tokens.Validate` keeps two live callers — `handler.go:92` (`GetCurrentUser`) and `service.go:250` (`Service.GetCurrentUser`). Nothing else dies.

- [ ] **Step 1: Rewrite the test to assert retirement**

`services/iam/handler_test.go:117-122` — replace `TestHandler_ValidateToken_Success`:

```go
func TestHandler_ValidateToken_Retired(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ValidateToken(context.Background(), &iamv1.ValidateTokenRequest{AccessToken: "anytoken"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./services/iam/ -run ValidateToken -v`
Expected: FAIL — the current handler validates the token and returns claims, not `Unimplemented`.

- [ ] **Step 3: Gut the handler**

`services/iam/handler.go:69-75` — replace the whole function:

```go
// ValidateToken is retired.
//
// Deprecated: every service verifies JWTs in-process against the shared public
// key (#138), so this RPC has had no callers. Left declared because
// proto/buf.yaml uses the FILE breaking category — removing an RPC fails CI —
// and removed from PublicMethods because an unauthenticated verification
// oracle on iam's port is exactly what #139 §4 asked to close.
func (h *Handler) ValidateToken(context.Context, *iamv1.ValidateTokenRequest) (*iamv1.Claims, error) {
	return nil, status.Error(codes.Unimplemented, "ValidateToken is retired; verify tokens in-process against the JWT public key")
}
```

- [ ] **Step 4: Remove the orphaned `claimsToProto`**

`services/iam/handler.go:812-823` — `claimsToProto` had exactly one caller (the old
`ValidateToken` body at :74). Delete the function. If `go build` then reports an unused
import (e.g. the proto/timestamp helper it used), remove that too.

Verify first: `grep -rn "claimsToProto" --include=*.go .` must return only the definition
before you delete it (and nothing after).

- [ ] **Step 5: Deprecate the RPC in the proto**

`proto/thittam/iam/v1/iam.proto:28` — add a comment above `rpc ValidateToken(...)`:

```proto
  // Deprecated: retired. Services verify JWTs in-process against the shared
  // public key (#138). Returns Unimplemented. Not deleted: buf's FILE breaking
  // category makes RPC removal a breaking change (#139 slice I).
  rpc ValidateToken(ValidateTokenRequest) returns (Claims);
```

Comment-only — do **not** run `buf generate` (it cannot resolve `google/api/annotations.proto` locally, and `gen/` comment drift is pre-existing).

- [ ] **Step 6: Remove the ValidateToken allowlist entry**

`pkg/interceptor/public.go` — delete the `iamv1.IAMService_ValidateToken_FullMethodName`
line and the now-empty "Service-to-service" comment block. Add to the "Deliberately
ABSENT, and why" block:

```go
//	ValidateToken        — retired to Unimplemented (#139 slice I). Services verify
//	                       JWTs in-process against the public key (#138); leaving it
//	                       public was an unauthenticated verification oracle.
```

- [ ] **Step 7: Update the deliberate map-size assertion**

`pkg/interceptor/authjwt_test.go:260` — `6` → `5`. The map should now hold exactly:
`Login`, `RefreshToken`, `AcceptInvitation`, `reflectionV1`, `reflectionV1Alpha`.

- [ ] **Step 8: Run the full gate**

Run: `go build ./... && go vet ./... && go test ./services/iam/... ./pkg/interceptor/... ./pkg/server/... && buf lint`
Expected: all pass. `buf breaking` (CI) must also pass — the RPC still exists, so it will.

- [ ] **Step 9: Commit**

```bash
git add services/iam/handler.go services/iam/handler_test.go proto/thittam/iam/v1/iam.proto pkg/interceptor/public.go pkg/interceptor/authjwt_test.go
git commit -m "feat(iam): retire ValidateToken to Unimplemented, drop its PublicMethods entry (#139 slice I)"
```

---

## Self-Review

- **Spec coverage:** drop both allowlist entries (Task 1 Step 5, Task 2 Step 6) ✓; scope `CheckPermission` to caller (Task 1 Step 3) ✓; retire `ValidateToken` + proto deprecation, never delete (Task 2 Steps 3/5) ✓; remove orphaned `claimsToProto` (Task 2 Step 4) ✓; unit + interceptor-chain tests (Task 1 Steps 1/7, Task 2 Step 1) ✓; machine tokens deferred (Global Constraints) ✓.
- **Placeholder scan:** every step carries concrete code. The one judgment call is flagged explicitly in Task 1 Step 1 (which `mockRepo` fn to hang the tripwire on — the implementer reads `service.go:410` to name it), rather than left vague.
- **Type consistency:** `CallerInfo.UserID` is `uuid.UUID` and `req.GetUserId()` is `string`, so the comparison is on the parsed `userID uuid.UUID` — never a string compare. `memberCtxAs(tid, uid)` is the only helper exposing a controllable `UserID`. The `PublicMethods` count is threaded 7 → 6 (Task 1) → 5 (Task 2) with no gap.
- **Blast radius verified against the tree:** the only affected callers are the 5 tests in `services/iam/handler_test.go` plus the count assertion. `services/iam/service_test.go` and `pkg/iamclient/permission_test.go` exercise the Service layer and a client stub respectively, not the real handler, and `e2e/` contains no reference to either RPC.
