package interceptor

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var fixedCallerID = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
var fixedTenantID = uuid.MustParse("a0000000-0000-0000-0000-000000000001")

// incomingCtx builds a context that looks like it came from Kong: metadata
// is placed in the "incoming" slot that FromIncomingContext reads.
func incomingCtx(kv ...string) context.Context {
	md := metadata.Pairs(kv...)
	return metadata.NewIncomingContext(context.Background(), md)
}

// runUnary runs the UnaryCallerInterceptor with the given context and returns
// the enriched context seen by the handler.
func runUnary(ctx context.Context) context.Context {
	interceptor := UnaryCallerInterceptor()
	var captured context.Context
	_, _ = interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(c context.Context, _ interface{}) (interface{}, error) {
		captured = c
		return nil, nil
	})
	return captured
}

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

// --- UnaryCallerInterceptor ---

func TestUnaryCallerInterceptor_PopulatesCallerInfo(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-tenant-id", fixedTenantID.String(),
		"x-caller-email", "admin@example.com",
		"x-caller-role", RolePlatformAdmin,
		"x-forwarded-for", "10.0.0.1",
	)

	enriched := runUnary(ctx)
	caller, ok := CallerFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, fixedCallerID, caller.UserID)
	assert.Equal(t, fixedTenantID, caller.TenantID)
	assert.Equal(t, "admin@example.com", caller.Email)
	assert.Equal(t, []string{RolePlatformAdmin}, caller.Roles)
	assert.Equal(t, "10.0.0.1", caller.IP)
}

func TestUnaryCallerInterceptor_PopulatesTenantContext(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-tenant-id", fixedTenantID.String(),
	)

	enriched := runUnary(ctx)
	tenantID, ok := tenant.IDFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, fixedTenantID, tenantID)
}

func TestUnaryCallerInterceptor_NoTenantHeader_TenantContextAbsent(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx("x-caller-id", fixedCallerID.String())

	enriched := runUnary(ctx)
	_, ok := tenant.IDFromContext(enriched)
	assert.False(t, ok, "tenant context must not be set when x-tenant-id is absent")
}

func TestUnaryCallerInterceptor_PopulatesAuditActor(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-caller-email", "admin@example.com",
		"x-forwarded-for", "10.0.0.2",
	)

	enriched := runUnary(ctx)
	actor, ok := audit.ActorFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, fixedCallerID, actor.UserID)
	assert.Equal(t, "admin@example.com", actor.Email)
	assert.Equal(t, "10.0.0.2", actor.IP)
}

func TestUnaryCallerInterceptor_NoMetadata_EmptyCaller(t *testing.T) {
	t.Parallel()
	// Request arrived without any Kong headers (e.g. direct gRPC call in tests).
	enriched := runUnary(context.Background())
	caller, ok := CallerFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, uuid.Nil, caller.UserID)
	assert.Empty(t, caller.Roles)
}

func TestUnaryCallerInterceptor_InvalidUUID_TreatedAsNil(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx("x-caller-id", "not-a-uuid")
	enriched := runUnary(ctx)
	caller, _ := CallerFromContext(enriched)
	assert.Equal(t, uuid.Nil, caller.UserID)
}

func TestUnaryCallerInterceptor_HandlerErrorPassedThrough(t *testing.T) {
	t.Parallel()
	wantErr := status.Error(codes.NotFound, "not found")
	interceptor := UnaryCallerInterceptor()
	_, gotErr := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ interface{}) (interface{}, error) {
		return nil, wantErr
	})
	assert.Equal(t, wantErr, gotErr)
}

// --- StreamCallerInterceptor ---

// stubServerStream is a minimal grpc.ServerStream whose Context() returns the
// supplied incoming context. All other methods panic — they are never called by
// the interceptor under test.
type stubServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stubServerStream) Context() context.Context { return s.ctx }

// runStream runs the StreamCallerInterceptor with the given context and returns
// the enriched context seen by the handler (via the wrapped stream).
func runStream(ctx context.Context) context.Context {
	interceptor := StreamCallerInterceptor()
	var captured context.Context
	_ = interceptor(nil, &stubServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, func(_ interface{}, ss grpc.ServerStream) error {
		captured = ss.Context()
		return nil
	})
	return captured
}

func TestStreamCallerInterceptor_PopulatesCallerInfo(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-tenant-id", fixedTenantID.String(),
		"x-caller-email", "stream@example.com",
		"x-caller-role", RoleTenantAdmin,
		"x-forwarded-for", "10.0.0.3",
	)

	enriched := runStream(ctx)
	caller, ok := CallerFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, fixedCallerID, caller.UserID)
	assert.Equal(t, fixedTenantID, caller.TenantID)
	assert.Equal(t, "stream@example.com", caller.Email)
	assert.Equal(t, []string{RoleTenantAdmin}, caller.Roles)
	assert.Equal(t, "10.0.0.3", caller.IP)
}

func TestStreamCallerInterceptor_PopulatesTenantContext(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-tenant-id", fixedTenantID.String(),
	)

	enriched := runStream(ctx)
	tenantID, ok := tenant.IDFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, fixedTenantID, tenantID)
}

func TestStreamCallerInterceptor_NoTenantHeader_TenantContextAbsent(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx("x-caller-id", fixedCallerID.String())

	enriched := runStream(ctx)
	_, ok := tenant.IDFromContext(enriched)
	assert.False(t, ok, "tenant context must not be set when x-tenant-id is absent")
}

func TestStreamCallerInterceptor_PopulatesAuditActor(t *testing.T) {
	t.Parallel()
	ctx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-caller-email", "stream@example.com",
		"x-forwarded-for", "10.0.0.4",
	)

	enriched := runStream(ctx)
	actor, ok := audit.ActorFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, fixedCallerID, actor.UserID)
	assert.Equal(t, "stream@example.com", actor.Email)
	assert.Equal(t, "10.0.0.4", actor.IP)
}

func TestStreamCallerInterceptor_NoMetadata_EmptyCaller(t *testing.T) {
	t.Parallel()
	enriched := runStream(context.Background())
	caller, ok := CallerFromContext(enriched)
	require.True(t, ok)
	assert.Equal(t, uuid.Nil, caller.UserID)
	assert.Empty(t, caller.Roles)
}

func TestStreamCallerInterceptor_HandlerErrorPassedThrough(t *testing.T) {
	t.Parallel()
	wantErr := status.Error(codes.NotFound, "stream not found")
	interceptor := StreamCallerInterceptor()
	gotErr := interceptor(nil, &stubServerStream{ctx: context.Background()}, &grpc.StreamServerInfo{}, func(_ interface{}, _ grpc.ServerStream) error {
		return wantErr
	})
	assert.Equal(t, wantErr, gotErr)
}

func TestWrappedStream_ContextReturnsEnrichedContext(t *testing.T) {
	t.Parallel()
	// wrappedStream.Context() must return the enriched ctx, not the original.
	original := context.Background()
	enrichedCtx := incomingCtx(
		"x-caller-id", fixedCallerID.String(),
		"x-tenant-id", fixedTenantID.String(),
	)
	ws := &wrappedStream{ServerStream: &stubServerStream{ctx: original}, ctx: enrichedCtx}
	assert.Equal(t, enrichedCtx, ws.Context())
	assert.NotEqual(t, original, ws.Context())
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
