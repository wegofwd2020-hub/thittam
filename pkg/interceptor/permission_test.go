package interceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stubChecker struct {
	// projectsAllowing is the set of (permission, projectID) tuples the user
	// holds via project-scoped roles. Tenant-wide perms come from tenantWide.
	tenantWide       map[string]bool
	projectsAllowing map[string]map[uuid.UUID]bool
	calls            []checkCall
	err              error
}

type checkCall struct {
	userID     uuid.UUID
	permission string
	projectID  *uuid.UUID
}

func (s *stubChecker) CheckPermission(_ context.Context, userID uuid.UUID, permission string, projectID *uuid.UUID) (bool, error) {
	s.calls = append(s.calls, checkCall{userID, permission, projectID})
	if s.err != nil {
		return false, s.err
	}
	if s.tenantWide[permission] {
		return true, nil
	}
	if projectID != nil && s.projectsAllowing[permission][*projectID] {
		return true, nil
	}
	return false, nil
}

func enableFlag(t *testing.T, value string) {
	t.Helper()
	t.Setenv("PROJECT_SCOPED_RBAC", value)
	resetProjectScopedRBACForTest()
	t.Cleanup(resetProjectScopedRBACForTest)
}

var (
	testUser     = uuid.MustParse("11111111-0000-0000-0000-000000000001")
	testProjectA = uuid.MustParse("aaaa0000-0000-0000-0000-000000000001")
	testProjectB = uuid.MustParse("bbbb0000-0000-0000-0000-000000000002")
)

func ctxWithCaller(projectID uuid.UUID) context.Context {
	return WithCaller(context.Background(), CallerInfo{
		UserID:    testUser,
		TenantID:  uuid.New(),
		ProjectID: projectID,
	})
}

func TestRequirePermission_FlagOff_FallsBackToTenantWide(t *testing.T) {
	enableFlag(t, "false")
	checker := &stubChecker{tenantWide: map[string]bool{"budget:write": true}}

	err := RequirePermission(ctxWithCaller(testProjectA), checker, "budget:write")
	require.NoError(t, err)

	require.Len(t, checker.calls, 1)
	assert.Nil(t, checker.calls[0].projectID, "flag off → projectID must be nil")
}

func TestRequirePermission_FlagOn_ProjectScopedPass(t *testing.T) {
	enableFlag(t, "true")
	checker := &stubChecker{
		projectsAllowing: map[string]map[uuid.UUID]bool{
			"expense:approve": {testProjectA: true},
		},
	}

	err := RequirePermission(ctxWithCaller(testProjectA), checker, "expense:approve")
	require.NoError(t, err)

	require.Len(t, checker.calls, 1)
	require.NotNil(t, checker.calls[0].projectID)
	assert.Equal(t, testProjectA, *checker.calls[0].projectID)
}

func TestRequirePermission_FlagOn_WrongProjectRejected(t *testing.T) {
	enableFlag(t, "true")
	checker := &stubChecker{
		projectsAllowing: map[string]map[uuid.UUID]bool{
			"expense:approve": {testProjectA: true},
		},
	}

	err := RequirePermission(ctxWithCaller(testProjectB), checker, "expense:approve")
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestRequirePermission_FlagOn_TenantWideStillWorks(t *testing.T) {
	enableFlag(t, "true")
	checker := &stubChecker{tenantWide: map[string]bool{"budget:approve": true}}

	// No x-project-id present — universal services / tenant-wide RPCs.
	err := RequirePermission(ctxWithCaller(uuid.Nil), checker, "budget:approve")
	require.NoError(t, err)

	require.Len(t, checker.calls, 1)
	assert.Nil(t, checker.calls[0].projectID)
}

func TestRequirePermission_NoCaller_Unauthenticated(t *testing.T) {
	enableFlag(t, "true")
	err := RequirePermission(context.Background(), &stubChecker{}, "budget:read")
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestRequirePermission_CheckerError_Internal(t *testing.T) {
	enableFlag(t, "true")
	checker := &stubChecker{err: errors.New("rpc down")}

	err := RequirePermission(ctxWithCaller(testProjectA), checker, "budget:read")
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
}
