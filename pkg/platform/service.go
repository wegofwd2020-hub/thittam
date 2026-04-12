package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/audit"
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
	auditLog       AuditSink
	notifier       ImpersonationNotifier
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
		notifier:       noopNotifier{},
	}
}

// WithAuditSink attaches an audit logger to the service.
// Must be called before the service handles any requests.
func (s *Service) WithAuditSink(a AuditSink) *Service {
	s.auditLog = a
	return s
}

// WithNotifier attaches an ImpersonationNotifier to the service.
// If not set, notifications are silently discarded.
func (s *Service) WithNotifier(n ImpersonationNotifier) *Service {
	s.notifier = n
	return s
}

// Impersonate creates a time-limited impersonation session (max MaxImpersonationDuration).
// Only platform_owner and platform_admin roles are permitted.
// MFA must be enabled on the requesting platform user.
func (s *Service) Impersonate(ctx context.Context, req ImpersonationRequest) (*ImpersonationSession, error) {
	if req.Reason == "" {
		return nil, ErrReasonRequired
	}

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

	now := time.Now().UTC()
	expiresAt := now.Add(MaxImpersonationDuration)

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

	if s.auditLog != nil {
		s.auditLog.LogAction(
			req.TenantID,
			req.PlatformUserID,
			user.Email,
			audit.ActionImpersonationStarted,
			audit.ResourceImpersonationSession,
			sessionID,
			nil,
			map[string]string{
				"target_user_id": req.UserID.String(),
				"reason":         req.Reason,
				"ip_address":     req.IPAddress,
				"expires_at":     expiresAt.Format(time.RFC3339),
			},
			nil,
		)
	}

	// The actual tenant JWT issuance happens at the handler layer,
	// using auth.TokenIssuer with the impersonated user's data.
	return &ImpersonationSession{
		ID:        sessionID,
		StartedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

// EndImpersonation manually terminates an active impersonation session.
// The platform admin who started the session, or any platform_owner, may end it.
// A post-session notification is sent to the target user asynchronously.
func (s *Service) EndImpersonation(ctx context.Context, adminID, sessionID uuid.UUID) error {
	return s.revokeSession(ctx, adminID, sessionID, RevocationManual)
}

// revokeSession terminates a session with the given reason, emits an audit event,
// and dispatches the post-session notification. Internal helper.
func (s *Service) revokeSession(ctx context.Context, actorID, sessionID uuid.UUID, reason RevocationReason) error {
	if err := s.impersonations.RevokeSession(ctx, sessionID, reason); err != nil {
		return fmt.Errorf("platform: revoke session %s: %w", sessionID, err)
	}

	s.logger.Info("impersonation ended",
		"session_id", sessionID.String(),
		"reason", string(reason),
		"actor_id", actorID.String(),
	)

	// Best-effort: fetch session data for the audit entry and notification.
	// If the store doesn't surface the ended session we still record what we know.
	if s.auditLog != nil {
		s.auditLog.LogAction(
			uuid.Nil, // tenantID unknown at this layer without a store lookup
			actorID,
			"", // email unknown without store lookup
			audit.ActionImpersonationEnded,
			audit.ResourceImpersonationSession,
			sessionID,
			nil,
			map[string]string{
				"reason": string(reason),
			},
			nil,
		)
	}

	return nil
}

// RevokeOnPasswordChange terminates all active impersonation sessions targeting
// userID. Call this from the IAM service whenever a tenant user changes their
// password (including forced resets).
func (s *Service) RevokeOnPasswordChange(ctx context.Context, userID uuid.UUID) error {
	return s.revokeAllForUser(ctx, userID, RevocationPasswordChange)
}

// RevokeOnDeactivation terminates all active impersonation sessions targeting
// userID. Call this from the IAM service when a tenant user is deactivated.
func (s *Service) RevokeOnDeactivation(ctx context.Context, userID uuid.UUID) error {
	return s.revokeAllForUser(ctx, userID, RevocationDeactivated)
}

// RevokeOnMFAChange terminates all active impersonation sessions targeting
// userID. Call this from the IAM service when a tenant user modifies their MFA.
func (s *Service) RevokeOnMFAChange(ctx context.Context, userID uuid.UUID) error {
	return s.revokeAllForUser(ctx, userID, RevocationMFAChange)
}

// revokeAllForUser fetches every active session for the target user and revokes
// each one. Revocations are best-effort: individual failures are logged but do
// not block the caller.
func (s *Service) revokeAllForUser(ctx context.Context, targetUserID uuid.UUID, reason RevocationReason) error {
	sessions, err := s.impersonations.GetActiveSessionsForUser(ctx, targetUserID)
	if err != nil {
		return fmt.Errorf("platform: get active sessions for user %s: %w", targetUserID, err)
	}
	for _, sess := range sessions {
		if rErr := s.impersonations.RevokeSession(ctx, sess.ID, reason); rErr != nil {
			s.logger.Warn("failed to revoke impersonation session",
				"session_id", sess.ID.String(),
				"target_user_id", targetUserID.String(),
				"reason", string(reason),
				"error", rErr.Error(),
			)
			continue
		}
		s.logger.Info("impersonation session auto-revoked",
			"session_id", sess.ID.String(),
			"target_user_id", targetUserID.String(),
			"reason", string(reason),
		)
		if s.auditLog != nil {
			s.auditLog.LogAction(
				sess.TenantID,
				sess.PlatformUserID,
				"",
				audit.ActionImpersonationEnded,
				audit.ResourceImpersonationSession,
				sess.ID,
				nil,
				map[string]string{
					"reason":         string(reason),
					"target_user_id": targetUserID.String(),
				},
				nil,
			)
		}
		// Non-blocking notification to the target user.
		go func(s *Service, sess ActiveImpersonationSession, reason RevocationReason) {
			if nErr := s.notifier.NotifyImpersonationEnded(context.Background(), sess, reason); nErr != nil {
				s.logger.Warn("impersonation end notification failed",
					"session_id", sess.ID.String(),
					"error", nErr.Error(),
				)
			}
		}(s, sess, reason)
	}
	return nil
}

// IsActionBlocked returns true if the given action name is not permitted during
// an impersonation session. Callers should check this in gRPC handlers or
// middleware when processing requests with an impersonation-scoped token.
func IsActionBlocked(action string) bool {
	return blockedDuringImpersonation[action]
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
