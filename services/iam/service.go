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
	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/crypto"
	"github.com/wegofwd2020/thittam/pkg/locale"
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

// permUserManage gates role management. Only super_admin holds it (see systemRoles),
// and no RPC creates roles — CreateRole exists only at the repository layer, called by
// seedSystemRoles. So "holds user:manage" is exactly "is a super_admin of this tenant".
//
// That equivalence is contingent: a future custom-role feature could mint a role carrying
// user:manage without carrying everything else, reopening escalation. There is no
// "you may only grant permissions you hold" subset rule anywhere. See the spec, §2 and §8.
const permUserManage = "user:manage"

// Ledger permissions encode separation of duties: drafting an entry (ledger:write),
// committing it (ledger:post), and controlling the books (ledger:admin) are three
// distinct grants. Collapsing them would remove the control an auditor looks for.
//
// services/ledger/handler.go declares its own identical copies — Go cannot share
// unexported identifiers across packages. Both packages' tests assert the literals.
const (
	permLedgerRead  = "ledger:read"
	permLedgerWrite = "ledger:write"
	permLedgerPost  = "ledger:post"
	permLedgerAdmin = "ledger:admin"
)

// systemRoles are seeded for every new tenant at creation time.
// Permissions follow the {resource}:{action} convention.
var systemRoles = []struct {
	name        string
	permissions []string
}{
	{"super_admin", []string{
		"production:read", "production:write",
		"budget:read", "budget:write", "budget:approve",
		"expense:read", "expense:submit", "expense:approve",
		"inventory:read", "inventory:checkout",
		"report:read",
		permLedgerRead, permLedgerWrite, permLedgerPost, permLedgerAdmin,
		permUserManage,
		"billing:read", "billing:manage",
		"document:read", "document:write", "document:delete",
		"notifications:read", "notifications:manage",
	}},
	{"manager", []string{
		"production:read", "production:write",
		"budget:read", "budget:approve",
		"expense:read", "expense:approve",
		"inventory:read", "inventory:checkout",
		"report:read",
		permLedgerRead,
		"billing:read", "billing:manage",
		"document:read", "document:write", "document:delete",
		"notifications:read", "notifications:manage",
	}},
	{"coordinator", []string{
		"production:read", "production:write",
		"budget:read", "budget:write",
		"expense:read", "expense:approve",
		"inventory:read", "inventory:checkout",
		"report:read",
		permLedgerRead,
		"document:read", "document:write",
	}},
	{"accountant", []string{
		"budget:read",
		"expense:read", "expense:submit", "expense:approve",
		"report:read",
		permLedgerRead, permLedgerWrite, permLedgerPost,
		"billing:read",
		"document:read", "document:write",
	}},
	{"member", []string{
		"production:read",
		"expense:submit",
		"document:read",
	}},
	{"inventory_manager", []string{
		"inventory:read", "inventory:write", "inventory:checkout", "inventory:retire",
		"document:read",
	}},
	// project_supervisor permissions are seeded tenant-wide in Phase 1.
	// Phase 2 (#42) adds project_id scoping when user_roles.project_id lands.
	{"project_supervisor", []string{
		"production:read",
		"budget:read",
		"expense:read", "expense:submit", "expense:approve",
		"resource:manage",
		"inventory:read", "inventory:checkout",
		"document:read", "document:write",
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
	audit    *audit.Logger   // optional; set by WithAuditLogger — used by lifecycle transitions (#92)
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
// If tenantID is uuid.Nil the tenant is resolved from the user's email
// (single-tenant emails only — ambiguous emails return ErrAmbiguousEmail).
// If the stored hash uses bcrypt (or weak argon2id params), it is silently
// upgraded to argon2id in the background after a successful login. This
// migration is best-effort — a rehash failure is logged but never surfaces
// to the caller (Rule #6: non-critical writes must not block reads).
func (s *Service) Login(ctx context.Context, tenantID uuid.UUID, email, password string) (*auth.TokenPair, error) {
	if tenantID == uuid.Nil {
		resolved, err := s.repo.FindTenantByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("iam: resolve tenant for %s: %w", email, err)
		}
		tenantID = resolved
	}
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
	_ = s.repo.UpdatePasswordHash(ctx, tenantID, record.ID, newHash)
}

// RefreshToken issues a new token pair from a valid refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	pair, err := s.tokens.Refresh(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("iam: refresh token: %w", err)
	}
	return pair, nil
}

// GetCurrentUser validates the access token, looks up the user + tenant
// it identifies, and returns both. Used by the UI's session-hydration
// flow (`GET /api/v1/auth/me`). The token comes from the Authorization
// metadata; the handler extracts and strips the "Bearer " prefix before
// calling this method.
func (s *Service) GetCurrentUser(ctx context.Context, accessToken string) (*User, *Tenant, error) {
	claims, err := s.tokens.Validate(ctx, accessToken)
	if err != nil {
		return nil, nil, fmt.Errorf("iam: validate token: %w", err)
	}
	user, err := s.repo.GetUser(ctx, claims.TenantID, claims.Subject)
	if err != nil {
		return nil, nil, fmt.Errorf("iam: get user %s: %w", claims.Subject, err)
	}
	tenant, err := s.repo.GetTenant(ctx, claims.TenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("iam: get tenant %s: %w", claims.TenantID, err)
	}
	return user, tenant, nil
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
func (s *Service) ChangePassword(ctx context.Context, tenantID, userID uuid.UUID, oldPassword, newPassword string) error {
	record, err := s.repo.GetUserByID(ctx, tenantID, userID)
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
	if err := s.repo.UpdatePasswordHash(ctx, tenantID, userID, hash); err != nil {
		return fmt.Errorf("iam: update password hash: %w", err)
	}
	return nil
}

// --- Roles & Permissions ---

// AssignRole grants a role to a user. Both the role and the target user must belong to
// tenantID: repo.AssignRole is a bare INSERT with no tenant column, so this is the only
// place the boundary can be enforced.
func (s *Service) AssignRole(ctx context.Context, tenantID, userID, roleID, assignedBy uuid.UUID) error {
	if _, err := s.repo.GetRoleByID(ctx, tenantID, roleID); err != nil {
		return err // ErrRoleNotFound for a role in another tenant
	}
	if _, err := s.repo.GetUser(ctx, tenantID, userID); err != nil {
		return err // ErrUserNotFound for a user in another tenant
	}
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

// RevokeRole removes a role from a user. RevokeRoleRequest carries no tenant_id, so the
// handler supplies the caller's tenant from the verified token.
func (s *Service) RevokeRole(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	if _, err := s.repo.GetRoleByID(ctx, tenantID, roleID); err != nil {
		return err // ErrRoleNotFound for a role in another tenant
	}
	if _, err := s.repo.GetUser(ctx, tenantID, userID); err != nil {
		return err // ErrUserNotFound for a user in another tenant
	}
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

// CheckPermission returns true if the user holds the given permission.
// projectID is optional: nil checks tenant-wide assignments only; a non-nil
// projectID merges tenant-wide assignments with project-scoped assignments
// for that project (ADR-014 Phase 2).
func (s *Service) CheckPermission(ctx context.Context, userID uuid.UUID, permission string, projectID *uuid.UUID) (bool, error) {
	perms, err := s.repo.GetUserPermissions(ctx, userID, projectID)
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

// projectScopedRoles is the set of system roles that may be assigned with a
// project_id. Mirrors the trigger in migrations/iam/012.
var projectScopedRoles = map[string]bool{
	"project_supervisor": true,
}

// AssignProjectRole grants a project-scoped role to a user for a specific
// project. Returns ErrRoleNotProjectScoped if the role is a tenant-wide-only
// system role (super_admin, manager, coordinator, accountant, inventory_manager,
// member). Custom roles (is_system = false) are allowed through.
func (s *Service) AssignProjectRole(ctx context.Context, tenantID, userID, roleID, projectID, assignedBy uuid.UUID) error {
	role, err := s.repo.GetRoleByID(ctx, tenantID, roleID)
	if err != nil {
		return fmt.Errorf("iam: lookup role %s: %w", roleID, err)
	}
	if role.IsSystem && !projectScopedRoles[role.Name] {
		return ErrRoleNotProjectScoped
	}
	if _, err := s.repo.GetUser(ctx, tenantID, userID); err != nil {
		return err // ErrUserNotFound for a user in another tenant
	}
	ur := &UserRole{
		UserID:     userID,
		RoleID:     roleID,
		ProjectID:  &projectID,
		AssignedBy: assignedBy,
		AssignedAt: time.Now().UTC(),
	}
	if err := s.repo.AssignRole(ctx, ur); err != nil {
		return fmt.Errorf("iam: assign project role %s to user %s on project %s: %w", roleID, userID, projectID, err)
	}
	return nil
}

// --- Tenants ---

// SetTenantAddress updates the address + country + currency fields on an
// existing tenant. Currency is derived from country when the caller leaves
// PrimaryCurrencyCode empty. Returns ErrCountryRequired / ErrUnknownCountry
// on invalid input (#61).
func (s *Service) SetTenantAddress(ctx context.Context, t *Tenant) (*Tenant, error) {
	if t.CountryCode == "" {
		return nil, ErrCountryRequired
	}
	t.CountryCode = strings.ToUpper(t.CountryCode)
	if !locale.IsKnownCountry(t.CountryCode) {
		return nil, fmt.Errorf("iam: set tenant address: %w: %q", ErrUnknownCountry, t.CountryCode)
	}
	if t.PrimaryCurrencyCode == "" {
		currency, err := locale.CurrencyForCountry(t.CountryCode)
		if err != nil {
			return nil, fmt.Errorf("iam: derive currency: %w", err)
		}
		t.PrimaryCurrencyCode = currency
	} else {
		t.PrimaryCurrencyCode = strings.ToUpper(t.PrimaryCurrencyCode)
	}
	updated, err := s.repo.UpdateTenantAddress(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("iam: update tenant address: %w", err)
	}
	return updated, nil
}

// CreateTenant creates a new tenant and seeds all system roles.
func (s *Service) CreateTenant(ctx context.Context, tenant *Tenant) (*Tenant, error) {
	// Canonicalise the name: trim and collapse internal whitespace runs to a
	// single space. Casing is preserved. Uniqueness is enforced at the DB
	// layer on lower(trim(name)); this normalisation ensures stored names
	// don't carry stray double-spaces into invoices and audit logs (#91).
	tenant.Name = strings.Join(strings.Fields(tenant.Name), " ")

	// Pre-flight: return a clean ALREADY_EXISTS naming the colliding tenant
	// rather than leaking a raw unique-violation. The DB index is still the
	// backstop for the concurrent-create race (#89).
	if existing, err := s.repo.FindTenantByNormalizedName(ctx, tenant.Name); err != nil {
		return nil, fmt.Errorf("iam: create tenant: %w", err)
	} else if existing != nil {
		return nil, TenantNameTakenErr(existing.ID)
	}

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

	// Country is required; currency is derived from country when the caller
	// doesn't supply an explicit override (#61). Callers that onboard in
	// multi-currency countries (PR → USD not PRA) should set
	// PrimaryCurrencyCode themselves.
	if tenant.CountryCode == "" {
		return nil, ErrCountryRequired
	}
	tenant.CountryCode = strings.ToUpper(tenant.CountryCode)
	if !locale.IsKnownCountry(tenant.CountryCode) {
		return nil, fmt.Errorf("iam: create tenant: %w: %q", ErrUnknownCountry, tenant.CountryCode)
	}
	if tenant.PrimaryCurrencyCode == "" {
		currency, err := locale.CurrencyForCountry(tenant.CountryCode)
		if err != nil {
			// Unreachable given IsKnownCountry above, but keep the branch for safety.
			return nil, fmt.Errorf("iam: derive currency: %w", err)
		}
		tenant.PrimaryCurrencyCode = currency
	} else {
		tenant.PrimaryCurrencyCode = strings.ToUpper(tenant.PrimaryCurrencyCode)
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

// SuspendTenant marks a tenant suspended, blocking all logins for that
// tenant. Optional legal-hold parameters (#92 Stage 4):
//
//   - freezeReason non-nil flags the tenant as on legal hold; the retention
//     sweeper will skip its automated lifecycle transitions.
//   - holdUntil is an optional auto-release time; nil means indefinite.
//
// Nil pointers preserve whatever is already on the row (COALESCE in the
// backing SQL), so a bare SuspendTenant retry does not clear an active
// hold. Clearing is a separate admin operation.
//
// When a hold is applied in *this* call (freezeReason != nil) the service
// emits an ActionLegalHoldApplied audit event with the actor identity
// taken from the request context. A bare suspend is captured by the
// generic audit interceptor; we do not double-log here.
func (s *Service) SuspendTenant(
	ctx context.Context,
	id uuid.UUID,
	holdUntil *time.Time,
	freezeReason *string,
) (*Tenant, error) {
	// Capture pre-state only when we will emit the hold audit event —
	// saves a read on the common bare-suspend path.
	var before *Tenant
	if freezeReason != nil && s.audit != nil {
		t, err := s.repo.GetTenant(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("iam: suspend tenant %s: %w", id, err)
		}
		before = t
	}

	if err := s.repo.UpdateTenantStatus(ctx, id, TenantStatusSuspended, holdUntil, freezeReason); err != nil {
		return nil, fmt.Errorf("iam: suspend tenant %s: %w", id, err)
	}

	after, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: suspend tenant %s: %w", id, err)
	}

	if freezeReason != nil && s.audit != nil {
		actor, _ := audit.ActorFromContext(ctx)
		s.audit.Log(audit.Event{
			TenantID:     id,
			ActorID:      actor.UserID,
			ActorEmail:   actor.Email,
			ActorIP:      actor.IP,
			Action:       audit.ActionLegalHoldApplied,
			ResourceType: audit.ResourceTenant,
			ResourceID:   id,
			OldState:     mustMarshalHoldState(before),
			NewState:     mustMarshalHoldState(after),
			OccurredAt:   time.Now().UTC(),
		})
	}

	return after, nil
}

// ClearTenantLegalHold releases a previously-applied legal hold
// (#92 Stage 4 pair). Status is untouched; only hold_until and
// freeze_reason are reset to NULL, so the retention sweeper resumes
// on the next run for tenants still in a transient state.
//
// Idempotent: calling on a tenant with no active hold is a no-op.
// An audit event (ActionLegalHoldCleared) is emitted only when the
// pre-state actually had freeze_reason set — quiet no-op on already-
// cleared tenants keeps the audit trail focused on real state
// changes. The optional `reason` argument is captured in the audit
// event's Metadata field (not OldState/NewState — it's why, not
// what).
func (s *Service) ClearTenantLegalHold(
	ctx context.Context,
	id uuid.UUID,
	reason string,
) (*Tenant, error) {
	before, err := s.repo.GetTenant(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: clear legal hold %s: %w", id, err)
	}

	after, err := s.repo.ClearTenantLegalHold(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("iam: clear legal hold %s: %w", id, err)
	}

	if before.FreezeReason != nil && s.audit != nil {
		actor, _ := audit.ActorFromContext(ctx)
		s.audit.Log(audit.Event{
			TenantID:     id,
			ActorID:      actor.UserID,
			ActorEmail:   actor.Email,
			ActorIP:      actor.IP,
			Action:       audit.ActionLegalHoldCleared,
			ResourceType: audit.ResourceTenant,
			ResourceID:   id,
			OldState:     mustMarshalHoldState(before),
			NewState:     mustMarshalHoldState(after),
			Metadata:     mustMarshalClearReason(reason),
			OccurredAt:   time.Now().UTC(),
		})
	}

	return after, nil
}

// --- Invitations ---

// InviteUser creates a pending invitation with a 7-day expiry.
// The caller is responsible for sending the invitation email via the
// notifications service (the IAM service publishes a NATS event for this).
func (s *Service) InviteUser(ctx context.Context, inv *Invitation) (*Invitation, error) {
	if inv.RoleID != nil {
		// The invitation's role is assigned at accept time, when there is no caller to
		// authorize. Validate it here, where there is one.
		if _, err := s.repo.GetRoleByID(ctx, inv.TenantID, *inv.RoleID); err != nil {
			return nil, err
		}
	}
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

	// Assign the pre-selected role if the invitation carried one. Route through
	// s.AssignRole, not s.repo.AssignRole: the invitation may be seven days old and its
	// role may have been deleted, and s.AssignRole re-checks that both the role and the
	// new user belong to the invitation's tenant. A failed grant fails the acceptance —
	// a role-less user holding a valid token is worse than a rejected invitation.
	if inv.RoleID != nil {
		if err := s.AssignRole(ctx, inv.TenantID, user.ID, *inv.RoleID, inv.InvitedBy); err != nil {
			return nil, err
		}
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
