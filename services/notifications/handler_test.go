package notifications

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	notificationsv1 "github.com/wegofwd2020/thittam/gen/notifications/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}, map[string]ChannelSender{}), allowAllPerm{})
}

func newHandlerWithRepo(r *mockRepo) *Handler {
	return NewHandler(NewService(r, map[string]ChannelSender{}), allowAllPerm{})
}

// allowAllPerm is a PermissionChecker double that grants every check.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm is a PermissionChecker double that denies every check.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}

// callerCtx returns a context carrying a verified caller in tenant tid, as
// UnaryAuthInterceptor would have produced from a valid token (#138).
// Handler tests bypass the interceptor, so they must inject the caller themselves.
func callerCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}

// callerCtxWithUser is callerCtx with a caller-controlled UserID, so a test can
// assert the recipient predicate the handler derives from the token.
func callerCtxWithUser(tid, uid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uid,
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
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
	}, map[string]ChannelSender{}), allowAllPerm{})

	_, err := h.Send(callerCtx(tenantID), &notificationsv1.SendRequest{
		TenantId:         tenantID.String(),
		RecipientId:      uuid.New().String(),
		RecipientContact: "user@example.com",
		Channel:          "email",
		EventType:        "expense.approved",
		Data:             map[string]string{"Name": "Jane"},
	})
	// No sender configured — expect ErrNoSenderForChannel → FailedPrecondition
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestHandler_Send_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Send(callerCtx(uuid.New()), &notificationsv1.SendRequest{
		TenantId:    "bad",
		RecipientId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_Send_InvalidRecipientID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().Send(callerCtx(tid), &notificationsv1.SendRequest{
		TenantId:    tid.String(),
		RecipientId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- Dispatch ---

func TestHandler_Dispatch_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	resp, err := newHandler().Dispatch(callerCtx(tid), &notificationsv1.DispatchRequest{
		TenantId:    tid.String(),
		RecipientId: uuid.New().String(),
		EventType:   "expense.approved",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_Dispatch_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().Dispatch(callerCtx(uuid.New()), &notificationsv1.DispatchRequest{
		TenantId:    "bad",
		RecipientId: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_Dispatch_InvalidRecipientID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().Dispatch(callerCtx(tid), &notificationsv1.DispatchRequest{
		TenantId:    tid.String(),
		RecipientId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateTemplate ---

func TestHandler_CreateTemplate_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := newHandler()

	resp, err := h.CreateTemplate(callerCtx(tenantID), &notificationsv1.CreateTemplateRequest{
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
	_, err := newHandler().CreateTemplate(callerCtx(uuid.New()), &notificationsv1.CreateTemplateRequest{
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
	}, map[string]ChannelSender{}), allowAllPerm{})

	resp, err := h.UpdateTemplate(callerCtx(tenantID), &notificationsv1.UpdateTemplateRequest{
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
	_, err := newHandler().UpdateTemplate(callerCtx(uuid.New()), &notificationsv1.UpdateTemplateRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_UpdateTemplate_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().UpdateTemplate(callerCtx(tid), &notificationsv1.UpdateTemplateRequest{
		TenantId: tid.String(), Id: "bad",
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
	}, map[string]ChannelSender{}), allowAllPerm{})

	resp, err := h.GetTemplate(callerCtx(tenantID), &notificationsv1.GetTemplateRequest{
		TenantId: tenantID.String(),
		Id:       tmplID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, tmplID.String(), resp.GetId())
}

func TestHandler_GetTemplate_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetTemplate(callerCtx(uuid.New()), &notificationsv1.GetTemplateRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetTemplate_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetTemplate(callerCtx(tid), &notificationsv1.GetTemplateRequest{
		TenantId: tid.String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetTemplate_NotFound(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTemplateFn: func(_ context.Context, _, _ uuid.UUID) (*Template, error) {
			return nil, ErrTemplateNotFound
		},
	}, map[string]ChannelSender{}), allowAllPerm{})

	_, err := h.GetTemplate(callerCtx(tid), &notificationsv1.GetTemplateRequest{
		TenantId: tid.String(), Id: uuid.New().String(),
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
	}, map[string]ChannelSender{}), allowAllPerm{})

	resp, err := h.ListTemplates(callerCtx(tenantID), &notificationsv1.ListTemplatesRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetTemplates(), 1)
}

func TestHandler_ListTemplates_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListTemplates(callerCtx(uuid.New()), &notificationsv1.ListTemplatesRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetNotification ---

func TestHandler_GetNotification_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	notifID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getNotificationFn: func(_ context.Context, tid, _, id uuid.UUID) (*Notification, error) {
			return &Notification{ID: id, TenantID: tid, Channel: "email", Status: "sent"}, nil
		},
	}, map[string]ChannelSender{}), allowAllPerm{})

	resp, err := h.GetNotification(callerCtx(tenantID), &notificationsv1.GetNotificationRequest{
		TenantId: tenantID.String(),
		Id:       notifID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, notifID.String(), resp.GetId())
}

func TestHandler_GetNotification_SelfScoped(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	uid := uuid.New()
	var gotRecipient uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		getNotificationFn: func(_ context.Context, _, recipientID, id uuid.UUID) (*Notification, error) {
			gotRecipient = recipientID
			return &Notification{ID: id, TenantID: tid}, nil
		},
	})
	_, err := h.GetNotification(callerCtxWithUser(tid, uid), &notificationsv1.GetNotificationRequest{
		TenantId: tid.String(), Id: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Equal(t, uid, gotRecipient)
}

func TestHandler_GetNotification_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetNotification(callerCtx(uuid.New()), &notificationsv1.GetNotificationRequest{
		TenantId: "bad", Id: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetNotification_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetNotification(callerCtx(tid), &notificationsv1.GetNotificationRequest{
		TenantId: tid.String(), Id: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListNotifications ---

func TestHandler_ListNotifications_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listNotificationsFn: func(_ context.Context, _, _ uuid.UUID, _, _ string, _, _ int) ([]Notification, error) {
			return []Notification{{ID: uuid.New(), TenantID: tenantID, Channel: "email", Status: "sent"}}, nil
		},
	}, map[string]ChannelSender{}), allowAllPerm{})

	resp, err := h.ListNotifications(callerCtx(tenantID), &notificationsv1.ListNotificationsRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetNotifications(), 1)
}

func TestHandler_ListNotifications_SelfScoped(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	uid := uuid.New()
	var gotRecipient uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		listNotificationsFn: func(_ context.Context, _, recipientID uuid.UUID, _, _ string, _, _ int) ([]Notification, error) {
			gotRecipient = recipientID
			return nil, nil
		},
	})
	_, err := h.ListNotifications(callerCtxWithUser(tid, uid), &notificationsv1.ListNotificationsRequest{TenantId: tid.String()})
	require.NoError(t, err)
	assert.Equal(t, uid, gotRecipient)
}

func TestHandler_ListNotifications_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListNotifications(callerCtx(uuid.New()), &notificationsv1.ListNotificationsRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CrossTenantRead_Denied(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	victimTenant := uuid.New()
	require.NotEqual(t, callerTenant, victimTenant)

	h := newHandlerWithRepo(&mockRepo{
		listNotificationsFn: func(context.Context, uuid.UUID, uuid.UUID, string, string, int, int) ([]Notification, error) {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil, nil
		},
	})

	_, err := h.ListNotifications(callerCtx(callerTenant), &notificationsv1.ListNotificationsRequest{
		TenantId: victimTenant.String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListNotifications_UsesTokenTenant(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenant uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		listNotificationsFn: func(_ context.Context, tenantID, _ uuid.UUID, _, _ string, _, _ int) ([]Notification, error) {
			gotTenant = tenantID
			return nil, nil
		},
	})

	// Request carries NO tenant at all: the token supplies it.
	_, err := h.ListNotifications(callerCtx(tid), &notificationsv1.ListNotificationsRequest{})
	require.NoError(t, err)
	assert.Equal(t, tid, gotTenant, "the repository must receive the token's tenant")
}

// --- permission gates ---

func TestHandler_Templates_Denied(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{}, map[string]ChannelSender{}), denyPerm{})
	_, err := h.ListTemplates(callerCtx(tid), &notificationsv1.ListTemplatesRequest{TenantId: tid.String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = h.Send(callerCtx(tid), &notificationsv1.SendRequest{TenantId: tid.String(), RecipientId: uuid.New().String(), Channel: "email", EventType: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// TestHandler_AllGatedRPCs_Denied is the complete #139 slice G grant-matrix
// check on the code side: every one of the six config/send RPCs must reject
// a denied caller with PermissionDenied, and it must do so before touching
// the repository (mockRepo has no functions set, so a repository call would
// panic on a nil func field, not just return a wrong value).
func TestHandler_AllGatedRPCs_Denied(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{}, map[string]ChannelSender{}), denyPerm{})

	cases := []struct {
		name string
		call func() error
	}{
		{"CreateTemplate", func() error {
			_, err := h.CreateTemplate(callerCtx(tid), &notificationsv1.CreateTemplateRequest{TenantId: tid.String()})
			return err
		}},
		{"UpdateTemplate", func() error {
			_, err := h.UpdateTemplate(callerCtx(tid), &notificationsv1.UpdateTemplateRequest{TenantId: tid.String(), Id: uuid.New().String()})
			return err
		}},
		{"Send", func() error {
			_, err := h.Send(callerCtx(tid), &notificationsv1.SendRequest{TenantId: tid.String(), RecipientId: uuid.New().String(), Channel: "email", EventType: "x"})
			return err
		}},
		{"Dispatch", func() error {
			_, err := h.Dispatch(callerCtx(tid), &notificationsv1.DispatchRequest{TenantId: tid.String(), RecipientId: uuid.New().String(), EventType: "x"})
			return err
		}},
		{"GetTemplate", func() error {
			_, err := h.GetTemplate(callerCtx(tid), &notificationsv1.GetTemplateRequest{TenantId: tid.String(), Id: uuid.New().String()})
			return err
		}},
		{"ListTemplates", func() error {
			_, err := h.ListTemplates(callerCtx(tid), &notificationsv1.ListTemplatesRequest{TenantId: tid.String()})
			return err
		}},
	}
	for _, tc := range cases {
		err := tc.call()
		require.Error(t, err, tc.name)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), tc.name)
	}
}

func TestHandler_Inbox_NotGated(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listNotificationsFn: func(context.Context, uuid.UUID, uuid.UUID, string, string, int, int) ([]Notification, error) {
			return nil, nil
		},
	}, map[string]ChannelSender{}), denyPerm{})
	_, err := h.ListNotifications(callerCtx(tid), &notificationsv1.ListNotificationsRequest{TenantId: tid.String()})
	require.NoError(t, err) // denyPerm must not block a self-scoped read
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
		{ErrNoSenderForChannel, codes.FailedPrecondition},
		{ErrNotificationNotFound, codes.NotFound},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
