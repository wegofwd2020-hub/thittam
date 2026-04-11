-- 001_create_tables.up.sql — billing service tables
-- Lives in the public (shared) schema — billing is a platform-level concern.

CREATE TABLE subscriptions (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID        NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    plan                 TEXT        NOT NULL DEFAULT 'starter'
                                     CHECK (plan IN ('starter', 'professional', 'enterprise')),
    status               TEXT        NOT NULL DEFAULT 'active'
                                     CHECK (status IN ('active', 'suspended', 'cancelled')),
    billing_cycle        TEXT        NOT NULL DEFAULT 'monthly'
                                     CHECK (billing_cycle IN ('monthly', 'annual')),
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end   TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_status ON subscriptions (status);

CREATE TABLE invoices (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL REFERENCES tenants(id),
    subscription_id UUID          NOT NULL REFERENCES subscriptions(id),
    invoice_number  TEXT          NOT NULL UNIQUE,
    amount          NUMERIC(14,2) NOT NULL,
    tax_amount      NUMERIC(14,2) NOT NULL DEFAULT 0,
    currency        TEXT          NOT NULL DEFAULT 'INR',
    status          TEXT          NOT NULL DEFAULT 'pending'
                                  CHECK (status IN ('pending', 'paid', 'failed', 'void')),
    due_date        DATE          NOT NULL,
    period_start    TIMESTAMPTZ   NOT NULL,
    period_end      TIMESTAMPTZ   NOT NULL,
    paid_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_invoices_tenant  ON invoices (tenant_id);
CREATE INDEX idx_invoices_status  ON invoices (status) WHERE status IN ('pending', 'failed');

CREATE TABLE dunning_attempts (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id       UUID        NOT NULL REFERENCES invoices(id),
    attempt_number   INT         NOT NULL,
    outcome          TEXT        NOT NULL CHECK (outcome IN ('succeeded', 'failed', 'retrying')),
    gateway_response TEXT,
    attempted_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dunning_invoice ON dunning_attempts (invoice_id);
