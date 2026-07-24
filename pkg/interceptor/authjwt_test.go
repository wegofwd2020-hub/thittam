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

	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
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

	cases := []struct {
		name string
		md   metadata.MD
	}{
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

// StreamAuthInterceptor is the streaming counterpart of UnaryAuthInterceptor.
// Thittam has no streaming RPCs today, but the interceptor must still enforce
// the same fail-closed rule and enrich the same way.
func TestAuth_StreamInterceptor_RejectsMissingToken(t *testing.T) {
	t.Parallel()
	v, _ := testVerifier(t)
	ctx := context.Background()

	err := StreamAuthInterceptor(v, PublicMethods)(nil, &stubServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: privateMethod}, func(_ interface{}, _ grpc.ServerStream) error {
		t.Fatal("handler must not run without a token")
		return nil
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestAuth_StreamInterceptor_ValidToken_PopulatesCaller(t *testing.T) {
	t.Parallel()
	v, key := testVerifier(t)
	tid := uuid.New()
	md := metadata.Pairs("authorization", "Bearer "+mintToken(t, key, tid, []string{RoleMember}, time.Now().Add(time.Hour)))
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var captured context.Context
	err := StreamAuthInterceptor(v, PublicMethods)(nil, &stubServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: privateMethod}, func(_ interface{}, ss grpc.ServerStream) error {
		captured = ss.Context()
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, captured)
	caller, ok := CallerFromContext(captured)
	require.True(t, ok)
	assert.Equal(t, []string{RoleMember}, caller.Roles)
	assert.Equal(t, tid, caller.TenantID)
}

func TestAuth_StreamInterceptor_PublicMethod_NoTokenRequired(t *testing.T) {
	t.Parallel()
	v, _ := testVerifier(t)
	ctx := context.Background()

	var handlerRan bool
	err := StreamAuthInterceptor(v, PublicMethods)(nil, &stubServerStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: iamv1.IAMService_Login_FullMethodName}, func(_ interface{}, _ grpc.ServerStream) error {
		handlerRan = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, handlerRan)
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
	assert.Len(t, PublicMethods, 5, "adding a public method is a security decision — update this count deliberately")
	assert.Contains(t, PublicMethods, iamv1.IAMService_Login_FullMethodName)
	assert.NotContains(t, PublicMethods, iamv1.IAMService_Logout_FullMethodName, "Logout requires an access token")
	assert.NotContains(t, PublicMethods, iamv1.IAMService_GetCurrentUser_FullMethodName, "GetCurrentUser requires an access token")
	// The size assertion alone would still pass if someone swapped one entry for
	// another. These two came off the list in #139 slice I and must stay off:
	// pkg/iamclient forwards the caller's token, so neither needs to be public.
	assert.NotContains(t, PublicMethods, iamv1.IAMService_CheckPermission_FullMethodName, "CheckPermission verifies the forwarded caller token (#139 slice I)")
	assert.NotContains(t, PublicMethods, iamv1.IAMService_ValidateToken_FullMethodName, "ValidateToken is retired to Unimplemented (#139 slice I)")
}
