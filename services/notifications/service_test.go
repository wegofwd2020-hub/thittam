package notifications

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Fixed test IDs ---

var (
	fixedTenantID    = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixedRecipientID = uuid.MustParse("b2000000-0000-0000-0000-000000000002")
	fixedTemplateID  = uuid.MustParse("c3000000-0000-0000-0000-000000000003")
	fixedNotifID     = uuid.MustParse("d4000000-0000-0000-0000-000000000004")
)

// --- Mock Repository ---

type mockRepo struct {
	createTemplateFn          func(ctx context.Context, tmpl *Template) error
	updateTemplateFn          func(ctx context.Context, tmpl *Template) error
	getTemplateFn             func(ctx context.Context, tenantID, id uuid.UUID) (*Template, error)
	getActiveTemplateFn       func(ctx context.Context, tenantID uuid.UUID, eventType, channel string) (*Template, error)
	listTemplatesFn           func(ctx context.Context, tenantID uuid.UUID) ([]Template, error)
	createNotificationFn      func(ctx context.Context, n *Notification) error
	updateNotificationStatusFn func(ctx context.Context, id uuid.UUID, status, providerMsgID, errMsg string, sentAt *time.Time) error
	getNotificationFn         func(ctx context.Context, tenantID, id uuid.UUID) (*Notification, error)
	listNotificationsFn       func(ctx context.Context, tenantID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error)
	incrementRetryCountFn     func(ctx context.Context, id uuid.UUID) error
}

func (m *mockRepo) CreateTemplate(ctx context.Context, tmpl *Template) error {
	if m.createTemplateFn != nil {
		return m.createTemplateFn(ctx, tmpl)
	}
	return nil
}
func (m *mockRepo) UpdateTemplate(ctx context.Context, tmpl *Template) error {
	if m.updateTemplateFn != nil {
		return m.updateTemplateFn(ctx, tmpl)
	}
	return nil
}
func (m *mockRepo) GetTemplate(ctx context.Context, tenantID, id uuid.UUID) (*Template, error) {
	if m.getTemplateFn != nil {
		return m.getTemplateFn(ctx, tenantID, id)
	}
	return &Template{ID: id, TenantID: tenantID, IsActive: true}, nil
}
func (m *mockRepo) GetActiveTemplate(ctx context.Context, tenantID uuid.UUID, eventType, channel string) (*Template, error) {
	if m.getActiveTemplateFn != nil {
		return m.getActiveTemplateFn(ctx, tenantID, eventType, channel)
	}
	return &Template{
		ID:           fixedTemplateID,
		TenantID:     tenantID,
		EventType:    eventType,
		Channel:      channel,
		Subject:      "Hello {{.Name}}",
		BodyTemplate: "Your {{.EventType}} notification.",
		IsActive:     true,
	}, nil
}
func (m *mockRepo) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]Template, error) {
	if m.listTemplatesFn != nil {
		return m.listTemplatesFn(ctx, tenantID)
	}
	return nil, nil
}
func (m *mockRepo) CreateNotification(ctx context.Context, n *Notification) error {
	if m.createNotificationFn != nil {
		return m.createNotificationFn(ctx, n)
	}
	return nil
}
func (m *mockRepo) UpdateNotificationStatus(ctx context.Context, id uuid.UUID, status, providerMsgID, errMsg string, sentAt *time.Time) error {
	if m.updateNotificationStatusFn != nil {
		return m.updateNotificationStatusFn(ctx, id, status, providerMsgID, errMsg, sentAt)
	}
	return nil
}
func (m *mockRepo) GetNotification(ctx context.Context, tenantID, id uuid.UUID) (*Notification, error) {
	if m.getNotificationFn != nil {
		return m.getNotificationFn(ctx, tenantID, id)
	}
	return &Notification{ID: id, TenantID: tenantID, Status: "sent"}, nil
}
func (m *mockRepo) ListNotifications(ctx context.Context, tenantID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error) {
	if m.listNotificationsFn != nil {
		return m.listNotificationsFn(ctx, tenantID, channel, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	if m.incrementRetryCountFn != nil {
		return m.incrementRetryCountFn(ctx, id)
	}
	return nil
}

// --- Mock ChannelSender ---

type mockSender struct {
	sendFn func(ctx context.Context, req *DeliveryRequest) (string, error)
}

func (m *mockSender) Send(ctx context.Context, req *DeliveryRequest) (string, error) {
	if m.sendFn != nil {
		return m.sendFn(ctx, req)
	}
	return "provider-msg-123", nil
}

// --- Helpers ---

func newTestService(repo Repository, senders map[string]ChannelSender) *Service {
	if senders == nil {
		senders = map[string]ChannelSender{
			"email":  &mockSender{},
			"sms":    &mockSender{},
			"in_app": &mockSender{},
			"push":   &mockSender{},
		}
	}
	return NewService(repo, senders)
}

// --- Tests: Send ---

func TestSend_Success(t *testing.T) {
	t.Parallel()
	var loggedStatus string
	svc := newTestService(&mockRepo{
		updateNotificationStatusFn: func(_ context.Context, _ uuid.UUID, status, _, _ string, _ *time.Time) error {
			loggedStatus = status
			return nil
		},
	}, nil)

	notif, err := svc.Send(context.Background(), &SendRequest{
		TenantID:         fixedTenantID,
		RecipientID:      fixedRecipientID,
		RecipientContact: "user@example.com",
		Channel:          "email",
		EventType:        "expense.approved",
		Data:             map[string]any{"Name": "Alice", "EventType": "expense.approved"},
	})
	require.NoError(t, err)
	assert.Equal(t, "sent", notif.Status)
	assert.Equal(t, "provider-msg-123", notif.ProviderMsgID)
	assert.Equal(t, "sent", loggedStatus)
}

func TestSend_InvalidChannel(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{}, nil)

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "carrier_pigeon",
		EventType: "expense.approved",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidChannel)
}

func TestSend_TemplateNotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, _ string) (*Template, error) {
			return nil, ErrTemplateNotFound
		},
	}, nil)

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "email",
		EventType: "unknown.event",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateNotFound)
}

func TestSend_TemplateRenderError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, _ string) (*Template, error) {
			return &Template{
				BodyTemplate: "{{.Unclosed", // invalid template
				IsActive:     true,
			}, nil
		},
	}, nil)

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "email",
		EventType: "expense.approved",
		Data:      map[string]any{},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateRender)
}

func TestSend_NoSenderConfigured(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{}, map[string]ChannelSender{
		// push sender is nil — not configured
	})

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "push",
		EventType: "production.created",
		Data:      map[string]any{},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoSenderForChannel)
}

func TestSend_SenderFailure_LogsAsFailed(t *testing.T) {
	t.Parallel()
	var loggedStatus string
	senderErr := errors.New("sendgrid: rate limit exceeded")

	svc := newTestService(&mockRepo{
		updateNotificationStatusFn: func(_ context.Context, _ uuid.UUID, status, _, errMsg string, _ *time.Time) error {
			loggedStatus = status
			return nil
		},
	}, map[string]ChannelSender{
		"email": &mockSender{
			sendFn: func(_ context.Context, _ *DeliveryRequest) (string, error) {
				return "", senderErr
			},
		},
	})

	notif, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "email",
		EventType: "budget.approved",
		Data:      map[string]any{},
	})
	require.Error(t, err)
	assert.Equal(t, "failed", notif.Status)
	assert.Equal(t, "failed", loggedStatus)
}

func TestSend_RendersTemplateVariables(t *testing.T) {
	t.Parallel()
	var deliveredBody string

	svc := newTestService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, _ string) (*Template, error) {
			return &Template{
				Subject:      "Expense {{.Status}}",
				BodyTemplate: "Dear {{.RecipientName}}, your claim of {{.Amount}} was {{.Status}}.",
				IsActive:     true,
			}, nil
		},
	}, map[string]ChannelSender{
		"email": &mockSender{
			sendFn: func(_ context.Context, req *DeliveryRequest) (string, error) {
				deliveredBody = req.Body
				return "msg-abc", nil
			},
		},
	})

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "email",
		EventType: "expense.approved",
		Data: map[string]any{
			"RecipientName": "Alice",
			"Amount":        "5000.00",
			"Status":        "approved",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Dear Alice, your claim of 5000.00 was approved.", deliveredBody)
}

// --- Tests: Dispatch ---

func TestDispatch_SendsToAllActiveChannels(t *testing.T) {
	t.Parallel()
	sentChannels := map[string]bool{}

	svc := newTestService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, channel string) (*Template, error) {
			// Only email and in_app are "active" for this event.
			if channel == "email" || channel == "in_app" {
				return &Template{Channel: channel, BodyTemplate: "body", IsActive: true}, nil
			}
			return nil, ErrTemplateNotFound
		},
	}, map[string]ChannelSender{
		"email":  &mockSender{sendFn: func(_ context.Context, r *DeliveryRequest) (string, error) { sentChannels["email"] = true; return "m1", nil }},
		"sms":    &mockSender{sendFn: func(_ context.Context, r *DeliveryRequest) (string, error) { sentChannels["sms"] = true; return "m2", nil }},
		"in_app": &mockSender{sendFn: func(_ context.Context, r *DeliveryRequest) (string, error) { sentChannels["in_app"] = true; return "m3", nil }},
		"push":   &mockSender{sendFn: func(_ context.Context, r *DeliveryRequest) (string, error) { sentChannels["push"] = true; return "m4", nil }},
	})

	errs := svc.Dispatch(context.Background(), fixedTenantID, fixedRecipientID, "user@example.com", "expense.approved", map[string]any{})
	assert.Empty(t, errs)
	assert.True(t, sentChannels["email"])
	assert.True(t, sentChannels["in_app"])
	assert.False(t, sentChannels["sms"])
	assert.False(t, sentChannels["push"])
}

// --- Tests: Templates ---

func TestCreateTemplate_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{}, nil)

	tmpl, err := svc.CreateTemplate(context.Background(), &Template{
		TenantID:     fixedTenantID,
		EventType:    "expense.approved",
		Channel:      "email",
		BodyTemplate: "Your expense was approved.",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, tmpl.ID)
	assert.True(t, tmpl.IsActive)
}

func TestCreateTemplate_InvalidChannel(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{}, nil)

	_, err := svc.CreateTemplate(context.Background(), &Template{
		TenantID:     fixedTenantID,
		EventType:    "expense.approved",
		Channel:      "fax",
		BodyTemplate: "body",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidChannel)
}

// --- Tests: Delivery history ---

func TestListNotifications_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := newTestService(&mockRepo{
		listNotificationsFn: func(_ context.Context, _ uuid.UUID, _, _ string, limit, _ int) ([]Notification, error) {
			capturedLimit = limit
			return nil, nil
		},
	}, nil)

	_, err := svc.ListNotifications(context.Background(), fixedTenantID, "", "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListNotifications_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := newTestService(&mockRepo{
		listNotificationsFn: func(_ context.Context, _ uuid.UUID, _, _ string, limit, _ int) ([]Notification, error) {
			capturedLimit = limit
			return nil, nil
		},
	}, nil)

	_, err := svc.ListNotifications(context.Background(), fixedTenantID, "", "", 9999, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

// --- Tests: renderString helper ---

func TestRenderString_PlainText(t *testing.T) {
	t.Parallel()
	out, err := renderString("Hello, World!", nil)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", out)
}

func TestRenderString_WithVariables(t *testing.T) {
	t.Parallel()
	out, err := renderString("Dear {{.Name}}, amount: {{.Amount}}", map[string]any{
		"Name":   "Bob",
		"Amount": "1000.00",
	})
	require.NoError(t, err)
	assert.Equal(t, "Dear Bob, amount: 1000.00", out)
}

func TestRenderString_Empty(t *testing.T) {
	t.Parallel()
	out, err := renderString("", nil)
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

func TestRenderString_InvalidTemplate(t *testing.T) {
	t.Parallel()
	_, err := renderString("{{.Unclosed", nil)
	require.Error(t, err)
}

// --- Tests: UpdateTemplate ---

func TestUpdateTemplate_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{}, nil)

	tmpl := &Template{
		ID:           fixedTemplateID,
		TenantID:     fixedTenantID,
		EventType:    "expense.approved",
		Channel:      "email",
		Subject:      "Updated subject",
		BodyTemplate: "Updated body",
		IsActive:     true,
	}
	result, err := svc.UpdateTemplate(context.Background(), tmpl)
	require.NoError(t, err)
	assert.Equal(t, "Updated subject", result.Subject)
}

func TestUpdateTemplate_RepoError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		updateTemplateFn: func(_ context.Context, _ *Template) error {
			return errors.New("db: connection lost")
		},
	}, nil)

	_, err := svc.UpdateTemplate(context.Background(), &Template{ID: fixedTemplateID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update template")
}

// --- Tests: GetTemplate ---

func TestGetTemplate_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getTemplateFn: func(_ context.Context, tenantID, id uuid.UUID) (*Template, error) {
			return &Template{ID: id, TenantID: tenantID, Channel: "email", IsActive: true}, nil
		},
	}, nil)

	tmpl, err := svc.GetTemplate(context.Background(), fixedTenantID, fixedTemplateID)
	require.NoError(t, err)
	assert.Equal(t, fixedTemplateID, tmpl.ID)
	assert.Equal(t, "email", tmpl.Channel)
}

func TestGetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getTemplateFn: func(_ context.Context, _, _ uuid.UUID) (*Template, error) {
			return nil, ErrTemplateNotFound
		},
	}, nil)

	_, err := svc.GetTemplate(context.Background(), fixedTenantID, fixedTemplateID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fixedTemplateID.String())
}

// --- Tests: ListTemplates ---

func TestListTemplates_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		listTemplatesFn: func(_ context.Context, tenantID uuid.UUID) ([]Template, error) {
			return []Template{
				{ID: fixedTemplateID, TenantID: tenantID, Channel: "email"},
				{ID: uuid.New(), TenantID: tenantID, Channel: "sms"},
			}, nil
		},
	}, nil)

	tmpls, err := svc.ListTemplates(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Len(t, tmpls, 2)
}

func TestListTemplates_RepoError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		listTemplatesFn: func(_ context.Context, _ uuid.UUID) ([]Template, error) {
			return nil, errors.New("db error")
		},
	}, nil)

	_, err := svc.ListTemplates(context.Background(), fixedTenantID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list templates")
}

// --- Tests: GetNotification ---

func TestGetNotification_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getNotificationFn: func(_ context.Context, tenantID, id uuid.UUID) (*Notification, error) {
			return &Notification{ID: id, TenantID: tenantID, Status: "sent", Channel: "email"}, nil
		},
	}, nil)

	n, err := svc.GetNotification(context.Background(), fixedTenantID, fixedNotifID)
	require.NoError(t, err)
	assert.Equal(t, fixedNotifID, n.ID)
	assert.Equal(t, "sent", n.Status)
}

func TestGetNotification_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getNotificationFn: func(_ context.Context, _, _ uuid.UUID) (*Notification, error) {
			return nil, errors.New("not found")
		},
	}, nil)

	_, err := svc.GetNotification(context.Background(), fixedTenantID, fixedNotifID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fixedNotifID.String())
}

// --- Tests: additional Send branches ---

func TestSend_CreateNotificationError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		createNotificationFn: func(_ context.Context, _ *Notification) error {
			return errors.New("db: insert failed")
		},
	}, nil)

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "email",
		EventType: "expense.approved",
		Data:      map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create log entry")
}

func TestSend_SubjectRenderError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, _ string) (*Template, error) {
			return &Template{
				Subject:      "{{.Unclosed", // bad subject template
				BodyTemplate: "body",
				IsActive:     true,
			}, nil
		},
	}, nil)

	_, err := svc.Send(context.Background(), &SendRequest{
		TenantID:  fixedTenantID,
		Channel:   "email",
		EventType: "budget.approved",
		Data:      map[string]any{},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateRender)
}

func TestDispatch_ChannelErrors_AreCollected(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, channel string) (*Template, error) {
			if channel == "email" {
				return &Template{Channel: "email", BodyTemplate: "body"}, nil
			}
			return nil, ErrTemplateNotFound
		},
		createNotificationFn: func(_ context.Context, _ *Notification) error {
			return errors.New("db down")
		},
	}, nil)

	errs := svc.Dispatch(context.Background(), fixedTenantID, fixedRecipientID, "a@b.com", "expense.approved", nil)
	assert.NotEmpty(t, errs, "dispatch should collect errors from failing channels")
}

func TestCreateTemplate_RepoError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		createTemplateFn: func(_ context.Context, _ *Template) error {
			return errors.New("db: constraint violation")
		},
	}, nil)

	_, err := svc.CreateTemplate(context.Background(), &Template{
		TenantID:  fixedTenantID,
		EventType: "expense.approved",
		Channel:   "email",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create template")
}
