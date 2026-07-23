package iam

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	iamv1 "github.com/wegofwd2020/thittam/gen/iam/v1"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements iamv1.IAMServiceServer by delegating to Service.
type Handler struct {
	iamv1.UnimplementedIAMServiceServer
	svc *Service
}

// NewHandler creates an IAM gRPC handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Compile-time check.
var _ iamv1.IAMServiceServer = (*Handler)(nil)

// --- Authentication ---

func (h *Handler) Login(ctx context.Context, req *iamv1.LoginRequest) (*iamv1.TokenPair, error) {
	// Empty tenant_id is allowed — the service resolves it from the email
	// (single-tenant emails only; ambiguous emails return ErrAmbiguousEmail
	// and the caller must retry with tenant_id explicitly set).
	var tenantID uuid.UUID
	if raw := req.GetTenantId(); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
		}
		tenantID = parsed
	}
	pair, err := h.svc.Login(ctx, tenantID, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, grpcError(err)
	}
	return tokenPairToProto(pair), nil
}

func (h *Handler) RefreshToken(ctx context.Context, req *iamv1.RefreshTokenRequest) (*iamv1.TokenPair, error) {
	pair, err := h.svc.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, grpcError(err)
	}
	return tokenPairToProto(pair), nil
}

func (h *Handler) Logout(ctx context.Context, req *iamv1.LogoutRequest) (*iamv1.LogoutResponse, error) {
	if err := h.svc.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.LogoutResponse{}, nil
}

func (h *Handler) ValidateToken(ctx context.Context, req *iamv1.ValidateTokenRequest) (*iamv1.Claims, error) {
	claims, err := h.svc.tokens.Validate(ctx, req.GetAccessToken())
	if err != nil {
		return nil, grpcError(err)
	}
	return claimsToProto(claims), nil
}

// GetCurrentUser extracts the bearer token from the Authorization metadata,
// resolves the user + tenant, and returns both. Roles + permissions on the
// returned User come from the validated JWT claims (the User table doesn't
// store them inline). The grpc-gateway forwards HTTP headers as gRPC
// metadata, so the UI's `Authorization: Bearer …` header arrives here
// verbatim.
func (h *Handler) GetCurrentUser(ctx context.Context, _ *iamv1.GetCurrentUserRequest) (*iamv1.GetCurrentUserResponse, error) {
	token, err := bearerTokenFromContext(ctx)
	if err != nil {
		return nil, err
	}
	user, tenant, err := h.svc.GetCurrentUser(ctx, token)
	if err != nil {
		return nil, grpcError(err)
	}
	claims, err := h.svc.tokens.Validate(ctx, token)
	if err != nil {
		return nil, grpcError(err)
	}
	userProto := userToProto(user)
	userProto.Roles = claims.Roles
	userProto.Permissions = claims.Permissions
	return &iamv1.GetCurrentUserResponse{
		User:   userProto,
		Tenant: tenantToProto(tenant),
	}, nil
}

// bearerTokenFromContext extracts a bearer access token from the incoming
// gRPC Authorization metadata. Returns Unauthenticated if missing or
// malformed.
func bearerTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}
	const prefix = "Bearer "
	v := values[0]
	if len(v) <= len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) {
		return "", status.Error(codes.Unauthenticated, "authorization header must be 'Bearer <token>'")
	}
	return v[len(prefix):], nil
}

// --- Users ---

func (h *Handler) CreateUser(ctx context.Context, req *iamv1.CreateUserRequest) (*iamv1.User, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	user := &User{
		TenantID:    tenantID,
		Email:       req.GetEmail(),
		DisplayName: req.GetDisplayName(),
	}
	created, err := h.svc.CreateUser(ctx, user, req.GetPassword())
	if err != nil {
		return nil, grpcError(err)
	}
	return userToProto(created), nil
}

func (h *Handler) GetUser(ctx context.Context, req *iamv1.GetUserRequest) (*iamv1.User, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	user, err := h.svc.GetUser(ctx, tenantID, id)
	if err != nil {
		return nil, grpcError(err)
	}
	return userToProto(user), nil
}

func (h *Handler) ListUsers(ctx context.Context, req *iamv1.ListUsersRequest) (*iamv1.ListUsersResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	users, err := h.svc.ListUsers(ctx, tenantID, req.GetStatus(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, grpcError(err)
	}
	out := make([]*iamv1.User, len(users))
	for i := range users {
		out[i] = userToProto(&users[i])
	}
	return &iamv1.ListUsersResponse{Users: out}, nil
}

func (h *Handler) UpdateUser(ctx context.Context, req *iamv1.UpdateUserRequest) (*iamv1.User, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	user := &User{
		ID:          id,
		TenantID:    tenantID,
		DisplayName: req.GetDisplayName(),
		Status:      req.GetStatus(),
	}
	updated, err := h.svc.UpdateUser(ctx, user)
	if err != nil {
		return nil, grpcError(err)
	}
	return userToProto(updated), nil
}

func (h *Handler) DeactivateUser(ctx context.Context, req *iamv1.DeactivateUserRequest) (*iamv1.DeactivateUserResponse, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	if err := h.svc.DeactivateUser(ctx, tenantID, id); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.DeactivateUserResponse{}, nil
}

func (h *Handler) ChangePassword(ctx context.Context, req *iamv1.ChangePasswordRequest) (*iamv1.ChangePasswordResponse, error) {
	// The subject comes from the verified token, never the request body: this RPC
	// previously accepted any user_id, so a caller who knew another user's password
	// could change it in any tenant (#139 slice A). ActorFromRequest returns the
	// caller's own id, so Service.ChangePassword cannot be aimed at anyone else.
	userID, err := interceptor.ActorFromRequest(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := h.svc.ChangePassword(ctx, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.ChangePasswordResponse{}, nil
}

// --- Roles & Permissions ---

// requireUserManage authorizes a role-management RPC.
//
// iam answers this from its own repository rather than dialling itself: the four other
// gated services hold a gRPC PermissionChecker (wired via interceptor) backed by a call
// to iam, and iam holding one would mean iam calling itself. After #138 a nil checker
// makes that interceptor path return Internal for every gated RPC.
//
// Fails closed on every path: no caller, a repository error, and a negative answer all deny.
func (h *Handler) requireUserManage(ctx context.Context) error {
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	allowed, err := h.svc.CheckPermission(ctx, caller.UserID, permUserManage, nil)
	if err != nil {
		// A failed permission lookup is a misconfiguration, not an authorization decision.
		return status.Error(codes.Internal, "permission check failed")
	}
	if !allowed {
		return status.Errorf(codes.PermissionDenied, "requires permission %s", permUserManage)
	}
	return nil
}

func (h *Handler) AssignRole(ctx context.Context, req *iamv1.AssignRoleRequest) (*iamv1.AssignRoleResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	roleID, err := uuid.Parse(req.GetRoleId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid role_id")
	}
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	assignedBy := caller.UserID
	if err := h.svc.AssignRole(ctx, tenantID, userID, roleID, assignedBy); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.AssignRoleResponse{}, nil
}

func (h *Handler) RevokeRole(ctx context.Context, req *iamv1.RevokeRoleRequest) (*iamv1.RevokeRoleResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	roleID, err := uuid.Parse(req.GetRoleId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid role_id")
	}
	// RevokeRoleRequest carries no tenant_id — derive it from the caller's verified
	// token.
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	if err := h.svc.RevokeRole(ctx, caller.TenantID, userID, roleID); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.RevokeRoleResponse{}, nil
}

func (h *Handler) ListRoles(ctx context.Context, req *iamv1.ListRolesRequest) (*iamv1.ListRolesResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	roles, err := h.svc.ListRoles(ctx, tenantID)
	if err != nil {
		return nil, grpcError(err)
	}
	out := make([]*iamv1.Role, len(roles))
	for i := range roles {
		out[i] = roleToProto(&roles[i])
	}
	return &iamv1.ListRolesResponse{Roles: out}, nil
}

func (h *Handler) CheckPermission(ctx context.Context, req *iamv1.CheckPermissionRequest) (*iamv1.CheckPermissionResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	var projectID *uuid.UUID
	if pid := req.GetProjectId(); pid != "" {
		parsed, err := uuid.Parse(pid)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid project_id")
		}
		projectID = &parsed
	}
	allowed, err := h.svc.CheckPermission(ctx, userID, req.GetPermission(), projectID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.CheckPermissionResponse{Allowed: allowed}, nil
}

func (h *Handler) AssignProjectRole(ctx context.Context, req *iamv1.AssignProjectRoleRequest) (*iamv1.AssignProjectRoleResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	roleID, err := uuid.Parse(req.GetRoleId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid role_id")
	}
	projectID, err := uuid.Parse(req.GetProjectId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid project_id")
	}
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	assignedBy := caller.UserID
	if err := h.svc.AssignProjectRole(ctx, tenantID, userID, roleID, projectID, assignedBy); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.AssignProjectRoleResponse{}, nil
}

// --- Tenants ---

func (h *Handler) CreateTenant(ctx context.Context, req *iamv1.CreateTenantRequest) (*iamv1.Tenant, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	tenant := &Tenant{
		Name:                req.GetName(),
		Plan:                req.GetPlan(),
		IsDemo:              req.GetIsDemo(),
		AddressLine1:        req.GetAddressLine1(),
		AddressLine2:        req.GetAddressLine2(),
		City:                req.GetCity(),
		CountryCode:         req.GetCountryCode(),
		PostalCode:          req.GetPostalCode(),
		PrimaryCurrencyCode: req.GetPrimaryCurrencyCode(),
	}
	created, err := h.svc.CreateTenant(ctx, tenant)
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(created), nil
}

// SetTenantAddress updates the company location + currency on an existing
// tenant. Restricted to callers with the tenant_admin role (or platform
// admin) — a regular user should not be able to change the currency.
func (h *Handler) SetTenantAddress(ctx context.Context, req *iamv1.SetTenantAddressRequest) (*iamv1.Tenant, error) {
	id, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	t := &Tenant{
		ID:                  id,
		AddressLine1:        req.GetAddressLine1(),
		AddressLine2:        req.GetAddressLine2(),
		City:                req.GetCity(),
		CountryCode:         req.GetCountryCode(),
		PostalCode:          req.GetPostalCode(),
		PrimaryCurrencyCode: req.GetPrimaryCurrencyCode(),
	}
	updated, err := h.svc.SetTenantAddress(ctx, t)
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(updated), nil
}

func (h *Handler) GetTenant(ctx context.Context, req *iamv1.GetTenantRequest) (*iamv1.Tenant, error) {
	// The id IS the tenant id: derive it from the caller's verified token rather
	// than the request body. #144 scoped itself to fields named tenant_id and so
	// missed this one, leaving any authenticated user able to read any tenant's
	// record by UUID (#139 slice B). TenantFromRequest returns the caller's tenant
	// when the field is empty and PermissionDenied when it differs.
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	tenant, err := h.svc.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(tenant), nil
}

func (h *Handler) SuspendTenant(ctx context.Context, req *iamv1.SuspendTenantRequest) (*iamv1.Tenant, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}

	// Optional legal-hold parameters (#92 Stage 4). Use the proto3 field
	// presence (the raw pointer fields, not the Get* accessors) so that
	// an unset hold_until / freeze_reason plumbs through as a nil pointer
	// and the repo COALESCEs to preserve any existing hold.
	var holdUntil *time.Time
	if t := req.HoldUntil; t != nil {
		v := t.AsTime()
		holdUntil = &v
	}
	freezeReason := req.FreezeReason // already *string for optional string fields

	tenant, err := h.svc.SuspendTenant(ctx, id, holdUntil, freezeReason)
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(tenant), nil
}

func (h *Handler) ClearTenantLegalHold(ctx context.Context, req *iamv1.ClearTenantLegalHoldRequest) (*iamv1.Tenant, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	// req.GetReason() returns "" when the optional field is unset,
	// which the service maps to empty audit metadata — no special
	// casing needed in the handler.
	tenant, err := h.svc.ClearTenantLegalHold(ctx, id, req.GetReason())
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(tenant), nil
}

func (h *Handler) SetTenantRetention(ctx context.Context, req *iamv1.SetTenantRetentionRequest) (*iamv1.Tenant, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	if strings.TrimSpace(req.GetFreezeReason()) == "" {
		return nil, status.Error(codes.InvalidArgument, "freeze_reason is required")
	}
	var holdUntil *time.Time
	if t := req.HoldUntil; t != nil { // raw field: preserve proto3 presence
		v := t.AsTime()
		holdUntil = &v
	}
	tenant, err := h.svc.SetTenantRetention(ctx, id, holdUntil, req.GetFreezeReason(), req.GetOverwrite())
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(tenant), nil
}

// --- PurgeTenant two-person-approval requests (#92 Stage 3) ---

func (h *Handler) RequestTenantPurge(ctx context.Context, req *iamv1.RequestTenantPurgeRequest) (*iamv1.TenantPurgeRequest, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	pr, err := h.svc.RequestTenantPurge(ctx, id, req.GetReason())
	if err != nil {
		return nil, grpcError(err)
	}
	return purgeRequestToProto(pr), nil
}

func (h *Handler) ApproveTenantPurge(ctx context.Context, req *iamv1.ApproveTenantPurgeRequest) (*iamv1.TenantPurgeRequest, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	pr, err := h.svc.ApproveTenantPurge(ctx, id, req.GetReason())
	if err != nil {
		return nil, grpcError(err)
	}
	return purgeRequestToProto(pr), nil
}

func (h *Handler) CancelTenantPurge(ctx context.Context, req *iamv1.CancelTenantPurgeRequest) (*iamv1.TenantPurgeRequest, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	pr, err := h.svc.CancelTenantPurge(ctx, id, req.GetReason())
	if err != nil {
		return nil, grpcError(err)
	}
	return purgeRequestToProto(pr), nil
}

// --- Invitations ---

func (h *Handler) InviteUser(ctx context.Context, req *iamv1.InviteUserRequest) (*iamv1.Invitation, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	var roleID *uuid.UUID
	if roleIDStr := req.GetRoleId(); roleIDStr != "" {
		parsed, err := uuid.Parse(roleIDStr)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid role_id")
		}
		roleID = &parsed
	}
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	inv := &Invitation{
		TenantID:  tenantID,
		Email:     req.GetEmail(),
		InvitedBy: caller.UserID,
		RoleID:    roleID,
	}
	created, err := h.svc.InviteUser(ctx, inv)
	if err != nil {
		return nil, grpcError(err)
	}
	return invitationToProto(created), nil
}

func (h *Handler) AcceptInvitation(ctx context.Context, req *iamv1.AcceptInvitationRequest) (*iamv1.TokenPair, error) {
	pair, err := h.svc.AcceptInvitation(ctx, req.GetToken(), req.GetPassword())
	if err != nil {
		return nil, grpcError(err)
	}
	return tokenPairToProto(pair), nil
}

// --- Impersonation ---

func (h *Handler) StartImpersonation(ctx context.Context, req *iamv1.StartImpersonationRequest) (*iamv1.ImpersonationSession, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	platformUserID, err := uuid.Parse(req.GetPlatformUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid platform_user_id")
	}
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	impersonatedUser, err := uuid.Parse(req.GetImpersonatedUser())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid impersonated_user")
	}
	if req.GetReason() == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}

	dur := time.Duration(req.GetDurationSeconds()) * time.Second

	session, err := h.svc.StartImpersonation(ctx, StartImpersonationParams{
		PlatformUserID:   platformUserID,
		TenantID:         tenantID,
		ImpersonatedUser: impersonatedUser,
		Reason:           req.GetReason(),
		Duration:         dur,
		IPAddress:        req.GetIpAddress(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return impersonationSessionToProto(session), nil
}

func (h *Handler) EndImpersonation(ctx context.Context, req *iamv1.EndImpersonationRequest) (*iamv1.EndImpersonationResponse, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid session_id")
	}
	caller, _ := interceptor.CallerFromContext(ctx)
	if err := h.svc.EndImpersonation(ctx, sessionID, caller.UserID); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.EndImpersonationResponse{}, nil
}

// --- OIDC configuration ---

func (h *Handler) SetOIDCConfig(ctx context.Context, req *iamv1.SetOIDCConfigRequest) (*iamv1.SetOIDCConfigResponse, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	params := OIDCConfigParams{
		TenantID:         req.GetTenantId(),
		IssuerURL:        req.GetIssuerUrl(),
		ClientID:         req.GetClientId(),
		ClientSecretEnc:  req.GetClientSecret(), // plaintext; service encrypts
		Scopes:           req.GetScopes(),
		EmailClaim:       req.GetEmailClaim(),
		DisplayNameClaim: req.GetDisplayNameClaim(),
		GroupsClaim:      req.GetGroupsClaim(),
		AutoProvision:    req.GetAutoProvision(),
		DefaultRole:      req.GetDefaultRole(),
	}
	if err := h.svc.SetOIDCConfig(ctx, params); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.SetOIDCConfigResponse{}, nil
}

// --- Mapping helpers ---

func userToProto(u *User) *iamv1.User {
	return &iamv1.User{
		Id:          u.ID.String(),
		TenantId:    u.TenantID.String(),
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Status:      u.Status,
		CreatedAt:   timestamppb.New(u.CreatedAt),
	}
}

// tenantToProto builds the gRPC Tenant message from the domain model,
// including #61 address + currency fields.
func tenantToProto(t *Tenant) *iamv1.Tenant {
	return &iamv1.Tenant{
		Id:                  t.ID.String(),
		Name:                t.Name,
		Slug:                t.Slug,
		Plan:                t.Plan,
		Status:              t.Status,
		IsDemo:              t.IsDemo,
		CreatedAt:           timestamppb.New(t.CreatedAt),
		AddressLine1:        t.AddressLine1,
		AddressLine2:        t.AddressLine2,
		City:                t.City,
		CountryCode:         t.CountryCode,
		PostalCode:          t.PostalCode,
		PrimaryCurrencyCode: t.PrimaryCurrencyCode,
	}
}

// purgeRequestToProto builds the gRPC TenantPurgeRequest message from the
// domain model (#92 Stage 3). uuid.UUID pointers become "" when nil;
// *time.Time fields become nil timestamppb.Timestamps (omitted) when nil.
func purgeRequestToProto(pr *TenantPurgeRequest) *iamv1.TenantPurgeRequest {
	pb := &iamv1.TenantPurgeRequest{
		Id:            pr.ID.String(),
		TenantId:      pr.TenantID.String(),
		Status:        pr.Status,
		RequestedBy:   pr.RequestedBy.String(),
		RequestedAt:   timestamppb.New(pr.RequestedAt),
		RequestReason: pr.RequestReason,
	}
	if pr.ApprovedBy != nil {
		pb.ApprovedBy = pr.ApprovedBy.String()
	}
	if pr.ApprovedAt != nil {
		pb.ApprovedAt = timestamppb.New(*pr.ApprovedAt)
	}
	if pr.ExecutedAt != nil {
		pb.ExecutedAt = timestamppb.New(*pr.ExecutedAt)
	}
	if pr.FailureReason != nil {
		pb.FailureReason = *pr.FailureReason
	}
	return pb
}

func roleToProto(r *Role) *iamv1.Role {
	return &iamv1.Role{
		Id:          r.ID.String(),
		TenantId:    r.TenantID.String(),
		Name:        r.Name,
		Permissions: r.Permissions,
		IsSystem:    r.IsSystem,
	}
}

func invitationToProto(inv *Invitation) *iamv1.Invitation {
	pb := &iamv1.Invitation{
		Id:        inv.ID.String(),
		TenantId:  inv.TenantID.String(),
		Email:     inv.Email,
		Status:    inv.Status,
		InvitedBy: inv.InvitedBy.String(),
		ExpiresAt: timestamppb.New(inv.ExpiresAt),
		CreatedAt: timestamppb.New(inv.CreatedAt),
	}
	if inv.RoleID != nil {
		pb.RoleId = inv.RoleID.String()
	}
	return pb
}

func impersonationSessionToProto(s *ImpersonationSession) *iamv1.ImpersonationSession {
	pb := &iamv1.ImpersonationSession{
		Id:               s.ID.String(),
		PlatformUserId:   s.PlatformUserID.String(),
		TenantId:         s.TenantID.String(),
		ImpersonatedUser: s.ImpersonatedUser.String(),
		Reason:           s.Reason,
		StartedAt:        timestamppb.New(s.StartedAt),
		ExpiresAt:        timestamppb.New(s.ExpiresAt),
		IpAddress:        s.IPAddress,
	}
	if s.EndedAt != nil {
		pb.EndedAt = timestamppb.New(*s.EndedAt)
	}
	return pb
}

func tokenPairToProto(p *auth.TokenPair) *iamv1.TokenPair {
	return &iamv1.TokenPair{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		TokenType:    p.TokenType,
		ExpiresIn:    int32(p.ExpiresIn),
		ExpiresAt:    timestamppb.New(p.ExpiresAt),
	}
}

func claimsToProto(c *auth.Claims) *iamv1.Claims {
	return &iamv1.Claims{
		Subject:     c.Subject.String(),
		TenantId:    c.TenantID.String(),
		Email:       c.Email,
		Roles:       c.Roles,
		Permissions: c.Permissions,
		AuthMethod:  string(c.AuthMethod),
		IssuedAt:    timestamppb.New(c.IssuedAt),
		ExpiresAt:   timestamppb.New(c.ExpiresAt),
	}
}

// grpcError maps IAM and auth domain errors to gRPC status codes.
// Internal errors are intentionally opaque — callers receive "internal error",
// not the underlying message, to prevent information leakage.
func grpcError(err error) error {
	switch {
	case errors.Is(err, ErrUserNotFound),
		errors.Is(err, ErrTenantNotFound),
		errors.Is(err, ErrRoleNotFound),
		errors.Is(err, ErrInvitationNotFound),
		errors.Is(err, ErrImpersonationNotFound):
		return status.Error(codes.NotFound, err.Error())

	case errors.Is(err, ErrImpersonationAlreadyEnded):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, ErrUserAlreadyExists),
		errors.Is(err, ErrTenantSlugTaken),
		errors.Is(err, ErrTenantNameTaken):
		return status.Error(codes.AlreadyExists, err.Error())

	case errors.Is(err, ErrInvitationExpired),
		errors.Is(err, ErrInvitationAccepted):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, ErrInvalidPlan),
		errors.Is(err, ErrRoleNotProjectScoped),
		errors.Is(err, ErrHoldUntilInPast):
		return status.Error(codes.InvalidArgument, err.Error())

	case errors.Is(err, ErrTenantNotHoldable),
		errors.Is(err, ErrTenantHoldExists),
		errors.Is(err, ErrHoldNarrowsIndefinite):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, ErrTenantNotPurgeable),
		errors.Is(err, ErrSelfApproval):
		return status.Error(codes.FailedPrecondition, err.Error())

	case errors.Is(err, ErrPurgeRequestExists):
		return status.Error(codes.AlreadyExists, err.Error())

	case errors.Is(err, ErrPurgeRequestNotFound):
		return status.Error(codes.NotFound, err.Error())

	case errors.Is(err, auth.ErrInvalidCredentials),
		errors.Is(err, auth.ErrTokenExpired),
		errors.Is(err, auth.ErrTokenInvalid),
		errors.Is(err, auth.ErrRefreshTokenNotFound):
		return status.Error(codes.Unauthenticated, err.Error())

	case errors.Is(err, auth.ErrTenantSuspended),
		errors.Is(err, auth.ErrAccountDeactivated):
		return status.Error(codes.PermissionDenied, err.Error())

	case errors.Is(err, auth.ErrAccountInvited):
		return status.Error(codes.FailedPrecondition, err.Error())

	default:
		return status.Error(codes.Internal, "internal error")
	}
}
