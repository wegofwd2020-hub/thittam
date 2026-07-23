package notifications

import (
	"context"
	"errors"

	"github.com/google/uuid"
	notificationsv1 "github.com/wegofwd2020/thittam/gen/notifications/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler implements the gRPC NotificationsService.
// The tenant is always taken from the caller's verified token
// (interceptor.TenantFromRequest), never from the request body; a request
// that names a different tenant is rejected. NATS consumers (Send/Dispatch
// triggered by events) call the Service layer directly and never construct
// or go through a Handler, so there is no caller-less path into these
// handler methods.
type Handler struct {
	notificationsv1.UnimplementedNotificationsServiceServer
	svc *Service
}

// NewHandler creates a Handler wrapping the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Compile-time interface check.
var _ notificationsv1.NotificationsServiceServer = (*Handler)(nil)

// --- Send / Dispatch ---

func (h *Handler) Send(ctx context.Context, req *notificationsv1.SendRequest) (*notificationsv1.Notification, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	recipientID, err := uuid.Parse(req.GetRecipientId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid recipient_id")
	}

	notif, err := h.svc.Send(ctx, &SendRequest{
		TenantID:         tenantID,
		RecipientID:      recipientID,
		RecipientContact: req.GetRecipientContact(),
		Channel:          req.GetChannel(),
		EventType:        req.GetEventType(),
		Data:             strMapToAny(req.GetData()),
	})
	if err != nil {
		return nil, grpcErr(err)
	}
	return notificationToProto(notif), nil
}

func (h *Handler) Dispatch(ctx context.Context, req *notificationsv1.DispatchRequest) (*notificationsv1.DispatchResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	recipientID, err := uuid.Parse(req.GetRecipientId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid recipient_id")
	}

	errs := h.svc.Dispatch(ctx, tenantID, recipientID, req.GetRecipientContact(), req.GetEventType(), strMapToAny(req.GetData()))

	var channelErrors []*notificationsv1.ChannelError
	for _, e := range errs {
		channelErrors = append(channelErrors, &notificationsv1.ChannelError{
			Error: e.Error(),
		})
	}
	return &notificationsv1.DispatchResponse{ChannelErrors: channelErrors}, nil
}

// --- Templates ---

func (h *Handler) CreateTemplate(ctx context.Context, req *notificationsv1.CreateTemplateRequest) (*notificationsv1.Template, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	tmpl, err := h.svc.CreateTemplate(ctx, &Template{
		TenantID:     tenantID,
		EventType:    req.GetEventType(),
		Channel:      req.GetChannel(),
		Subject:      req.GetSubject(),
		BodyTemplate: req.GetBodyTemplate(),
	})
	if err != nil {
		return nil, grpcErr(err)
	}
	return templateToProto(tmpl), nil
}

func (h *Handler) UpdateTemplate(ctx context.Context, req *notificationsv1.UpdateTemplateRequest) (*notificationsv1.Template, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid template id")
	}

	// Fetch existing to preserve immutable fields (event_type, channel).
	existing, err := h.svc.GetTemplate(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}

	existing.Subject = req.GetSubject()
	existing.BodyTemplate = req.GetBodyTemplate()
	existing.IsActive = req.GetIsActive()

	tmpl, err := h.svc.UpdateTemplate(ctx, existing)
	if err != nil {
		return nil, grpcErr(err)
	}
	return templateToProto(tmpl), nil
}

func (h *Handler) GetTemplate(ctx context.Context, req *notificationsv1.GetTemplateRequest) (*notificationsv1.Template, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid template id")
	}

	tmpl, err := h.svc.GetTemplate(ctx, tenantID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return templateToProto(tmpl), nil
}

func (h *Handler) ListTemplates(ctx context.Context, req *notificationsv1.ListTemplatesRequest) (*notificationsv1.ListTemplatesResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}

	templates, err := h.svc.ListTemplates(ctx, tenantID)
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*notificationsv1.Template, len(templates))
	for i := range templates {
		out[i] = templateToProto(&templates[i])
	}
	return &notificationsv1.ListTemplatesResponse{Templates: out}, nil
}

// --- Delivery history ---

func (h *Handler) GetNotification(ctx context.Context, req *notificationsv1.GetNotificationRequest) (*notificationsv1.Notification, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	recipientID, err := interceptor.ActorFromRequest(ctx, "")
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid notification id")
	}

	notif, err := h.svc.GetNotification(ctx, tenantID, recipientID, id)
	if err != nil {
		return nil, grpcErr(err)
	}
	return notificationToProto(notif), nil
}

func (h *Handler) ListNotifications(ctx context.Context, req *notificationsv1.ListNotificationsRequest) (*notificationsv1.ListNotificationsResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	recipientID, err := interceptor.ActorFromRequest(ctx, "")
	if err != nil {
		return nil, err
	}

	notifs, err := h.svc.ListNotifications(ctx, tenantID, recipientID, req.GetChannel(), req.GetStatus(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, grpcErr(err)
	}

	out := make([]*notificationsv1.Notification, len(notifs))
	for i := range notifs {
		out[i] = notificationToProto(&notifs[i])
	}
	return &notificationsv1.ListNotificationsResponse{Notifications: out}, nil
}

// --- proto conversion helpers ---

func templateToProto(t *Template) *notificationsv1.Template {
	return &notificationsv1.Template{
		Id:           t.ID.String(),
		TenantId:     t.TenantID.String(),
		EventType:    t.EventType,
		Channel:      t.Channel,
		Subject:      t.Subject,
		BodyTemplate: t.BodyTemplate,
		IsActive:     t.IsActive,
	}
}

func notificationToProto(n *Notification) *notificationsv1.Notification {
	out := &notificationsv1.Notification{
		Id:            n.ID.String(),
		TenantId:      n.TenantID.String(),
		RecipientId:   n.RecipientID.String(),
		Channel:       n.Channel,
		EventType:     n.EventType,
		Subject:       n.Subject,
		Status:        n.Status,
		RetryCount:    int32(n.RetryCount),
		ProviderMsgId: n.ProviderMsgID,
		ErrorMessage:  n.ErrorMessage,
		CreatedAt:     timestamppb.New(n.CreatedAt),
	}
	if n.SentAt != nil {
		out.SentAt = timestamppb.New(*n.SentAt)
	}
	if n.DeliveredAt != nil {
		out.DeliveredAt = timestamppb.New(*n.DeliveredAt)
	}
	return out
}

// strMapToAny converts map[string]string to map[string]any for template rendering.
func strMapToAny(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// grpcErr maps domain sentinel errors to gRPC status codes.
func grpcErr(err error) error {
	switch {
	case errors.Is(err, ErrTemplateNotFound):
		return status.Error(codes.NotFound, "template not found")
	case errors.Is(err, ErrTemplateRender):
		return status.Error(codes.InvalidArgument, "template render failed")
	case errors.Is(err, ErrInvalidChannel):
		return status.Error(codes.InvalidArgument, "invalid channel; must be email, sms, in_app, or push")
	case errors.Is(err, ErrNoSenderForChannel):
		return status.Error(codes.FailedPrecondition, "no sender configured for channel")
	case errors.Is(err, ErrNotificationNotFound):
		return status.Error(codes.NotFound, "notification not found")
	}
	return status.Error(codes.Internal, "internal error")
}
