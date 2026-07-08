-- 002_event_outbox.up.sql — transactional outbox for billing domain events (#126).
-- Rows are written in the same tx as the domain change; an in-process relay in
-- cmd/billing publishes them and marks sent_at. Generic (subject column) so any
-- billing event can use it.
CREATE TABLE event_outbox (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    subject    TEXT        NOT NULL,
    tenant_id  UUID        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ,
    attempts   INTEGER     NOT NULL DEFAULT 0,
    last_error TEXT
);

-- The relay's claim query: unsent rows, oldest first.
CREATE INDEX idx_event_outbox_unsent ON event_outbox (created_at) WHERE sent_at IS NULL;
