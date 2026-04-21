-- 016_tenants_retention_lifecycle.up.sql
-- #92: scheduled tenant retention sweeper.
--
-- Adds the timestamp columns the state machine needs:
--
--   suspended_at     — set when the billing webhook suspends a tenant
--   deactivated_at   — set when Grace ends (60d after suspended_at)
--
-- The original design had a generated column purge_after = deactivated_at
-- + 180 days, but Postgres marks `timestamptz + interval` as STABLE rather
-- than IMMUTABLE (DST awareness), so it cannot back a GENERATED column.
-- Instead, the sweeper query compares deactivated_at <= now() - INTERVAL
-- '180 days' directly — same performance, one fewer column.
--
-- Also broadens the tenants.status CHECK to include the two new states the
-- policy in thittam_docs §1.3a requires: 'grace' (read-only after 30d of
-- suspension) and 'purge_eligible' (after the 180d retention window).
--
-- Partial indexes support the sweeper's three transition queries:
--
--   1. status='suspended'       + suspended_at   older than 30d  → grace
--   2. status='grace'           + suspended_at   older than 90d  → deactivated
--   3. status='deactivated'     + deactivated_at older than 180d → purge_eligible
--
-- All three are cheap because only a small minority of tenants are ever in
-- those transient states.

-- Broaden the status enum. CHECK constraints cannot be altered in place, so
-- drop + recreate. Safe: no rows use the new values yet.
ALTER TABLE tenants
    DROP CONSTRAINT IF EXISTS tenants_status_check;

ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active', 'suspended', 'grace', 'deactivated', 'purge_eligible'));

ALTER TABLE tenants
    ADD COLUMN suspended_at   TIMESTAMPTZ,
    ADD COLUMN deactivated_at TIMESTAMPTZ;

CREATE INDEX idx_tenants_suspended_at_due
    ON tenants (suspended_at)
    WHERE status = 'suspended';

CREATE INDEX idx_tenants_grace_due
    ON tenants (suspended_at)
    WHERE status = 'grace';

CREATE INDEX idx_tenants_deactivated_at_due
    ON tenants (deactivated_at)
    WHERE status = 'deactivated';

-- Backfill timestamps for tenants that are already in a non-active state.
-- Without this, the first sweeper run would treat them as "just suspended"
-- with NULL clock and skip them indefinitely. Setting suspended_at = now()
-- for existing suspended rows gives operators a fresh 30-day grace window
-- to inspect and adjust before any automatic transitions kick in.
UPDATE tenants
   SET suspended_at = COALESCE(suspended_at, now())
 WHERE status = 'suspended';

UPDATE tenants
   SET deactivated_at = COALESCE(deactivated_at, now())
 WHERE status = 'deactivated';
