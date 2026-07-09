package interceptor

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var fixedCallerID = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
var fixedTenantID = uuid.MustParse("a0000000-0000-0000-0000-000000000001")

// --- CallerFromContext ---

func TestCallerFromContext_AbsentReturnsZeroAndFalse(t *testing.T) {
	t.Parallel()
	_, ok := CallerFromContext(context.Background())
	assert.False(t, ok)
}

func TestWithCaller_RoundTrip(t *testing.T) {
	t.Parallel()
	want := CallerInfo{UserID: fixedCallerID, TenantID: fixedTenantID, Email: "a@b.com", Roles: []string{RolePlatformAdmin}, IP: "1.2.3.4"}
	got, ok := CallerFromContext(WithCaller(context.Background(), want))
	require.True(t, ok)
	assert.Equal(t, want, got)
}

// stubServerStream is a minimal grpc.ServerStream whose Context() returns the
// supplied context. All other methods panic — they are never called by the
// interceptors under test. Used by authjwt_test.go's StreamAuthInterceptor tests.
type stubServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stubServerStream) Context() context.Context { return s.ctx }

func TestWrappedStream_ContextReturnsEnrichedContext(t *testing.T) {
	t.Parallel()
	// wrappedStream.Context() must return the enriched ctx, not the original.
	type marker struct{}
	original := context.Background()
	enrichedCtx := context.WithValue(context.Background(), marker{}, "enriched")
	ws := &wrappedStream{ServerStream: &stubServerStream{ctx: original}, ctx: enrichedCtx}
	assert.Equal(t, enrichedCtx, ws.Context())
	assert.NotEqual(t, original, ws.Context())
}

// --- uuidFromMD ---

func TestUuidFromMD_InvalidUUID_ReturnsNil(t *testing.T) {
	t.Parallel()
	md := metadata.Pairs("x-project-id", "not-a-uuid")
	assert.Equal(t, uuid.Nil, uuidFromMD(md, "x-project-id"))
}

// --- RequireRole ---

func TestRequireRole_CorrectRole_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{Roles: []string{RolePlatformAdmin}})
	assert.NoError(t, RequireRole(ctx, RolePlatformAdmin))
}

func TestRequireRole_WrongRole_ReturnsPermissionDenied(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{Roles: []string{RoleMember}})
	err := RequireRole(ctx, RolePlatformAdmin)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRequireRole_NoCaller_ReturnsPermissionDenied(t *testing.T) {
	t.Parallel()
	err := RequireRole(context.Background(), RolePlatformAdmin)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRequireRole_TenantAdmin_NotSufficientForPlatformAdmin(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{Roles: []string{RoleTenantAdmin}})
	err := RequireRole(ctx, RolePlatformAdmin)
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRequireRole_MembershipInMultiRoleCaller(t *testing.T) {
	t.Parallel()
	ctx := WithCaller(context.Background(), CallerInfo{Roles: []string{RoleMember, RoleTenantAdmin}})
	assert.NoError(t, RequireRole(ctx, RoleTenantAdmin), "membership, not equality")
	assert.NoError(t, RequireRole(ctx, RoleMember))
	assert.Error(t, RequireRole(ctx, RolePlatformAdmin))
}
