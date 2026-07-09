# Verify the JWT in-process, fail closed (#138)

**Status:** approved (design), 2026-07-09
**Issue:** [#138](https://github.com/wegofwd2020-hub/thittam/issues/138)
**Blocks:** [#60](https://github.com/wegofwd2020-hub/thittam/issues/60) (REST→gRPC bridge / Kong)
**Follows:** [#139](https://github.com/wegofwd2020-hub/thittam/issues/139) — authorization policy per RPC (filed alongside this spec)

## 1. Problem

`pkg/interceptor/auth.go` builds caller identity — user ID, tenant ID, and **role** — from
gRPC metadata keys `x-caller-id`, `x-caller-role`, `x-tenant-id`. **Nothing in the Go
request path verifies a signature.** `RequireRole` then gates admin handlers on that
`x-caller-role` string.

The package doc states the assumption: *"Kong validates the JWT, then injects caller
identity as HTTP headers."*

Four verified facts:

1. **No Kong JWT plugin exists.** `grep -rn 'plugin: jwt' infra/` returns nothing.
   `infra/k8s/kong/jwt-tenant-header.yaml` reads `kong.ctx.shared["authenticated_jwt_token"]`,
   populated only *by* the jwt plugin, and comments that it is configured "separately."
   It is not. Only rate-limiting plugins and that pre-function are committed.
2. **Nothing strips client-supplied `x-caller-*` headers.** No `request_transformer`
   anywhere in `infra/`. Kong forwards unknown headers by default.
3. **`cmd/project-management/main.go:121-169` forwards them into gRPC metadata** via
   `WithIncomingHeaderMatcher` passing `X-Tenant-Id`, `X-Caller-Id`, `X-Caller-Email`,
   `X-Caller-Role`.
4. **The browser already sends them** — `web/src/lib/api/client.ts:134-136`.

Anyone who can reach project-management's REST port `:9080` can send
`X-Caller-Role: platform_admin` and satisfy `RequireRole`. Locally that is every
developer. In-cluster, any pod can reach any service port.

Compounding it, `UnaryCallerInterceptor` is **fail-open by construction** — its own doc:
*"Requests that arrive without `x-caller-id` metadata … are allowed through with an empty
`CallerInfo` — individual handlers … call `RequireRole` to enforce access."* Authorization
is opt-in per handler, and a handler that forgets is silently public.

## 2. Scope

This spec covers **authentication only**: making the request path verify who the caller is.

In scope:

- A verify-only JWT path in `pkg/auth` (public key, no Redis, no private key).
- A fail-closed auth interceptor (unary + stream) with an explicit public allowlist.
- `CallerInfo` derived from verified claims; `x-caller-*` and `x-tenant-id` metadata ignored.
- Wiring into all ten services — **five have no caller interceptor today**.
- `RequirePermission` denies when its checker is nil (today it silently passes).
- gRPC reflection registered only when explicitly enabled.
- Gateway and web client stop forwarding/sending caller headers.

Out of scope, deferred to **#139**:

- Authorization policy for the ~100 RPCs that check nothing.
- iam's privilege-escalation RPCs (`AssignRole`, `RevokeRole`, `CreateUser`,
  `CreateTenant`, `InviteUser` call no authz check).
- Service-to-service identity (machine tokens / mTLS).
- Impersonation semantics (`StartImpersonation` mints no token — see §9).
- Kong's jwt plugin and header stripping (#60).

## 3. What this changes, honestly

It converts **anyone who can reach the port** into **any authenticated user**.

It does **not** stop an authenticated tenant user from calling `PostJournalEntry` or
`AssignRole`. Roughly 100 RPCs still check nothing, and `AssignRole` remains a
self-promotion primitive for anyone holding a valid token. That is #139. This spec must
not be read as "the platform is now authorized."

## 4. Verify-only path — `pkg/auth`

`JWTIssuer.Validate` (`pkg/auth/jwt.go:204`) derives its key from `&j.privateKey.PublicKey`
and requires a Redis client. Services must never hold the private key. New, separate type:

```go
// Verifier checks access-token signatures using only the RSA public key.
// It holds no private key and needs no Redis: access tokens are short-lived
// (DefaultAccessTTL) and are not revocable. Only refresh tokens are.
type Verifier struct{ publicKey *rsa.PublicKey }

// NewVerifier accepts PKIX ("PUBLIC KEY") and PKCS#1 ("RSA PUBLIC KEY") PEM blocks.
func NewVerifier(publicKeyPEM []byte) (*Verifier, error)

// Verify returns the claims of a valid token, or ErrTokenExpired / ErrTokenInvalid.
func (v *Verifier) Verify(accessToken string) (*Claims, error)
```

`Verify` enforces RS256 explicitly — rejecting `alg: none` and any HMAC algorithm, which
would otherwise let an attacker sign tokens with the *public* key as an HMAC secret. It
uses `jwt.WithExpirationRequired()` and maps failures onto the existing sentinels in
`pkg/auth/errors.go`. No `context.Context` parameter: there is nothing to cancel.

`Claims` (`pkg/auth/token.go:20`) is unchanged and already carries what we need:

```go
type Claims struct {
	Subject     uuid.UUID    `json:"sub"`
	TenantID    uuid.UUID    `json:"tid"`
	Email       string       `json:"email"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"perms"`
	AuthMethod  ProviderType `json:"auth_method"`
	IssuedAt    time.Time    `json:"iat"`
	ExpiresAt   time.Time    `json:"exp"`
}
```

To keep nine `main.go` files from each growing PEM plumbing:

```go
// VerifierFromSecrets loads the public key from a secrets.Source:
// "jwt_public.pem" (FileSource, local) or "iam/jwt-public-key" (VaultSource, prod).
func VerifierFromSecrets(ctx context.Context, src secrets.Source) (*Verifier, error)
```

`pkg/secrets.Source` is already name-agnostic (`GetSecret(ctx, name) ([]byte, error)`).
`infra/local/keys/jwt_public.pem` is already committed. Today **nothing loads it** —
`grep jwt_public` returns zero hits.

## 5. Fail-closed interceptor — `pkg/interceptor`

```go
func UnaryAuthInterceptor(v *auth.Verifier, public map[string]string) grpc.UnaryServerInterceptor
func StreamAuthInterceptor(v *auth.Verifier, public map[string]string) grpc.StreamServerInterceptor
```

The `public` map is method name → **reason it is public** (§6). Values are never read at
runtime; they exist so the allowlist documents itself at its definition.

For any method **not** in `public`:

1. Read `authorization` from incoming metadata. Absent → `codes.Unauthenticated`.
2. Require a case-insensitive `Bearer ` prefix. Malformed → `codes.Unauthenticated`.
3. `v.Verify(token)`. Expired → `Unauthenticated` ("token expired"); any other failure →
   `Unauthenticated` ("token invalid"). **Never** distinguish "bad signature" from
   "malformed" in the message returned to the client.
4. Build `CallerInfo` from the claims. Populate `tenant.WithID` and `audit.WithActor`
   exactly as `UnaryCallerInterceptor` does today.

For a method in `public`: proceed with an empty `CallerInfo`. `RequireRole` still denies.

**`x-caller-id`, `x-caller-role`, `x-caller-email`, `x-tenant-id` are read from metadata
nowhere.** Not preferred-if-absent; not merged; not read. `x-project-id` survives — it is a
resource selector, not identity. `x-forwarded-for` survives for audit IP.

`UnaryCallerInterceptor` / `StreamCallerInterceptor` are **deleted**, not deprecated. A
header-trusting interceptor that still compiles is one `main.go` edit away from being
re-enabled.

### Why the stream half ships too

There are zero streaming RPCs today (`grep 'returns (stream'` → nothing), so the unary
interceptor achieves complete coverage. But `pkg/server` already installs stream
interceptors, and five services already pass `StreamCallerInterceptor`. Shipping only the
unary half leaves a fail-open path that opens itself the day someone adds a stream. Both,
or the design has a hole with a timer on it.

## 6. The allowlist, with reasons

```go
// PublicMethods maps a fully-qualified gRPC method to the reason it may be
// called without a valid access token. An entry here is a deliberate,
// reviewable decision — silence in this map means "authentication required."
var PublicMethods = map[string]string{
	"/thittam.iam.v1.IAMService/Login":            "caller has no token yet, by definition",
	"/thittam.iam.v1.IAMService/RefreshToken":     "presents a refresh token, not an access token",
	"/thittam.iam.v1.IAMService/AcceptInvitation": "invitee has no account yet; the invitation token is in the path",

	// Service-to-service. Neither GRANTS anything: CheckPermission answers a bool
	// from the database, and the calling service authorizes against its own
	// verified caller. Residual risk is an information leak — anyone reaching
	// iam's port can enumerate whether a user holds a permission. Accepted for
	// this slice; closed by service tokens in #139.
	"/thittam.iam.v1.IAMService/CheckPermission": "service-to-service; grants nothing; see #139",
	"/thittam.iam.v1.IAMService/ValidateToken":   "service-to-service verification oracle; grants nothing; see #139",

	// Registered only when Config.EnableReflection is set (default false).
	"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo":      "reflection, dev only",
	"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo": "reflection, dev only",
}
```

**Deliberate omissions**, recorded because an allowlist's silences matter as much as its entries:

- **`Logout`** — takes a refresh token in the body, but the caller holds an access token.
  Requiring it costs nothing and closes an unauthenticated write.
- **`HandlePaymentWebhook`** — nothing routes to it today (no `google.api.http` annotation),
  so failing closed breaks nothing. When webhooks go live they need gateway **signature**
  verification, not a JWT. Tracked in #139.
- **`GetCurrentUser`** (`/api/v1/auth/me`) — the web client calls it immediately after
  login with the freshly-issued bearer token. It is authenticated, and must stay so: it is
  how the client learns its own tenant.

### Guarding the allowlist against drift

A unit test asserts that every key in `PublicMethods` is a method that actually exists in
the compiled service descriptors. A typo — `/thittam.iam.v1.IAMService/Login ` with a
trailing space — would otherwise create a private method that silently fails closed
(annoying) or, worse, a stale entry that keeps a since-renamed method public (dangerous).

## 7. `CallerInfo` derives from claims

The token asserts `roles []string`. `CallerInfo.Role` is a single string, and
`RequireRole` compares equality. The bridge between them is a "highest role" flattening
that Kong was supposed to perform, that does not exist, and whose ordering is written down
nowhere.

```go
type CallerInfo struct {
	UserID      uuid.UUID
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	Email       string
	Roles       []string  // was: Role string
	Permissions []string  // new — from the verified token
	IP          string
}

// RequireRole returns PermissionDenied unless the caller's verified roles
// contain `required`.
func RequireRole(ctx context.Context, required string) error
```

Membership, not equality. This fixes a live bug class: a user holding
`[viewer, tenant_admin]` today passes or fails a `tenant_admin` check depending on a
flattening that never happens.

Ten call sites in `services/iam/handler.go` keep their signatures unchanged; only the
comparison inside `RequireRole` changes. `RoleTenantAdmin` and `RoleMember` are defined but
referenced nowhere outside tests — left as-is.

`CallerInfo.TenantID` comes from the `tid` claim. `x-tenant-id` is not consulted, so it can
be neither spoofed nor required.

## 8. Wiring, and two other fail-open paths

**All ten services** pass `UnaryAuthInterceptor` and `StreamAuthInterceptor` via
`Config.ExtraUnaryInterceptors` / `ExtraStreamInterceptors`. Five gain a caller in context
for the first time: **billing, document, general-ledger, notifications, reporting-analytics**
(`cmd/billing/main.go:99`, `cmd/document/main.go:80`, `cmd/general-ledger/main.go:69`,
`cmd/notifications/main.go:105`, `cmd/reporting-analytics/main.go:91`).

`pkg/server.Config` gains no verifier field — the interceptor is constructed in each
`main.go` from `auth.VerifierFromSecrets` and injected through the existing `Extra*`
slices. That keeps `pkg/server` unaware of authentication, which is where it should stay.

**`RequirePermission` must deny when its checker is nil.** Every service wires it as
`if iamPerm != nil { handler = handler.WithPermissionChecker(iamPerm) }`, and
`iamclient.DialFromEnv` can return nil. Today the ~18 permission checks silently no-op when
IAM dialing is unconfigured. A permission check that passes because a dial failed is not a
check. Nil checker → `codes.Internal` ("permission checker unavailable"), never nil.

**Reflection** moves behind `Config.EnableReflection` (default **false**), set from
`GRPC_REFLECTION` in `scripts/dev-start.sh`. `pkg/server/server.go:112` currently calls
`reflection.Register(gs)` unconditionally, on every service, in every environment.

**`cmd/project-management/main.go`'s `WithIncomingHeaderMatcher`** stops forwarding
`X-Caller-*` and `X-Tenant-Id`. It keeps `X-Project-Id`. `Authorization` reaches metadata
without a matcher — it is a permanent HTTP header, which is why iam's gateway works today
with no matcher at all.

**`web/src/lib/api/client.ts`** stops sending `X-Caller-Id`, `X-Caller-Email`,
`X-Caller-Role`, and `X-Tenant-Id`. Harmless once ignored, but leaving them invites the next
reader to believe they mean something. `Authorization: Bearer` stays.

## 9. What we are not pretending to fix

**Impersonation does not impersonate.** `StartImpersonation` (`services/iam/handler.go:551`)
writes an `impersonation_session` row and an audit entry. It mints no token, sets no `act`
claim, and `CallerInfo` has no impersonation field. Subsequent requests still carry the
platform admin's own identity. The audit log records a session the request path knows
nothing about. This spec does not change that; it is noted so the reader does not assume
the verified claims carry an impersonation subject. #139.

## 10. Testing

**`pkg/auth`** — `Verify` accepts a token signed by the matching private key; rejects an
expired token (`ErrTokenExpired`); a token signed by a *different* key; a token with
`alg: none`; a token HMAC-signed using the public key as the secret; and garbage. The
`alg` cases are the ones that matter: they are the difference between a verifier and a
decoder.

**`pkg/interceptor`** — table-driven:

- every method in `PublicMethods` proceeds with no token
- a representative private method returns `Unauthenticated` with: no metadata, no
  `authorization` key, `Authorization: <token>` (no `Bearer`), an expired token, a
  wrong-key token
- a valid token yields `CallerInfo` populated from the claims, and `tenant.FromContext`
  returns the `tid` tenant
- **`x-caller-role: platform_admin` sent alongside a valid token whose claims carry only
  `[member]` does not escalate** — `RequireRole(platform_admin)` denies. This single test
  is the entire point of the change.
- `RequireRole` succeeds on membership in a multi-role claim, denies otherwise
- `RequirePermission` with a nil checker returns `Internal`, not success

**`services/iam/handler_test.go`** — helpers move from `Role:` to `Roles:`.

**One integration test** dials a real `pkg/server` through the full interceptor chain with a
token signed by a test key, asserting a private RPC rejects tokenless and accepts a valid
token. Every existing handler test calls handlers directly, and `e2e/critical_path`
authenticates by fiat via `stubTokenIssuer` — `grep -r 'Bearer\|Authorization' e2e/critical_path/`
returns zero. **Nothing in the current suite would notice if the interceptor were deleted.**

Coverage: `iam` threshold is ≥ 85%; `pkg/` follows the ≥ 75% default. Neither may regress.

## 11. Blast radius

`pkg/interceptor` and `pkg/auth` are linked by all ten services, so this changes every
service's request path in one branch. Per CLAUDE.md: senior-engineer review required
(`iam` / security), two approvals.

Deleting `UnaryCallerInterceptor` widens the change but is deliberate — see §5.

A whole-tree `go vet ./...` is the gate. `go build ./services/...` will not surface the e2e
doubles or the five `cmd/*` wirings.

## 12. Rollout

There is no flag and no phased enablement. A partially-authenticated fleet is a fleet with
a bypass, and the flag would be the bypass.

Before merge, confirm:

- `jwt_public.pem` is present in every environment's secret source under the names
  `VerifierFromSecrets` expects (`jwt_public.pem` / `iam/jwt-public-key`). **A service that
  cannot load the key must refuse to start**, not start unauthenticated.
- `scripts/dev-start.sh` exports `GRPC_REFLECTION=1` so `grpcurl` keeps working locally.
