# Service-to-Service Auth: close the PublicMethods allowlist (#139 slice I) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issue:** #139 §4 (service-to-service identity), slice I — the last slice of #139's core
**Branch:** `fix/service-auth-allowlist-139i` off `main` (`467f9d8`)
**Migration:** none. **New infrastructure:** none. **Proto breaking change:** none.

## Goal

Remove the two service-to-service entries from `interceptor.PublicMethods`
(`CheckPermission`, `ValidateToken`), scope `CheckPermission` to the authenticated
caller, and retire the dead `ValidateToken` RPC. This closes #139 §4 and finishes
#139's core authorization arc.

## Context — §4's premise is stale, and that shrinks the slice

#139 §4 says:

> #138 allowlists `/IAMService/CheckPermission` and `/IAMService/ValidateToken`
> because the five services that call them attach no credentials
> (`grep AppendToOutgoingContext` → zero hits tree-wide). … Close it with machine
> tokens (each service holds a signed service JWT, attached by a client
> interceptor) or mTLS. Then remove both allowlist entries.

That was true when #138 was written. It is **no longer true**. `pkg/interceptor/forward.go`
has since landed, and `pkg/iamclient/dial.go:42` attaches
`interceptor.ForwardAuthUnaryClientInterceptor()` to every IAM dial. Verified:

| fact | evidence |
|---|---|
| The caller's bearer token already reaches iam on every `CheckPermission` | `pkg/iamclient/dial.go:42` (forwarding interceptor) + `pkg/iamclient/permission.go:49` passes `ctx` straight through |
| Incoming metadata survives the timeout wrapper | `interceptor.RequirePermission:81` builds `checkCtx` from `ctx`, not `context.Background()` |
| All nine services dial through that one function | `iamclient.DialFromEnv` in all nine `cmd/*/main.go` |
| Removing an allowlist entry enforces auth immediately | `cmd/iam/main.go:253` installs `UnaryAuthInterceptor(jwtVerifier, interceptor.PublicMethods)` |
| `ValidateToken` has **zero** callers | none in-tree; no `google.api.http` annotation (not gateway/Kong exposed); no infra/script reference |
| `CheckPermission` is gRPC-only | no `google.api.http` annotation |
| No machine-token infrastructure exists | no `ServiceToken`/`MachineToken`/`ServiceAccount` anywhere |

**Therefore machine tokens are not required to meet §4's stated goal.** They are
deferred (see Non-goals). The allowlist entries are a leftover, not a load-bearing
mechanism.

### The real exposure

The allowlist entry is the lesser problem. `CheckPermission` accepts an **unscoped
`user_id`** (`services/iam/handler.go:335-353` parses it from the request and never
consults the caller), so today anyone who reaches iam's gRPC port can enumerate
whether any user in any tenant holds a permission — exactly the residual risk
`public.go:36-42` documents. Requiring a token closes the anonymous case; scoping
`user_id` to the caller closes it outright.

## Design

### 1. Drop both `PublicMethods` entries

`pkg/interceptor/public.go` — delete the `CheckPermission` and `ValidateToken`
entries and the "Service-to-service" comment block. Add both to the file's existing
**"Deliberately ABSENT, and why"** doc block, recording *why they no longer need to be
public* — that `iamclient` forwards the caller's token — so a future reader does not
re-add them on the old reasoning.

Nothing else in the map changes. `Login`, `RefreshToken`, `AcceptInvitation`, and the
two reflection entries stay.

### 2. Scope `CheckPermission` to the authenticated caller

`services/iam/handler.go` `CheckPermission` — require a caller and enforce identity
before doing any work:

```go
// All UUID parses first — including the optional project_id — so a malformed
// id is never masked as PermissionDenied (house rule).
userID, err := uuid.Parse(req.GetUserId())
if err != nil {
    return nil, status.Error(codes.InvalidArgument, "invalid user_id")
}
// ... existing optional project_id parse ...

caller, ok := interceptor.CallerFromContext(ctx)
if !ok || caller.UserID == uuid.Nil {
    return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
}
if userID != caller.UserID {
    // Deliberately does not echo either id: confirming the other exists is an oracle.
    return nil, status.Error(codes.PermissionDenied, "user_id does not match the authenticated caller")
}
```

then the existing `h.svc.CheckPermission(...)` call, unchanged.

**Compatible with every existing caller:** `interceptor.RequirePermission:84` always
passes `caller.UserID`, and that caller identity comes from the same token iam is now
verifying. Guard order follows the house rule — tenant/caller resolution, then **all**
UUID parses (including the optional `project_id`), then the decision — so a malformed
`project_id` still yields `InvalidArgument`, not `PermissionDenied`.

An "admin checks another user's permissions" flow, if ever needed, is a separate gated
RPC — not a loosening of this one.

### 3. Retire `ValidateToken`

`services/iam/handler.go:69-75` is a thin wrapper over `h.svc.tokens.Validate`. There is
no `Service.ValidateToken`. Replace the body with:

```go
// Deprecated: retired. Services verify JWTs in-process against the shared public
// key (#138); this RPC has no callers and was an unauthenticated verification
// oracle on iam's port (#139 §4).
func (h *Handler) ValidateToken(context.Context, *iamv1.ValidateTokenRequest) (*iamv1.Claims, error) {
    return nil, status.Error(codes.Unimplemented, "ValidateToken is retired; verify tokens in-process against the JWT public key")
}
```

Add a matching `// Deprecated:` comment to the RPC in
`proto/thittam/iam/v1/iam.proto`.

**The RPC is never deleted.** `proto/buf.yaml` enables the `FILE` breaking category and
CI runs `buf breaking … --against main`, so removing an RPC fails CI. Retirement means
gutting the implementation and deprecating by comment — the D8 precedent. The proto edit
is comment-only, so no `buf generate` is needed (and it cannot run locally —
`google/api/annotations.proto` is unresolvable without BSR; `gen/` comment drift is
pre-existing and accepted).

`h.svc.tokens.Validate` stays — it is shared with the live login/refresh/GetCurrentUser
paths. Only the handler body becomes dead. If `claimsToProto` loses its last caller, it
is removed in the same commit (CI's `unused` linter would flag it otherwise).

## Testing

The failure mode here is a **regression, not a vulnerability**: nine services depend on
`CheckPermission` succeeding. If any caller reaches it without a forwarded token, every
gated RPC across the platform fails closed.

- **Unit** (`services/iam/handler_test.go`): tokenless context → `Unauthenticated`;
  `user_id` ≠ caller → `PermissionDenied`, asserting the service is **never reached**
  (a write-fn/`t.Fatal` tripwire, since iam's `mockRepo` unset defaults return usable
  rows and a status-code-only assertion can pass vacuously); matching `user_id` →
  passes through; malformed `project_id` → still `InvalidArgument`. `ValidateToken` →
  `Unimplemented`.
- **Interceptor chain** (`pkg/server/integration_test.go`, the bufconn test — the only
  place that installs the real chain; `e2e/critical_path` drives the Service layer and
  bypasses interceptors, so it proves nothing here): assert `CheckPermission` is
  rejected without a token and accepted with a valid one, now that it is no longer in
  `PublicMethods`.
- **Sweep:** any existing test constructing an iam client or calling `CheckPermission`
  without a token now fails. That is the intended signal — fix the call sites; do not
  re-add the allowlist entry.

Coverage floor: iam ≥ 85%.

## Non-goals

- **Machine tokens / service JWTs / mTLS.** Deferred, not rejected. Nothing in-tree
  needs a service identity today: every gated RPC path runs through `RequirePermission`
  (which already demands a caller), and workers/NATS consumers call the Service layer
  directly rather than gated RPCs. Build them when a genuinely credential-less caller
  must reach a gated RPC — with that use case in hand to shape issuance, distribution,
  rotation, and verification. Recorded here so the deferral is a decision, not a gap.
- No change to the other `PublicMethods` entries.
- No change to `RequirePermission`, `forward.go`, or `iamclient` — they already do the
  right thing; this slice only stops iam from accepting anonymous calls.
- Impersonation (#139 §5) and payment-webhook signatures (#139 §6) remain open.

## Review weight

Touches `iam` and `pkg/interceptor` — senior engineer + 2 approvals per CLAUDE.md.
Small diff, wide blast radius: the whole-branch review runs on the most capable model.
