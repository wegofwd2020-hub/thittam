# Verify the JWT In-Process, Fail Closed — Implementation Plan (#138)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Thittam gRPC service verify the caller's JWT itself and reject unauthenticated requests, replacing today's trust in forgeable `x-caller-*` headers.

**Architecture:** A verify-only `auth.Verifier` (RSA public key, no Redis, no private key) is constructed in each of the ten `cmd/*/main.go` and passed to a new fail-closed `interceptor.UnaryAuthInterceptor` / `StreamAuthInterceptor`. Any method absent from an explicit, self-documenting `PublicMethods` allowlist requires a valid `Authorization: Bearer` token; `CallerInfo` is built from the verified claims and `x-caller-*` metadata is read nowhere. The old header-trusting interceptors are deleted.

**Tech Stack:** Go 1.22+, `github.com/golang-jwt/jwt/v5`, gRPC, `pkg/secrets` (FileSource/VaultSource), testify, `google.golang.org/grpc/test/bufconn`.

**Spec:** `docs/superpowers/specs/2026-07-09-verify-jwt-fail-closed-138-design.md`

## Global Constraints

- **This is a security change.** Per CLAUDE.md: senior-engineer review, 2 approvals (`iam`/security). Every task must leave the tree building and every test passing.
- **Whole-tree `go vet ./...` is the gate.** `go build ./services/...` misses the ten `cmd/*` wirings and the e2e doubles.
- **errcheck runs in CI; golangci-lint is not installed here.** Check every error return.
- **Local DB safety (CLAUDE.md, non-negotiable):** Never run `docker compose … -v` / `down` / `up` against `infra/local/`. That compose is project-scoped; `-v` deletes ALL local volumes (it once destroyed unrelated MinIO dev data). **No task in this plan needs a database or Docker at all.**
- **`slog`, no PII, no secrets.** Never log a token, a key, or the `authorization` metadata value. A token in a log line is a credential in a log aggregator.
- **Never return a distinguishing error to the client.** Expired, malformed, wrong-key, and absent all produce `codes.Unauthenticated`. Do not tell an attacker which of those it was, beyond "token expired" vs "token invalid" (the former is needed by clients to trigger a refresh).
- **Coverage:** `iam` ≥ 85%; `pkg/` ≥ 75% (default). Neither may regress.
- **Error wrapping:** `fmt.Errorf("auth: <op>: %w", err)` inline, matching `pkg/auth`.
- **Commits:** Conventional Commits, scope `iam` for `pkg/auth`/`pkg/interceptor`/services, `infra` for scripts, `api` for the web client.

## File Structure

| File | Responsibility |
|---|---|
| `pkg/auth/verifier.go` | new — public-key-only `Verifier`, `NewVerifier`, `Verify`, `VerifierFromEnv` |
| `pkg/auth/verifier_test.go` | new — the `alg` attacks, expiry, wrong key |
| `pkg/interceptor/auth.go` | `CallerInfo.Roles`, `RequireRole` membership; later, delete the header-trust interceptors |
| `pkg/interceptor/public.go` | new — `PublicMethods`, typed against generated `FullMethodName` constants |
| `pkg/interceptor/authjwt.go` | new — `UnaryAuthInterceptor`, `StreamAuthInterceptor` |
| `pkg/interceptor/authjwt_test.go` | new — the escalation test that is the point of this change |
| `pkg/interceptor/permission.go` | nil checker → deny |
| `pkg/server/server.go` | `Config.EnableReflection`, default false |
| `cmd/*/main.go` (×10) | construct the verifier; pass the auth interceptor |
| `scripts/dev-start.sh` | export `IAM_SERVICE_ADDR` and `GRPC_REFLECTION` |
| `web/src/lib/api/client.ts` | stop sending `X-Caller-*` and `X-Tenant-Id` |
| `pkg/server/integration_test.go` | new — first bufconn harness; proves the chain rejects tokenless calls |

**Execution order rationale.** Tasks 1–5 are additive and leave behaviour unchanged. Task 6 flips five already-wired services onto the new interceptor. Task 7 wires the five that never had one and *deletes* the old interceptors — deletion lands only once nothing references them. Tasks 8–9 are client-side and test-side. No task ends on a red tree.

---

### Task 1: `pkg/auth` — a verify-only Verifier

**Files:**
- Create: `pkg/auth/verifier.go`
- Create: `pkg/auth/verifier_test.go`

**Interfaces:**
- Consumes: `Claims` (`pkg/auth/token.go:19`), `ErrTokenExpired`/`ErrTokenInvalid` (`pkg/auth/errors.go`), `secrets.Source` (`pkg/secrets/secrets.go:53`).
- Produces, relied on by Tasks 3, 6, 7:
  ```go
  type Verifier struct{ /* unexported */ }
  func NewVerifier(publicKeyPEM []byte) (*Verifier, error)
  func (v *Verifier) Verify(accessToken string) (*Claims, error)
  func VerifierFromEnv(ctx context.Context) (*Verifier, error)
  ```

**Why a new type.** `JWTIssuer.Validate` (`pkg/auth/jwt.go:202`) verifies against `&j.privateKey.PublicKey` — it *is* the issuer. Nine services must verify without ever holding the signing key.

**Constraint:** no database, no Docker, no network. Pure unit tests.

- [ ] **Step 1: Write the failing tests**

Create `pkg/auth/verifier_test.go`. Generate an RSA key in-test (`rsa.GenerateKey(rand.Reader, 2048)`) and marshal the public half with `x509.MarshalPKIXPublicKey` → `pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", ...})`.

```go
package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeyPair(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return key, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func signRS256(t *testing.T, key *rsa.PrivateKey, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		TenantID: uuid.New().String(),
		Email:    "user@example.com",
		Roles:    []string{"member", "tenant_admin"},
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func TestVerifier_ValidToken(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	claims, err := v.Verify(signRS256(t, key, time.Now().Add(time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", claims.Email)
	assert.Equal(t, []string{"member", "tenant_admin"}, claims.Roles)
	assert.NotEqual(t, uuid.Nil, claims.Subject)
	assert.NotEqual(t, uuid.Nil, claims.TenantID)
}

func TestVerifier_Expired(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)
	_, err := v.Verify(signRS256(t, key, time.Now().Add(-time.Minute)))
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestVerifier_WrongKey(t *testing.T) {
	t.Parallel()
	attacker, _ := testKeyPair(t)
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)
	_, err := v.Verify(signRS256(t, attacker, time.Now().Add(time.Hour)))
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// A token with alg:none must never be accepted. jwt/v5 refuses to sign with
// "none" unless handed the sentinel key, which is exactly how an attacker
// crafts one.
func TestVerifier_AlgNone_Rejected(t *testing.T) {
	t.Parallel()
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: uuid.New().String(),
	})
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, verr := v.Verify(s)
	assert.ErrorIs(t, verr, ErrTokenInvalid)
}

// Algorithm confusion: sign with HMAC using the PUBLIC key bytes as the shared
// secret. The public key is not secret, so a verifier that trusts the token's
// own `alg` header would accept this. This is the classic RS256->HS256 attack.
func TestVerifier_HMACConfusion_Rejected(t *testing.T) {
	t.Parallel()
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: uuid.New().String(),
		Roles:    []string{"platform_admin"},
	})
	s, err := tok.SignedString(pub) // the public key, as an HMAC secret
	require.NoError(t, err)

	_, verr := v.Verify(s)
	assert.ErrorIs(t, verr, ErrTokenInvalid)
}

func TestVerifier_Garbage(t *testing.T) {
	t.Parallel()
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)
	for _, s := range []string{"", "not-a-token", "a.b.c"} {
		_, err := v.Verify(s)
		assert.ErrorIs(t, err, ErrTokenInvalid, "input %q", s)
	}
}

func TestNewVerifier_BadPEM(t *testing.T) {
	t.Parallel()
	for _, pem := range [][]byte{nil, []byte(""), []byte("-----BEGIN PUBLIC KEY-----\ngarbage\n-----END PUBLIC KEY-----")} {
		_, err := NewVerifier(pem)
		assert.Error(t, err)
	}
}

// A PRIVATE key PEM must be rejected: services must never be handed one, and
// silently accepting it would hide a catastrophic misconfiguration.
func TestNewVerifier_RejectsPrivateKeyPEM(t *testing.T) {
	t.Parallel()
	key, _ := testKeyPair(t)
	der := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	_, err := NewVerifier(privPEM)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/auth/ -run TestVerifier -v`
Expected: FAIL — `undefined: NewVerifier`. That is the correct first failure.

- [ ] **Step 3: Implement the Verifier**

Create `pkg/auth/verifier.go`. Mirror `JWTIssuer.Validate`'s parse block (`pkg/auth/jwt.go:202-247`) exactly — including the `*jwt.SigningMethodRSA` type assertion, which is what defeats both `alg: none` and HMAC confusion — but key off a public key.

```go
package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/secrets"
)

// Verifier checks access-token signatures using only the RSA public key.
//
// It holds no private key and needs no Redis: access tokens are short-lived
// (DefaultAccessTTL) and are not revocable — only refresh tokens are, and those
// never reach a service other than iam. Every service can therefore verify a
// caller's token locally, with no network call and nothing worth stealing.
type Verifier struct {
	publicKey *rsa.PublicKey
}

// NewVerifier accepts PKIX ("PUBLIC KEY") and PKCS#1 ("RSA PUBLIC KEY") PEM.
// A private-key PEM is rejected: a service holding one is a misconfiguration
// severe enough that it must not start.
func NewVerifier(publicKeyPEM []byte) (*Verifier, error) {
	if len(publicKeyPEM) == 0 {
		return nil, errors.New("auth: verifier: public key PEM is empty")
	}
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, errors.New("auth: verifier: failed to decode PEM block")
	}

	switch block.Type {
	case "PUBLIC KEY": // PKIX
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: verifier: parse PKIX key: %w", err)
		}
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("auth: verifier: PKIX key is not RSA")
		}
		return &Verifier{publicKey: rsaKey}, nil

	case "RSA PUBLIC KEY": // PKCS#1
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: verifier: parse PKCS#1 key: %w", err)
		}
		return &Verifier{publicKey: key}, nil

	default:
		return nil, fmt.Errorf("auth: verifier: unsupported PEM block type %q "+
			"(a service must be given a PUBLIC key, never a private one)", block.Type)
	}
}

// Verify parses and verifies an access token, returning its claims.
// Returns ErrTokenExpired or ErrTokenInvalid — and nothing more specific,
// so a caller cannot use the error text as an oracle.
func (v *Verifier) Verify(accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		accessToken,
		&jwtClaims{},
		func(t *jwt.Token) (interface{}, error) {
			// Rejects alg:none and every HMAC algorithm. Without this, a token
			// signed with HS256 using the (public, therefore known) key bytes
			// as the shared secret would verify.
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("auth: verifier: unexpected signing method %v", t.Header["alg"])
			}
			return v.publicKey, nil
		},
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	c, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	tenantID, err := uuid.Parse(c.TenantID)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	return &Claims{
		Subject:     userID,
		TenantID:    tenantID,
		Email:       c.Email,
		Roles:       c.Roles,
		Permissions: c.Permissions,
		AuthMethod:  c.AuthMethod,
		IssuedAt:    c.IssuedAt.Time,
		ExpiresAt:   c.ExpiresAt.Time,
	}, nil
}

// VerifierFromEnv loads the JWT public key the same way cmd/iam selects its
// secret source: VAULT_ADDR present → Vault ("iam/jwt-public-key"); absent →
// files under IAM_KEY_DIR (default ./keys), name "jwt_public.pem".
//
// A service that cannot load the key must refuse to start. Returning an error
// here — rather than a permissive fallback — is what makes that true.
func VerifierFromEnv(ctx context.Context) (*Verifier, error) {
	var src secrets.Source
	var name string

	if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		src = secrets.NewVaultSource(secrets.VaultConfig{
			Address:  addr,
			Mount:    getenvOr("VAULT_KV_MOUNT", "secret"),
			RoleID:   os.Getenv("VAULT_ROLE_ID"),
			SecretID: os.Getenv("VAULT_SECRET_ID"),
		})
		name = "iam/jwt-public-key"
	} else {
		src = secrets.NewFileSource(getenvOr("IAM_KEY_DIR", "./keys"))
		name = "jwt_public.pem"
	}

	loadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pemBytes, err := src.GetSecret(loadCtx, name)
	if err != nil {
		return nil, fmt.Errorf("auth: verifier: load %s: %w", name, err)
	}
	return NewVerifier(pemBytes)
}

func getenvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

Check that `pkg/secrets` does not import `pkg/auth` before adding this import (`grep -rn 'pkg/auth' pkg/secrets/`); it must not, or you have an import cycle.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/auth/ -run 'TestVerifier|TestNewVerifier' -v`
Expected: PASS, all 7 tests. `TestVerifier_HMACConfusion_Rejected` and `TestVerifier_AlgNone_Rejected` are the two that matter — if either fails, the verifier is a decoder.

- [ ] **Step 5: Full package + vet**

Run: `go test ./pkg/auth/ && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 6: Commit**

```bash
git add pkg/auth/verifier.go pkg/auth/verifier_test.go
git commit -m "feat(iam): public-key-only JWT verifier (#138)"
```

---

### Task 2: `CallerInfo.Roles` — membership, not a flattened string

**Files:**
- Modify: `pkg/interceptor/auth.go` (struct ~line 37; `RequireRole` ~67-76; both interceptor constructions ~88, ~124)
- Modify: `pkg/interceptor/auth_test.go` (lines 50, 74, 123, 184, 232, 262, 268, 283)
- Modify: `services/iam/handler_test.go` (line 23)

**Interfaces:**
- Produces, relied on by Tasks 3, 6, 7:
  ```go
  type CallerInfo struct {
      UserID, TenantID, ProjectID uuid.UUID
      Email       string
      Roles       []string
      Permissions []string
      IP          string
  }
  func RequireRole(ctx context.Context, required string) error  // membership
  ```

**Why.** The token asserts `roles []string` (`auth.Claims.Roles` is *already* `[]string`). `CallerInfo.Role` is one string, bridged by a "highest role" flattening that Kong was meant to perform, does not exist, and whose ordering is written down nowhere. A user holding `[viewer, tenant_admin]` currently passes or fails a `tenant_admin` check depending on that non-existent flattening.

This task changes **no behaviour** — the old interceptors keep reading `x-caller-role` and store it as a one-element slice. It exists so Task 3 has the type it needs, on a green tree.

**Constraint:** no database, no Docker.

- [ ] **Step 1: Change the struct and `RequireRole`**

In `pkg/interceptor/auth.go`:

```go
// CallerInfo holds the authenticated caller's identity, derived from the
// verified access token (see UnaryAuthInterceptor).
type CallerInfo struct {
	UserID      uuid.UUID
	TenantID    uuid.UUID
	ProjectID   uuid.UUID // x-project-id; uuid.Nil for non-project-scoped requests
	Email       string
	Roles       []string
	Permissions []string
	IP          string
}

// HasRole reports whether the caller holds the named role.
func (c CallerInfo) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// RequireRole returns PermissionDenied unless the caller's verified roles
// contain `required`. Membership, not equality: a token asserting
// [viewer, tenant_admin] satisfies RequireRole(tenant_admin).
func RequireRole(ctx context.Context, required string) error {
	caller, ok := CallerFromContext(ctx)
	if !ok {
		return status.Error(codes.PermissionDenied, "caller identity not present in context")
	}
	if !caller.HasRole(required) {
		return status.Errorf(codes.PermissionDenied, "requires role %s", required)
	}
	return nil
}
```

Note the error message no longer echoes the caller's roles back. It told an attacker what they had; it never told a legitimate user anything they didn't know.

- [ ] **Step 2: Update the two existing interceptor constructions**

In `UnaryCallerInterceptor` (~line 88) and `StreamCallerInterceptor` (~line 124), replace `Role: firstMD(md, "x-caller-role"),` with:

```go
		Roles: rolesFromMD(md, "x-caller-role"),
```

and add the helper next to `firstMD`:

```go
// rolesFromMD reads the legacy single-valued x-caller-role header into a slice.
// Transitional: both header-trusting interceptors are deleted in #138 Task 7.
func rolesFromMD(md metadata.MD, key string) []string {
	if v := firstMD(md, key); v != "" {
		return []string{v}
	}
	return nil
}
```

- [ ] **Step 3: Update the tests**

`pkg/interceptor/auth_test.go`: `CallerInfo{Role: X}` → `CallerInfo{Roles: []string{X}}` at lines 50, 262, 268, 283. Asserts: `assert.Equal(t, RolePlatformAdmin, caller.Role)` → `assert.Equal(t, []string{RolePlatformAdmin}, caller.Roles)` (line 74), similarly line 184 for `RoleTenantAdmin`; `assert.Empty(t, caller.Role)` → `assert.Empty(t, caller.Roles)` (lines 123, 232).

Add one new test that pins the actual fix:

```go
func TestRequireRole_MembershipInMultiRoleCaller(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{Roles: []string{RoleMember, RoleTenantAdmin}})
	assert.NoError(t, RequireRole(ctx, RoleTenantAdmin), "membership, not equality")
	assert.NoError(t, RequireRole(ctx, RoleMember))
	assert.Error(t, RequireRole(ctx, RolePlatformAdmin))
}
```

`services/iam/handler_test.go:23`: `Role: interceptor.RolePlatformAdmin,` → `Roles: []string{interceptor.RolePlatformAdmin},`.

- [ ] **Step 4: Whole-tree vet, then the affected suites**

Run: `go vet ./...`
Expected: clean. If anything outside `pkg/interceptor` and `services/iam` fails, a fourth consumer of `CallerInfo.Role` exists that this plan did not anticipate — stop and report it.

Run: `go test ./pkg/interceptor/... ./services/iam/... -short`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/interceptor/auth.go pkg/interceptor/auth_test.go services/iam/handler_test.go
git commit -m "refactor(iam): CallerInfo carries verified roles; RequireRole checks membership (#138)"
```

---

### Task 3: The fail-closed interceptor and its allowlist

**Files:**
- Create: `pkg/interceptor/public.go`
- Create: `pkg/interceptor/authjwt.go`
- Create: `pkg/interceptor/authjwt_test.go`

**Interfaces:**
- Consumes: `auth.Verifier`, `auth.ErrTokenExpired` (Task 1); `CallerInfo`, `WithCaller` (Task 2).
- Produces, relied on by Tasks 6, 7:
  ```go
  var PublicMethods map[string]string   // method -> reason it is public
  func UnaryAuthInterceptor(v *auth.Verifier, public map[string]string) grpc.UnaryServerInterceptor
  func StreamAuthInterceptor(v *auth.Verifier, public map[string]string) grpc.StreamServerInterceptor
  ```

**Nothing uses these yet.** The tree stays green; Tasks 6 and 7 wire them.

**Constraint:** no database, no Docker. Pure unit tests.

- [ ] **Step 1: Write the allowlist, typed against generated constants**

Create `pkg/interceptor/public.go`. `gen/iam/v1/iam_grpc.pb.go` exports `FullMethodName` constants, so a renamed RPC breaks the **build** instead of silently leaving a stale entry public.

```go
package interceptor

import iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"

// Reflection method names have no generated constants — the grpc-go reflection
// package does not export them. They are only reachable when a server is built
// with Config.EnableReflection (default false).
const (
	reflectionV1      = "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"
	reflectionV1Alpha = "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"
)

// PublicMethods maps a fully-qualified gRPC method to the reason it may be
// called without a valid access token.
//
// An entry here is a deliberate, reviewable decision. Silence in this map means
// "authentication required" — that is the point of the map's existence, and why
// it is keyed on generated constants rather than string literals.
//
// Deliberately ABSENT, and why:
//
//	Logout               — carries a refresh token in the body, but the caller
//	                       holds an access token. Requiring it costs nothing and
//	                       closes an unauthenticated write.
//	HandlePaymentWebhook — nothing routes to it (no google.api.http annotation),
//	                       so failing closed breaks nothing. When webhooks ship
//	                       they need gateway signature verification, not a JWT (#139).
//	GetCurrentUser       — the web client calls it right after login with the
//	                       freshly-issued bearer token. It is how a client learns
//	                       its own tenant, and it must stay authenticated.
var PublicMethods = map[string]string{
	iamv1.IAMService_Login_FullMethodName:            "caller has no token yet, by definition",
	iamv1.IAMService_RefreshToken_FullMethodName:     "presents a refresh token, not an access token",
	iamv1.IAMService_AcceptInvitation_FullMethodName: "invitee has no account yet; the invitation token is in the path",

	// Service-to-service. Neither GRANTS anything: CheckPermission answers a
	// bool from the database, and the calling service authorizes against its
	// own verified caller. Residual risk is an information leak — anyone who
	// reaches iam's port can enumerate whether a user holds a permission.
	// Accepted for this change; closed by service tokens in #139.
	iamv1.IAMService_CheckPermission_FullMethodName: "service-to-service; grants nothing; see #139",
	iamv1.IAMService_ValidateToken_FullMethodName:   "service-to-service verification oracle; grants nothing; see #139",

	reflectionV1:      "reflection; only registered when Config.EnableReflection",
	reflectionV1Alpha: "reflection; only registered when Config.EnableReflection",
}
```

- [ ] **Step 2: Write the failing interceptor tests**

Create `pkg/interceptor/authjwt_test.go`. Build a verifier from an in-test key, exactly as Task 1's `testKeyPair` does (duplicate the helper here — it is unexported in `pkg/auth`).

```go
package interceptor

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/wegofwd2020/thittam/pkg/auth"
	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/tenant"
)

const privateMethod = "/thittam.ledger.v1.LedgerService/PostJournalEntry"

func testVerifier(t *testing.T) (*auth.Verifier, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	v, err := auth.NewVerifier(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	require.NoError(t, err)
	return v, key
}

// mintToken signs a token with the given roles and expiry. Claim names must
// match pkg/auth's wire format: sub, tid, email, roles, perms.
func mintToken(t *testing.T, key *rsa.PrivateKey, tenantID uuid.UUID, roles []string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   uuid.New().String(),
		"tid":   tenantID.String(),
		"email": "user@example.com",
		"roles": roles,
		"perms": []string{},
		"exp":   jwt.NewNumericDate(exp),
		"iat":   jwt.NewNumericDate(time.Now()),
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)
	return s
}

// invoke runs the unary auth interceptor and reports the handler's context
// (nil if the interceptor rejected the call) plus the returned error.
func invoke(t *testing.T, v *auth.Verifier, method string, md metadata.MD) (context.Context, error) {
	t.Helper()
	ctx := context.Background()
	if md != nil {
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	var captured context.Context
	_, err := UnaryAuthInterceptor(v, PublicMethods)(
		ctx, nil, &grpc.UnaryServerInfo{FullMethod: method},
		func(c context.Context, _ interface{}) (interface{}, error) { captured = c; return nil, nil },
	)
	return captured, err
}

func TestAuth_PublicMethods_PassWithoutToken(t *testing.T) {
	t.Parallel()
	v, _ := testVerifier(t)
	for method, reason := range PublicMethods {
		ctx, err := invoke(t, v, method, nil)
		require.NoError(t, err, "public method %s (%s) must not require a token", method, reason)
		require.NotNil(t, ctx)
		caller, _ := CallerFromContext(ctx)
		assert.Empty(t, caller.Roles, "a public call has no verified identity")
	}
}

func TestAuth_PrivateMethod_RejectsBadCredentials(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	_, wrongKey := testVerifier(t)

	cases := []struct{ name string; md metadata.MD }{
		{"no metadata", nil},
		{"no authorization key", metadata.Pairs("x-caller-id", uuid.New().String())},
		{"missing Bearer prefix", metadata.Pairs("authorization", mintToken(t, key, uuid.New(), []string{"member"}, time.Now().Add(time.Hour)))},
		{"empty bearer", metadata.Pairs("authorization", "Bearer ")},
		{"expired", metadata.Pairs("authorization", "Bearer "+mintToken(t, key, uuid.New(), []string{"member"}, time.Now().Add(-time.Minute)))},
		{"wrong key", metadata.Pairs("authorization", "Bearer "+mintToken(t, wrongKey, uuid.New(), []string{"member"}, time.Now().Add(time.Hour)))},
		{"garbage", metadata.Pairs("authorization", "Bearer not.a.token")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := invoke(t, v, privateMethod, tc.md)
			assert.Nil(t, ctx, "handler must not run")
			require.Error(t, err)
			assert.Equal(t, codes.Unauthenticated, status.Code(err))
		})
	}
}

func TestAuth_ValidToken_PopulatesCallerAndTenant(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	tid := uuid.New()
	md := metadata.Pairs(
		"authorization", "Bearer "+mintToken(t, key, tid, []string{"member", "tenant_admin"}, time.Now().Add(time.Hour)),
		"x-forwarded-for", "203.0.113.9",
	)

	ctx, err := invoke(t, v, privateMethod, md)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	caller, ok := CallerFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, []string{"member", "tenant_admin"}, caller.Roles)
	assert.Equal(t, tid, caller.TenantID)
	assert.Equal(t, "user@example.com", caller.Email)
	assert.Equal(t, "203.0.113.9", caller.IP)

	gotTenant, ok := tenant.IDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, tid, gotTenant)
}

// THE test. A valid non-admin token, accompanied by forged caller headers
// asserting platform_admin. Before #138 this escalated. It must not now.
func TestAuth_ForgedCallerHeaders_DoNotEscalate(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	md := metadata.Pairs(
		"authorization", "Bearer "+mintToken(t, key, uuid.New(), []string{RoleMember}, time.Now().Add(time.Hour)),
		"x-caller-role", RolePlatformAdmin,
		"x-caller-id", uuid.New().String(),
		"x-caller-email", "attacker@evil.example",
		"x-tenant-id", uuid.New().String(),
	)

	ctx, err := invoke(t, v, privateMethod, md)
	require.NoError(t, err)

	caller, _ := CallerFromContext(ctx)
	assert.Equal(t, []string{RoleMember}, caller.Roles, "roles come from the token, never the headers")
	assert.Equal(t, "user@example.com", caller.Email, "email comes from the token")
	assert.Error(t, RequireRole(ctx, RolePlatformAdmin), "forged x-caller-role must not escalate")
}

// x-tenant-id must not override the token's tid claim.
func TestAuth_ForgedTenantHeader_Ignored(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	realTenant, forged := uuid.New(), uuid.New()
	md := metadata.Pairs(
		"authorization", "Bearer "+mintToken(t, key, realTenant, []string{RoleMember}, time.Now().Add(time.Hour)),
		"x-tenant-id", forged.String(),
	)
	ctx, err := invoke(t, v, privateMethod, md)
	require.NoError(t, err)
	caller, _ := CallerFromContext(ctx)
	assert.Equal(t, realTenant, caller.TenantID)
	assert.NotEqual(t, forged, caller.TenantID)
}

// x-project-id survives: it is a resource selector, not identity.
func TestAuth_ProjectHeader_StillRead(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	pid := uuid.New()
	md := metadata.Pairs(
		"authorization", "Bearer "+mintToken(t, key, uuid.New(), []string{RoleMember}, time.Now().Add(time.Hour)),
		"x-project-id", pid.String(),
	)
	ctx, err := invoke(t, v, privateMethod, md)
	require.NoError(t, err)
	caller, _ := CallerFromContext(ctx)
	assert.Equal(t, pid, caller.ProjectID)
}

func TestAuth_BearerPrefixIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	tok := mintToken(t, key, uuid.New(), []string{RoleMember}, time.Now().Add(time.Hour))
	for _, prefix := range []string{"Bearer ", "bearer ", "BEARER "} {
		ctx, err := invoke(t, v, privateMethod, metadata.Pairs("authorization", prefix+tok))
		require.NoError(t, err, "prefix %q", prefix)
		assert.NotNil(t, ctx)
	}
}

// The allowlist must never contain a method that no longer exists. The iam
// entries are generated constants and cannot rot; these two are literals.
func TestPublicMethods_ReflectionNamesWellFormed(t *testing.T) {
	t.Parallel()
	for _, m := range []string{reflectionV1, reflectionV1Alpha} {
		_, ok := PublicMethods[m]
		assert.True(t, ok)
		assert.Regexp(t, `^/[a-z0-9.]+\.ServerReflection/ServerReflectionInfo$`, m)
	}
	assert.Len(t, PublicMethods, 7, "adding a public method is a security decision — update this count deliberately")
	assert.Contains(t, PublicMethods, iamv1.IAMService_Login_FullMethodName)
	assert.NotContains(t, PublicMethods, iamv1.IAMService_Logout_FullMethodName, "Logout requires an access token")
	assert.NotContains(t, PublicMethods, iamv1.IAMService_GetCurrentUser_FullMethodName, "GetCurrentUser requires an access token")
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./pkg/interceptor/ -run TestAuth -v`
Expected: FAIL — `undefined: UnaryAuthInterceptor`.

- [ ] **Step 4: Implement the interceptors**

Create `pkg/interceptor/authjwt.go`:

```go
package interceptor

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/tenant"
)

const bearerPrefix = "bearer "

// authenticate verifies the caller's access token and returns an enriched
// context. Methods present in `public` proceed with an empty CallerInfo.
//
// x-caller-id, x-caller-role, x-caller-email and x-tenant-id are NEVER read:
// identity comes from the signed token or from nowhere. x-project-id survives
// because it selects a resource, and x-forwarded-for because it names the
// client for the audit trail — neither confers authority.
func authenticate(ctx context.Context, method string, v *auth.Verifier, public map[string]string) (context.Context, error) {
	if _, ok := public[method]; ok {
		return ctx, nil
	}

	md, _ := metadata.FromIncomingContext(ctx)

	raw := firstMD(md, "authorization")
	if raw == "" {
		return nil, status.Error(codes.Unauthenticated, "missing authorization token")
	}
	if len(raw) <= len(bearerPrefix) || !strings.EqualFold(raw[:len(bearerPrefix)], bearerPrefix) {
		return nil, status.Error(codes.Unauthenticated, "authorization must be a Bearer token")
	}

	claims, err := v.Verify(raw[len(bearerPrefix):])
	if err != nil {
		if errors.Is(err, auth.ErrTokenExpired) {
			// Clients need this one to know to refresh. It leaks nothing an
			// attacker holding the token does not already have.
			return nil, status.Error(codes.Unauthenticated, "token expired")
		}
		return nil, status.Error(codes.Unauthenticated, "token invalid")
	}

	caller := CallerInfo{
		UserID:      claims.Subject,
		TenantID:    claims.TenantID,
		ProjectID:   uuidFromMD(md, "x-project-id"),
		Email:       claims.Email,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		IP:          firstMD(md, "x-forwarded-for"),
	}

	ctx = WithCaller(ctx, caller)
	if caller.TenantID != uuid.Nil {
		ctx = tenant.WithID(ctx, caller.TenantID)
	}
	ctx = audit.WithActor(ctx, audit.ActorInfo{
		UserID: caller.UserID,
		Email:  caller.Email,
		IP:     caller.IP,
	})
	return ctx, nil
}

// UnaryAuthInterceptor rejects any unary RPC that is not in `public` and does
// not present a valid access token.
func UnaryAuthInterceptor(v *auth.Verifier, public map[string]string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		authCtx, err := authenticate(ctx, info.FullMethod, v, public)
		if err != nil {
			return nil, err
		}
		return handler(authCtx, req)
	}
}

// StreamAuthInterceptor is the streaming counterpart.
//
// Thittam has no streaming RPCs today. This ships anyway: pkg/server already
// installs stream interceptors, so omitting this would leave a fail-open path
// that opens itself the day someone adds a stream.
func StreamAuthInterceptor(v *auth.Verifier, public map[string]string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		authCtx, err := authenticate(ss.Context(), info.FullMethod, v, public)
		if err != nil {
			return err
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: authCtx})
	}
}
```

`firstMD`, `uuidFromMD` and `wrappedStream` already exist in `pkg/interceptor/auth.go` — reuse them, do not redefine.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./pkg/interceptor/ -v`
Expected: PASS. `TestAuth_ForgedCallerHeaders_DoNotEscalate` is the change, expressed once.

- [ ] **Step 6: Whole-tree vet**

Run: `go vet ./...`
Expected: clean. `pkg/interceptor` now imports `gen/iam/v1`; confirm no import cycle (`gen/` imports nothing from `pkg/`).

- [ ] **Step 7: Commit**

```bash
git add pkg/interceptor/public.go pkg/interceptor/authjwt.go pkg/interceptor/authjwt_test.go
git commit -m "feat(iam): fail-closed JWT auth interceptor + public allowlist (#138)"
```

---

### Task 4: `RequirePermission` denies on a nil checker

**Files:**
- Modify: `pkg/interceptor/permission.go` (`RequirePermission`, ~line 47)
- Modify: `pkg/interceptor/permission_test.go` (add one case)
- Modify: `scripts/dev-start.sh` (export `IAM_SERVICE_ADDR` unconditionally)

**Interfaces:** none new.

**Why.** Every service wires the checker as `if iamPerm != nil { handler = handler.WithPermissionChecker(iamPerm) }`, and `iamclient.DialFromEnv` returns a nil checker when `IAM_SERVICE_ADDR` is unset — logging *"IAM permission checks DISABLED (handlers run without authz)"*. So today the ~18 `RequirePermission` calls silently pass when IAM dialing is unconfigured. A permission check that succeeds because a dial failed is not a check.

**This breaks local dev unless `dev-start.sh` changes too.** That script only exports `IAM_SERVICE_ADDR` under `--with-project-rbac`. Flip nil-to-deny without touching it and every budget/expense/inventory/project write RPC returns `Internal` on a default `./scripts/dev-start.sh` run. Both edits belong in this one commit.

**Constraint:** no database, no Docker. Do **not** run `dev-start.sh`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/interceptor/permission_test.go`:

```go
func TestRequirePermission_NilChecker_Denies(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: uuid.New()})
	err := RequirePermission(ctx, nil, "budget:write")
	require.Error(t, err, "a permission check must never pass because the checker is missing")
	assert.Equal(t, codes.Internal, status.Code(err))
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./pkg/interceptor/ -run TestRequirePermission_NilChecker -v`
Expected: FAIL — a nil checker currently panics (nil interface call) or passes. Record which.

- [ ] **Step 3: Deny on a nil checker**

In `RequirePermission`, immediately after the caller check:

```go
	if checker == nil {
		// Misconfiguration, not authorization: the service could not reach IAM.
		// Failing closed is the only safe answer — a check that passes because
		// a dial failed is not a check.
		return status.Error(codes.Internal, "permission checker unavailable")
	}
```

- [ ] **Step 4: Make local dev configure IAM**

In `scripts/dev-start.sh`, the `--with-project-rbac` block (~lines 65-71) exports `IAM_SERVICE_ADDR`. Move that one export out of the flag block so it always runs, leaving `PROJECT_SCOPED_RBAC` behind the flag:

```bash
# IAM must be reachable: RequirePermission now fails closed when the checker is
# nil (#138), so every service needs IAM_SERVICE_ADDR whether or not
# project-scoped RBAC is enabled.
export IAM_SERVICE_ADDR="${IAM_SERVICE_ADDR:-localhost:8086}"
```

`dev-start.sh` already starts iam first and waits on its `/readyz`, so ordering is satisfied.

- [ ] **Step 5: Verify**

Run: `go test ./pkg/interceptor/ -v && go vet ./...`
Expected: PASS, clean.

Run: `bash -n scripts/dev-start.sh`
Expected: no output (syntax OK). **Do not execute the script.**

- [ ] **Step 6: Commit**

```bash
git add pkg/interceptor/permission.go pkg/interceptor/permission_test.go scripts/dev-start.sh
git commit -m "fix(iam): RequirePermission fails closed when the checker is unavailable (#138)"
```

---

### Task 5: gRPC reflection off by default

**Files:**
- Modify: `pkg/server/server.go` (`Config` ~27-38; `New` ~112)
- Modify: `scripts/dev-start.sh` (`start_svc` env, ~lines 170-175)

**Interfaces:**
- Produces: `server.Config.EnableReflection bool` — Tasks 6 and 7 do **not** set it; only `dev-start.sh` turns it on via env.

**Why.** `reflection.Register(gs)` runs unconditionally on every service in every environment. Reflection lets an unauthenticated client enumerate every RPC, message, and field. It is a debugging convenience, not a production one.

**Constraint:** no database, no Docker.

- [ ] **Step 1: Add the config field**

In `pkg/server/server.go`'s `Config`:

```go
	// EnableReflection registers the gRPC reflection service. Default false.
	// Reflection lets any client enumerate every RPC and message; it is a
	// local-development convenience. Set from GRPC_REFLECTION in dev-start.sh.
	EnableReflection bool
```

- [ ] **Step 2: Gate the registration**

Replace the unconditional `reflection.Register(gs)` (line 112) with:

```go
	if cfg.EnableReflection {
		reflection.Register(gs)
	}
```

- [ ] **Step 3: Read it from the environment, once, in `New`**

Services should not each re-derive this. Immediately before the `grpc.NewServer` call:

```go
	if v := os.Getenv("GRPC_REFLECTION"); v == "1" || v == "true" {
		cfg.EnableReflection = true
	}
```

so an explicit `Config.EnableReflection: true` and the env var both work, and no `cmd/*/main.go` changes. Add `"os"` to the imports if absent.

- [ ] **Step 4: Turn it on for local dev**

In `scripts/dev-start.sh`'s `start_svc`, add to the `env_overrides` array (~line 170-175):

```bash
    "GRPC_REFLECTION=${GRPC_REFLECTION:-1}"
```

so `grpcurl` keeps working locally and a developer can opt out with `GRPC_REFLECTION=0`.

- [ ] **Step 5: Test the gate**

Add to `pkg/server/server_test.go` (create the file if absent):

```go
func TestNew_ReflectionOffByDefault(t *testing.T) {
	t.Setenv("GRPC_REFLECTION", "")
	s := New(Config{Name: "t", Port: 0, MetricsPort: 0}, nil)
	require.NotNil(t, s)
	_, ok := s.gs.GetServiceInfo()["grpc.reflection.v1.ServerReflection"]
	assert.False(t, ok, "reflection must not be registered by default")
}

func TestNew_ReflectionOnViaEnv(t *testing.T) {
	t.Setenv("GRPC_REFLECTION", "1")
	s := New(Config{Name: "t", Port: 0, MetricsPort: 0}, nil)
	_, ok := s.gs.GetServiceInfo()["grpc.reflection.v1.ServerReflection"]
	assert.True(t, ok)
}
```

These are in-package (`package server`) so they can read `s.gs`. Do not use `t.Parallel()` with `t.Setenv`.

- [ ] **Step 6: Verify**

Run: `go test ./pkg/server/ -v && go vet ./... && bash -n scripts/dev-start.sh`
Expected: PASS, clean, no output.

- [ ] **Step 7: Commit**

```bash
git add pkg/server/server.go pkg/server/server_test.go scripts/dev-start.sh
git commit -m "fix(iam): register gRPC reflection only when explicitly enabled (#138)"
```

---

### Task 6: Wire the five services that already have an interceptor

**Files:**
- Modify: `cmd/iam/main.go` (~246-252)
- Modify: `cmd/project-management/main.go` (~98-108, and the `headerMatcher` at ~128-136)
- Modify: `cmd/budget-planning/main.go` (~101-108)
- Modify: `cmd/expense-tracking/main.go` (~89-96)
- Modify: `cmd/inventory-management/main.go` (~68-75)

**Interfaces:**
- Consumes: `auth.VerifierFromEnv` (Task 1); `interceptor.UnaryAuthInterceptor`, `StreamAuthInterceptor`, `PublicMethods` (Task 3).

**The old interceptors are NOT deleted here** — the five services in Task 7 still reference nothing, but `pkg/interceptor` still exports them and its own tests still cover them. Deletion is Task 7's last step, once no `main.go` names them.

**Constraint:** no database, no Docker. `go build` and `go vet` only.

- [ ] **Step 1: In each of the five, construct the verifier before `server.New`**

The verifier must be built early enough that a missing key kills the process before it serves traffic. Insert immediately before the `srv := server.New(...)` call:

```go
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("<service>: startup: load JWT public key: %v", err)
	}
```

Replace `<service>` with that binary's name, matching the file's existing `log.Fatalf("<service>: startup: …")` convention. Add `"github.com/wegofwd2020/thittam/pkg/auth"` to the imports.

`cmd/iam/main.go` already has a `ctx` and imports `pkg/auth`; the others may need `ctx := context.Background()` — check what exists rather than adding a second one.

- [ ] **Step 2: Swap the interceptors**

In each `server.Config`, replace:

```go
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryCallerInterceptor()},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamCallerInterceptor()},
```

with:

```go
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
```

- [ ] **Step 3: Stop project-management's gateway forwarding forged headers**

`cmd/project-management/main.go:128-136`. `Authorization` reaches gRPC metadata without a matcher — it is a permanent HTTP header, which is why iam's gateway needs no matcher at all. Only `X-Project-Id` still needs explicit forwarding:

```go
		headerMatcher := func(key string) (string, bool) {
			// x-caller-* and x-tenant-id are deliberately NOT forwarded: identity
			// comes from the verified token (#138), and forwarding them would let a
			// browser assert its own role. X-Project-Id selects a resource, not an
			// identity. Authorization arrives without a matcher (permanent header).
			if key == "X-Project-Id" {
				return key, true
			}
			return runtime.DefaultHeaderMatcher(key)
		}
```

- [ ] **Step 4: Stop iam's CORS advertising the caller headers**

`cmd/iam/main.go` (~315-319) lists `X-Caller-Id`, `X-Caller-Email`, `X-Caller-Role` in the CORS `AllowedHeaders`. Remove those three. Keep `Authorization`, `Content-Type`, `Accept`, and `X-Tenant-Id` if present — a stale `X-Tenant-Id` allowance is harmless now that it is ignored, but remove it too if it is there, so the surface tells the truth.

- [ ] **Step 5: Build and vet**

Run: `go build ./cmd/... && go vet ./...`
Expected: clean. `UnaryCallerInterceptor` is now referenced only by the five services in Task 7 and by `pkg/interceptor`'s own tests.

- [ ] **Step 6: Run the affected suites**

Run: `go test ./pkg/... ./services/iam/... -short`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/iam/main.go cmd/project-management/main.go cmd/budget-planning/main.go \
        cmd/expense-tracking/main.go cmd/inventory-management/main.go
git commit -m "feat(iam): verify JWTs in iam, project, budget, expense, inventory (#138)"
```

---

### Task 7: Wire the five unprotected services, and delete the header-trust interceptors

**Files:**
- Modify: `cmd/general-ledger/main.go` (~69-74)
- Modify: `cmd/reporting-analytics/main.go` (~91-96)
- Modify: `cmd/notifications/main.go` (~105-109)
- Modify: `cmd/document/main.go` (~80-84)
- Modify: `cmd/billing/main.go` (~99-104)
- Modify: `pkg/interceptor/auth.go` (delete `UnaryCallerInterceptor`, `StreamCallerInterceptor`, `rolesFromMD`)
- Modify: `pkg/interceptor/auth_test.go` (delete their tests)

**Interfaces:** consumes Task 1 and Task 3.

**These five have no caller in context today at all.** `general-ledger` is the sharp edge: `CreateJournalEntry`, `PostJournalEntry`, `VoidJournalEntry`, `CloseAccountingPeriod` are reachable by anyone who can open a socket. After this task they require a verified token. They still perform **no authorization** — that is #139, and this plan must not imply otherwise.

**Note on `reporting-analytics`:** it has a vertical `Loader`. `pkg/server.New` appends `ExtraUnaryInterceptors` *before* the vertical interceptor precisely so the tenant is in context when vertical resolves its config. The auth interceptor now supplies that tenant, from the token. Order is already correct; do not reorder.

**Constraint:** no database, no Docker.

- [ ] **Step 1: Add the verifier and interceptors to each of the five**

Each `main.go` currently calls `server.New` with no `Extra*` interceptors. For each, immediately before `srv := server.New(...)`:

```go
	verifier, err := auth.VerifierFromEnv(ctx)
	if err != nil {
		log.Fatalf("<service>: startup: load JWT public key: %v", err)
	}
```

and add to the `server.Config` literal:

```go
		ExtraUnaryInterceptors:  []grpc.UnaryServerInterceptor{interceptor.UnaryAuthInterceptor(verifier, interceptor.PublicMethods)},
		ExtraStreamInterceptors: []grpc.StreamServerInterceptor{interceptor.StreamAuthInterceptor(verifier, interceptor.PublicMethods)},
```

Add imports: `"google.golang.org/grpc"`, `"github.com/wegofwd2020/thittam/pkg/auth"`, `"github.com/wegofwd2020/thittam/pkg/interceptor"`. Several of these files do not currently import `grpc` — check each.

If a file has no `ctx` in scope at that point, use the one it already builds for its pool/NATS setup rather than creating a second `context.Background()`.

- [ ] **Step 2: Delete the header-trust interceptors**

From `pkg/interceptor/auth.go`, delete `UnaryCallerInterceptor`, `StreamCallerInterceptor`, and `rolesFromMD`. Keep `wrappedStream`, `firstMD`, `uuidFromMD`, `CallerInfo`, `WithCaller`, `CallerFromContext`, `HasRole`, `RequireRole`, and the role constants — `authjwt.go` uses them.

Rewrite the package doc comment, which currently describes the deleted design:

```go
// Package interceptor provides gRPC server interceptors for the Thittam platform.
//
// UnaryAuthInterceptor verifies the caller's access token against the platform's
// RSA public key and derives CallerInfo from the signed claims. Any method absent
// from PublicMethods is rejected with codes.Unauthenticated before it reaches a
// handler.
//
// Caller identity is NEVER read from request metadata. x-caller-id, x-caller-role,
// x-caller-email and x-tenant-id confer nothing: only the token does. x-project-id
// selects a resource and x-forwarded-for names the client for the audit trail.
//
// RequireRole and RequirePermission remain as defence in depth. They gate on the
// verified identity the interceptor established.
```

Delete the now-dead tests in `pkg/interceptor/auth_test.go`: `TestUnaryCallerInterceptor_*`, `TestStreamCallerInterceptor_*`, `runUnary`, `incomingCtx`, and `stubServerStream` if nothing else uses them. Keep every `TestRequireRole_*`.

- [ ] **Step 3: Whole-tree vet — the real gate**

Run: `go vet ./...`
Expected: clean. If any file still references `UnaryCallerInterceptor`, it is a `cmd/*` or an e2e double this plan missed. `go build ./services/...` would not have told you.

- [ ] **Step 4: Full unit suite**

Run: `go test ./... -short`
Expected: PASS.

Handler tests call handlers directly and inject `CallerInfo` via `WithCaller`, so they bypass the interceptor entirely and are unaffected. `e2e/critical_path` authenticates by fiat through `stubTokenIssuer` and likewise never runs the chain. That is precisely the coverage hole Task 9 closes.

- [ ] **Step 5: Coverage**

Run: `go test ./services/iam/... -short -coverprofile=/tmp/cov138.out && go tool cover -func=/tmp/cov138.out | tail -1`
Expected: ≥ 85% (the iam threshold).

- [ ] **Step 6: Commit**

```bash
git add cmd/general-ledger/main.go cmd/reporting-analytics/main.go cmd/notifications/main.go \
        cmd/document/main.go cmd/billing/main.go \
        pkg/interceptor/auth.go pkg/interceptor/auth_test.go
git commit -m "feat(iam): verify JWTs in the five unprotected services; delete header-trust interceptors (#138)"
```

---

### Task 8: The web client stops sending forgeable headers

**Files:**
- Modify: `web/src/lib/api/client.ts` (~120-137)

**Interfaces:** none.

**Why.** `X-Caller-Id`, `X-Caller-Email`, `X-Caller-Role` are set from client-side state, and `X-Tenant-Id` from `localStorage`. They are now ignored by every service. Leaving them invites the next reader to believe they mean something, and leaves a live forgery surface pointed at any service that ever forgets the interceptor.

**Constraint:** no database, no Docker, no dev server. Type-check only.

- [ ] **Step 1: Remove the header block**

In `web/src/lib/api/client.ts`, the request-header block currently reads:

```ts
    if (this.tenantId) {
      headers["X-Tenant-Id"] = this.tenantId;
    }

    if (this.caller) {
      headers["X-Caller-Id"] = this.caller.id;
      headers["X-Caller-Email"] = this.caller.email;
      headers["X-Caller-Role"] = this.caller.role;
    }
```

Delete both blocks. Keep `Authorization: Bearer ${this.token}`, `Content-Type`, and `Accept`. Add a comment where they were:

```ts
    // Identity travels in the bearer token alone. The server derives the caller
    // and the tenant from its verified claims (#138) and ignores any
    // X-Caller-* / X-Tenant-Id header, so sending them is at best misleading.
```

- [ ] **Step 2: Remove now-dead state**

`setTenantId` / `setCaller` (and the `tenantId` / `caller` fields) may now be unused. Check every call site before deleting: `web/src/lib/auth/context.tsx` calls `api.setTenantId(me.tenant.id)`. The tenant id is likely still needed by UI code for display or routing — **do not remove it from the auth context**, only from the outgoing headers. If `setCaller` has no remaining consumer, delete it; if `setTenantId` is only used to feed the header, keep the field and stop sending it, leaving a one-line comment saying why the field survives.

Be conservative: this task's contract is "stop sending four headers," not "refactor the client."

- [ ] **Step 3: Type-check**

Run: `cd web && npx tsc --noEmit`
Expected: clean. If `tsc` is not installed, run `cd web && npm run build 2>&1 | tail -20` and report what happens. **Do not `npm install`** if `node_modules` is absent — report instead.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api/client.ts
git commit -m "fix(api): stop sending forgeable X-Caller-* and X-Tenant-Id headers (#138)"
```

---

### Task 9: One integration test that proves the chain rejects

**Files:**
- Create: `pkg/server/integration_test.go`

**Interfaces:** consumes Tasks 1, 3, 5.

**Why this task exists.** Every handler test calls handlers directly. `e2e/critical_path` builds its context with `stubTokenIssuer` and never sends an `Authorization` header — `grep -r 'Bearer\|Authorization' e2e/critical_path/` returns zero. **Nothing in the suite today would notice if the auth interceptor were deleted from every service.** This test would.

There is no `bufconn` anywhere in the tree; this introduces the first server-boot harness. `google.golang.org/grpc/test/bufconn` ships inside the grpc module — no `go.mod` change.

**Constraint:** no database, no Docker, no real ports. `bufconn` only.

- [ ] **Step 1: Write the test**

Create `pkg/server/integration_test.go`. Register the real iam service descriptor with a stub implementation, so the method names the allowlist keys on are the ones actually served.

```go
package server_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
)

// stubIAM implements just enough of IAMServiceServer to exercise the chain:
// Login is on the public allowlist, ListRoles is not.
type stubIAM struct {
	iamv1.UnimplementedIAMServiceServer
}

func (s *stubIAM) Login(context.Context, *iamv1.LoginRequest) (*iamv1.TokenPair, error) {
	return &iamv1.TokenPair{AccessToken: "stub", TokenType: "Bearer"}, nil
}

func (s *stubIAM) ListRoles(ctx context.Context, _ *iamv1.ListRolesRequest) (*iamv1.ListRolesResponse, error) {
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Internal, "no caller in context")
	}
	// Echo the verified email back so the test can prove it came from the token.
	return &iamv1.ListRolesResponse{Roles: []*iamv1.Role{{Name: caller.Email}}}, nil
}

func startServer(t *testing.T, v *auth.Verifier) iamv1.IAMServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptor.UnaryAuthInterceptor(v, interceptor.PublicMethods)),
	)
	iamv1.RegisterIAMServiceServer(gs, &stubIAM{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return iamv1.NewIAMServiceClient(conn)
}

func keyAndVerifier(t *testing.T) (*rsa.PrivateKey, *auth.Verifier) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	v, err := auth.NewVerifier(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	require.NoError(t, err)
	return key, v
}

func bearer(t *testing.T, key *rsa.PrivateKey) context.Context {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": uuid.New().String(), "tid": uuid.New().String(),
		"email": "real@example.com", "roles": []string{"member"},
		"exp": jwt.NewNumericDate(time.Now().Add(time.Hour)),
		"iat": jwt.NewNumericDate(time.Now()),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	require.NoError(t, err)
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+tok))
}

func TestChain_PublicMethodReachableWithoutToken(t *testing.T) {
	_, v := keyAndVerifier(t)
	client := startServer(t, v)

	_, err := client.Login(context.Background(), &iamv1.LoginRequest{Email: "a@b.c", Password: "x"})
	require.NoError(t, err, "Login is on the allowlist")
}

func TestChain_PrivateMethodRejectsTokenlessCall(t *testing.T) {
	_, v := keyAndVerifier(t)
	client := startServer(t, v)

	_, err := client.ListRoles(context.Background(), &iamv1.ListRolesRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// Forged caller headers over the wire, with no token at all. This is the
// pre-#138 escalation path, end to end.
func TestChain_ForgedHeadersAloneAreRejected(t *testing.T) {
	_, v := keyAndVerifier(t)
	client := startServer(t, v)

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
		"x-caller-id", uuid.New().String(),
		"x-caller-role", interceptor.RolePlatformAdmin,
		"x-tenant-id", uuid.New().String(),
	))
	_, err := client.ListRoles(ctx, &iamv1.ListRolesRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "headers alone must never authenticate")
}

func TestChain_ValidTokenReachesHandlerWithVerifiedIdentity(t *testing.T) {
	key, v := keyAndVerifier(t)
	client := startServer(t, v)

	resp, err := client.ListRoles(bearer(t, key), &iamv1.ListRolesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetRoles(), 1)
	assert.Equal(t, "real@example.com", resp.GetRoles()[0].GetName(), "identity came from the token")
}
```

If `iamv1.ListRolesResponse`/`Role` field names differ from the above, adjust to the generated types rather than changing the proto. Pick any authenticated iam RPC whose response can echo a string.

- [ ] **Step 2: Run it**

Run: `go test ./pkg/server/ -run TestChain -v`
Expected: PASS, four tests.

- [ ] **Step 3: Prove the test has teeth**

Temporarily comment out the `grpc.ChainUnaryInterceptor(...)` line in `startServer`, re-run, and confirm `TestChain_PrivateMethodRejectsTokenlessCall` and `TestChain_ForgedHeadersAloneAreRejected` **fail**. Restore the line. Record both outputs in your report.

A test that passes with the interceptor removed is the thing we are trying to stop shipping.

- [ ] **Step 4: Race and vet**

Run: `go test -race ./pkg/server/ && go vet ./...`
Expected: PASS, clean.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/integration_test.go
git commit -m "test(iam): bufconn chain test — tokenless and forged-header calls are rejected (#138)"
```

---

## Verification (whole branch, before PR)

- [ ] `go vet ./...` — clean. The only check that catches all ten `cmd/*` wirings.
- [ ] `go test ./... -short` — PASS.
- [ ] `go test -race ./pkg/... ./services/iam/...` — PASS.
- [ ] `go test ./services/iam/... -short -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out | tail -1` — ≥ 85%.
- [ ] `go test ./pkg/... -short -coverprofile=/tmp/p.out && go tool cover -func=/tmp/p.out | tail -1` — ≥ 75%.
- [ ] `grep -rn 'UnaryCallerInterceptor\|StreamCallerInterceptor' --include=*.go .` — **zero hits.** The header-trusting interceptors are gone, not merely unused.
- [ ] `grep -rn 'x-caller-role\|x-caller-id\|x-caller-email' --include=*.go pkg/ services/ cmd/` — hits only in comments and in `authjwt_test.go`'s forgery tests.
- [ ] `grep -rn 'X-Caller-' web/src/` — zero hits.
- [ ] `bash -n scripts/dev-start.sh` — no output.
- [ ] `gofmt -l pkg cmd services` — nothing among files this branch touched.
- [ ] `git log --oneline` — nine commits, each building green on its own.

## Deploy-time notes (not code, and blocking)

- **`jwt_public.pem` must exist in every environment's secret source before this deploys.** Vault: `iam/jwt-public-key`. Local: `infra/local/keys/jwt_public.pem` (already committed). **A service that cannot load it refuses to start** — that is deliberate (§12 of the spec), and it means the Vault secret must be created *before* the rollout, not during it. Derive it from the existing private key: `openssl rsa -in jwt_private.pem -pubout -out jwt_public.pem`.
- **There is no feature flag and no phased enablement.** A partially-authenticated fleet is a fleet with a bypass, and the flag would be the bypass.
- `IAM_SERVICE_ADDR` must be set for every service that calls `RequirePermission` (budget, expense, inventory, project). It now fails closed without it.
- Kong's routing still points `/api/v1/auth/*` at iam's **gRPC** port 8086 while REST lives on 9086 (`infra/k8s/kong/auth-rate-limit.yaml`). Pre-existing, tracked in #60. This change does not fix it.

## What this does NOT fix

Roughly 100 RPCs still enforce no authorization. `general-ledger` will require a valid token and then let any authenticated user post a journal entry. iam's `AssignRole` remains a self-promotion primitive for anyone holding a token.

That is **#139**, filed. Do not let this merge be read as "the platform is authorized."
