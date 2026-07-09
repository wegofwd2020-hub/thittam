// Package interceptor provides gRPC server interceptors for the Thittam platform.
//
// The auth interceptor sits between the Kong API Gateway and service handlers.
// Kong validates the JWT, then injects caller identity as HTTP headers that
// become gRPC metadata (all lowercase per the gRPC metadata spec):
//
//	x-caller-id    — UUID of the authenticated user
//	x-caller-role  — highest role of the caller (e.g. "platform_admin", "tenant_admin")
//	x-caller-email — caller's email address
//	x-forwarded-for — original client IP
//
// The interceptor populates CallerInfo and pkg/audit.ActorInfo in the request
// context so handlers and downstream code can read them without re-parsing metadata.
package interceptor

import (
	"context"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Role constants match the values Kong injects via x-caller-role.
const (
	RolePlatformAdmin = "platform_admin"
	RoleTenantAdmin   = "tenant_admin"
	RoleMember        = "member"
)

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

type callerKey struct{}

// WithCaller attaches CallerInfo to the context. Called by the interceptor;
// also useful in tests to inject a synthetic caller.
func WithCaller(ctx context.Context, caller CallerInfo) context.Context {
	return context.WithValue(ctx, callerKey{}, caller)
}

// CallerFromContext retrieves the CallerInfo set by UnaryCallerInterceptor.
// Returns zero value and false if the interceptor has not run.
func CallerFromContext(ctx context.Context) (CallerInfo, bool) {
	c, ok := ctx.Value(callerKey{}).(CallerInfo)
	return c, ok
}

// RequireRole returns PermissionDenied unless the caller's verified roles
// contain `required`. Membership, not equality: a token asserting
// [viewer, tenant_admin] satisfies RequireRole(tenant_admin).
//
//	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
//	    return nil, err
//	}
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

// UnaryCallerInterceptor reads Kong-injected metadata headers, builds CallerInfo,
// and stores it (plus audit.ActorInfo) in the request context.
//
// Requests that arrive without x-caller-id metadata (e.g. direct gRPC calls
// that bypassed Kong) are allowed through with an empty CallerInfo — individual
// handlers that require authentication call RequireRole to enforce access.
func UnaryCallerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, _ := metadata.FromIncomingContext(ctx)

		caller := CallerInfo{
			UserID:    uuidFromMD(md, "x-caller-id"),
			TenantID:  uuidFromMD(md, "x-tenant-id"),
			ProjectID: uuidFromMD(md, "x-project-id"),
			Email:     firstMD(md, "x-caller-email"),
			Roles:     rolesFromMD(md, "x-caller-role"),
			IP:        firstMD(md, "x-forwarded-for"),
		}

		ctx = WithCaller(ctx, caller)

		// Populate tenant context so vertical-aware services and the vertical
		// middleware can resolve the tenant schema without re-parsing metadata.
		if caller.TenantID != uuid.Nil {
			ctx = tenant.WithID(ctx, caller.TenantID)
		}

		// Also populate the audit context so audit log helpers work throughout
		// the call stack without needing to thread CallerInfo manually.
		ctx = audit.WithActor(ctx, audit.ActorInfo{
			UserID: caller.UserID,
			Email:  caller.Email,
			IP:     caller.IP,
		})

		return handler(ctx, req)
	}
}

// StreamCallerInterceptor is the streaming counterpart of UnaryCallerInterceptor.
// Most Thittam RPCs are unary; this is provided for completeness.
func StreamCallerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		md, _ := metadata.FromIncomingContext(ctx)

		caller := CallerInfo{
			UserID:    uuidFromMD(md, "x-caller-id"),
			TenantID:  uuidFromMD(md, "x-tenant-id"),
			ProjectID: uuidFromMD(md, "x-project-id"),
			Email:     firstMD(md, "x-caller-email"),
			Roles:     rolesFromMD(md, "x-caller-role"),
			IP:        firstMD(md, "x-forwarded-for"),
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

		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// wrappedStream overrides Context() so the enriched context flows downstream.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// firstMD returns the first value for key from gRPC metadata, or "".
func firstMD(md metadata.MD, key string) string {
	vals := md.Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// rolesFromMD reads the legacy single-valued x-caller-role header into a slice.
// Transitional: both header-trusting interceptors are deleted in #138 Task 7.
func rolesFromMD(md metadata.MD, key string) []string {
	if v := firstMD(md, key); v != "" {
		return []string{v}
	}
	return nil
}

// uuidFromMD parses a UUID from gRPC metadata. Returns uuid.Nil on missing or
// invalid values — callers that require a valid UUID call RequireRole which will
// fail on missing CallerInfo before a nil UUID causes harm.
func uuidFromMD(md metadata.MD, key string) uuid.UUID {
	s := firstMD(md, key)
	if s == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}
