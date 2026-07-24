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

func (s *stubIAM) CheckPermission(ctx context.Context, _ *iamv1.CheckPermissionRequest) (*iamv1.CheckPermissionResponse, error) {
	if _, ok := interceptor.CallerFromContext(ctx); !ok {
		return nil, status.Error(codes.Internal, "handler reached without a verified caller")
	}
	return &iamv1.CheckPermissionResponse{Allowed: true}, nil
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

func TestChain_CheckPermissionRejectsTokenlessCall(t *testing.T) {
	_, v := keyAndVerifier(t)
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
