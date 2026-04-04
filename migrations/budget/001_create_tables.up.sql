-- 001_create_tables.up.sql — budget-planning service tables
-- These tables live in the tenant-scoped schema (tenant_<uuid>).

CREATE TABLE budgets (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    production_id UUID        NOT NULL,
    tenant_id     UUID        NOT NULL,
    label         TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'draft'
                              CHECK (status IN ('draft','submitted','approved','locked')),
    currency      TEXT        NOT NULL DEFAULT 'INR',
    total_amount  NUMERIC(14,2) NOT NULL DEFAULT 0,
    submitted_by  UUID,
    approved_by   UUID,
    submitted_at  TIMESTAMPTZ,
    approved_at   TIMESTAMPTZ,
    created_by    UUID        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_budgets_tenant ON budgets (tenant_id);
CREATE INDEX idx_budgets_production ON budgets (production_id);
CREATE INDEX idx_budgets_status ON budgets (tenant_id, status);

CREATE TABLE budget_line_items (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id        UUID        NOT NULL REFERENCES budgets(id) ON DELETE CASCADE,
    tenant_id        UUID        NOT NULL,
    category_id      TEXT        NOT NULL,
    description      TEXT        NOT NULL,
    account_code     TEXT        NOT NULL,
    budgeted_amount  NUMERIC(14,2) NOT NULL DEFAULT 0,
    actual_amount    NUMERIC(14,2) NOT NULL DEFAULT 0,
    committed_amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    is_locked        BOOLEAN     NOT NULL DEFAULT false,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_line_items_budget ON budget_line_items (budget_id);
CREATE INDEX idx_line_items_category ON budget_line_items (budget_id, category_id);
