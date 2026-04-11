package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wegofwd2020/thittam/services/notifications"
)

// Postgres implements notifications.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed notifications repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ notifications.Repository = (*Postgres)(nil)

// --- Templates ---

func (p *Postgres) CreateTemplate(ctx context.Context, tmpl *notifications.Template) error {
	_, err := p.q.UpsertTemplate(ctx, UpsertTemplateParams{
		ID:           tmpl.ID,
		TenantID:     tmpl.TenantID,
		EventType:    tmpl.EventType,
		Channel:      tmpl.Channel,
		Subject:      pgTextFromString(tmpl.Subject),
		BodyTemplate: tmpl.BodyTemplate,
		IsActive:     tmpl.IsActive,
	})
	if err != nil {
		return fmt.Errorf("notifications: create template: %w", err)
	}
	return nil
}

func (p *Postgres) UpdateTemplate(ctx context.Context, tmpl *notifications.Template) error {
	const sql = `UPDATE notification_templates
		SET subject = $3, body_template = $4, is_active = $5
		WHERE id = $1 AND tenant_id = $2`
	tag, err := p.db.Exec(ctx, sql,
		tmpl.ID,
		tmpl.TenantID,
		pgTextFromString(tmpl.Subject),
		tmpl.BodyTemplate,
		tmpl.IsActive,
	)
	if err != nil {
		return fmt.Errorf("notifications: update template: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return notifications.ErrTemplateNotFound
	}
	return nil
}

func (p *Postgres) GetTemplate(ctx context.Context, tenantID, id uuid.UUID) (*notifications.Template, error) {
	const sql = `SELECT id, tenant_id, event_type, channel, subject, body_template, is_active
		FROM notification_templates WHERE id = $1 AND tenant_id = $2`

	var row NotificationTemplate
	err := p.db.QueryRow(ctx, sql, id, tenantID).Scan(
		&row.ID,
		&row.TenantID,
		&row.EventType,
		&row.Channel,
		&row.Subject,
		&row.BodyTemplate,
		&row.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notifications.ErrTemplateNotFound
		}
		return nil, fmt.Errorf("notifications: get template: %w", err)
	}
	return templateFromDB(row), nil
}

func (p *Postgres) GetActiveTemplate(ctx context.Context, tenantID uuid.UUID, eventType, channel string) (*notifications.Template, error) {
	row, err := p.q.GetTemplate(ctx, GetTemplateParams{
		TenantID:  tenantID,
		EventType: eventType,
		Channel:   channel,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notifications.ErrTemplateNotFound
		}
		return nil, fmt.Errorf("notifications: get active template: %w", err)
	}
	return templateFromDB(row), nil
}

func (p *Postgres) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]notifications.Template, error) {
	rows, err := p.q.ListTemplates(ctx, ListTemplatesParams{
		TenantID: tenantID,
		Column2:  "", // no event_type filter
	})
	if err != nil {
		return nil, fmt.Errorf("notifications: list templates: %w", err)
	}
	result := make([]notifications.Template, len(rows))
	for i, row := range rows {
		result[i] = *templateFromDB(row)
	}
	return result, nil
}

// --- Delivery log ---

func (p *Postgres) CreateNotification(ctx context.Context, n *notifications.Notification) error {
	_, err := p.q.CreateNotificationLog(ctx, CreateNotificationLogParams{
		ID:          n.ID,
		TenantID:    n.TenantID,
		RecipientID: n.RecipientID,
		Channel:     n.Channel,
		EventType:   n.EventType,
		Subject:     pgTextFromString(n.Subject),
	})
	if err != nil {
		return fmt.Errorf("notifications: create notification log: %w", err)
	}
	return nil
}

func (p *Postgres) UpdateNotificationStatus(ctx context.Context, id uuid.UUID, status, providerMsgID, errMsg string, sentAt *time.Time) error {
	switch status {
	case "sent":
		return p.q.UpdateNotificationLogSent(ctx, UpdateNotificationLogSentParams{
			ID:            id,
			ProviderMsgID: pgTextFromString(providerMsgID),
		})
	case "failed":
		return p.q.UpdateNotificationLogFailed(ctx, UpdateNotificationLogFailedParams{
			ID:           id,
			ErrorMessage: pgTextFromString(errMsg),
		})
	default:
		// For "delivered", "pending", or any other status — generic update.
		const sql = `UPDATE notification_log SET status = $2 WHERE id = $1`
		_, err := p.db.Exec(ctx, sql, id, status)
		if err != nil {
			return fmt.Errorf("notifications: update notification status: %w", err)
		}
		return nil
	}
}

func (p *Postgres) GetNotification(ctx context.Context, tenantID, id uuid.UUID) (*notifications.Notification, error) {
	const sql = `SELECT id, tenant_id, recipient_id, channel, event_type, subject,
		provider_msg_id, status, retry_count, error_message, sent_at, delivered_at, created_at
		FROM notification_log WHERE id = $1 AND tenant_id = $2`

	var row NotificationLog
	err := p.db.QueryRow(ctx, sql, id, tenantID).Scan(
		&row.ID,
		&row.TenantID,
		&row.RecipientID,
		&row.Channel,
		&row.EventType,
		&row.Subject,
		&row.ProviderMsgID,
		&row.Status,
		&row.RetryCount,
		&row.ErrorMessage,
		&row.SentAt,
		&row.DeliveredAt,
		&row.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notifications.ErrNotificationNotFound
		}
		return nil, fmt.Errorf("notifications: get notification: %w", err)
	}
	return notificationFromDB(row), nil
}

func (p *Postgres) ListNotifications(ctx context.Context, tenantID uuid.UUID, channel, status string, limit, offset int) ([]notifications.Notification, error) {
	const sql = `SELECT id, tenant_id, recipient_id, channel, event_type, subject,
		provider_msg_id, status, retry_count, error_message, sent_at, delivered_at, created_at
		FROM notification_log
		WHERE tenant_id = $1
		  AND ($2 = '' OR channel = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5`

	rows, err := p.db.Query(ctx, sql, tenantID, channel, status, int32(limit), int32(offset))
	if err != nil {
		return nil, fmt.Errorf("notifications: list notifications: %w", err)
	}
	defer rows.Close()

	var result []notifications.Notification
	for rows.Next() {
		var row NotificationLog
		if err := rows.Scan(
			&row.ID,
			&row.TenantID,
			&row.RecipientID,
			&row.Channel,
			&row.EventType,
			&row.Subject,
			&row.ProviderMsgID,
			&row.Status,
			&row.RetryCount,
			&row.ErrorMessage,
			&row.SentAt,
			&row.DeliveredAt,
			&row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("notifications: scan notification: %w", err)
		}
		result = append(result, *notificationFromDB(row))
	}
	return result, rows.Err()
}

func (p *Postgres) IncrementRetryCount(ctx context.Context, id uuid.UUID) error {
	_, err := p.db.Exec(ctx, "UPDATE notification_log SET retry_count = retry_count + 1 WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("notifications: increment retry count: %w", err)
	}
	return nil
}

// --- model conversion helpers ---

func templateFromDB(row NotificationTemplate) *notifications.Template {
	return &notifications.Template{
		ID:           row.ID,
		TenantID:     row.TenantID,
		EventType:    row.EventType,
		Channel:      row.Channel,
		Subject:      stringFromPgText(row.Subject),
		BodyTemplate: row.BodyTemplate,
		IsActive:     row.IsActive,
	}
}

func notificationFromDB(row NotificationLog) *notifications.Notification {
	n := &notifications.Notification{
		ID:            row.ID,
		TenantID:      row.TenantID,
		RecipientID:   row.RecipientID,
		Channel:       row.Channel,
		EventType:     row.EventType,
		Subject:       stringFromPgText(row.Subject),
		Status:        row.Status,
		RetryCount:    int(row.RetryCount),
		ProviderMsgID: stringFromPgText(row.ProviderMsgID),
		ErrorMessage:  stringFromPgText(row.ErrorMessage),
		CreatedAt:     row.CreatedAt,
	}
	if row.SentAt.Valid {
		t := row.SentAt.Time
		n.SentAt = &t
	}
	if row.DeliveredAt.Valid {
		t := row.DeliveredAt.Time
		n.DeliveredAt = &t
	}
	return n
}

// --- pgtype conversion helpers ---

func pgTextFromString(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func stringFromPgText(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}
