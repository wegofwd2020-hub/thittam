-- 017_tenants_legal_hold.up.sql
-- #92 Stage 4: legal-hold columns on tenants.
--
-- SuspendTenant grows optional hold_until + freeze_reason parameters so a
-- platform admin can pause automated retention-lifecycle transitions (the
-- sweeper introduced in migration 016 / #92 Stages 1+2) for litigation
-- holds, regulatory freezes, or similar.
--
-- Semantics:
--   freeze_reason IS NOT NULL  → tenant is on legal hold
--   hold_until                 → optional auto-release time;
--                                NULL means indefinite (admin must clear)
--
-- The sweeper's ListTenantsDueForLifecycle query excludes rows where the
-- hold is still active:
--
--   AND (freeze_reason IS NULL
--        OR (hold_until IS NOT NULL AND hold_until <= now()))
--
-- An indefinite hold therefore pins the tenant in its current transient
-- state forever, exactly what a legal hold requires.
--
-- No new index: the sweeper's existing partial indexes on suspended_at /
-- deactivated_at already narrow the row set to a handful of candidates;
-- the hold predicate is a cheap filter on top of that. Adding an index
-- would cost more to maintain than it would save on a query that already
-- returns <1% of the table.
--
-- No CHECK constraint linking the two columns: indefinite holds
-- (freeze_reason set, hold_until NULL) are a valid and common case.

ALTER TABLE tenants
    ADD COLUMN hold_until    TIMESTAMPTZ,
    ADD COLUMN freeze_reason TEXT;
