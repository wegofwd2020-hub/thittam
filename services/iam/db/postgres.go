package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// Compile-time interface checks — Postgres must satisfy both the service Repository
// and the auth store interfaces so it can be passed to auth.NewLocalProvider directly.
var (
	_ iam.Repository   = (*Postgres)(nil)
	_ auth.UserStore   = (*Postgres)(nil)
	_ auth.TenantStore = (*Postgres)(nil)
)

// --- auth.UserStore ---

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
func (p *Postgres) GetUserByID(ctx context.Context, userID uuid.UUID) (*auth.UserRecord, error) {
	u, err := p.q.GetUser(ctx, userID)
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
	row, err := p.q.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrUserNotFound
		}
		return nil, fmt.Errorf("iam/db: get user: %w", err)
	}
	// Enforce tenant isolation — the query fetches by PK only for efficiency.
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
	const q = `UPDATE users SET display_name = $2, status = $3 WHERE id = $1 AND tenant_id = $4`
	tag, err := p.db.Exec(ctx, q, u.ID, u.DisplayName, u.Status, u.TenantID)
	if err != nil {
		return fmt.Errorf("iam/db: update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return iam.ErrUserNotFound
	}
	return nil
}

func (p *Postgres) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string) error {
	if err := p.q.UpdateUserPasswordHash(ctx, UpdateUserPasswordHashParams{
		ID:           userID,
		PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("iam/db: update password hash: %w", err)
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
		ID:     t.ID,
		Name:   t.Name,
		Slug:   t.Slug,
		Plan:   t.Plan,
		Status: t.Status,
	})
	if err != nil {
		return fmt.Errorf("iam/db: create tenant: %w", err)
	}
	if row.ID == uuid.Nil {
		return iam.ErrTenantSlugTaken
	}
	t.CreatedAt = row.CreatedAt
	t.IsDemo = row.IsDemo
	return nil
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

func (p *Postgres) UpdateTenantStatus(ctx context.Context, id uuid.UUID, newStatus string) error {
	_, err := p.q.UpdateTenantStatus(ctx, UpdateTenantStatusParams{
		ID:     id,
		Status: newStatus,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return iam.ErrTenantNotFound
		}
		return fmt.Errorf("iam/db: update tenant status: %w", err)
	}
	return nil
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
	if err := p.q.AssignRole(ctx, AssignRoleParams{
		UserID:     ur.UserID,
		RoleID:     ur.RoleID,
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

// GetUserPermissions returns the union of all permissions held by the user across
// all assigned roles. Deduplication is done in-memory — sets are small (typically <50).
func (p *Postgres) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	roles, err := p.q.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("iam/db: get user permissions: %w", err)
	}
	seen := make(map[string]struct{})
	for _, r := range roles {
		for _, perm := range r.Permissions {
			seen[perm] = struct{}{}
		}
	}
	perms := make([]string, 0, len(seen))
	for p := range seen {
		perms = append(perms, p)
	}
	return perms, nil
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
		ID:        t.ID,
		Name:      t.Name,
		Slug:      t.Slug,
		Plan:      t.Plan,
		Status:    t.Status,
		IsDemo:    t.IsDemo,
		CreatedAt: t.CreatedAt,
	}
}
