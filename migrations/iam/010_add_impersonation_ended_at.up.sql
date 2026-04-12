-- 010_add_impersonation_ended_at.up.sql
-- Adds ended_at to track when an impersonation session was explicitly terminated
-- (via EndImpersonation RPC) or expired by the background cleanup ticker.
-- NULL means the session is still active (or has not yet been cleaned up).

ALTER TABLE platform_impersonation_log
    ADD COLUMN ended_at TIMESTAMPTZ;

-- Partial index for the expiry ticker query: only rows where the session is
-- still active AND the TTL has passed. Keeps the ticker query sub-millisecond
-- even with millions of historical rows.
CREATE INDEX idx_impersonation_active_expired
    ON platform_impersonation_log (expires_at)
    WHERE ended_at IS NULL;
