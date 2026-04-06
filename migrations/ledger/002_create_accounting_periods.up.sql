-- 002_create_accounting_periods.up.sql
-- One row per calendar month per tenant. Posting to a closed period is rejected
-- at the service layer. Closing is a manual approval-gated action.

CREATE TABLE accounting_periods (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    year       INT         NOT NULL CHECK (year >= 2000),
    month      INT         NOT NULL CHECK (month BETWEEN 1 AND 12),
    status     TEXT        NOT NULL DEFAULT 'open'
                           CHECK (status IN ('open', 'closed')),
    closed_by  UUID,
    closed_at  TIMESTAMPTZ,

    UNIQUE (tenant_id, year, month)
);

CREATE INDEX idx_periods_tenant ON accounting_periods (tenant_id);
CREATE INDEX idx_periods_open   ON accounting_periods (tenant_id, status) WHERE status = 'open';
