package notifications

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/google/uuid"
)

// ChannelSender delivers a rendered notification via one specific channel.
// Implementations wrap SendGrid (email), Twilio (sms), WebSocket/SSE (in_app),
// and Firebase FCM (push). All provider credentials come from env vars — never
// hardcoded.
type ChannelSender interface {
	Send(ctx context.Context, req *DeliveryRequest) (providerMsgID string, err error)
}

// Service implements notifications business logic.
type Service struct {
	repo    Repository
	senders map[string]ChannelSender // keyed by channel name
}

// NewService creates a notifications service.
// senders is a map from channel name ("email", "sms", "in_app", "push") to its
// implementation. A nil entry means that channel is disabled — attempts return
// ErrNoSenderForChannel.
func NewService(repo Repository, senders map[string]ChannelSender) *Service {
	return &Service{repo: repo, senders: senders}
}

// --- Synchronous send ---

// Send resolves the template for the request's event type and channel, renders
// it with the provided data, dispatches via the channel sender, and writes a
// delivery log entry. It is the single entry point for both gRPC-triggered and
// NATS-event-triggered sends.
func (s *Service) Send(ctx context.Context, req *SendRequest) (*Notification, error) {
	if !isValidChannel(req.Channel) {
		return nil, ErrInvalidChannel
	}

	// Resolve template.
	tmpl, err := s.repo.GetActiveTemplate(ctx, req.TenantID, req.EventType, req.Channel)
	if err != nil {
		return nil, ErrTemplateNotFound
	}

	// Render subject and body.
	subject, err := renderString(tmpl.Subject, req.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: subject: %v", ErrTemplateRender, err)
	}
	body, err := renderString(tmpl.BodyTemplate, req.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: body: %v", ErrTemplateRender, err)
	}

	// Create a pending log entry before attempting delivery so we have a record
	// even if the send crashes (non-critical writes never block the caller).
	notif := &Notification{
		ID:          uuid.New(),
		TenantID:    req.TenantID,
		RecipientID: req.RecipientID,
		Channel:     req.Channel,
		EventType:   req.EventType,
		Subject:     subject,
		Status:      "pending",
	}
	if err := s.repo.CreateNotification(ctx, notif); err != nil {
		return nil, fmt.Errorf("notifications: create log entry: %w", err)
	}

	// Dispatch.
	sender, ok := s.senders[req.Channel]
	if !ok || sender == nil {
		_ = s.repo.UpdateNotificationStatus(ctx, notif.ID, "failed", "", ErrNoSenderForChannel.Error(), nil)
		return nil, ErrNoSenderForChannel
	}

	providerMsgID, sendErr := sender.Send(ctx, &DeliveryRequest{
		RecipientContact: req.RecipientContact,
		RecipientID:      req.RecipientID,
		Subject:          subject,
		Body:             body,
	})

	now := time.Now().UTC()
	if sendErr != nil {
		_ = s.repo.UpdateNotificationStatus(ctx, notif.ID, "failed", "", sendErr.Error(), nil)
		notif.Status = "failed"
		notif.ErrorMessage = sendErr.Error()
		return notif, fmt.Errorf("notifications: send via %s: %w", req.Channel, sendErr)
	}

	_ = s.repo.UpdateNotificationStatus(ctx, notif.ID, "sent", providerMsgID, "", &now)
	notif.Status = "sent"
	notif.ProviderMsgID = providerMsgID
	notif.SentAt = &now
	return notif, nil
}

// Dispatch is a convenience method that sends to every active channel registered
// for the given event type. Used by NATS event consumers.
// Errors from individual channels are collected but do not abort other channels.
func (s *Service) Dispatch(ctx context.Context, tenantID, recipientID uuid.UUID, contact, eventType string, data map[string]any) []error {
	channels := []string{"email", "sms", "in_app", "push"}
	var errs []error
	for _, ch := range channels {
		_, err := s.repo.GetActiveTemplate(ctx, tenantID, eventType, ch)
		if err != nil {
			continue // no template for this channel — skip silently
		}
		if _, err := s.Send(ctx, &SendRequest{
			TenantID:         tenantID,
			RecipientID:      recipientID,
			RecipientContact: contact,
			Channel:          ch,
			EventType:        eventType,
			Data:             data,
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// --- Templates ---

// CreateTemplate creates a new notification template for a tenant.
func (s *Service) CreateTemplate(ctx context.Context, tmpl *Template) (*Template, error) {
	if !isValidChannel(tmpl.Channel) {
		return nil, ErrInvalidChannel
	}
	if tmpl.ID == uuid.Nil {
		tmpl.ID = uuid.New()
	}
	if !tmpl.IsActive {
		tmpl.IsActive = true
	}
	if err := s.repo.CreateTemplate(ctx, tmpl); err != nil {
		return nil, fmt.Errorf("notifications: create template: %w", err)
	}
	return tmpl, nil
}

// UpdateTemplate replaces a template's subject and body. Channel and event type
// are immutable after creation.
func (s *Service) UpdateTemplate(ctx context.Context, tmpl *Template) (*Template, error) {
	if err := s.repo.UpdateTemplate(ctx, tmpl); err != nil {
		return nil, fmt.Errorf("notifications: update template %s: %w", tmpl.ID, err)
	}
	return tmpl, nil
}

// GetTemplate retrieves a template by ID.
func (s *Service) GetTemplate(ctx context.Context, tenantID, id uuid.UUID) (*Template, error) {
	t, err := s.repo.GetTemplate(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("notifications: get template %s: %w", id, err)
	}
	return t, nil
}

// ListTemplates returns templates for a tenant. Capped at 500 — a tenant
// realistically configures ≤50 templates; 500 is a generous safety ceiling.
func (s *Service) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]Template, error) {
	templates, err := s.repo.ListTemplates(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("notifications: list templates: %w", err)
	}
	const maxTemplates = 500
	if len(templates) > maxTemplates {
		templates = templates[:maxTemplates]
	}
	return templates, nil
}

// --- Delivery history ---

// GetNotification retrieves a notification log entry by ID, scoped to the
// caller's own recipient id — a member cannot read another member's inbox.
func (s *Service) GetNotification(ctx context.Context, tenantID, recipientID, id uuid.UUID) (*Notification, error) {
	n, err := s.repo.GetNotification(ctx, tenantID, recipientID, id)
	if err != nil {
		return nil, fmt.Errorf("notifications: get notification %s: %w", id, err)
	}
	return n, nil
}

// ListNotifications returns the delivery log for a tenant with optional
// filters, scoped to the caller's own recipient id — a member cannot list
// another member's inbox.
func (s *Service) ListNotifications(ctx context.Context, tenantID, recipientID uuid.UUID, channel, status string, limit, offset int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	notifications, err := s.repo.ListNotifications(ctx, tenantID, recipientID, channel, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("notifications: list notifications: %w", err)
	}
	return notifications, nil
}

// --- Helpers ---

// renderString parses and executes a Go html/template string with the given data.
// An empty template string returns an empty string without error.
func renderString(tmplStr string, data map[string]any) (string, error) {
	if tmplStr == "" {
		return "", nil
	}
	t, err := template.New("").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}

func isValidChannel(ch string) bool {
	switch ch {
	case "email", "sms", "in_app", "push":
		return true
	}
	return false
}
