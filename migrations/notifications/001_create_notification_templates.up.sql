-- 001_create_notification_templates.up.sql
-- Per-tenant notification templates. One row per (tenant, event_type, channel).
-- Body is a Go html/template string rendered at delivery time.
-- System-default templates (tenant_id = nil) can be used as fallbacks,
-- but per-tenant rows always take precedence.

CREATE TABLE notification_templates (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL,
    event_type    TEXT        NOT NULL,
    channel       TEXT        NOT NULL
                              CHECK (channel IN ('email', 'sms', 'in_app', 'push')),
    subject       TEXT,
    body_template TEXT        NOT NULL,
    is_active     BOOLEAN     NOT NULL DEFAULT true,

    UNIQUE (tenant_id, event_type, channel)
);

CREATE INDEX idx_templates_tenant_event ON notification_templates (tenant_id, event_type);
CREATE INDEX idx_templates_active       ON notification_templates (tenant_id, event_type, channel)
    WHERE is_active = true;
