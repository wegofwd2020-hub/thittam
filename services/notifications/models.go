// Package notifications implements the notifications service.
// It delivers transactional messages across email, SMS, in-app, and push
// channels. Other services trigger it via NATS events; urgent synchronous
// sends use the thin gRPC SendNotification RPC.
package notifications

import (
	"time"

	"github.com/google/uuid"
)

// Template is a per-tenant, per-channel message template.
// The BodyTemplate field is a Go html/template string rendered at send time.
type Template struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	EventType    string    `json:"event_type"` // e.g. "expense.approved"
	Channel      string    `json:"channel"`    // email|sms|in_app|push
	Subject      string    `json:"subject,omitempty"`
	BodyTemplate string    `json:"body_template"`
	IsActive     bool      `json:"is_active"`
}

// Notification is a single delivery attempt recorded in the log.
type Notification struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	RecipientID   uuid.UUID  `json:"recipient_id"`
	Channel       string     `json:"channel"`
	EventType     string     `json:"event_type"`
	Subject       string     `json:"subject,omitempty"`
	Status        string     `json:"status"` // pending|sent|delivered|failed
	RetryCount    int        `json:"retry_count"`
	ProviderMsgID string     `json:"provider_msg_id,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
	DeliveredAt   *time.Time `json:"delivered_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// SendRequest carries everything needed to dispatch one notification.
type SendRequest struct {
	TenantID         uuid.UUID      // tenant the notification belongs to
	RecipientID      uuid.UUID      // user being notified
	RecipientContact string         // email address or phone number
	Channel          string         // email|sms|in_app|push
	EventType        string         // must match a notification_templates row
	Data             map[string]any // template variables
}

// DeliveryRequest is the rendered payload handed to a ChannelSender.
type DeliveryRequest struct {
	RecipientContact string    // email address or phone number
	RecipientID      uuid.UUID // for in-app / push targeting
	Subject          string
	Body             string
}
