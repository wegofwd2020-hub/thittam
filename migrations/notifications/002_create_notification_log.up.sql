-- 002_create_notification_log.up.sql
-- Delivery log for every notification attempt. Kept for audit and retry purposes.
-- PII in this table (recipient contacts) must be masked after 90 days per GDPR.
-- Failed rows with status='failed' and retry_count >= 4 are considered dead-lettered.

CREATE TABLE notification_log (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL,
    recipient_id    UUID        NOT NULL,
    channel         TEXT        NOT NULL
                                CHECK (channel IN ('email', 'sms', 'in_app', 'push')),
    event_type      TEXT        NOT NULL,
    subject         TEXT,
    provider_msg_id TEXT,
    status          TEXT        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'sent', 'delivered', 'failed')),
    retry_count     INT         NOT NULL DEFAULT 0,
    error_message   TEXT,
    sent_at         TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notif_log_tenant      ON notification_log (tenant_id);
CREATE INDEX idx_notif_log_recipient   ON notification_log (recipient_id);
CREATE INDEX idx_notif_log_status      ON notification_log (status) WHERE status IN ('pending', 'failed');
CREATE INDEX idx_notif_log_event       ON notification_log (tenant_id, event_type);
