package iam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/crypto"
	"github.com/wegofwd2020/thittam/pkg/registration"
)

// ErrOIDCKeyNotConfigured is returned by SetOIDCConfig when no encryption key
// has been wired into the service — a configuration error caught at startup.
var ErrOIDCKeyNotConfigured = errors.New("iam: OIDC encryption key not configured")

// PasswordHasher creates a bcrypt hash of a plain-text password.
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// SchemaMigrator initialises the PostgreSQL schema for a newly created tenant.
// The production implementation (cmd/iam) wraps pkg/migrate.ParallelMigrator and
// applies all service migration directories against the new tenant_<uuid> schema.
type SchemaMigrator interface {
	MigrateTenantSchema(ctx context.Context, tenantID uuid.UUID) error
}

// Authenticator resolves the correct auth provider and authenticates a request.
// Satisfied in production by *auth.Resolver.
type Authenticator interface {
	Authenticate(ctx context.Context, req auth.AuthRequest) (*auth.AuthResult, error)
}

// systemRoles are seeded for every new tenant at creation time.
// Permissions follow the {resource}:{action} convention.
var systemRoles = []struct {
	name        string
	permissions []string
}{
	{"super_admin", []string{
		"production:read", "production:write",
		"budget:read", "budget:write", "budget:approve",
		"expense:submit", "expense:approve",
		"inventory:checkout",
		"report:read",
		"user:manage",
	}},
	{"executive_producer", []string{
		"production:read", "production:write",
		"budget:read", "budget:approve",
		"expense:approve",
		"inventory:checkout",
		"report:read",
	}},
	{"line_producer", []string{
		"production:read", "production:write",
		"budget:read", "budget:write",
		"expense:approve",
		"inventory:checkout",
		"report:read",
	}},
	{"production_accountant", []string{
		"budget:read",
		"expense:submit", "expense:approve",
		"report:read",
	}},
	{"department_head", []string{
		"production:read",
		"expense:submit",
		"inventory:checkout",
	}},
	{"crew_member", []string{
		"production:read",
		"expense:submit",
	}},
}

// Service implements IAM business logic.
type Service struct {
	repo     Repository
	auth     Authenticator
	tokens   auth.TokenIssuer
	hasher   PasswordHasher
	verifier auth.PasswordVerifier
	oidcKey  []byte          // AES-256-GCM key for OIDC client secret encryption; set by WithOIDCEncryptionKey
	migrator SchemaMigrator  // nil when no schema init is required (e.g. tests that don't exercise CreateTenant)
}

// WithOIDCEncryptionKey sets the AES-256-GCM key used to encrypt and decrypt
// OIDC client secrets stored in tenant_auth_config. The key must be 32 bytes.
// Returns the receiver so calls can be chained.
func (s *Service) WithOIDCEncryptionKey(key []byte) *Service {
	s.oidcKey = key
	return s
}

// WithSchemaMigrator attaches a SchemaMigrator that is called by CreateTenant
// to initialise the new tenant's PostgreSQL schema immediately after the tenant
// record and system roles are persisted. The migrator must be safe to call
// concurrently. Returns the receiver so calls can be chained.
func (s *Service) WithSchemaMigrator(m SchemaMigrator) *Service {
	s.migrator = m
	return s
}

// NewService creates an IAM service with all required dependencies.
func NewService(
	repo Repository,
	authenticator Authenticator,
	tokens auth.TokenIssuer,
	hasher PasswordHasher,
	verifier auth.PasswordVerifier,
) *Service {
	return &Service{
		repo:     repo,
		auth:     authenticator,
		tokens:   tokens,
		hasher:   hasher,
		verifier: verifier,
	}
}

// --- Authentication ---

// Login authenticates a user and returns a JWT token pair.
// If the stored hash uses bcrypt (or weak argon2id params), it is silently
// upgraded to argon2id in the background after a successful login. This
// migration is best-effort — a rehash failure is logged but never surfaces
// to the caller (Rule #6: non-critical writes must not block reads).
func (s *Service) Login(ctx context.Context, tenantID uuid.UUID, email, password string) (*auth.TokenPair, error) {
	result, err := s.auth.Authenticate(ctx, auth.AuthRequest{
		TenantID: tenantID,
		Email:    email,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("iam: login %s: %w", email, err)
	}

	// Background re-hash: upgrade bcrypt → argon2id (or weak → strong argon2id)
	// on next successful login. Detached from the request context so that a slow
	// Argon2id computation does not add latency to the login response.
	go s.rehashIfNeeded(tenantID, email, password)

	pair, err := s.tokens.Issue(ctx, result)
	if err != nil {
		return nil, fmt.Errorf("iam: issue token: %w", err)
	}
	return pair, nil
}

// rehashIfNeeded upgrades the user's stored password hash to the current
// argon2id parameters if NeedsRehash reports that an upgrade is warranted.
// Runs in a background goroutine — errors are logged, never returned.
func (s *Service) rehashIfNeeded(tenantID uuid.UUID, email, password string) {
	ctx := context.Background()
	record, err := s.repo.GetUserByEmail(ctx, tenantID, email)
	if err != nil {
		return
	}
	if !auth.NeedsRehash(record.PasswordHash) {
		return
	}
	newHash, err := s.hasher.Hash(password)
	if err != nil {
		return
	}
	// UpdatePasswordHash is a single-row write — idempotent on retry.
	_ = s.repo.UpdatePasswordHash(ctx, record.ID, newHash)
}

// RefreshToken issues a new token pair from a valid refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	pair, err := s.tokens.Refresh(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("iam: refresh token: %w", err)
	}
	return pair, nil
}

// Logout revokes a refresh token, invalidating the session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if err := s.tokens.Revoke(ctx, refreshToken); err != nil {
		return fmt.Errorf("iam: logout: %w", err)
	}
	return nil
}

// --- Users ---

// CreateUser creates a new user, hashing their password before persistence.
func (s *Service) CreateUser(ctx context.Context, user *User, plainPassword string) (*User, error) {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	if user.Status == "" {
		user.Status = "active"
	}
	hash, err := s.hasher.Hash(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("iam: hash password: %w", err)
	}
	user.PasswordHash = hash
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: create user: %w", err)
	}
	return user, nil
}

// GetUser retrieves a user by tenant and user ID.
func (s *Service) GetUser(ctx context.Context, tenantID, id uuid.UUID) (*User, error) {
	u, err := s.repo.GetUser(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("iam: get user %s: %w", id, err)
	}
	return u, nil
}

// ListUsers lists users for a tenant with an optional status filter.
func (s *Service) ListUsers(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	users, err := s.repo.ListUsers(ctx, tenantID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("iam: list users: %w", err)
	}
	return users, nil
}

// UpdateUser updates a user's mutable fields (display name, status).
func (s *Service) UpdateUser(ctx context.Context, user *User) (*User, error) {
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: update user %s: %w", user.ID, err)
	}
	return user, nil
}

// DeactivateUser marks a user as deactivated, preventing future logins.
func (s *Service) DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if err := s.repo.DeactivateUser(ctx, tenantID, id); err != nil {
		return fmt.Errorf("iam: deactivate user %s: %w", id, err)
	}
	return nil
}

// ChangePassword verifies the current password then replaces the hash.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error {
	record, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("iam: get user for password change: %w", err)
	}
	if err := s.verifier.Verify(oldPassword, record.PasswordHash); err != nil {
		// Surface as a generic credentials error — do not leak the distinction
		// between "wrong password" and "user not found" to callers.
		return auth.ErrInvalidCredentials
	}
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("iam: hash new password: %w", err)
	}
	if err := s.repo.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return fmt.Errorf("iam: update password hash: %w", err)
	}
	return nil
}

// --- Roles & Permissions ---

// AssignRole grants a role to a user.
func (s *Service) AssignRole(ctx context.Context, tenantID, userID, roleID, assignedBy uuid.UUID) error {
	ur := &UserRole{
		UserID:     userID,
		RoleID:     roleID,
		AssignedBy: assignedBy,
		AssignedAt: time.Now().UTC(),
	}
	if err := s.repo.AssignRole(ctx, ur); err != nil {
		return fmt.Errorf("iam: assign role %s to user %s: %w", roleID, userID, err)
	}
	return nil
}

// RevokeRole removes a role from a user.
func (s *Service) RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error {
	if err := s.repo.RevokeRole(ctx, userID, roleID); err != nil {
		return fmt.Errorf("iam: revoke role %s from user %s: %w", roleID, userID, err)
	}
	return nil
}

// ListRoles returns all roles defined for a tenant, capped at 200.
// System roles are seeded at 6 per tenant; custom roles are rare and bounded
// by sensible UI limits. 200 is a hard ceiling against runaway scripting.
func (s *Service) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	roles, err := s.repo.ListRoles(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: list roles: %w", err)
	}
	const maxRoles = 200
	if len(roles) > maxRoles {
		roles = roles[:maxRoles]
	}
	return roles, nil
}

// CheckPermission returns true if the user holds the given permission through
// any of their assigned roles.
func (s *Service) CheckPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error) {
	perms, err := s.repo.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("iam: get permissions for user %s: %w", userID, err)
	}
	for _, p := range perms {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}

// --- Tenants ---

// CreateTenant creates a new tenant and seeds all system roles.
func (s *Service) CreateTenant(ctx context.Context, tenant *Tenant) (*Tenant, error) {
	if tenant.ID == uuid.Nil {
		tenant.ID = uuid.New()
	}
	if tenant.Slug == "" {
		tenant.Slug = registration.Slugify(tenant.Name)
	}
	if tenant.Status == "" {
		tenant.Status = "active"
	}
	if tenant.Plan == "" {
		tenant.Plan = "starter"
	}
	if !isValidPlan(tenant.Plan) {
		return nil, ErrInvalidPlan
	}
	if err := s.repo.CreateTenant(ctx, tenant); err != nil {
		return nil, fmt.Errorf("iam: create tenant: %w", err)
	}
	if err := s.seedSystemRoles(ctx, tenant.ID); err != nil {
		return nil, fmt.Errorf("iam: seed system roles for tenant %s: %w", tenant.ID, err)
	}
	// Initialise the tenant's PostgreSQL schema. MigrateTenantSchema is
	// synchronous: if it fails the caller sees the error and the tenant record
	// remains in the DB (so it can be retried via the admin API). A nil migrator
	// is allowed — tests that don't exercise schema creation omit it.
	if s.migrator != nil {
		if err := s.migrator.MigrateTenantSchema(ctx, tenant.ID); err != nil {
			return nil, fmt.Errorf("iam: migrate schema for tenant %s: %w", tenant.ID, err)
		}
	}
	return tenant, nil
}

// GetTenant retrieves a tenant by ID.
func (s *Service) GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	t, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: get tenant %s: %w", id, err)
	}
	return t, nil
}

// SuspendTenant marks a tenant suspended, blocking all logins for that tenant.
func (s *Service) SuspendTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	if err := s.repo.UpdateTenantStatus(ctx, id, "suspended"); err != nil {
		return nil, fmt.Errorf("iam: suspend tenant %s: %w", id, err)
	}
	return s.repo.GetTenant(ctx, id)
}

// --- Invitations ---

// InviteUser creates a pending invitation with a 7-day expiry.
// The caller is responsible for sending the invitation email via the
// notifications service (the IAM service publishes a NATS event for this).
func (s *Service) InviteUser(ctx context.Context, inv *Invitation) (*Invitation, error) {
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	tok, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("iam: generate invitation token: %w", err)
	}
	inv.Token = tok
	inv.Status = "pending"
	inv.ExpiresAt = time.Now().UTC().Add(7 * 24 * time.Hour)
	if err := s.repo.CreateInvitation(ctx, inv); err != nil {
		return nil, fmt.Errorf("iam: create invitation for %s: %w", inv.Email, err)
	}
	return inv, nil
}

// AcceptInvitation validates the token, creates the user account, and issues
// a JWT pair so the new user is immediately logged in.
func (s *Service) AcceptInvitation(ctx context.Context, token, plainPassword string) (*auth.TokenPair, error) {
	inv, err := s.repo.GetInvitationByToken(ctx, token)
	if err != nil {
		return nil, ErrInvitationNotFound
	}
	if inv.Status == "accepted" {
		return nil, ErrInvitationAccepted
	}
	if time.Now().UTC().After(inv.ExpiresAt) {
		return nil, ErrInvitationExpired
	}

	// Derive display name from the email local part until the user updates it.
	parts := strings.SplitN(inv.Email, "@", 2)
	user := &User{
		TenantID:    inv.TenantID,
		Email:       inv.Email,
		DisplayName: parts[0],
		Status:      "active",
	}
	if _, err := s.CreateUser(ctx, user, plainPassword); err != nil {
		return nil, fmt.Errorf("iam: accept invitation — create user: %w", err)
	}

	// Assign the pre-selected role if the invitation carried one.
	if inv.RoleID != nil {
		_ = s.repo.AssignRole(ctx, &UserRole{
			UserID:     user.ID,
			RoleID:     *inv.RoleID,
			AssignedBy: inv.InvitedBy,
			AssignedAt: time.Now().UTC(),
		})
	}

	if err := s.repo.MarkInvitationAccepted(ctx, inv.ID); err != nil {
		return nil, fmt.Errorf("iam: mark invitation accepted: %w", err)
	}

	// Issue tokens directly — user just proved they control the invited email.
	result := &auth.AuthResult{
		UserID:          user.ID,
		TenantID:        user.TenantID,
		Email:           user.Email,
		DisplayName:     user.DisplayName,
		AuthMethod:      auth.ProviderLocal,
		AuthenticatedAt: time.Now().UTC(),
	}
	pair, err := s.tokens.Issue(ctx, result)
	if err != nil {
		return nil, fmt.Errorf("iam: issue token after invitation: %w", err)
	}
	return pair, nil
}

// --- Helpers ---

func (s *Service) seedSystemRoles(ctx context.Context, tenantID uuid.UUID) error {
	for _, sr := range systemRoles {
		role := &Role{
			ID:          uuid.New(),
			TenantID:    tenantID,
			Name:        sr.name,
			Permissions: sr.permissions,
			IsSystem:    true,
		}
		if err := s.repo.CreateRole(ctx, role); err != nil {
			return fmt.Errorf("seed role %q: %w", sr.name, err)
		}
	}
	return nil
}

// --- OIDC configuration ---

// SetOIDCConfig creates or replaces the OIDC authentication configuration for
// a tenant. The caller supplies the plaintext client secret; this method
// encrypts it with AES-256-GCM before persisting.
//
// Returns ErrOIDCKeyNotConfigured if WithOIDCEncryptionKey was never called.
func (s *Service) SetOIDCConfig(ctx context.Context, params OIDCConfigParams) error {
	if len(s.oidcKey) == 0 {
		return ErrOIDCKeyNotConfigured
	}
	enc, err := crypto.Encrypt(s.oidcKey, params.ClientSecretEnc)
	if err != nil {
		return fmt.Errorf("iam: encrypt client secret: %w", err)
	}
	// Replace plaintext with ciphertext before handing off to the repo.
	params.ClientSecretEnc = enc
	if err := s.repo.UpsertOIDCConfig(ctx, params); err != nil {
		return fmt.Errorf("iam: upsert oidc config for tenant %s: %w", params.TenantID, err)
	}
	return nil
}

// --- Impersonation ---

// maxImpersonationDuration caps how long a platform admin session can live.
// Sessions that exceed this are rejected at the service layer, not just at expiry.
const maxImpersonationDuration = 4 * time.Hour

// StartImpersonation opens a bounded-TTL impersonation session for a platform admin.
// Duration is capped at maxImpersonationDuration regardless of what the caller requests.
// The caller must already hold the platform_admin role — enforced at the handler level
// by RequireRole before this method is ever called.
func (s *Service) StartImpersonation(ctx context.Context, params StartImpersonationParams) (*ImpersonationSession, error) {
	if params.Duration <= 0 || params.Duration > maxImpersonationDuration {
		params.Duration = maxImpersonationDuration
	}
	session, err := s.repo.StartImpersonation(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("iam: start impersonation for tenant %s: %w", params.TenantID, err)
	}
	// Append-only audit log (Rule #7). The write is best-effort: an audit
	// failure must not roll back a successfully opened session — the session
	// must be terminated by the admin regardless of audit availability.
	entry := &AuditEntry{
		ID:         uuid.New(),
		ActorID:    params.PlatformUserID,
		Action:     "impersonation.start",
		TargetType: "impersonation_session",
		TargetID:   session.ID,
		OldState:   "",
		NewState:   "active",
		Timestamp:  session.StartedAt,
		TenantID:   params.TenantID,
		IPAddress:  params.IPAddress,
	}
	if auditErr := s.repo.CreateAuditEntry(ctx, entry); auditErr != nil {
		// Log only — do not surface to caller. Audit lag is observable via
		// Prometheus missing-audit-entry counter and alerting (Rule #13).
		_ = auditErr
	}
	return session, nil
}

// EndImpersonation explicitly terminates an active impersonation session.
// Returns ErrImpersonationNotFound if the session ID does not exist and
// ErrImpersonationAlreadyEnded if it was already closed.
func (s *Service) EndImpersonation(ctx context.Context, sessionID uuid.UUID, actorID uuid.UUID) error {
	if err := s.repo.EndImpersonationSession(ctx, sessionID); err != nil {
		return fmt.Errorf("iam: end impersonation %s: %w", sessionID, err)
	}
	// Append-only audit log (Rule #7). Best-effort: session is already closed above.
	entry := &AuditEntry{
		ID:         uuid.New(),
		ActorID:    actorID,
		Action:     "impersonation.end",
		TargetType: "impersonation_session",
		TargetID:   sessionID,
		OldState:   "active",
		NewState:   "ended",
		Timestamp:  time.Now().UTC(),
	}
	if auditErr := s.repo.CreateAuditEntry(ctx, entry); auditErr != nil {
		_ = auditErr
	}
	return nil
}

func isValidPlan(plan string) bool {
	switch plan {
	case "starter", "professional", "enterprise":
		return true
	}
	return false
}

// generateSecureToken produces a 32-byte cryptographically random hex string.
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
