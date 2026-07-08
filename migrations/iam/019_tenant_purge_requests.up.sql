-- PurgeTenant (#92 Stage 3): two-person approval record for hard-deleting a
-- purge_eligible tenant, plus the tombstone column on tenants.

CREATE TABLE tenant_purge_requests (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending','approved','executed','failed','cancelled')),
    requested_by   UUID        NOT NULL,
    requested_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_reason TEXT        NOT NULL,
    approved_by    UUID,
    approved_at    TIMESTAMPTZ,
    cancelled_by   UUID,
    cancelled_at   TIMESTAMPTZ,
    executed_at    TIMESTAMPTZ,
    failure_reason TEXT,
    -- Forensic snapshot captured at request time (the tombstone overwrites the
    -- live tenant name with a sentinel).
    tenant_name    TEXT        NOT NULL,
    tenant_slug    TEXT        NOT NULL
);

-- At most one OPEN (pending|approved) request per tenant.
CREATE UNIQUE INDEX tenant_purge_requests_one_open
    ON tenant_purge_requests (tenant_id)
    WHERE status IN ('pending', 'approved');

-- Worker poll: approved requests, oldest first.
CREATE INDEX idx_tenant_purge_requests_approved
    ON tenant_purge_requests (approved_at)
    WHERE status = 'approved';

-- Tombstone marker on the retained tenants row.
ALTER TABLE tenants ADD COLUMN purged_at TIMESTAMPTZ;

-- Terminal 'purged' state.
ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active','suspended','grace','deactivated','purge_eligible','purged'));
