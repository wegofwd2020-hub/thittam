package critical_path_test

// Critical Path 6: Notification Dispatch
//
// Verifies end-to-end notification flow:
//   - A template is registered for an event type and channel
//   - Send() routes through the matching ChannelSender, records delivery log
//   - Dispatch() fans out to multiple channels and collects errors gracefully
//   - Missing template causes ErrNoTemplateForEventType (not a panic)

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/services/notifications"
)

// ── Deterministic IDs for notifications e2e ──────────────────────────────

var (
	fixedNotifTenantID    = uuid.MustParse("e0000000-0000-0000-0000-000000000001")
	fixedNotifRecipientID = uuid.MustParse("e0000000-0000-0000-0000-000000000002")
	fixedNotifTemplateID  = uuid.MustParse("e0000000-0000-0000-0000-000000000003")
)

// ── Mock implementations ──────────────────────────────────────────────────

type notifRepo struct {
	mu        sync.Mutex
	templates []*notifications.Template
	notifs    []*notifications.Notification
}

func newNotifRepo() *notifRepo { return &notifRepo{} }

func (r *notifRepo) CreateTemplate(_ context.Context, t *notifications.Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates = append(r.templates, t)
	return nil
}

func (r *notifRepo) UpdateTemplate(_ context.Context, t *notifications.Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, tmpl := range r.templates {
		if tmpl.ID == t.ID {
			r.templates[i] = t
		}
	}
	return nil
}

func (r *notifRepo) GetTemplate(_ context.Context, tenantID, id uuid.UUID) (*notifications.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.templates {
		if t.TenantID == tenantID && t.ID == id {
			return t, nil
		}
	}
	return nil, notifications.ErrTemplateNotFound
}

func (r *notifRepo) GetActiveTemplate(_ context.Context, tenantID uuid.UUID, eventType, channel string) (*notifications.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.templates {
		if t.TenantID == tenantID && t.EventType == eventType && t.Channel == channel && t.IsActive {
			return t, nil
		}
	}
	return nil, notifications.ErrTemplateNotFound
}

func (r *notifRepo) ListTemplates(_ context.Context, tenantID uuid.UUID) ([]notifications.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []notifications.Template
	for _, t := range r.templates {
		if t.TenantID == tenantID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (r *notifRepo) CreateNotification(_ context.Context, n *notifications.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notifs = append(r.notifs, n)
	return nil
}

func (r *notifRepo) UpdateNotificationStatus(_ context.Context, id uuid.UUID, status, providerMsgID, errMsg string, sentAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.notifs {
		if n.ID == id {
			n.Status = status
			n.ProviderMsgID = providerMsgID
			n.ErrorMessage = errMsg
			n.SentAt = sentAt
		}
	}
	return nil
}

func (r *notifRepo) GetNotification(_ context.Context, tenantID, recipientID, id uuid.UUID) (*notifications.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, n := range r.notifs {
		if n.TenantID == tenantID && n.RecipientID == recipientID && n.ID == id {
			return n, nil
		}
	}
	return nil, notifications.ErrNotificationNotFound
}

func (r *notifRepo) ListNotifications(_ context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]notifications.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []notifications.Notification
	for _, n := range r.notifs {
		if n.TenantID != tenantID {
			continue
		}
		if n.RecipientID != recipientID {
			continue
		}
		if channel != "" && n.Channel != channel {
			continue
		}
		if status != "" && n.Status != status {
			continue
		}
		out = append(out, *n)
	}
	return out, nil
}

func (r *notifRepo) IncrementRetryCount(_ context.Context, _, id uuid.UUID) error { return nil }

// stubSender captures send calls and returns a canned provider message ID.
type stubSender struct {
	mu    sync.Mutex
	calls []*notifications.DeliveryRequest
}

func (s *stubSender) Send(_ context.Context, req *notifications.DeliveryRequest) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	return "provider-msg-001", nil
}

// ── Test: send email notification via template ────────────────────────────

func TestCriticalPath_NotificationDispatch(t *testing.T) {
	t.Parallel()

	repo := newNotifRepo()
	emailSender := &stubSender{}
	svc := notifications.NewService(repo, map[string]notifications.ChannelSender{
		"email": emailSender,
	})

	// Seed a template for the expense.approved event on the email channel.
	tmpl := &notifications.Template{
		ID:           fixedNotifTemplateID,
		TenantID:     fixedNotifTenantID,
		EventType:    "expense.approved",
		Channel:      "email",
		Subject:      "Your expense has been approved",
		BodyTemplate: "Hi {{.Name}}, your expense of {{.Amount}} has been approved.",
		IsActive:     true,
	}
	require.NoError(t, repo.CreateTemplate(context.Background(), tmpl))

	// Send the notification.
	req := &notifications.SendRequest{
		TenantID:         fixedNotifTenantID,
		RecipientID:      fixedNotifRecipientID,
		RecipientContact: "crew@example.com",
		Channel:          "email",
		EventType:        "expense.approved",
		Data: map[string]any{
			"Name":   "Alice",
			"Amount": "₹15,000",
		},
	}
	notif, err := svc.Send(context.Background(), req)
	require.NoError(t, err)

	// Notification is created and transitions to sent.
	assert.Equal(t, fixedNotifTenantID, notif.TenantID)
	assert.Equal(t, fixedNotifRecipientID, notif.RecipientID)
	assert.Equal(t, "email", notif.Channel)
	assert.Equal(t, "sent", notif.Status)
	assert.Equal(t, "provider-msg-001", notif.ProviderMsgID)

	// The sender was called exactly once.
	assert.Len(t, emailSender.calls, 1)
	assert.Equal(t, "crew@example.com", emailSender.calls[0].RecipientContact)
}

// ── Test: Dispatch fans out to multiple channels ──────────────────────────

func TestCriticalPath_NotificationDispatch_MultiChannel(t *testing.T) {
	t.Parallel()

	repo := newNotifRepo()
	emailSender := &stubSender{}
	smsSender := &stubSender{}
	svc := notifications.NewService(repo, map[string]notifications.ChannelSender{
		"email": emailSender,
		"sms":   smsSender,
	})

	// Register templates for two channels.
	for _, ch := range []string{"email", "sms"} {
		require.NoError(t, repo.CreateTemplate(context.Background(), &notifications.Template{
			ID:           uuid.New(),
			TenantID:     fixedNotifTenantID,
			EventType:    "budget.approved",
			Channel:      ch,
			Subject:      "Budget approved",
			BodyTemplate: "Budget approved.",
			IsActive:     true,
		}))
	}

	errs := svc.Dispatch(context.Background(),
		fixedNotifTenantID, fixedNotifRecipientID,
		"pm@example.com", "budget.approved",
		map[string]any{"Budget": "Main 2026"},
	)
	// Both channels succeed — no errors.
	assert.Empty(t, errs)
	assert.Len(t, emailSender.calls, 1)
	assert.Len(t, smsSender.calls, 1)
}

// ── Test: missing template returns typed error ────────────────────────────

func TestCriticalPath_NotificationSend_MissingTemplate(t *testing.T) {
	t.Parallel()

	repo := newNotifRepo()
	svc := notifications.NewService(repo, map[string]notifications.ChannelSender{
		"email": &stubSender{},
	})

	_, err := svc.Send(context.Background(), &notifications.SendRequest{
		TenantID:         fixedNotifTenantID,
		RecipientID:      fixedNotifRecipientID,
		RecipientContact: "x@example.com",
		Channel:          "email",
		EventType:        "nonexistent.event",
		Data:             nil,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, notifications.ErrTemplateNotFound)
}
