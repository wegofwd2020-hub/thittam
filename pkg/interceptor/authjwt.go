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
