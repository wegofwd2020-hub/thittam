-- Notifications service queries.

-- name: GetTemplate :one
SELECT * FROM notification_templates
WHERE tenant_id = $1 AND event_type = $2 AND channel = $3 AND is_active = true;

-- name: UpsertTemplate :one
INSERT INTO notification_templates (id, tenant_id, event_type, channel, subject, body_template, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, event_type, channel) DO UPDATE
    SET subject       = EXCLUDED.subject,
        body_template = EXCLUDED.body_template,
        is_active     = EXCLUDED.is_active
RETURNING *;

-- name: ListTemplates :many
SELECT * FROM notification_templates
WHERE tenant_id = $1 AND ($2 = '' OR event_type = $2)
ORDER BY event_type, channel;

-- name: CreateNotificationLog :one
INSERT INTO notification_log (id, tenant_id, recipient_id, channel, event_type, subject, status)
VALUES ($1, $2, $3, $4, $5, $6, 'pending')
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: UpdateNotificationLogSent :exec
UPDATE notification_log
SET status          = 'sent',
    provider_msg_id = $2,
    sent_at         = now()
WHERE id = $1;

-- Correct unscoped (#159/#172): `id` is never caller-supplied. The only path
-- here is Service.Send → UpdateNotificationStatus, passing the id of a
-- notification the service itself just created. Contrast IncrementRetryCount,
-- which was reachable by arbitrary id and so carries a tenant predicate plus a
-- RowsAffected check. If this ever gains a caller that accepts an id from a
-- request, it needs the same treatment.
-- name: UpdateNotificationLogFailed :exec
UPDATE notification_log
SET status        = 'failed',
    error_message = $2,
    retry_count   = retry_count + 1
WHERE id = $1;

