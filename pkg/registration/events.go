package registration

import (
	"time"

	"github.com/google/uuid"
)

// TenantCreatedEvent is published to subject "thittam.iam.tenant.created"
// after a successful tenant registration.
type TenantCreatedEvent struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Plan        string    `json:"plan"`
	VerticalID  string    `json:"vertical_id"`
	AdminEmail  string    `json:"admin_email"`
	AdminUserID uuid.UUID `json:"admin_user_id"`
	OccurredAt  time.Time `json:"occurred_at"`
}
