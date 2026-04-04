package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Logger defines structured logging for the platform package.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// Service implements platform administration operations.
type Service struct {
	users          UserStore
	tenants        TenantManager
	impersonations ImpersonationStore
	verticals      VerticalManager
	logger         Logger
}

// NewService creates a platform administration service.
func NewService(
	users UserStore,
	tenants TenantManager,
	impersonations ImpersonationStore,
	verticals VerticalManager,
	logger Logger,
) *Service {
	return &Service{
		users:          users,
		tenants:        tenants,
		impersonations: impersonations,
		verticals:      verticals,
		logger:         logger,
	}
}

// Impersonate creates a time-limited impersonation session.
// Only platform_owner and platform_admin roles are allowed.
func (s *Service) Impersonate(ctx context.Context, req ImpersonationRequest) (*ImpersonationSession, error) {
	// Validate the request
	if req.Reason == "" {
		return nil, ErrReasonRequired
	}

	// Verify the platform user exists and has the right role
	user, err := s.users.GetUserByID(ctx, req.PlatformUserID)
	if err != nil {
		return nil, fmt.Errorf("platform: get user: %w", err)
	}

	if user.Role != RoleOwner && user.Role != RoleAdmin {
		return nil, ErrImpersonationDenied
	}

	if !user.MFAEnabled {
		return nil, ErrMFARequired
	}

	// Log the impersonation
	expiresAt := time.Now().Add(30 * time.Minute)
	sessionID, err := s.impersonations.LogImpersonation(ctx, req, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("platform: log impersonation: %w", err)
	}

	s.logger.Info("impersonation started",
		"platform_user_id", req.PlatformUserID.String(),
		"tenant_id", req.TenantID.String(),
		"impersonated_user", req.UserID.String(),
		"reason", req.Reason,
		"expires_at", expiresAt.Format(time.RFC3339),
	)

	// The actual tenant JWT issuance happens at the handler level,
	// using the auth.TokenIssuer with the impersonated user's data.
	return &ImpersonationSession{
		ID:        sessionID,
		ExpiresAt: expiresAt,
	}, nil
}

// CheckAccess verifies that a platform user has the required role.
func CheckAccess(user *PlatformUser, requiredRole Role) error {
	hierarchy := map[Role]int{
		RoleOwner:   3,
		RoleAdmin:   2,
		RoleSupport: 1,
	}

	userLevel, ok := hierarchy[user.Role]
	if !ok {
		return ErrNotPlatformUser
	}
	requiredLevel, ok := hierarchy[requiredRole]
	if !ok {
		return ErrInsufficientRole
	}

	if userLevel < requiredLevel {
		return ErrInsufficientRole
	}
	return nil
}

// SeedPlatformOwner creates the initial platform owner account.
// Called during platform bootstrap (first run).
func (s *Service) SeedPlatformOwner(ctx context.Context, email, displayName, passwordHash string) (uuid.UUID, error) {
	id, err := s.users.CreateUser(ctx, email, displayName, passwordHash, RoleOwner, uuid.Nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("platform: seed owner: %w", err)
	}

	s.logger.Info("platform owner created",
		"user_id", id.String(),
		"email", email,
	)

	return id, nil
}
