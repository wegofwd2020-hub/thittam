-- 001_create_tables.up.sql — expense-tracking service tables
-- These tables live in the tenant-scoped schema (tenant_<uuid>).

CREATE TABLE purchase_orders (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    production_id   UUID        NOT NULL,
    tenant_id       UUID        NOT NULL,
    budget_line_id  UUID,
    po_number       TEXT        NOT NULL UNIQUE,
    vendor_name     TEXT        NOT NULL,
    vendor_gstin    TEXT,
    description     TEXT,
    amount          NUMERIC(14,2) NOT NULL,
    currency        TEXT        NOT NULL DEFAULT 'INR',
    status          TEXT        NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','approved','partially_invoiced','closed','cancelled')),
    raised_by       UUID        NOT NULL,
    approved_by     UUID,
    raised_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_po_tenant ON purchase_orders (tenant_id);
CREATE INDEX idx_po_production ON purchase_orders (production_id);
CREATE INDEX idx_po_status ON purchase_orders (tenant_id, status);

CREATE TABLE expenses (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    production_id     UUID        NOT NULL,
    tenant_id         UUID        NOT NULL,
    budget_line_id    UUID,
    purchase_order_id UUID        REFERENCES purchase_orders(id),
    category_id       TEXT        NOT NULL,
    description       TEXT,
    amount            NUMERIC(14,2) NOT NULL,
    currency          TEXT        NOT NULL DEFAULT 'INR',
    tax_amount        NUMERIC(14,2) NOT NULL DEFAULT 0,
    status            TEXT        NOT NULL DEFAULT 'draft'
                      CHECK (status IN ('draft','submitted','approved','rejected','paid')),
    submitted_by      UUID        NOT NULL,
    approved_by       UUID,
    submitted_at      TIMESTAMPTZ,
    approved_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_expenses_tenant ON expenses (tenant_id);
CREATE INDEX idx_expenses_production ON expenses (production_id);
CREATE INDEX idx_expenses_status ON expenses (tenant_id, status);
CREATE INDEX idx_expenses_po ON expenses (purchase_order_id);

CREATE TABLE petty_cash_advances (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    production_id  UUID        NOT NULL,
    tenant_id      UUID        NOT NULL,
    issued_to      UUID        NOT NULL,
    amount         NUMERIC(14,2) NOT NULL,
    purpose        TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'issued'
                   CHECK (status IN ('issued','partially_settled','settled')),
    issued_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at     TIMESTAMPTZ,
    unspent_amount NUMERIC(14,2) NOT NULL DEFAULT 0
);

CREATE INDEX idx_petty_cash_tenant ON petty_cash_advances (tenant_id);
CREATE INDEX idx_petty_cash_production ON petty_cash_advances (production_id);
CREATE INDEX idx_petty_cash_status ON petty_cash_advances (tenant_id, status);
