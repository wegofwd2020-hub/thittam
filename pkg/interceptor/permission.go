package interceptor

import (
	"context"
	"os"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PermissionChecker is the narrow contract the interceptor needs from the IAM
// service. Defined here (not in services/iam) so this package never imports the
// IAM service. The IAM gRPC client implements this interface.
//
// projectID nil → tenant-wide check.
// projectID non-nil → tenant-wide + project-scoped permissions for that project.
type PermissionChecker interface {
	CheckPermission(ctx context.Context, userID uuid.UUID, permission string, projectID *uuid.UUID) (bool, error)
}

// projectScopedRBACOnce / projectScopedRBACEnabled cache the env flag so we do
// not re-read os.Getenv on every RPC.
var (
	projectScopedRBACOnce    sync.Once
	projectScopedRBACEnabled bool
)

// ProjectScopedRBACEnabled reports whether project-scoped RBAC enforcement is
// active. Controlled by env PROJECT_SCOPED_RBAC=true. Defaults to false until
// the Phase 2 rollout is complete.
func ProjectScopedRBACEnabled() bool {
	projectScopedRBACOnce.Do(func() {
		v := os.Getenv("PROJECT_SCOPED_RBAC")
		projectScopedRBACEnabled = v == "true" || v == "1"
	})
	return projectScopedRBACEnabled
}

// RequirePermission enforces an IAM permission check from a service handler.
// It pulls the caller from context, applies the project-scope rules, and
// returns codes.PermissionDenied if the check fails.
//
// Behaviour:
//   - PROJECT_SCOPED_RBAC unset/false → tenant-wide check only (current
//     behaviour, backwards compatible for services not yet updated).
//   - flag enabled and x-project-id present → project-scoped check.
//   - flag enabled and x-project-id absent → tenant-wide check (universal
//     services and RPCs that do not operate on a single project).
func RequirePermission(ctx context.Context, checker PermissionChecker, permission string) error {
	caller, ok := CallerFromContext(ctx)
	if !ok || caller.UserID == uuid.Nil {
		return status.Error(codes.Unauthenticated, "caller identity not present in context")
	}

	var projectID *uuid.UUID
	if ProjectScopedRBACEnabled() && caller.ProjectID != uuid.Nil {
		pid := caller.ProjectID
		projectID = &pid
	}

	allowed, err := checker.CheckPermission(ctx, caller.UserID, permission, projectID)
	if err != nil {
		return status.Errorf(codes.Internal, "permission check failed: %v", err)
	}
	if !allowed {
		return status.Errorf(codes.PermissionDenied, "missing required permission: %s", permission)
	}
	return nil
}

// resetProjectScopedRBACForTest is a test-only escape hatch to re-read the env.
// Production code never calls this; tests in this package use it to flip the
// flag between cases without spawning subprocesses.
func resetProjectScopedRBACForTest() {
	projectScopedRBACOnce = sync.Once{}
}
