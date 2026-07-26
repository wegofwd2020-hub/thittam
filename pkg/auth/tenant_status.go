package auth

// tenantStatusError returns the login-blocking error for a tenant status, or nil if
// the tenant may authenticate. Only "active" tenants may log in; every lifecycle state
// past active (suspended/grace/deactivated/purge_eligible, or any unknown value) blocks.
// grace's documented read-only access is unenforced, so it fails closed until built (#206).
func tenantStatusError(status string) error {
	switch status {
	case "active":
		return nil
	case "suspended":
		return ErrTenantSuspended
	default: // grace, deactivated, purge_eligible, or any unknown status
		return ErrTenantInactive
	}
}
