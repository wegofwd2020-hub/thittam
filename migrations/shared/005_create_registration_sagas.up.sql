-- Registration sagas table.
-- Stores durable state for the 9-step tenant registration pipeline.
-- Lives in the platform (shared) schema — NOT a tenant schema.
--
-- Each row is written after every step transition so that a process crash
-- or network partition leaves a recoverable record. The orchestrator reads
-- this table on Resume to skip already-completed steps.

CREATE TABLE IF NOT EXISTS registration_sagas (
    id            UUID        PRIMARY KEY,

    -- Deduplication key: only one non-terminal saga per email is permitted.
    email         TEXT        NOT NULL,

    -- Set as steps 1 and 2 complete. NULL until the corresponding step runs.
    tenant_id     UUID,
    user_id       UUID,

    status        TEXT        NOT NULL
                              CHECK (status IN (
                                  'in_progress',
                                  'completed',
                                  'failed',
                                  'compensating',
                                  'compensated',
                                  'compensation_failed'
                              )),

    -- JSON array of step names that have committed successfully.
    -- Used by Resume to skip already-completed steps on retry.
    completed_steps  JSONB    NOT NULL DEFAULT '[]',

    -- Step that caused the transition to "failed". NULL for successful sagas.
    failed_step      TEXT,
    failure_reason   TEXT,

    -- Errors from compensating transactions. Non-empty only on compensation_failed.
    compensation_errors  JSONB NOT NULL DEFAULT '[]',

    -- Full original request stored for idempotent resume without caller re-supplying input.
    request       JSONB       NOT NULL,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fast lookup by email for deduplication on every Start call.
CREATE INDEX IF NOT EXISTS idx_registration_sagas_email
    ON registration_sagas (email);

-- Fast lookup of in-progress sagas for the background reconciler.
CREATE INDEX IF NOT EXISTS idx_registration_sagas_status
    ON registration_sagas (status)
    WHERE status NOT IN ('completed', 'compensated');
