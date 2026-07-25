package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/services/iam"
)

// Postgres implements iam.Repository using sqlc-generated queries over a pgx/v5 pool.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres repository backed by the given pgx connection pool.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{
		q:  New(db),
		db: db,
	}
}

// Compile-time interface checks.
var (
	_ iam.Repository       = (*Postgres)(nil)
	_ auth.UserStore       = (*Postgres)(nil)
	_ auth.TenantStore     = (*Postgres)(nil)
	_ auth.OIDCConfigStore = (*Postgres)(nil)
)

// --- auth.UserStore ---

// FindTenantByEmail scans every tenant for a user with this email. Used by
// Login when the caller does not know the tenant. Returns:
//   - the unique tenant_id, no error → caller can proceed
//   - uuid.Nil + iam.ErrUserNotFound → no such email
//   - uuid.Nil + iam.ErrAmbiguousEmail → email in multiple tenants; caller
//     must supply tenant_id explicitly.
//
// users(tenant_id, email) is unique, so within a single tenant the email is
// unique; ambiguity is only possible when the same email is invited into
// two distinct tenants.
func (p *Postgres) FindTenantByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	rows, err := p.db.Query(ctx,
		`SELECT tenant_id FROM users WHERE email = $1 LIMIT 2`, email)
	if err != nil {
		return uuid.Nil, fmt.Errorf("iam/db: find tenant by email: %w", err)
	}
	defer rows.Close()
	var tenants []uuid.UUID
	for rows.Next() {
		var tid uuid.UUID
		if err := rows.Scan(&tid); err != nil {
			return uuid.Nil, fmt.Errorf("iam/db: scan tenant_id: %w", err)
		}
		tenants = append(tenants, tid)
	}
	switch len(tenants) {
	case 0:
		return uuid.Nil, iam.ErrUserNotFound
	case 1:
		return tenants[0], nil
	default:
		return uuid.Nil, iam.ErrAmbiguousEmail
	}
}

// GetUserByEmail returns the user record needed for authentication.
// Roles and permissions are loaded alongside so the JWT can carry them.
func (p *Postgres) GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error) {
	u, err := p.q.GetUserByEmail(ctx, GetUserByEmailParams{TenantID: tenantID, Email: email})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("iam/db: get user by email: %w", err)
	}
	roles, perms, err := p.loadRolesAndPerms(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &auth.UserRecord{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		PasswordHash: u.PasswordHash,
		Status:       u.Status,
		Roles:        roles,
		Permissions:  perms,
	}, nil
}

// GetUserByID returns the user record for refresh-token validation.
func (p *Postgres) GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error) {
	u, err := p.q.GetUser(ctx, GetUserParams{ID: userID, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrUserNotFound
		}
		return nil, fmt.Errorf("iam/db: get user by id: %w", err)
	}
	roles, perms, err := p.loadRolesAndPerms(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return &auth.UserRecord{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		PasswordHash: u.PasswordHash,
		Status:       u.Status,
		Roles:        roles,
		Permissions:  perms,
	}, nil
}

// CreateOIDCUser JIT-provisions a user on first OIDC login with an empty password hash.
func (p *Postgres) CreateOIDCUser(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error) {
	u, err := p.q.CreateUser(ctx, CreateUserParams{
		ID:           uuid.New(),
		TenantID:     tenantID,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: "", // OIDC users have no local password
		Status:       "active",
	})
	if err != nil {
		return nil, fmt.Errorf("iam/db: create oidc user: %w", err)
	}
	return &auth.UserRecord{
		ID:          u.ID,
		TenantID:    u.TenantID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Status:      u.Status,
	}, nil
}

// --- auth.TenantStore ---

// GetTenantStatus returns the tenant's status string for auth gating.
func (p *Postgres) GetTenantStatus(ctx context.Context, tenantID uuid.UUID) (string, error) {
	t, err := p.q.GetTenant(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", iam.ErrTenantNotFound
		}
		return "", fmt.Errorf("iam/db: get tenant status: %w", err)
	}
	return t.Status, nil
}

// --- auth.OIDCConfigStore ---

// GetOIDCConfig returns the OIDC configuration for a tenant.
// Returns auth.ErrOIDCConfigMissing if the tenant has no OIDC row or uses local auth.
func (p *Postgres) GetOIDCConfig(ctx context.Context, tenantID uuid.UUID) (*auth.OIDCConfig, error) {
	const q = `
		SELECT issuer_url, client_id, client_secret_enc, scopes,
		       email_claim, display_name_claim, groups_claim,
		       auto_provision, default_role
		FROM   tenant_auth_config
		WHERE  tenant_id = $1 AND auth_method = 'oidc'`

	var (
		issuerURL       string
		clientID        string
		clientSecretEnc string
		scopes          []string
		emailClaim      string
		displayClaim    string
		groupsClaim     pgtype.Text
		autoProvision   bool
		defaultRole     string
	)
	err := p.db.QueryRow(ctx, q, tenantID).Scan(
		&issuerURL, &clientID, &clientSecretEnc, &scopes,
		&emailClaim, &displayClaim, &groupsClaim,
		&autoProvision, &defaultRole,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrOIDCConfigMissing
		}
		return nil, fmt.Errorf("iam/db: get oidc config: %w", err)
	}
	cfg := &auth.OIDCConfig{
		TenantID:         tenantID,
		IssuerURL:        issuerURL,
		ClientID:         clientID,
		ClientSecret:     clientSecretEnc, // encrypted at rest; exchanger decrypts
		Scopes:           scopes,
		EmailClaim:       emailClaim,
		DisplayNameClaim: displayClaim,
		AutoProvision:    autoProvision,
		DefaultRole:      defaultRole,
	}
	if groupsClaim.Valid {
		cfg.GroupsClaim = groupsClaim.String
	}
	return cfg, nil
}

// --- iam.Repository: Users ---

func (p *Postgres) CreateUser(ctx context.Context, u *iam.User) error {
	row, err := p.q.CreateUser(ctx, CreateUserParams{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		PasswordHash: u.PasswordHash,
		Status:       u.Status,
	})
	if err != nil {
		return fmt.Errorf("iam/db: create user: %w", err)
	}
	// ON CONFLICT DO NOTHING returns no rows on a conflict — detect duplicate.
	if row.ID == uuid.Nil {
		return iam.ErrUserAlreadyExists
	}
	u.CreatedAt = row.CreatedAt
	return nil
}

func (p *Postgres) GetUser(ctx context.Context, tenantID, id uuid.UUID) (*iam.User, error) {
	row, err := p.q.GetUser(ctx, GetUserParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrUserNotFound
		}
		return nil, fmt.Errorf("iam/db: get user: %w", err)
	}
	// Defense in depth: the query above now also filters by tenant_id, but this
	// keeps the explicit guard in case that predicate is ever weakened again.
	if row.TenantID != tenantID {
		return nil, iam.ErrUserNotFound
	}
	return dbUserToDomain(row), nil
}

func (p *Postgres) ListUsers(ctx context.Context, tenantID uuid.UUID, statusFilter string, limit, offset int) ([]iam.User, error) {
	rows, err := p.q.ListUsers(ctx, ListUsersParams{
		TenantID: tenantID,
		Column2:  statusFilter,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("iam/db: list users: %w", err)
	}
	users := make([]iam.User, len(rows))
	for i, r := range rows {
		users[i] = *dbUserToDomain(r)
	}
	return users, nil
}

func (p *Postgres) UpdateUser(ctx context.Context, u *iam.User) error {
	// An empty status means "leave it alone", not "clear it". status is
	// security-critical: pkg/auth/local.go refuses login for 'deactivated' and
	// 'invited'. Before this guard, a client updating only a display name sent
	// status: "" and silently reactivated a deactivated account (#139 slice B).
	const q = `UPDATE users SET display_name = $2, status = COALESCE(NULLIF($3, ''), status) WHERE id = $1 AND tenant_id = $4`
	tag, err := p.db.Exec(ctx, q, u.ID, u.DisplayName, u.Status, u.TenantID)
	if err != nil {
		return fmt.Errorf("iam/db: update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return iam.ErrUserNotFound
	}
	return nil
}

func (p *Postgres) UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, hash string) error {
	n, err := p.q.UpdateUserPasswordHash(ctx, UpdateUserPasswordHashParams{
		ID:           userID,
		PasswordHash: hash,
		TenantID:     tenantID,
	})
	if err != nil {
		return fmt.Errorf("iam/db: update password hash: %w", err)
	}
	if n == 0 {
		return iam.ErrUserNotFound
	}
	return nil
}

func (p *Postgres) DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	row, err := p.q.UpdateUserStatus(ctx, UpdateUserStatusParams{
		ID:       id,
		Status:   "deactivated",
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("iam/db: deactivate user: %w", err)
	}
	if row.ID == uuid.Nil {
		return iam.ErrUserNotFound
	}
	return nil
}

// --- iam.Repository: Tenants ---

func (p *Postgres) CreateTenant(ctx context.Context, t *iam.Tenant) error {
	row, err := p.q.CreateTenant(ctx, CreateTenantParams{
		ID:                  t.ID,
		Name:                t.Name,
		Slug:                t.Slug,
		Plan:                t.Plan,
		Status:              t.Status,
		AddressLine1:        pgTextFromString(t.AddressLine1),
		AddressLine2:        pgTextFromString(t.AddressLine2),
		City:                pgTextFromString(t.City),
		CountryCode:         t.CountryCode,
		PostalCode:          pgTextFromString(t.PostalCode),
		PrimaryCurrencyCode: t.PrimaryCurrencyCode,
	})
	if err != nil {
		// Slug collisions are absorbed by ON CONFLICT (slug) DO NOTHING above,
		// but name collisions have no ON CONFLICT target and surface here as
		// PgError 23505 on tenants_name_ci_unique (see migration 015).
		if isUniqueViolationOn(err, "tenants_name_ci_unique") {
			// Concurrent-create race: the pre-flight missed a same-name insert
			// that landed first. Look up the winner so the caller still gets a
			// UUID-naming ALREADY_EXISTS. Fall back to the bare sentinel if the
			// lookup itself fails.
			if existing, lookupErr := p.FindTenantByNormalizedName(ctx, t.Name); lookupErr == nil && existing != nil {
				return iam.TenantNameTakenErr(existing.ID)
			}
			return iam.ErrTenantNameTaken
		}
		return fmt.Errorf("iam/db: create tenant: %w", err)
	}
	if row.ID == uuid.Nil {
		return iam.ErrTenantSlugTaken
	}
	t.CreatedAt = row.CreatedAt
	t.IsDemo = row.IsDemo
	return nil
}

// isUniqueViolationOn reports whether err is a Postgres unique_violation (23505)
// on the named constraint/index.
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// UpdateTenantAddress replaces the address + country + currency on an
// existing tenant. Used by the Settings → Company page (#61).
func (p *Postgres) UpdateTenantAddress(ctx context.Context, t *iam.Tenant) (*iam.Tenant, error) {
	row, err := p.q.UpdateTenantAddress(ctx, UpdateTenantAddressParams{
		ID:                  t.ID,
		AddressLine1:        pgTextFromString(t.AddressLine1),
		AddressLine2:        pgTextFromString(t.AddressLine2),
		City:                pgTextFromString(t.City),
		CountryCode:         t.CountryCode,
		PostalCode:          pgTextFromString(t.PostalCode),
		PrimaryCurrencyCode: t.PrimaryCurrencyCode,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrTenantNotFound
		}
		return nil, fmt.Errorf("iam/db: update tenant address: %w", err)
	}
	return dbTenantToDomain(row), nil
}

// pgTextFromString wraps a Go string as a pgtype.Text, treating the empty
// string as NULL so the DB stores NULL for unspecified optional fields
// rather than empty strings.
func pgTextFromString(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// pgTextToString returns the inner string or empty if NULL — used for
// serialising DB Tenant rows back to the domain model.
func pgTextToString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func (p *Postgres) GetTenant(ctx context.Context, id uuid.UUID) (*iam.Tenant, error) {
	row, err := p.q.GetTenant(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrTenantNotFound
		}
		return nil, fmt.Errorf("iam/db: get tenant: %w", err)
	}
	return dbTenantToDomain(row), nil
}

// FindTenantByNormalizedName implements iam.Repository. A no-rows result is
// not an error here — it means the name is free, so return (nil, nil).
func (p *Postgres) FindTenantByNormalizedName(ctx context.Context, name string) (*iam.Tenant, error) {
	row, err := p.q.FindTenantByNormalizedName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("iam/db: find tenant by name: %w", err)
	}
	return dbTenantToDomain(row), nil
}

// UpdateTenantStatus sets the tenant's status, stamps lifecycle
// timestamps on first transition, and optionally applies legal-hold
// fields (#92 Stage 4). holdUntil and freezeReason are optional; nil
// pointers preserve whatever is currently in the row (COALESCE in the
// backing SQL) so subsequent bare SuspendTenant calls don't clear an
// active hold. Clearing a hold is a separate admin operation, not this
// RPC.
func (p *Postgres) UpdateTenantStatus(
	ctx context.Context,
	id uuid.UUID,
	newStatus string,
	holdUntil *time.Time,
	freezeReason *string,
) error {
	_, err := p.q.UpdateTenantStatus(ctx, UpdateTenantStatusParams{
		ID:           id,
		Status:       newStatus,
		HoldUntil:    pgTimestamptzFromTimePtr(holdUntil),
		FreezeReason: pgTextFromStringPtr(freezeReason),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return iam.ErrTenantNotFound
		}
		return fmt.Errorf("iam/db: update tenant status: %w", err)
	}
	return nil
}

// CountTenantsOnHold returns the number of tenants whose
// freeze_reason is set (#92 Stage 5). Cheap even without an index
// because the held set is expected to be tiny.
func (p *Postgres) CountTenantsOnHold(ctx context.Context) (int64, error) {
	n, err := p.q.CountTenantsOnHold(ctx)
	if err != nil {
		return 0, fmt.Errorf("iam/db: count tenants on hold: %w", err)
	}
	return n, nil
}

// ClearTenantLegalHold resets hold_until and freeze_reason to NULL
// and returns the updated tenant (#92 Stage 4 pair). Idempotent —
// the UPDATE runs unconditionally, so a tenant with no active hold
// simply returns unchanged. Service-layer decides whether to emit
// an audit event based on the pre-state.
func (p *Postgres) ClearTenantLegalHold(ctx context.Context, id uuid.UUID) (*iam.Tenant, error) {
	row, err := p.q.ClearTenantLegalHold(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrTenantNotFound
		}
		return nil, fmt.Errorf("iam/db: clear tenant legal hold: %w", err)
	}
	return dbTenantToDomain(row), nil
}

// SetTenantLegalHold sets hold_until and freeze_reason to the given values
// and returns the updated tenant (#119). Status, suspended_at, and
// deactivated_at are NOT touched — this applies (or extends) a hold without
// regressing the retention clock. freezeReason is written verbatim; a nil
// holdUntil writes NULL (indefinite hold).
func (p *Postgres) SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*iam.Tenant, error) {
	row, err := p.q.SetTenantLegalHold(ctx, SetTenantLegalHoldParams{
		ID:           id,
		HoldUntil:    pgTimestamptzFromTimePtr(holdUntil),
		FreezeReason: pgtype.Text{String: freezeReason, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrTenantNotFound
		}
		return nil, fmt.Errorf("iam/db: set tenant legal hold: %w", err)
	}
	return dbTenantToDomain(row), nil
}

// TransitionTenantStatus performs a conditional transition guarded by the
// current status. Returns (tenant, true, nil) on successful transition,
// (nil, false, nil) when another worker already advanced the row (the
// WHERE status = from clause didn't match), and a non-nil error otherwise.
// Backing SQL sets suspended_at / deactivated_at on first entry (#92).
func (p *Postgres) TransitionTenantStatus(
	ctx context.Context,
	id uuid.UUID,
	from, to string,
) (*iam.Tenant, bool, error) {
	row, err := p.q.TransitionTenantStatus(ctx, TransitionTenantStatusParams{
		ID:         id,
		FromStatus: from,
		ToStatus:   to,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Either the tenant doesn't exist, or its current status differs
			// from `from`. Distinguishing the two requires an extra lookup;
			// callers treat "no transition" as the idempotent outcome.
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("iam/db: transition tenant status: %w", err)
	}
	return dbTenantToDomain(row), true, nil
}

// ListTenantsDueForLifecycle returns tenants whose next lifecycle
// transition is due at or before `now`. Backed by partial indexes on
// suspended_at (for suspended/grace) and purge_after (for deactivated).
func (p *Postgres) ListTenantsDueForLifecycle(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]*iam.Tenant, error) {
	rows, err := p.q.ListTenantsDueForLifecycle(ctx, ListTenantsDueForLifecycleParams{
		Column1: now.UTC(),
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("iam/db: list due-for-lifecycle: %w", err)
	}
	out := make([]*iam.Tenant, 0, len(rows))
	for _, row := range rows {
		out = append(out, dbTenantToDomain(row))
	}
	return out, nil
}

// --- iam.Repository: Roles ---

func (p *Postgres) CreateRole(ctx context.Context, r *iam.Role) error {
	row, err := p.q.CreateRole(ctx, CreateRoleParams{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Permissions: r.Permissions,
		IsSystem:    r.IsSystem,
	})
	if err != nil {
		return fmt.Errorf("iam/db: create role: %w", err)
	}
	if row.ID == uuid.Nil {
		// ON CONFLICT DO NOTHING — role already exists, which is fine for seeding.
		return nil
	}
	r.ID = row.ID
	return nil
}

func (p *Postgres) GetRole(ctx context.Context, tenantID uuid.UUID, name string) (*iam.Role, error) {
	const q = `SELECT id, tenant_id, name, permissions, is_system FROM roles WHERE tenant_id = $1 AND name = $2`
	row := p.db.QueryRow(ctx, q, tenantID, name)
	var r Role
	if err := row.Scan(&r.ID, &r.TenantID, &r.Name, &r.Permissions, &r.IsSystem); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrRoleNotFound
		}
		return nil, fmt.Errorf("iam/db: get role: %w", err)
	}
	return &iam.Role{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Permissions: r.Permissions,
		IsSystem:    r.IsSystem,
	}, nil
}

func (p *Postgres) GetRoleByID(ctx context.Context, tenantID, roleID uuid.UUID) (*iam.Role, error) {
	const q = `SELECT id, tenant_id, name, permissions, is_system FROM roles WHERE tenant_id = $1 AND id = $2`
	row := p.db.QueryRow(ctx, q, tenantID, roleID)
	var r Role
	if err := row.Scan(&r.ID, &r.TenantID, &r.Name, &r.Permissions, &r.IsSystem); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrRoleNotFound
		}
		return nil, fmt.Errorf("iam/db: get role by id: %w", err)
	}
	return &iam.Role{
		ID:          r.ID,
		TenantID:    r.TenantID,
		Name:        r.Name,
		Permissions: r.Permissions,
		IsSystem:    r.IsSystem,
	}, nil
}

func (p *Postgres) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]iam.Role, error) {
	rows, err := p.q.ListRoles(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam/db: list roles: %w", err)
	}
	roles := make([]iam.Role, len(rows))
	for i, r := range rows {
		roles[i] = iam.Role{
			ID:          r.ID,
			TenantID:    r.TenantID,
			Name:        r.Name,
			Permissions: r.Permissions,
			IsSystem:    r.IsSystem,
		}
	}
	return roles, nil
}

func (p *Postgres) AssignRole(ctx context.Context, ur *iam.UserRole) error {
	var projectID pgtype.UUID
	if ur.ProjectID != nil {
		projectID = pgtype.UUID{Bytes: *ur.ProjectID, Valid: true}
	}
	if err := p.q.AssignRole(ctx, AssignRoleParams{
		UserID:     ur.UserID,
		RoleID:     ur.RoleID,
		ProjectID:  projectID,
		AssignedBy: ur.AssignedBy,
	}); err != nil {
		return fmt.Errorf("iam/db: assign role: %w", err)
	}
	return nil
}

func (p *Postgres) RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error {
	if err := p.q.RevokeRole(ctx, RevokeRoleParams{
		UserID: userID,
		RoleID: roleID,
	}); err != nil {
		return fmt.Errorf("iam/db: revoke role: %w", err)
	}
	return nil
}

// GetUserPermissions returns the union of permissions held by the user.
// projectID nil → tenant-wide assignments only (project_id IS NULL).
// projectID non-nil → tenant-wide assignments + project-scoped assignments
// for that specific project (ADR-014 Phase 2).
// Deduplication is done in-memory — sets are small (typically <50).
func (p *Postgres) GetUserPermissions(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]string, error) {
	const q = `
		SELECT r.permissions
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		  AND (ur.project_id IS NULL OR ur.project_id = $2)`
	rows, err := p.db.Query(ctx, q, userID, projectID)
	if err != nil {
		return nil, fmt.Errorf("iam/db: get user permissions: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]struct{})
	for rows.Next() {
		var perms []string
		if err := rows.Scan(&perms); err != nil {
			return nil, fmt.Errorf("iam/db: scan user permissions: %w", err)
		}
		for _, perm := range perms {
			seen[perm] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam/db: iterate user permissions: %w", err)
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out, nil
}

// --- iam.Repository: Invitations ---

func (p *Postgres) CreateInvitation(ctx context.Context, inv *iam.Invitation) error {
	params := CreateInvitationParams{
		ID:        inv.ID,
		TenantID:  inv.TenantID,
		Email:     inv.Email,
		InvitedBy: inv.InvitedBy,
		Token:     inv.Token,
		ExpiresAt: inv.ExpiresAt,
		RoleID:    pgUUIDFromPtr(inv.RoleID),
	}
	row, err := p.q.CreateInvitation(ctx, params)
	if err != nil {
		return fmt.Errorf("iam/db: create invitation: %w", err)
	}
	inv.CreatedAt = row.CreatedAt
	return nil
}

func (p *Postgres) GetInvitationByToken(ctx context.Context, token string) (*iam.Invitation, error) {
	row, err := p.q.GetInvitationByToken(ctx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrInvitationNotFound
		}
		return nil, fmt.Errorf("iam/db: get invitation by token: %w", err)
	}
	inv := &iam.Invitation{
		ID:        row.ID,
		TenantID:  row.TenantID,
		Email:     row.Email,
		Token:     row.Token,
		Status:    row.Status,
		InvitedBy: row.InvitedBy,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}
	if row.RoleID.Valid {
		roleID := row.RoleID.Bytes
		uid := uuid.UUID(roleID)
		inv.RoleID = &uid
	}
	return inv, nil
}

func (p *Postgres) MarkInvitationAccepted(ctx context.Context, id uuid.UUID) error {
	if err := p.q.AcceptInvitation(ctx, id); err != nil {
		return fmt.Errorf("iam/db: mark invitation accepted: %w", err)
	}
	return nil
}

// --- iam.Repository: OIDC configuration ---

// UpsertOIDCConfig creates or replaces the OIDC configuration for a tenant.
// The caller must pre-encrypt ClientSecretEnc with AES-256-GCM before calling.
func (p *Postgres) UpsertOIDCConfig(ctx context.Context, params iam.OIDCConfigParams) error {
	scopes := params.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}
	emailClaim := params.EmailClaim
	if emailClaim == "" {
		emailClaim = "email"
	}
	displayClaim := params.DisplayNameClaim
	if displayClaim == "" {
		displayClaim = "name"
	}
	defaultRole := params.DefaultRole
	if defaultRole == "" {
		defaultRole = "member"
	}

	tenantID, err := uuid.Parse(params.TenantID)
	if err != nil {
		return fmt.Errorf("iam/db: upsert oidc config: invalid tenant_id: %w", err)
	}

	const q = `
		INSERT INTO tenant_auth_config (
			tenant_id, auth_method,
			issuer_url, client_id, client_secret_enc, scopes,
			email_claim, display_name_claim, groups_claim,
			auto_provision, default_role
		) VALUES (
			$1, 'oidc',
			$2, $3, $4, $5,
			$6, $7, $8,
			$9, $10
		)
		ON CONFLICT (tenant_id) DO UPDATE SET
			auth_method        = 'oidc',
			issuer_url         = EXCLUDED.issuer_url,
			client_id          = EXCLUDED.client_id,
			client_secret_enc  = EXCLUDED.client_secret_enc,
			scopes             = EXCLUDED.scopes,
			email_claim        = EXCLUDED.email_claim,
			display_name_claim = EXCLUDED.display_name_claim,
			groups_claim       = EXCLUDED.groups_claim,
			auto_provision     = EXCLUDED.auto_provision,
			default_role       = EXCLUDED.default_role,
			updated_at         = now()`

	var groupsClaim interface{}
	if params.GroupsClaim != "" {
		groupsClaim = params.GroupsClaim
	}

	_, err = p.db.Exec(ctx, q,
		tenantID,
		params.IssuerURL, params.ClientID, params.ClientSecretEnc, scopes,
		emailClaim, displayClaim, groupsClaim,
		params.AutoProvision, defaultRole,
	)
	if err != nil {
		return fmt.Errorf("iam/db: upsert oidc config for tenant %s: %w", tenantID, err)
	}
	return nil
}

// CreateAuditEntry inserts an append-only audit record.
// The table must have an insert-only policy in production (no UPDATE/DELETE).
func (p *Postgres) CreateAuditEntry(ctx context.Context, entry *iam.AuditEntry) error {
	const q = `
		INSERT INTO iam_audit_log
			(id, actor_id, action, target_type, target_id, old_state, new_state, timestamp, tenant_id, ip_address)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := p.db.Exec(ctx, q,
		entry.ID,
		entry.ActorID,
		entry.Action,
		entry.TargetType,
		entry.TargetID,
		entry.OldState,
		entry.NewState,
		entry.Timestamp,
		entry.TenantID,
		entry.IPAddress,
	)
	if err != nil {
		return fmt.Errorf("iam/db: create audit entry: %w", err)
	}
	return nil
}

// --- Internal helpers ---

// loadRolesAndPerms loads the role names and deduplicated permissions for a user.
func (p *Postgres) loadRolesAndPerms(ctx context.Context, userID uuid.UUID) ([]string, []string, error) {
	roles, err := p.q.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("iam/db: load roles for user %s: %w", userID, err)
	}
	roleNames := make([]string, len(roles))
	seen := make(map[string]struct{})
	for i, r := range roles {
		roleNames[i] = r.Name
		for _, perm := range r.Permissions {
			seen[perm] = struct{}{}
		}
	}
	perms := make([]string, 0, len(seen))
	for perm := range seen {
		perms = append(perms, perm)
	}
	return roleNames, perms, nil
}

func dbUserToDomain(u User) *iam.User {
	return &iam.User{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		DisplayName:  u.DisplayName,
		PasswordHash: u.PasswordHash,
		Status:       u.Status,
		CreatedAt:    u.CreatedAt,
	}
}

func dbTenantToDomain(t Tenant) *iam.Tenant {
	return &iam.Tenant{
		ID:                  t.ID,
		Name:                t.Name,
		Slug:                t.Slug,
		Plan:                t.Plan,
		Status:              t.Status,
		IsDemo:              t.IsDemo,
		CreatedAt:           t.CreatedAt,
		AddressLine1:        pgTextToString(t.AddressLine1),
		AddressLine2:        pgTextToString(t.AddressLine2),
		City:                pgTextToString(t.City),
		CountryCode:         t.CountryCode,
		PostalCode:          pgTextToString(t.PostalCode),
		PrimaryCurrencyCode: t.PrimaryCurrencyCode,
		SuspendedAt:         pgTimestamptzToTimePtr(t.SuspendedAt),
		DeactivatedAt:       pgTimestamptzToTimePtr(t.DeactivatedAt),
		HoldUntil:           pgTimestamptzToTimePtr(t.HoldUntil),
		FreezeReason:        pgTextToStringPtr(t.FreezeReason),
	}
}

// pgTextToStringPtr returns a pointer to the text value or nil when the
// column is NULL. Distinct from pgTextToString (which collapses NULL to
// ""); the legal-hold flag (#92 Stage 4) needs NULL / "" / set-value
// distinguished at the domain level.
func pgTextToStringPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// pgTextFromStringPtr wraps a *string as a pgtype.Text. nil pointer maps
// to a NULL pgtype.Text (Valid=false), which COALESCE in UpdateTenantStatus
// treats as "preserve existing column value" (#92 Stage 4). Empty string
// is written as-is and is distinct from NULL.
func pgTextFromStringPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// pgTimestamptzFromTimePtr wraps a *time.Time as a pgtype.Timestamptz.
// nil maps to NULL — see pgTextFromStringPtr for the COALESCE rationale.
func pgTimestamptzFromTimePtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// pgTimestamptzToTimePtr returns a pointer to time.Time or nil when the
// column is NULL. Used for optional lifecycle timestamps (#92).
func pgTimestamptzToTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// pgUUIDToPtr returns a pointer to the uuid.UUID value or nil when the
// column is NULL. Used for optional foreign keys such as approved_by /
// cancelled_by on tenant_purge_requests (#92 Stage 3).
func pgUUIDToPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}

// pgUUIDFromPtr wraps a *uuid.UUID as a pgtype.UUID. nil maps to NULL.
func pgUUIDFromPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// --- iam.Repository: Tenant purge (two-person approval, #92 Stage 3) ---

func dbPurgeRequestToDomain(r TenantPurgeRequest) *iam.TenantPurgeRequest {
	return &iam.TenantPurgeRequest{
		ID:            r.ID,
		TenantID:      r.TenantID,
		Status:        r.Status,
		RequestedBy:   r.RequestedBy,
		RequestedAt:   r.RequestedAt,
		RequestReason: r.RequestReason,
		ApprovedBy:    pgUUIDToPtr(r.ApprovedBy),
		ApprovedAt:    pgTimestamptzToTimePtr(r.ApprovedAt),
		CancelledBy:   pgUUIDToPtr(r.CancelledBy),
		CancelledAt:   pgTimestamptzToTimePtr(r.CancelledAt),
		ExecutedAt:    pgTimestamptzToTimePtr(r.ExecutedAt),
		FailureReason: pgTextToStringPtr(r.FailureReason),
		TenantName:    r.TenantName,
		TenantSlug:    r.TenantSlug,
	}
}

// CreateTenantPurgeRequest inserts a new pending purge request. Maps a
// unique-violation on tenant_purge_requests_one_open (at most one open
// request per tenant) to iam.ErrPurgeRequestExists.
func (p *Postgres) CreateTenantPurgeRequest(ctx context.Context, req *iam.TenantPurgeRequest) error {
	row, err := p.q.CreateTenantPurgeRequest(ctx, CreateTenantPurgeRequestParams{
		ID:            req.ID,
		TenantID:      req.TenantID,
		RequestedBy:   req.RequestedBy,
		RequestReason: req.RequestReason,
		TenantName:    req.TenantName,
		TenantSlug:    req.TenantSlug,
	})
	if err != nil {
		if isUniqueViolationOn(err, "tenant_purge_requests_one_open") {
			return iam.ErrPurgeRequestExists
		}
		return fmt.Errorf("iam/db: create purge request: %w", err)
	}
	*req = *dbPurgeRequestToDomain(row)
	return nil
}

// GetOpenTenantPurgeRequest returns the tenant's open (pending|approved)
// purge request, or iam.ErrPurgeRequestNotFound if none exists.
func (p *Postgres) GetOpenTenantPurgeRequest(ctx context.Context, tenantID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.GetOpenTenantPurgeRequest(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound
		}
		return nil, fmt.Errorf("iam/db: get open purge request: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

// ApproveTenantPurgeRequest transitions a pending request to approved.
// Returns iam.ErrPurgeRequestNotFound if the request is missing or no
// longer pending (the WHERE status = 'pending' guard didn't match).
func (p *Postgres) ApproveTenantPurgeRequest(ctx context.Context, tenantID, requestID, approverID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.ApproveTenantPurgeRequest(ctx, ApproveTenantPurgeRequestParams{
		ID:         requestID,
		ApprovedBy: pgUUIDFromPtr(&approverID),
		TenantID:   tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound // no longer pending
		}
		return nil, fmt.Errorf("iam/db: approve purge request: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

// CancelTenantPurgeRequest transitions an open (pending|approved) request
// to cancelled. Returns iam.ErrPurgeRequestNotFound if the request is
// missing or already terminal.
func (p *Postgres) CancelTenantPurgeRequest(ctx context.Context, tenantID, requestID, cancellerID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.CancelTenantPurgeRequest(ctx, CancelTenantPurgeRequestParams{
		ID:          requestID,
		CancelledBy: pgUUIDFromPtr(&cancellerID),
		TenantID:    tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound
		}
		return nil, fmt.Errorf("iam/db: cancel purge request: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

// ListApprovedTenantPurgeRequests returns approved requests oldest-first,
// capped at limit. Polled by the purge worker (#92 Stage 3).
func (p *Postgres) ListApprovedTenantPurgeRequests(ctx context.Context, limit int) ([]*iam.TenantPurgeRequest, error) {
	rows, err := p.q.ListApprovedTenantPurgeRequests(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("iam/db: list approved purge requests: %w", err)
	}
	out := make([]*iam.TenantPurgeRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbPurgeRequestToDomain(r))
	}
	return out, nil
}

// MarkTenantPurgeRequestFailed sets a request to failed with the given
// reason. Returns iam.ErrPurgeRequestNotFound if the request is missing.
func (p *Postgres) MarkTenantPurgeRequestFailed(ctx context.Context, requestID uuid.UUID, reason string) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.MarkTenantPurgeRequestFailed(ctx, MarkTenantPurgeRequestFailedParams{
		ID:            requestID,
		FailureReason: pgTextFromString(reason),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound
		}
		return nil, fmt.Errorf("iam/db: mark purge request failed: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

// PurgeTenantSchemaAndTombstone hard-deletes a purge_eligible tenant: claim
// the purge request, DROP SCHEMA tenant_<uuid> CASCADE, and tombstone the
// tenants row — all in one owner-privileged transaction, claim FIRST so
// nothing destructive runs before the request's approved-ness is
// re-verified.
//
// Ordering matters (#118 review — claim-first restructure):
//  1. Claim: status-guarded UPDATE tenant_purge_requests ... WHERE
//     status='approved'. Zero rows means the request left 'approved'
//     since the worker listed it — most likely CancelTenantPurge ran
//     mid-flight, or (rarely) a previous run's ambiguous commit already
//     executed it. Either way NOTHING destructive has happened yet, so
//     this makes a mid-flight cancel authoritative: return
//     iam.ErrPurgeRequestNotApproved and the deferred Rollback is a no-op.
//  2. Drop the tenant schema.
//  3. Tombstone: status-guarded UPDATE tenants ... WHERE
//     status='purge_eligible'. Zero rows means the tenant left
//     purge_eligible since approval (race / manual change); returning
//     iam.ErrTenantNotPurgeable rolls back both the claim and the drop, so
//     nothing is destroyed.
//  4. Commit.
func (p *Postgres) PurgeTenantSchemaAndTombstone(ctx context.Context, tenantID, requestID uuid.UUID) error {
	// Schema name = tenant_<uuid> (dashes kept). Interpolated because Postgres
	// cannot parameterize identifiers; safe because tenantID is a validated
	// uuid.UUID (see pkg/tenantdb doc: uuid.String() yields only hex+hyphens).
	schema := "tenant_" + tenantID.String()

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iam/db: purge begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	claimCt, err := tx.Exec(ctx,
		`UPDATE tenant_purge_requests SET status='executed', executed_at=now() WHERE id=$1 AND status='approved'`,
		requestID)
	if err != nil {
		return fmt.Errorf("iam/db: claim purge request %s: %w", requestID, err)
	}
	if claimCt.RowsAffected() == 0 {
		// Request was cancelled mid-flight or already processed. Nothing
		// destructive has run yet — the deferred Rollback is a no-op.
		return iam.ErrPurgeRequestNotApproved
	}

	if _, err := tx.Exec(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`); err != nil {
		return fmt.Errorf("iam/db: drop schema %s: %w", schema, err)
	}

	ct, err := tx.Exec(ctx, `UPDATE tenants SET status='purged',
		name = 'purged-' || id::text,
		address_line1=NULL, address_line2=NULL, city=NULL, postal_code=NULL,
		purged_at=now()
	 WHERE id=$1 AND status='purge_eligible'`, tenantID)
	if err != nil {
		return fmt.Errorf("iam/db: tombstone tenant %s: %w", tenantID, err)
	}
	if ct.RowsAffected() == 0 {
		// Tenant left purge_eligible since approval (race / manual change). The
		// claim and schema drop both roll back with the tx — nothing is destroyed.
		return iam.ErrTenantNotPurgeable
	}

	return tx.Commit(ctx)
}
