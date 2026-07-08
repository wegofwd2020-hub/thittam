package iam

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	// ErrNotPlatformAdmin is returned when an admin-only RPC is called by a
	// caller that does not hold the platform_admin role.
	ErrNotPlatformAdmin = errors.New("iam: caller does not have platform_admin role")

	ErrTenantNotFound     = errors.New("iam: tenant not found")
	ErrImpersonationNotFound   = errors.New("iam: impersonation session not found")
	ErrImpersonationAlreadyEnded = errors.New("iam: impersonation session already ended")
	ErrUserNotFound       = errors.New("iam: user not found")
	ErrUserAlreadyExists  = errors.New("iam: user already exists for tenant")
	ErrRoleNotFound       = errors.New("iam: role not found")
	ErrInvitationNotFound = errors.New("iam: invitation not found")
	ErrInvitationExpired  = errors.New("iam: invitation has expired")
	ErrInvitationAccepted = errors.New("iam: invitation already accepted")
	ErrTenantSlugTaken    = errors.New("iam: tenant slug already taken")
	// ErrTenantNameTaken is returned by CreateTenant when another tenant
	// already exists with the same name (case-insensitive, whitespace-collapsed).
	// Enforced by the tenants_name_ci_unique index added in migration 015.
	ErrTenantNameTaken    = errors.New("iam: tenant name already taken")
	ErrInvalidPlan        = errors.New("iam: invalid plan; must be starter, professional, or enterprise")
	// ErrRoleNotProjectScoped is returned by AssignProjectRole when the caller
	// supplies a role that must be tenant-wide (e.g. super_admin, manager).
	ErrRoleNotProjectScoped = errors.New("iam: role cannot be project-scoped")
	// ErrAmbiguousEmail is returned by Login when no tenant_id was supplied
	// and the email exists in more than one tenant. The caller must retry
	// with an explicit tenant_id.
	ErrAmbiguousEmail = errors.New("iam: email exists in multiple tenants — supply tenant_id")
	// ErrCountryRequired is returned by CreateTenant when country_code is
	// empty. Onboarding must collect it for currency derivation (#61).
	ErrCountryRequired = errors.New("iam: country_code is required")
	// ErrUnknownCountry is returned when country_code has no currency
	// mapping in pkg/locale. Extend the map or supply primary_currency_code
	// explicitly.
	ErrUnknownCountry = errors.New("iam: unknown country_code")
	// ErrTenantNotHoldable is returned by SetTenantRetention when the tenant's
	// status has no running retention clock to hold — 'active' (not yet
	// suspended) or 'purge_eligible' (terminal). Maps to FailedPrecondition.
	ErrTenantNotHoldable = errors.New("iam: tenant status has no retention clock to hold")
	// ErrTenantHoldExists is returned by SetTenantRetention when the tenant
	// already has an active hold and overwrite was not requested. Maps to
	// FailedPrecondition. The wrapped message names the existing freeze_reason.
	ErrTenantHoldExists = errors.New("iam: tenant already has an active hold; pass overwrite to replace it")
	// ErrHoldUntilInPast is returned by SetTenantRetention when hold_until is at
	// or before now. Maps to InvalidArgument.
	ErrHoldUntilInPast = errors.New("iam: hold_until must be in the future")
)

// TenantNameTakenErr wraps ErrTenantNameTaken with the colliding tenant's
// UUID so the ALREADY_EXISTS surfaced to the caller names the existing
// tenant. errors.Is(err, ErrTenantNameTaken) stays true, so grpcError keeps
// returning codes.AlreadyExists.
func TenantNameTakenErr(id uuid.UUID) error {
	return fmt.Errorf("%w (existing tenant %s)", ErrTenantNameTaken, id)
}
