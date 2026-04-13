// Package iamclient provides thin Go-typed adapters around the generated IAM
// gRPC client for use by service handlers.
//
// The adapters bridge between the gRPC client's protobuf request/response types
// and the narrow interfaces consumed elsewhere in the codebase (notably
// pkg/interceptor.PermissionChecker), so callers don't repeat the boilerplate
// of constructing CheckPermissionRequest in every handler that gates a call.
package iamclient

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
)

// PermissionChecker adapts iamv1.IAMServiceClient to the
// interceptor.PermissionChecker interface used by handler-side authorisation.
//
// It does no caching — every call hits IAM. If/when traffic warrants caching,
// add an L1 (in-process TTL) layer here without changing the surface.
type PermissionChecker struct {
	client iamv1.IAMServiceClient
}

// NewPermissionChecker wraps an IAM gRPC client.
func NewPermissionChecker(client iamv1.IAMServiceClient) *PermissionChecker {
	return &PermissionChecker{client: client}
}

// CheckPermission satisfies interceptor.PermissionChecker. A nil projectID
// means tenant-wide check; a non-nil projectID means project-scoped (tenant
// permissions still merged in by the IAM service).
func (c *PermissionChecker) CheckPermission(
	ctx context.Context,
	userID uuid.UUID,
	permission string,
	projectID *uuid.UUID,
) (bool, error) {
	req := &iamv1.CheckPermissionRequest{
		UserId:     userID.String(),
		Permission: permission,
	}
	if projectID != nil {
		req.ProjectId = projectID.String()
	}
	resp, err := c.client.CheckPermission(ctx, req)
	if err != nil {
		return false, fmt.Errorf("iamclient: CheckPermission(user=%s, perm=%s): %w", userID, permission, err)
	}
	return resp.GetAllowed(), nil
}
