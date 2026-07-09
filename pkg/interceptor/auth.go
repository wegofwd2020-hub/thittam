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
package interceptor

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Role constants match the role names asserted in a verified token's claims.
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

// CallerFromContext retrieves the CallerInfo set by UnaryAuthInterceptor.
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
