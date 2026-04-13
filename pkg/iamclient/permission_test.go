package iamclient

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
)

// stubIAM implements iamv1.IAMServiceClient just enough for the adapter test.
// We embed UnimplementedIAMServiceServer for forward compatibility with
// future RPCs but only stub CheckPermission here.
type stubIAM struct {
	iamv1.IAMServiceClient
	gotReq    *iamv1.CheckPermissionRequest
	respValue bool
	err       error
}

func (s *stubIAM) CheckPermission(_ context.Context, in *iamv1.CheckPermissionRequest, _ ...grpc.CallOption) (*iamv1.CheckPermissionResponse, error) {
	s.gotReq = in
	if s.err != nil {
		return nil, s.err
	}
	return &iamv1.CheckPermissionResponse{Allowed: s.respValue}, nil
}

func TestPermissionChecker_TenantWide(t *testing.T) {
	t.Parallel()
	stub := &stubIAM{respValue: true}
	pc := NewPermissionChecker(stub)
	userID := uuid.New()

	allowed, err := pc.CheckPermission(context.Background(), userID, "budget:approve", nil)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, userID.String(), stub.gotReq.GetUserId())
	assert.Equal(t, "budget:approve", stub.gotReq.GetPermission())
	assert.Empty(t, stub.gotReq.GetProjectId(), "nil projectID must produce empty proto field")
}

func TestPermissionChecker_ProjectScoped(t *testing.T) {
	t.Parallel()
	stub := &stubIAM{respValue: false}
	pc := NewPermissionChecker(stub)
	userID := uuid.New()
	projectID := uuid.New()

	allowed, err := pc.CheckPermission(context.Background(), userID, "expense:approve", &projectID)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, projectID.String(), stub.gotReq.GetProjectId())
}

func TestPermissionChecker_Error(t *testing.T) {
	t.Parallel()
	stub := &stubIAM{err: errors.New("rpc unavailable")}
	pc := NewPermissionChecker(stub)

	allowed, err := pc.CheckPermission(context.Background(), uuid.New(), "production:read", nil)
	require.Error(t, err)
	assert.False(t, allowed)
	assert.Contains(t, err.Error(), "rpc unavailable")
}
