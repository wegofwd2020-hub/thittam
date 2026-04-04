-- 001_create_tables.up.sql — reporting-analytics service tables
-- These tables live in the tenant-scoped schema (tenant_<uuid>).
-- Fact tables are populated via NATS JetStream events from other services.

CREATE TABLE expense_facts (
    expense_id     UUID        PRIMARY KEY,
    production_id  UUID        NOT NULL,
    tenant_id      UUID        NOT NULL,
    budget_line_id UUID,
    category_id    TEXT        NOT NULL,
    amount         NUMERIC(14,2) NOT NULL,
    approved_at    TIMESTAMPTZ
);

CREATE INDEX idx_expense_facts_tenant ON expense_facts (tenant_id);
CREATE INDEX idx_expense_facts_production ON expense_facts (production_id);
CREATE INDEX idx_expense_facts_category ON expense_facts (tenant_id, category_id);

CREATE TABLE budget_facts (
    budget_id       UUID        PRIMARY KEY,
    production_id   UUID        NOT NULL,
    tenant_id       UUID        NOT NULL,
    total_budgeted  NUMERIC(14,2) NOT NULL DEFAULT 0,
    total_actual    NUMERIC(14,2) NOT NULL DEFAULT 0,
    total_committed NUMERIC(14,2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_budget_facts_tenant ON budget_facts (tenant_id);
CREATE INDEX idx_budget_facts_production ON budget_facts (production_id);
