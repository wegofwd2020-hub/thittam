-- 003_event_outbox_dead.up.sql — dead-letter queue for the billing outbox (#134).
-- A row lands here when the relay fails to publish it maxOutboxAttempts times
-- while at least one batch-mate succeeded — i.e. the event is poison, not the
-- victim of a NATS outage. Rows move OUT of event_outbox so the relay's claim
-- query and its partial index never see them. Replay is human-triggered via
-- cmd/outbox-admin; nothing drains this table automatically, by design.
CREATE TABLE event_outbox_dead (
    id         UUID        PRIMARY KEY,          -- carried from event_outbox
    subject    TEXT        NOT NULL,
    tenant_id  UUID        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,             -- original enqueue time, preserved
    attempts   INTEGER     NOT NULL,
    last_error TEXT,
    died_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Operator queries list most-recently-parked events first.
CREATE INDEX idx_event_outbox_dead_died_at ON event_outbox_dead (died_at);
