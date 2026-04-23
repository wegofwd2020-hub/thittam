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
// as if the UnaryCallerInterceptor had already run and Kong had injected the headers.
func platformAdminCtx() context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID: uuid.MustParse("a0000000-0000-0000-0000-000000000001"),
		Email:  "admin@platform.internal",
		Role:   interceptor.RolePlatformAdmin,
		IP:     "127.0.0.1",
	})
}

func newHandler() *Handler {
	return NewHandler(newTestService(&mockRepo{}))
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
	resp, err := newHandler().CreateUser(context.Background(), &iamv1.CreateUserRequest{
		TenantId:    uuid.New().String(),
		Email:       "new@example.com",
		DisplayName: "New User",
		Password:    "pass123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreateUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateUser(context.Background(), &iamv1.CreateUserRequest{
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

	resp, err := h.GetUser(context.Background(), &iamv1.GetUserRequest{
		TenantId: tenantID.String(),
		Id:       userID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, userID.String(), resp.GetId())
}

func TestHandler_GetUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetUser(context.Background(), &iamv1.GetUserRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetUser_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetUser(context.Background(), &iamv1.GetUserRequest{TenantId: uuid.New().String(), Id: "bad"})
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

	resp, err := h.ListUsers(context.Background(), &iamv1.ListUsersRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetUsers(), 1)
}

func TestHandler_ListUsers_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListUsers(context.Background(), &iamv1.ListUsersRequest{TenantId: "bad"})
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

	resp, err := h.UpdateUser(context.Background(), &iamv1.UpdateUserRequest{
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
	_, err := newHandler().UpdateUser(context.Background(), &iamv1.UpdateUserRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateUser_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateUser(context.Background(), &iamv1.UpdateUserRequest{TenantId: uuid.New().String(), Id: "bad"})
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
	resp, err := newHandler().AssignRole(context.Background(), &iamv1.AssignRoleRequest{
		TenantId:   uuid.New().String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		AssignedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_AssignRole_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().AssignRole(context.Background(), &iamv1.AssignRoleRequest{
		TenantId: "bad", UserId: uuid.New().String(), RoleId: uuid.New().String(), AssignedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_AssignRole_InvalidUserID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().AssignRole(context.Background(), &iamv1.AssignRoleRequest{
		TenantId: uuid.New().String(), UserId: "bad", RoleId: uuid.New().String(), AssignedBy: uuid.New().String(),
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

	resp, err := h.ListRoles(context.Background(), &iamv1.ListRolesRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetRoles(), 1)
}

func TestHandler_ListRoles_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListRoles(context.Background(), &iamv1.ListRolesRequest{TenantId: "bad"})
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

	resp, err := h.AssignProjectRole(context.Background(), &iamv1.AssignProjectRoleRequest{
		TenantId:   uuid.New().String(),
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

	_, err := h.AssignProjectRole(context.Background(), &iamv1.AssignProjectRoleRequest{
		TenantId:   uuid.New().String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		ProjectId:  uuid.New().String(),
		AssignedBy: uuid.New().String(),
	})
	// ErrRoleNotProjectScoped maps to InvalidArgument in grpcError.
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_AssignProjectRole_InvalidArgs(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, field string }{
		{"invalid tenant_id", "tenant"},
		{"invalid user_id", "user"},
		{"invalid role_id", "role"},
		{"invalid project_id", "project"},
		{"invalid assigned_by", "assigned_by"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := &iamv1.AssignProjectRoleRequest{
				TenantId:   uuid.New().String(),
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
			case "assigned_by":
				req.AssignedBy = "bad"
			}
			_, err := newHandler().AssignProjectRole(context.Background(), req)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

// --- CreateTenant ---

func TestHandler_CreateTenant_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().CreateTenant(context.Background(), &iamv1.CreateTenantRequest{
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

	resp, err := h.SetTenantAddress(context.Background(), &iamv1.SetTenantAddressRequest{
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
	_, err := newHandler().SetTenantAddress(context.Background(), &iamv1.SetTenantAddressRequest{
		TenantId:    "not-a-uuid",
		CountryCode: "IN",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_SetTenantAddress_MissingCountry(t *testing.T) {
	t.Parallel()
	h := newHandler()
	_, err := h.SetTenantAddress(context.Background(), &iamv1.SetTenantAddressRequest{
		TenantId: uuid.New().String(),
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

// --- InviteUser ---

func TestHandler_InviteUser_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().InviteUser(context.Background(), &iamv1.InviteUserRequest{
		TenantId:  uuid.New().String(),
		Email:     "invite@example.com",
		InvitedBy: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_InviteUser_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InviteUser(context.Background(), &iamv1.InviteUserRequest{
		TenantId: "bad", Email: "e@e.com", InvitedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InviteUser_InvalidInvitedBy(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InviteUser(context.Background(), &iamv1.InviteUserRequest{
		TenantId: uuid.New().String(), Email: "e@e.com", InvitedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_InviteUser_InvalidRoleID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().InviteUser(context.Background(), &iamv1.InviteUserRequest{
		TenantId: uuid.New().String(), Email: "e@e.com", InvitedBy: uuid.New().String(), RoleId: "bad",
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
