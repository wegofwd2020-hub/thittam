package platform

import (
	"context"
	"fmt"

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
	users     UserStore
	tenants   TenantManager
	verticals VerticalManager
	logger    Logger
	auditLog  AuditSink
}

// NewService creates a platform administration service.
func NewService(
	users UserStore,
	tenants TenantManager,
	verticals VerticalManager,
	logger Logger,
) *Service {
	return &Service{
		users:     users,
		tenants:   tenants,
		verticals: verticals,
		logger:    logger,
	}
}

// WithAuditSink attaches an audit logger to the service.
// Must be called before the service handles any requests.
func (s *Service) WithAuditSink(a AuditSink) *Service {
	s.auditLog = a
	return s
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
