package interceptor

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TenantFromRequest returns the caller's tenant, taken from the verified token.
//
// If the request also carries a tenant id and it differs, the call is refused. A
// client asking about a tenant that is not its own is either broken or hostile;
// either way we would rather say so than quietly answer about a different tenant
// than the one it named.
//
// The returned tenant NEVER comes from the request. A handler cannot query with a
// caller-supplied tenant, because the only tenant it can obtain is this one.
//
// Platform RPCs that legitimately act on another tenant (SuspendTenant, PurgeTenant)
// must NOT use this helper: they read the request tenant behind RequireRole.
func TenantFromRequest(ctx context.Context, reqTenantID string) (uuid.UUID, error) {
	caller, ok := CallerFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if caller.TenantID == uuid.Nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "token carries no tenant")
	}
	if reqTenantID == "" {
		return caller.TenantID, nil
	}
	reqID, err := uuid.Parse(reqTenantID)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	if reqID != caller.TenantID {
		// Deliberately does not echo either id: the caller already knows its own,
		// and confirming the existence of the other is an oracle.
		return uuid.Nil, status.Error(codes.PermissionDenied, "tenant_id does not match the authenticated tenant")
	}
	return caller.TenantID, nil
}
