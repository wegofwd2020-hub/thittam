-- 002_create_dashboard_views.up.sql
-- Materialized view tables for executive dashboard.
-- These are populated by NATS event consumers and refreshed near real-time.

-- Portfolio snapshot: one row per active project per tenant.
CREATE TABLE portfolio_snapshots (
    tenant_id     UUID        NOT NULL,
    project_id    UUID        NOT NULL,
    title         TEXT        NOT NULL,
    status        TEXT        NOT NULL,
    current_phase TEXT,
    budget_total  NUMERIC(14,2) NOT NULL DEFAULT 0,
    actual_spend  NUMERIC(14,2) NOT NULL DEFAULT 0,
    committed     NUMERIC(14,2) NOT NULL DEFAULT 0,
    start_date    DATE,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant_id, project_id)
);

CREATE INDEX idx_portfolio_tenant ON portfolio_snapshots (tenant_id);

-- Monthly spend aggregation for trend charts.
CREATE TABLE monthly_spend (
    tenant_id    UUID        NOT NULL,
    month        TEXT        NOT NULL, -- "2026-01" format
    actual       NUMERIC(14,2) NOT NULL DEFAULT 0,
    budget       NUMERIC(14,2) NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant_id, month)
);

-- Category spend breakdown.
CREATE TABLE category_spend (
    tenant_id     UUID        NOT NULL,
    category_id   TEXT        NOT NULL,
    category_name TEXT        NOT NULL,
    budgeted      NUMERIC(14,2) NOT NULL DEFAULT 0,
    actual        NUMERIC(14,2) NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant_id, category_id)
);

-- Pending approval items (refreshed by event consumers).
CREATE TABLE pending_approvals (
    id            UUID        PRIMARY KEY,
    tenant_id     UUID        NOT NULL,
    item_type     TEXT        NOT NULL CHECK (item_type IN ('expense', 'budget')),
    description   TEXT        NOT NULL,
    amount        NUMERIC(14,2) NOT NULL,
    submitted_by  TEXT        NOT NULL,
    submitted_at  TIMESTAMPTZ NOT NULL,
    project_title TEXT,

    CONSTRAINT unique_pending UNIQUE (tenant_id, id)
);

CREATE INDEX idx_pending_tenant ON pending_approvals (tenant_id, submitted_at DESC);

-- Team assignment snapshot.
CREATE TABLE team_assignments (
    tenant_id    UUID NOT NULL,
    project_id   UUID NOT NULL,
    project_title TEXT NOT NULL,
    member_count INT  NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant_id, project_id)
);

-- Compliance flags.
CREATE TABLE compliance_flags (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL,
    flag_type     TEXT        NOT NULL,
    description   TEXT        NOT NULL,
    severity      TEXT        NOT NULL CHECK (severity IN ('warning', 'critical')),
    project_title TEXT,
    detected_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_compliance_tenant ON compliance_flags (tenant_id, detected_at DESC);
