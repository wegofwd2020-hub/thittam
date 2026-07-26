package auth

import (
	"errors"
	"testing"
)

func TestTenantStatusError(t *testing.T) {
	cases := []struct {
		status string
		want   error
	}{
		{"active", nil},
		{"suspended", ErrTenantSuspended},
		{"grace", ErrTenantInactive},
		{"deactivated", ErrTenantInactive},
		{"purge_eligible", ErrTenantInactive},
		{"something_unknown", ErrTenantInactive},
	}
	for _, c := range cases {
		if got := tenantStatusError(c.status); !errors.Is(got, c.want) {
			t.Errorf("tenantStatusError(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}
