package iam

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// platformAdminCtx returns a context with a synthetic platform_admin caller,
// as if UnaryAuthInterceptor had already verified the caller's token (#138).
// These tests call handlers directly and so bypass the interceptor entirely;
// pkg/server/integration_test.go is what proves the chain rejects a tokenless call.
func platformAdminCtx() context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID: uuid.MustParse("a0000000-0000-0000-0000-000000000001"),
		Email:  "admin@platform.internal",
		Roles:  []string{interceptor.RolePlatformAdmin},
		IP:     "127.0.0.1",
	})
}

func newHandler() *Handler {
	return NewHandler(newTestService(&mockRepo{}))
}

func newHandlerWithRepo(r *mockRepo) *Handler { return NewHandler(newTestService(r)) }

// memberCtx returns a caller in tenant tid holding only the `member` role —
// enough to pass authentication, not enough for platform-admin gates.
func memberCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "member@example.com",
		Roles:    []string{interceptor.RoleMember},
	})
}

// --- Login ---

func TestHandler_Login_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().Login(context.Background(), &iamv1.LoginRequest{
		TenantId: fixedTenantID.String(),
		Email:    "user@example.com",
		Password: "pass",
	})
	require.NoError(t, err)
	assert.Equal(t, "access", resp.GetAccessToken())
	assert.Equal(t, "Bearer", resp.GetTokenType())
}

func TestHandler_Login_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Login(context.Background(), &iamv1.LoginRequest{
		TenantId: "bad",
		Email:    "user@example.com",
		Password: "pass",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- RefreshToken ---

func TestHandler_RefreshToken_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().RefreshToken(context.Background(), &iamv1.RefreshTokenRequest{
		RefreshToken: "anytoken",
	})
	require.NoError(t, err)
	assert.Equal(t, "new-access", resp.GetAccessToken())
}

// --- Logout ---

func TestHandler_Logout_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().Logout(context.Background(), &iamv1.LogoutRequest{RefreshToken: "tok"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// --- ValidateToken ---

func TestHandler_ValidateToken_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().ValidateToken(context.Background(), &iamv1.ValidateTokenRequest{AccessToken: "anytoken"})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetSubject())
}

// --- CreateUser ---

func TestHandler_CreateUser_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().CreateUser(memberCtx(tid), &iamv1.CreateUserRequest{
		TenantId:    tid.String(),
		Email:       "new@example.com",
		DisplayName: "New User",
		Password:    "pass123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreateUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateUser(memberCtx(uuid.New()), &iamv1.CreateUserRequest{
		TenantId: "bad",
		Email:    "user@example.com",
		Password: "pass",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetUser ---

func TestHandler_GetUser_Success(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getUserFn: func(_ context.Context, _, id uuid.UUID) (*User, error) {
			return &User{ID: id, TenantID: tenantID, Email: "u@x.com"}, nil
		},
	}))

	resp, err := h.GetUser(memberCtx(tenantID), &iamv1.GetUserRequest{
		TenantId: tenantID.String(),
		Id:       userID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, userID.String(), resp.GetId())
}

func TestHandler_GetUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetUser(memberCtx(uuid.New()), &iamv1.GetUserRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetUser_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetUser(memberCtx(tid), &iamv1.GetUserRequest{TenantId: tid.String(), Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListUsers ---

func TestHandler_ListUsers_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listUsersFn: func(_ context.Context, _ uuid.UUID, _ string, _, _ int) ([]User, error) {
			return []User{{ID: uuid.New(), TenantID: tenantID, Email: "a@b.com", Status: "active"}}, nil
		},
	}))

	resp, err := h.ListUsers(memberCtx(tenantID), &iamv1.ListUsersRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetUsers(), 1)
}

func TestHandler_ListUsers_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListUsers(memberCtx(uuid.New()), &iamv1.ListUsersRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- UpdateUser ---

func TestHandler_UpdateUser_Success(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getUserFn: func(_ context.Context, _, id uuid.UUID) (*User, error) {
			return &User{ID: id, TenantID: tenantID, Email: "u@x.com", DisplayName: "Updated", Status: "active"}, nil
		},
	}))

	resp, err := h.UpdateUser(memberCtx(tenantID), &iamv1.UpdateUserRequest{
		TenantId:    tenantID.String(),
		Id:          userID.String(),
		DisplayName: "Updated",
		Status:      "active",
	})
	require.NoError(t, err)
	assert.Equal(t, userID.String(), resp.GetId())
}

func TestHandler_UpdateUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateUser(memberCtx(uuid.New()), &iamv1.UpdateUserRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateUser_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().UpdateUser(memberCtx(tid), &iamv1.UpdateUserRequest{TenantId: tid.String(), Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- DeactivateUser ---

func TestHandler_DeactivateUser_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().DeactivateUser(platformAdminCtx(), &iamv1.DeactivateUserRequest{
		TenantId: uuid.New().String(),
		Id:       uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_DeactivateUser_PermissionDenied(t *testing.T) {
	t.Parallel()
	// Calling without a platform_admin caller context must be rejected.
	_, err := newHandler().DeactivateUser(context.Background(), &iamv1.DeactivateUserRequest{
		TenantId: uuid.New().String(),
		Id:       uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_DeactivateUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().DeactivateUser(platformAdminCtx(), &iamv1.DeactivateUserRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_DeactivateUser_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().DeactivateUser(platformAdminCtx(), &iamv1.DeactivateUserRequest{TenantId: uuid.New().String(), Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ChangePassword ---

func TestHandler_ChangePassword_Success(t *testing.T) {
	t.Parallel()
	userID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, id uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{ID: id, PasswordHash: "hashed:old"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}))

	resp, err := h.ChangePassword(context.Background(), &iamv1.ChangePasswordRequest{
		UserId:      userID.String(),
		OldPassword: "old",
		NewPassword: "newpass",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_ChangePassword_InvalidUserID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ChangePassword(context.Background(), &iamv1.ChangePasswordRequest{
		UserId: "bad", OldPassword: "old", NewPassword: "new",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- AssignRole ---

func TestHandler_AssignRole_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().AssignRole(memberCtx(tid), &iamv1.AssignRoleRequest{
		TenantId:   tid.String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		AssignedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_AssignRole_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().AssignRole(memberCtx(uuid.New()), &iamv1.AssignRoleRequest{
		TenantId: "bad", UserId: uuid.New().String(), RoleId: uuid.New().String(), AssignedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_AssignRole_InvalidUserID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().AssignRole(memberCtx(tid), &iamv1.AssignRoleRequest{
		TenantId: tid.String(), UserId: "bad", RoleId: uuid.New().String(), AssignedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- RevokeRole ---

func TestHandler_RevokeRole_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().RevokeRole(context.Background(), &iamv1.RevokeRoleRequest{
		UserId: uuid.New().String(),
		RoleId: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_RevokeRole_InvalidUserID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RevokeRole(context.Background(), &iamv1.RevokeRoleRequest{UserId: "bad", RoleId: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListRoles ---

func TestHandler_ListRoles_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		listRolesFn: func(_ context.Context, _ uuid.UUID) ([]Role, error) {
			return []Role{{ID: uuid.New(), Name: "producer"}}, nil
		},
	}))

	resp, err := h.ListRoles(memberCtx(tenantID), &iamv1.ListRolesRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetRoles(), 1)
}

func TestHandler_ListRoles_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListRoles(memberCtx(uuid.New()), &iamv1.ListRolesRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CheckPermission ---

func TestHandler_CheckPermission_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]string, error) {
			return []string{"budget:read"}, nil
		},
	}))

	resp, err := h.CheckPermission(context.Background(), &iamv1.CheckPermissionRequest{
		UserId:     uuid.New().String(),
		Permission: "budget:read",
	})
	require.NoError(t, err)
	assert.True(t, resp.GetAllowed())
}

func TestHandler_CheckPermission_InvalidUserID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckPermission(context.Background(), &iamv1.CheckPermissionRequest{UserId: "bad", Permission: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CheckPermission_WithProjectID(t *testing.T) {
	t.Parallel()
	wantProject := uuid.New()
	var seenProject *uuid.UUID
	h := NewHandler(newTestService(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, projectID *uuid.UUID) ([]string, error) {
			seenProject = projectID
			return []string{"expense:approve"}, nil
		},
	}))

	resp, err := h.CheckPermission(context.Background(), &iamv1.CheckPermissionRequest{
		UserId:     uuid.New().String(),
		Permission: "expense:approve",
		ProjectId:  wantProject.String(),
	})
	require.NoError(t, err)
	assert.True(t, resp.GetAllowed())
	require.NotNil(t, seenProject)
	assert.Equal(t, wantProject, *seenProject)
}

func TestHandler_CheckPermission_InvalidProjectID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CheckPermission(context.Background(), &iamv1.CheckPermissionRequest{
		UserId:     uuid.New().String(),
		Permission: "x",
		ProjectId:  "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- AssignProjectRole ---

func TestHandler_AssignProjectRole_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			return &Role{ID: roleID, TenantID: tenantID, Name: "project_supervisor", IsSystem: true}, nil
		},
	}))

	tid := uuid.New()
	resp, err := h.AssignProjectRole(memberCtx(tid), &iamv1.AssignProjectRoleRequest{
		TenantId:   tid.String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		ProjectId:  uuid.New().String(),
		AssignedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_AssignProjectRole_RejectsTenantWideRole(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			return &Role{ID: roleID, TenantID: tenantID, Name: "manager", IsSystem: true}, nil
		},
	}))

	tid := uuid.New()
	_, err := h.AssignProjectRole(memberCtx(tid), &iamv1.AssignProjectRoleRequest{
		TenantId:   tid.String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		ProjectId:  uuid.New().String(),
		AssignedBy: uuid.New().String(),
	})
	// ErrRoleNotProjectScoped maps to InvalidArgument in grpcError.
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestHandler_AssignProjectRole_InvalidArgs no longer covers "assigned_by":
// the handler stopped reading req.GetAssignedBy() (it now sources the audit
// identity from the verified caller), so an invalid assigned_by string is no
// longer a validation error — the sub-case was removed for that reason. The
// replacement behaviour (a forged assigned_by is ignored in favor of the
// caller's identity) is covered by TestAssignProjectRole_AssignedByIsTheCaller.
func TestHandler_AssignProjectRole_InvalidArgs(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, field string }{
		{"invalid tenant_id", "tenant"},
		{"invalid user_id", "user"},
		{"invalid role_id", "role"},
		{"invalid project_id", "project"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tid := uuid.New()
			req := &iamv1.AssignProjectRoleRequest{
				TenantId:   tid.String(),
				UserId:     uuid.New().String(),
				RoleId:     uuid.New().String(),
				ProjectId:  uuid.New().String(),
				AssignedBy: uuid.New().String(),
			}
			switch tc.field {
			case "tenant":
				req.TenantId = "bad"
			case "user":
				req.UserId = "bad"
			case "role":
				req.RoleId = "bad"
			case "project":
				req.ProjectId = "bad"
			}
			_, err := newHandler().AssignProjectRole(memberCtx(tid), req)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

// --- CreateTenant ---

func TestHandler_CreateTenant_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().CreateTenant(platformAdminCtx(), &iamv1.CreateTenantRequest{
		Name:        "Red Chillies Entertainment",
		Plan:        "starter",
		CountryCode: "IN", // #61 — required; currency derived as INR
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
	assert.Equal(t, "starter", resp.GetPlan())
}

// --- SetTenantAddress ---

func TestHandler_SetTenantAddress_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		updateTenantAddressFn: func(_ context.Context, tn *Tenant) (*Tenant, error) {
			return tn, nil
		},
	}))

	resp, err := h.SetTenantAddress(memberCtx(tenantID), &iamv1.SetTenantAddressRequest{
		TenantId:    tenantID.String(),
		CountryCode: "IN",
		City:        "Chennai",
	})
	require.NoError(t, err)
	assert.Equal(t, "IN", resp.GetCountryCode())
	assert.Equal(t, "INR", resp.GetPrimaryCurrencyCode())
}

func TestHandler_SetTenantAddress_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SetTenantAddress(memberCtx(uuid.New()), &iamv1.SetTenantAddressRequest{
		TenantId:    "not-a-uuid",
		CountryCode: "IN",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetTenantAddress_MissingCountry(t *testing.T) {
	t.Parallel()
	h := newHandler()
	tid := uuid.New()
	_, err := h.SetTenantAddress(memberCtx(tid), &iamv1.SetTenantAddressRequest{
		TenantId: tid.String(),
	})
	require.Error(t, err)
}

// --- GetTenant ---

func TestHandler_GetTenant_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Name: "Test Co", Plan: "basic", Status: "active"}, nil
		},
	}))

	resp, err := h.GetTenant(context.Background(), &iamv1.GetTenantRequest{Id: tenantID.String()})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetId())
}

func TestHandler_GetTenant_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetTenant(context.Background(), &iamv1.GetTenantRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- SuspendTenant ---

func TestHandler_SuspendTenant_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Name: "Test", Status: "suspended"}, nil
		},
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ *time.Time, _ *string) error { return nil },
	}))

	resp, err := h.SuspendTenant(platformAdminCtx(), &iamv1.SuspendTenantRequest{Id: tenantID.String()})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetId())
}

func TestHandler_SuspendTenant_PermissionDenied(t *testing.T) {
	t.Parallel()
	// Calling without a platform_admin caller context must be rejected.
	_, err := newHandler().SuspendTenant(context.Background(), &iamv1.SuspendTenantRequest{Id: uuid.New().String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SuspendTenant_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SuspendTenant(platformAdminCtx(), &iamv1.SuspendTenantRequest{Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SuspendTenant_LegalHoldFieldsPropagate(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	holdUntil := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reason := "litigation pending — preserve data"

	var gotHoldUntil *time.Time
	var gotFreezeReason *string
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Name: "Test", Status: "suspended"}, nil
		},
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, holdUntil *time.Time, freezeReason *string) error {
			gotHoldUntil = holdUntil
			gotFreezeReason = freezeReason
			return nil
		},
	}))

	req := &iamv1.SuspendTenantRequest{
		Id:           tenantID.String(),
		HoldUntil:    timestamppb.New(holdUntil),
		FreezeReason: &reason,
	}
	_, err := h.SuspendTenant(platformAdminCtx(), req)
	require.NoError(t, err)

	require.NotNil(t, gotHoldUntil)
	assert.True(t, gotHoldUntil.Equal(holdUntil), "hold_until round-trips through proto Timestamp")
	require.NotNil(t, gotFreezeReason)
	assert.Equal(t, reason, *gotFreezeReason)
}

// --- ClearTenantLegalHold ---

func TestHandler_ClearTenantLegalHold_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	var gotID uuid.UUID
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
		clearTenantLegalHoldFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			gotID = id
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
	}))

	resp, err := h.ClearTenantLegalHold(platformAdminCtx(), &iamv1.ClearTenantLegalHoldRequest{
		Id: tenantID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetId())
	assert.Equal(t, tenantID, gotID)
}

func TestHandler_ClearTenantLegalHold_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ClearTenantLegalHold(context.Background(), &iamv1.ClearTenantLegalHoldRequest{
		Id: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ClearTenantLegalHold_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ClearTenantLegalHold(platformAdminCtx(), &iamv1.ClearTenantLegalHoldRequest{
		Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ClearTenantLegalHold_ReasonPassesThrough(t *testing.T) {
	t.Parallel()
	// The reason is captured by the service in audit metadata; here we
	// just need to prove it round-trips from proto to the service
	// without being dropped by the handler.
	tenantID := uuid.New()
	held := "court order 2026-CV-789"
	hu := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Service-level capture: the handler hands the reason to svc, which
	// then hands it to the audit path. We verify the audit metadata
	// round-trips in the service tests; here we just assert no error
	// and that the call reaches the service.
	called := false
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended", HoldUntil: &hu, FreezeReason: &held}, nil
		},
		clearTenantLegalHoldFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			called = true
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
	}))

	reason := "compliance review completed"
	_, err := h.ClearTenantLegalHold(platformAdminCtx(), &iamv1.ClearTenantLegalHoldRequest{
		Id:     tenantID.String(),
		Reason: &reason,
	})
	require.NoError(t, err)
	assert.True(t, called, "service's ClearTenantLegalHold should be invoked")
}

// --- SetTenantRetention ---

func TestHandler_SetTenantRetention_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended", FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
		},
	}))
	resp, err := h.SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id: tenantID.String(), FreezeReason: "support escalation",
	})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetId())
}

func TestHandler_SetTenantRetention_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SetTenantRetention(context.Background(), &iamv1.SetTenantRetentionRequest{
		Id: uuid.New().String(), FreezeReason: "x",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SetTenantRetention_EmptyReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id: uuid.New().String(), FreezeReason: "  ",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetTenantRetention_HoldUntilPresenceRoundTrips(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	future := time.Now().Add(60 * 24 * time.Hour).UTC()
	ts := timestamppb.New(future)

	var gotHoldUntil *time.Time
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
		setTenantLegalHoldFn: func(_ context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
			gotHoldUntil = holdUntil
			return &Tenant{ID: id, Status: "suspended", FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
		},
	}))

	_, err := h.SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id:           tenantID.String(),
		FreezeReason: "x",
		HoldUntil:    ts,
	})
	require.NoError(t, err)
	require.NotNil(t, gotHoldUntil, "handler must read the raw req.HoldUntil field, not drop it")
	assert.True(t, gotHoldUntil.Equal(ts.AsTime()), "hold_until must round-trip through proto Timestamp")
}

func TestHandler_SetTenantRetention_NotHoldableMapsFailedPrecondition(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "active"}, nil
		},
	}))
	_, err := h.SetTenantRetention(platformAdminCtx(), &iamv1.SetTenantRetentionRequest{
		Id: uuid.New().String(), FreezeReason: "x",
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestHandler_SuspendTenant_BareRequest_PlumbsNilHoldFields(t *testing.T) {
	t.Parallel()
	// A request with only Id (no hold fields) must reach the service
	// with nil pointers so the repo's COALESCE preserves any existing
	// hold on the row.
	tenantID := uuid.New()

	var gotHoldUntil *time.Time
	var gotFreezeReason *string
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "suspended"}, nil
		},
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, holdUntil *time.Time, freezeReason *string) error {
			gotHoldUntil = holdUntil
			gotFreezeReason = freezeReason
			return nil
		},
	}))

	_, err := h.SuspendTenant(platformAdminCtx(), &iamv1.SuspendTenantRequest{Id: tenantID.String()})
	require.NoError(t, err)
	assert.Nil(t, gotHoldUntil)
	assert.Nil(t, gotFreezeReason)
}

// --- RequestTenantPurge / ApproveTenantPurge / CancelTenantPurge (#92 Stage 5) ---

func TestHandler_RequestTenantPurge_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusPurgeEligible, Name: "X", Slug: "x"}, nil
		},
		createTenantPurgeRequestFn: func(_ context.Context, _ *TenantPurgeRequest) error { return nil },
	}))
	resp, err := h.RequestTenantPurge(platformAdminCtx(), &iamv1.RequestTenantPurgeRequest{TenantId: tid.String(), Reason: "gdpr"})
	require.NoError(t, err)
	assert.Equal(t, "pending", resp.GetStatus())
	assert.Equal(t, tid.String(), resp.GetTenantId())
}

func TestHandler_RequestTenantPurge_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RequestTenantPurge(context.Background(), &iamv1.RequestTenantPurgeRequest{TenantId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_RequestTenantPurge_EmptyReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RequestTenantPurge(platformAdminCtx(), &iamv1.RequestTenantPurgeRequest{TenantId: uuid.New().String(), Reason: "   "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_RequestTenantPurge_NotEligible_FailedPrecondition(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "active"}, nil
		},
	}))
	_, err := h.RequestTenantPurge(platformAdminCtx(), &iamv1.RequestTenantPurgeRequest{TenantId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestHandler_ApproveTenantPurge_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	// requester differs from the zero-value uuid.UUID{} that the handler's
	// ctx resolves to via audit.ActorFromContext (platformAdminCtx only sets
	// the interceptor caller, not the audit actor) so this isn't a self-approval.
	requester := uuid.New()
	openReq := &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestPending, RequestedBy: requester}
	h := NewHandler(newTestService(&mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, _ uuid.UUID) (*TenantPurgeRequest, error) {
			return openReq, nil
		},
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusPurgeEligible}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: requestID, TenantID: tid, Status: PurgeRequestApproved, RequestedBy: requester, ApprovedBy: &approverID}, nil
		},
	}))
	resp, err := h.ApproveTenantPurge(platformAdminCtx(), &iamv1.ApproveTenantPurgeRequest{TenantId: tid.String(), Reason: "ok"})
	require.NoError(t, err)
	assert.Equal(t, "approved", resp.GetStatus())
	assert.Equal(t, uuid.UUID{}.String(), resp.GetApprovedBy())
}

func TestHandler_ApproveTenantPurge_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveTenantPurge(context.Background(), &iamv1.ApproveTenantPurgeRequest{TenantId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ApproveTenantPurge_EmptyReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveTenantPurge(platformAdminCtx(), &iamv1.ApproveTenantPurgeRequest{TenantId: uuid.New().String(), Reason: ""})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestHandler_ApproveTenantPurge_SelfApproval_FailedPrecondition exercises the
// full handler -> service -> grpcError chain: platformAdminCtx's caller is
// not attached to the audit-actor context, so ActorFromContext resolves to
// the zero uuid.UUID{} — matching an open request whose RequestedBy is also
// left at its zero value simulates the same caller requesting and approving.
func TestHandler_ApproveTenantPurge_SelfApproval_FailedPrecondition(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, _ uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestPending, RequestedBy: uuid.UUID{}}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, _, _ uuid.UUID) (*TenantPurgeRequest, error) {
			t.Fatal("must not approve on self-approval")
			return nil, nil
		},
	}))
	_, err := h.ApproveTenantPurge(platformAdminCtx(), &iamv1.ApproveTenantPurgeRequest{TenantId: tid.String(), Reason: "ok"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestHandler_CancelTenantPurge_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	openReq := &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestPending, RequestedBy: uuid.New()}
	h := NewHandler(newTestService(&mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, _ uuid.UUID) (*TenantPurgeRequest, error) {
			return openReq, nil
		},
		cancelTenantPurgeRequestFn: func(_ context.Context, requestID, _ uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: requestID, TenantID: tid, Status: PurgeRequestCancelled, RequestedBy: openReq.RequestedBy}, nil
		},
	}))
	resp, err := h.CancelTenantPurge(platformAdminCtx(), &iamv1.CancelTenantPurgeRequest{TenantId: tid.String(), Reason: "changed my mind"})
	require.NoError(t, err)
	assert.Equal(t, "cancelled", resp.GetStatus())
}

func TestHandler_CancelTenantPurge_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CancelTenantPurge(context.Background(), &iamv1.CancelTenantPurgeRequest{TenantId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CancelTenantPurge_EmptyReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CancelTenantPurge(platformAdminCtx(), &iamv1.CancelTenantPurgeRequest{TenantId: uuid.New().String(), Reason: " "})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CancelTenantPurge_NoOpenRequest_NotFound(t *testing.T) {
	t.Parallel()
	// mockRepo default (no getOpenTenantPurgeRequestFn) returns ErrPurgeRequestNotFound.
	h := NewHandler(newTestService(&mockRepo{}))
	_, err := h.CancelTenantPurge(platformAdminCtx(), &iamv1.CancelTenantPurgeRequest{TenantId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- InviteUser ---

func TestHandler_InviteUser_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().InviteUser(memberCtx(tid), &iamv1.InviteUserRequest{
		TenantId:  tid.String(),
		Email:     "invite@example.com",
		InvitedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_InviteUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InviteUser(memberCtx(uuid.New()), &iamv1.InviteUserRequest{
		TenantId: "bad", Email: "e@e.com", InvitedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// Note: there is no TestHandler_InviteUser_InvalidInvitedBy — the handler no
// longer reads req.GetInvitedBy() at all (invited_by is now sourced from the
// verified caller), so an invalid invited_by string in the request is no
// longer a validation error and the sub-case was removed. The replacement
// behaviour (a forged invited_by is ignored in favor of the caller's
// identity) is covered by TestInviteUser_InvitedByIsTheCaller.

func TestHandler_InviteUser_InvalidRoleID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().InviteUser(memberCtx(tid), &iamv1.InviteUserRequest{
		TenantId: tid.String(), Email: "e@e.com", InvitedBy: uuid.New().String(), RoleId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- AcceptInvitation ---

func TestHandler_AcceptInvitation_Success(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getInvitationByTokenFn: func(_ context.Context, token string) (*Invitation, error) {
			return &Invitation{
				ID:        uuid.New(),
				TenantID:  fixedTenantID,
				Email:     "invite@example.com",
				Token:     token,
				Status:    "pending",
				InvitedBy: uuid.New(),
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
	}))

	resp, err := h.AcceptInvitation(context.Background(), &iamv1.AcceptInvitationRequest{
		Token:    "sometoken",
		Password: "pass123",
	})
	require.NoError(t, err)
	assert.Equal(t, "access", resp.GetAccessToken())
}

// --- StartImpersonation ---

func TestHandler_StartImpersonation_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().StartImpersonation(platformAdminCtx(), &iamv1.StartImpersonationRequest{
		PlatformUserId:   uuid.New().String(),
		TenantId:         uuid.New().String(),
		ImpersonatedUser: uuid.New().String(),
		Reason:           "support ticket #99",
		DurationSeconds:  1800,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_StartImpersonation_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().StartImpersonation(context.Background(), &iamv1.StartImpersonationRequest{
		PlatformUserId:   uuid.New().String(),
		TenantId:         uuid.New().String(),
		ImpersonatedUser: uuid.New().String(),
		Reason:           "test",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_StartImpersonation_MissingReason(t *testing.T) {
	t.Parallel()
	_, err := newHandler().StartImpersonation(platformAdminCtx(), &iamv1.StartImpersonationRequest{
		PlatformUserId:   uuid.New().String(),
		TenantId:         uuid.New().String(),
		ImpersonatedUser: uuid.New().String(),
		Reason:           "", // empty
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- EndImpersonation ---

func TestHandler_EndImpersonation_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().EndImpersonation(platformAdminCtx(), &iamv1.EndImpersonationRequest{
		SessionId: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_EndImpersonation_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().EndImpersonation(context.Background(), &iamv1.EndImpersonationRequest{
		SessionId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_EndImpersonation_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		endImpersonationSessionFn: func(_ context.Context, _ uuid.UUID) error {
			return ErrImpersonationNotFound
		},
	}))
	_, err := h.EndImpersonation(platformAdminCtx(), &iamv1.EndImpersonationRequest{
		SessionId: uuid.New().String(),
	})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestHandler_EndImpersonation_AlreadyEnded(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		endImpersonationSessionFn: func(_ context.Context, _ uuid.UUID) error {
			return ErrImpersonationAlreadyEnded
		},
	}))
	_, err := h.EndImpersonation(platformAdminCtx(), &iamv1.EndImpersonationRequest{
		SessionId: uuid.New().String(),
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

// --- SetOIDCConfig ---

func TestHandler_SetOIDCConfig_PermissionDenied(t *testing.T) {
	t.Parallel()
	// Calling without a platform_admin caller context must be rejected before
	// any field validation or service logic runs.
	_, err := newHandler().SetOIDCConfig(context.Background(), &iamv1.SetOIDCConfigRequest{
		TenantId:     uuid.New().String(),
		IssuerUrl:    "https://accounts.google.com",
		ClientId:     "client-id",
		ClientSecret: "secret",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrUserNotFound, codes.NotFound},
		{ErrTenantNotFound, codes.NotFound},
		{ErrRoleNotFound, codes.NotFound},
		{ErrInvitationNotFound, codes.NotFound},
		{ErrUserAlreadyExists, codes.AlreadyExists},
		{ErrTenantSlugTaken, codes.AlreadyExists},
		{ErrInvitationExpired, codes.FailedPrecondition},
		{ErrInvitationAccepted, codes.FailedPrecondition},
		{ErrInvalidPlan, codes.InvalidArgument},
		{auth.ErrInvalidCredentials, codes.Unauthenticated},
		{auth.ErrTokenExpired, codes.Unauthenticated},
		{auth.ErrTokenInvalid, codes.Unauthenticated},
		{auth.ErrRefreshTokenNotFound, codes.Unauthenticated},
		{auth.ErrTenantSuspended, codes.PermissionDenied},
		{auth.ErrAccountDeactivated, codes.PermissionDenied},
		{auth.ErrAccountInvited, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcError(tc.err)))
	}
}

func TestHandler_SetOIDCConfig_Success(t *testing.T) {
	t.Parallel()
	// Platform admin caller with a valid OIDC encryption key configured.
	svc := newTestService(&mockRepo{
		upsertOIDCConfigFn: func(_ context.Context, _ OIDCConfigParams) error {
			return nil
		},
	})
	svc.WithOIDCEncryptionKey(oidcTestKey)
	h := NewHandler(svc)

	resp, err := h.SetOIDCConfig(platformAdminCtx(), &iamv1.SetOIDCConfigRequest{
		TenantId:     fixedTenantID.String(),
		IssuerUrl:    "https://accounts.google.com",
		ClientId:     "my-client-id",
		ClientSecret: "plaintext-secret",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_SetOIDCConfig_ServiceError_ReturnsInternalError(t *testing.T) {
	t.Parallel()
	// Service returns a generic error after encryption — handler must map to Internal.
	svc := newTestService(&mockRepo{
		upsertOIDCConfigFn: func(_ context.Context, _ OIDCConfigParams) error {
			return fmt.Errorf("db unavailable")
		},
	})
	svc.WithOIDCEncryptionKey(oidcTestKey)
	h := NewHandler(svc)

	_, err := h.SetOIDCConfig(platformAdminCtx(), &iamv1.SetOIDCConfigRequest{
		TenantId:     fixedTenantID.String(),
		IssuerUrl:    "https://accounts.google.com",
		ClientId:     "my-client-id",
		ClientSecret: "plaintext-secret",
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// --- #144: tenant boundary + forgeable audit identity ---

// CreateTenant reads no tenant_id from the request (it creates one), so
// TenantFromRequest does not apply here — it must instead gate on
// RequireRole(platform_admin), same as the platform RPCs.
func TestCreateTenant_RequiresPlatformAdmin(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateTenant(memberCtx(uuid.New()), &iamv1.CreateTenantRequest{Name: "x"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// A caller authenticated for tenant A must not be able to act on tenant B by
// simply naming B in the request body. The repository must never be reached.
func TestCreateUser_CrossTenant_Denied(t *testing.T) {
	t.Parallel()
	caller, victim := uuid.New(), uuid.New()
	require.NotEqual(t, caller, victim)

	h := newHandlerWithRepo(&mockRepo{
		createUserFn: func(context.Context, *User) error {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil
		},
	})
	_, err := h.CreateUser(memberCtx(caller), &iamv1.CreateUserRequest{
		TenantId: victim.String(), Email: "a@b.c", Password: "x",
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAssignRole_CrossTenant_Denied(t *testing.T) {
	t.Parallel()
	caller, victim := uuid.New(), uuid.New()
	require.NotEqual(t, caller, victim)

	h := newHandlerWithRepo(&mockRepo{
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil
		},
	})
	_, err := h.AssignRole(memberCtx(caller), &iamv1.AssignRoleRequest{
		TenantId: victim.String(), UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// The audit trail must name the caller, not whoever the request names.
func TestAssignRole_AssignedByIsTheCaller(t *testing.T) {
	t.Parallel()
	tid, callerID := uuid.New(), uuid.New()
	var gotAssignedBy uuid.UUID

	h := newHandlerWithRepo(&mockRepo{
		assignRoleFn: func(_ context.Context, ur *UserRole) error {
			gotAssignedBy = ur.AssignedBy
			return nil
		},
	})
	ctx := interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID: callerID, TenantID: tid, Roles: []string{interceptor.RoleMember},
	})

	_, err := h.AssignRole(ctx, &iamv1.AssignRoleRequest{
		TenantId:   tid.String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		AssignedBy: uuid.New().String(), // a lie the handler must ignore
	})
	require.NoError(t, err)
	assert.Equal(t, callerID, gotAssignedBy, "assigned_by must come from the token, not the request")
}

// The audit trail must name the caller, not whoever the request names.
// Mirrors TestAssignRole_AssignedByIsTheCaller for the AssignProjectRole path,
// which has its own req.GetAssignedBy() forgery hole closed independently of
// AssignRole (see handler.go AssignProjectRole).
func TestAssignProjectRole_AssignedByIsTheCaller(t *testing.T) {
	t.Parallel()
	tid, callerID, projectID := uuid.New(), uuid.New(), uuid.New()
	var gotAssignedBy uuid.UUID
	var gotProjectID *uuid.UUID

	h := newHandlerWithRepo(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			return &Role{ID: roleID, TenantID: tenantID, Name: "project_supervisor", IsSystem: true}, nil
		},
		assignRoleFn: func(_ context.Context, ur *UserRole) error {
			gotAssignedBy = ur.AssignedBy
			gotProjectID = ur.ProjectID
			return nil
		},
	})
	ctx := interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID: callerID, TenantID: tid, Roles: []string{interceptor.RoleMember},
	})

	_, err := h.AssignProjectRole(ctx, &iamv1.AssignProjectRoleRequest{
		TenantId:   tid.String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		ProjectId:  projectID.String(),
		AssignedBy: uuid.New().String(), // a lie the handler must ignore
	})
	require.NoError(t, err)
	assert.Equal(t, callerID, gotAssignedBy, "assigned_by must come from the token, not the request")
	require.NotNil(t, gotProjectID, "project_id must survive to the repository call")
	assert.Equal(t, projectID, *gotProjectID, "project_id must survive to the repository call")
}

// The audit trail must name the caller, not whoever the request names.
// Mirrors TestAssignRole_AssignedByIsTheCaller for the InviteUser path, which
// has its own req.GetInvitedBy() forgery hole closed independently of
// AssignRole (see handler.go InviteUser).
func TestInviteUser_InvitedByIsTheCaller(t *testing.T) {
	t.Parallel()
	tid, callerID := uuid.New(), uuid.New()
	var gotInvitedBy uuid.UUID

	h := newHandlerWithRepo(&mockRepo{
		createInvitationFn: func(_ context.Context, inv *Invitation) error {
			gotInvitedBy = inv.InvitedBy
			return nil
		},
	})
	ctx := interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID: callerID, TenantID: tid, Roles: []string{interceptor.RoleMember},
	})

	_, err := h.InviteUser(ctx, &iamv1.InviteUserRequest{
		TenantId:  tid.String(),
		Email:     "invite@example.com",
		InvitedBy: uuid.New().String(), // a lie the handler must ignore
	})
	require.NoError(t, err)
	assert.Equal(t, callerID, gotInvitedBy, "invited_by must come from the token, not the request")
}
