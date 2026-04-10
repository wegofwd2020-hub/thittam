-- 003_create_projection_watermarks.up.sql
-- Tracks the high-water-mark (most-recent event time) per NATS subject for the
-- reporting-analytics projection consumer.
--
-- Purpose: LagChecker reads OldestWatermark() to determine whether the
-- reporting projections are within the 30-second SLA. If the oldest watermark
-- is > 30 s behind wall clock, /readyz returns 503 and the pod is removed from
-- rotation until the consumer catches up.
--
-- This table lives in the platform schema (not a tenant schema) because
-- projection lag is a service-level health concern, not a tenant data concern.

CREATE TABLE IF NOT EXISTS projection_watermarks (
    -- NATS subject (e.g. "thittam.expense.approved"). One row per subject.
    subject       TEXT        PRIMARY KEY,

    -- Timestamp of the most-recently processed event on this subject.
    last_event_at TIMESTAMPTZ NOT NULL,

    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
