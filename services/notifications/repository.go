package notifications

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines all data access required by the notifications service.
type Repository interface {
	// Templates
	CreateTemplate(ctx context.Context, tmpl *Template) error
	UpdateTemplate(ctx context.Context, tmpl *Template) error
	GetTemplate(ctx context.Context, tenantID, id uuid.UUID) (*Template, error)
	GetActiveTemplate(ctx context.Context, tenantID uuid.UUID, eventType, channel string) (*Template, error)
	ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]Template, error)

	// Delivery log
	CreateNotification(ctx context.Context, n *Notification) error
	UpdateNotificationStatus(ctx context.Context, id uuid.UUID, status, providerMsgID, errMsg string, sentAt *time.Time) error
	GetNotification(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*Notification, error)
	ListNotifications(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error)
	IncrementRetryCount(ctx context.Context, tenantID, id uuid.UUID) error
}
