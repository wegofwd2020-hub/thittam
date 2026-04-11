package notifications

import (
	"context"
	"testing"

	"github.com/google/uuid"
	notificationsv1 "github.com/wegofwd2020/thittam/gen/notifications/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}, map[string]ChannelSender{}))
}

// --- Send ---

func TestHandler_Send_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getActiveTemplateFn: func(_ context.Context, _ uuid.UUID, _, _ string) (*Template, error) {
			return &Template{
				ID:           uuid.New(),
				TenantID:     tenantID,
				EventType:    "expense.approved",
				Channel:      "email",
				BodyTemplate: "Hello {{.Name}}",
				IsActive:     true,
			}, nil
		},
	}, map[string]ChannelSender{}))

	_, err := h.Send(context.Background(), &notificationsv1.SendRequest{
		TenantId:         tenantID.String(),
		RecipientId:      uuid.New().String(),
		RecipientContact: "user@example.com",
		Channel:          "email",
		EventType:        "expense.approved",
		Data:             map[string]string{"Name": "Jane"},
	})
	// No sender configured — expect ErrNoSenderForChannel → Unimplemented
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestHandler_Send_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Send(context.Background(), &notificationsv1.SendRequest{
		TenantId:    "bad",
		RecipientId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_Send_InvalidRecipientID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Send(context.Background(), &notificationsv1.SendRequest{
		TenantId:    uuid.New().String(),
		RecipientId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- Dispatch ---

func TestHandler_Dispatch_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandler().Dispatch(context.Background(), &notificationsv1.DispatchRequest{
		TenantId:    uuid.New().String(),
		RecipientId: uuid.New().String(),
		EventType:   "expense.approved",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_Dispatch_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Dispatch(context.Background(), &notificationsv1.DispatchRequest{
		TenantId:    "bad",
		RecipientId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_Dispatch_InvalidRecipientID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Dispatch(context.Background(), &notificationsv1.DispatchRequest{
		TenantId:    uuid.New().String(),
		RecipientId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateTemplate ---

func TestHandler_CreateTemplate_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateTemplate(context.Background(), &notificationsv1.CreateTemplateRequest{
		TenantId:     tenantID.String(),
		EventType:    "expense.approved",
		Channel:      "email",
		Subject:      "Expense Approved",
		BodyTemplate: "Your expense of {{.Amount}} has been approved.",
	})
	require.NoError(t, err)
	assert.Equal(t, tenantID.String(), resp.GetTenantId())
	assert.Equal(t, "expense.approved", resp.GetEventType())
}

func TestHandler_CreateTemplate_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateTemplate(context.Background(), &notificationsv1.CreateTemplateRequest{
		TenantId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- UpdateTemplate ---

func TestHandler_UpdateTemplate_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	tmplID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTemplateFn: func(_ context.Context, tid, id uuid.UUID) (*Template, error) {
			return &Template{ID: id, TenantID: tid, EventType: "expense.approved", Channel: "email", IsActive: true}, nil
		},
	}, map[string]ChannelSender{}))

	resp, err := h.UpdateTemplate(context.Background(), &notificationsv1.UpdateTemplateRequest{
		TenantId:     tenantID.String(),
		Id:           tmplID.String(),
		Subject:      "Updated Subject",
		BodyTemplate: "Updated body",
		IsActive:     true,
	})
	require.NoError(t, err)
	assert.Equal(t, tmplID.String(), resp.GetId())
}

func TestHandler_UpdateTemplate_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateTemplate(context.Background(), &notificationsv1.UpdateTemplateRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateTemplate_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().UpdateTemplate(context.Background(), &notificationsv1.UpdateTemplateRequest{
		TenantId: uuid.New().String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetTemplate ---

func TestHandler_GetTemplate_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	tmplID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTemplateFn: func(_ context.Context, tid, id uuid.UUID) (*Template, error) {
			return &Template{ID: id, TenantID: tid, EventType: "budget.approved", Channel: "sms"}, nil
		},
	}, map[string]ChannelSender{}))

	resp, err := h.GetTemplate(context.Background(), &notificationsv1.GetTemplateRequest{
		TenantId: tenantID.String(),
		Id:       tmplID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, tmplID.String(), resp.GetId())
}

func TestHandler_GetTemplate_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetTemplate(context.Background(), &notificationsv1.GetTemplateRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetTemplate_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetTemplate(context.Background(), &notificationsv1.GetTemplateRequest{
		TenantId: uuid.New().String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getTemplateFn: func(_ context.Context, _, _ uuid.UUID) (*Template, error) {
			return nil, ErrTemplateNotFound
		},
	}, map[string]ChannelSender{}))

	_, err := h.GetTemplate(context.Background(), &notificationsv1.GetTemplateRequest{
		TenantId: uuid.New().String(), Id: uuid.New().String(),
	})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// --- ListTemplates ---

func TestHandler_ListTemplates_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listTemplatesFn: func(_ context.Context, _ uuid.UUID) ([]Template, error) {
			return []Template{{ID: uuid.New(), TenantID: tenantID, EventType: "x", Channel: "email"}}, nil
		},
	}, map[string]ChannelSender{}))

	resp, err := h.ListTemplates(context.Background(), &notificationsv1.ListTemplatesRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetTemplates(), 1)
}

func TestHandler_ListTemplates_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListTemplates(context.Background(), &notificationsv1.ListTemplatesRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetNotification ---

func TestHandler_GetNotification_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	notifID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getNotificationFn: func(_ context.Context, tid, id uuid.UUID) (*Notification, error) {
			return &Notification{ID: id, TenantID: tid, Channel: "email", Status: "sent"}, nil
		},
	}, map[string]ChannelSender{}))

	resp, err := h.GetNotification(context.Background(), &notificationsv1.GetNotificationRequest{
		TenantId: tenantID.String(),
		Id:       notifID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, notifID.String(), resp.GetId())
}

func TestHandler_GetNotification_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetNotification(context.Background(), &notificationsv1.GetNotificationRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetNotification_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetNotification(context.Background(), &notificationsv1.GetNotificationRequest{
		TenantId: uuid.New().String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListNotifications ---

func TestHandler_ListNotifications_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listNotificationsFn: func(_ context.Context, _ uuid.UUID, _, _ string, _, _ int) ([]Notification, error) {
			return []Notification{{ID: uuid.New(), TenantID: tenantID, Channel: "email", Status: "sent"}}, nil
		},
	}, map[string]ChannelSender{}))

	resp, err := h.ListNotifications(context.Background(), &notificationsv1.ListNotificationsRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetNotifications(), 1)
}

func TestHandler_ListNotifications_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListNotifications(context.Background(), &notificationsv1.ListNotificationsRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrTemplateNotFound, codes.NotFound},
		{ErrTemplateRender, codes.InvalidArgument},
		{ErrInvalidChannel, codes.InvalidArgument},
		{ErrNoSenderForChannel, codes.Unimplemented},
		{ErrNotificationNotFound, codes.NotFound},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
