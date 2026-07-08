-- 001_create_tables.up.sql — billing service tables
-- Lives in the public (shared) schema — billing is a platform-level concern.
-- Schema matches services/billing/db/postgres.go (the live raw-SQL repository).

CREATE TABLE subscriptions (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID        NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,
    plan                 TEXT        NOT NULL DEFAULT 'starter'
                                     CHECK (plan IN ('starter', 'professional', 'enterprise')),
    status               TEXT        NOT NULL DEFAULT 'active'
                                     CHECK (status IN ('trialing', 'active', 'past_due', 'cancelled', 'suspended')),
    billing_cycle        TEXT        NOT NULL DEFAULT 'monthly'
                                     CHECK (billing_cycle IN ('monthly', 'annual')),
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end   TIMESTAMPTZ NOT NULL,
    trial_ends_at        TIMESTAMPTZ,
    cancelled_at         TIMESTAMPTZ,
    suspended_at         TIMESTAMPTZ,
    razorpay_sub_id      TEXT,
    stripe_sub_id        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_status ON subscriptions (status);

CREATE TABLE invoices (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL REFERENCES tenants(id),
    subscription_id UUID          NOT NULL REFERENCES subscriptions(id),
    invoice_number  TEXT          NOT NULL UNIQUE,
    plan            TEXT          NOT NULL,
    amount          NUMERIC(14,2) NOT NULL,
    tax_amount      NUMERIC(14,2) NOT NULL DEFAULT 0,
    total_amount    NUMERIC(14,2) NOT NULL,
    currency        TEXT          NOT NULL DEFAULT 'INR',
    status          TEXT          NOT NULL DEFAULT 'pending'
                                  CHECK (status IN ('pending', 'paid', 'overdue', 'void', 'failed')),
    due_date        DATE          NOT NULL,
    period_start    TIMESTAMPTZ   NOT NULL,
    period_end      TIMESTAMPTZ   NOT NULL,
    paid_at         TIMESTAMPTZ,
    payment_method  TEXT,
    gateway_txn_id  TEXT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_invoices_tenant ON invoices (tenant_id);
-- Partial index: only actionable/unpaid states need indexing (paid/void excluded intentionally).
CREATE INDEX idx_invoices_status ON invoices (status) WHERE status IN ('pending', 'failed', 'overdue');

CREATE TABLE payment_methods (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type           TEXT        NOT NULL,
    display_name   TEXT        NOT NULL,
    is_default     BOOLEAN     NOT NULL DEFAULT false,
    razorpay_token TEXT,
    stripe_pm_id   TEXT,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_methods_tenant ON payment_methods (tenant_id);

CREATE TABLE usage_records (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID          NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period_start       TIMESTAMPTZ   NOT NULL,
    period_end         TIMESTAMPTZ   NOT NULL,
    active_productions INT           NOT NULL,
    user_count         INT           NOT NULL,
    storage_gb         NUMERIC(14,2) NOT NULL,
    recorded_at        TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_usage_records_tenant ON usage_records (tenant_id);

CREATE TABLE invoice_sequences (
    year     INT PRIMARY KEY,
    last_seq INT NOT NULL DEFAULT 0
);

CREATE TABLE dunning_attempts (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id       UUID        NOT NULL REFERENCES invoices(id),
    attempt_number   INT         NOT NULL,
    outcome          TEXT        NOT NULL
                                 CHECK (outcome IN ('success', 'failed', 'card_declined', 'insufficient_funds')),
    gateway_response TEXT,
    next_retry_at    TIMESTAMPTZ,
    attempted_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dunning_invoice ON dunning_attempts (invoice_id);
