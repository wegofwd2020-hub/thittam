-- 001_create_accounts.up.sql
-- Chart of accounts per tenant. Accounts are hierarchical: a parent_id
-- links sub-accounts to their category account.
-- Seeded at tenant registration via the vertical's default_chart_of_accounts.

CREATE TABLE accounts (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID        NOT NULL,
    code         TEXT        NOT NULL,
    name         TEXT        NOT NULL,
    account_type TEXT        NOT NULL
                             CHECK (account_type IN ('asset', 'liability', 'equity', 'revenue', 'expense')),
    parent_id    UUID        REFERENCES accounts(id) ON DELETE RESTRICT,
    is_active    BOOLEAN     NOT NULL DEFAULT true,

    UNIQUE (tenant_id, code)
);

CREATE INDEX idx_accounts_tenant      ON accounts (tenant_id);
CREATE INDEX idx_accounts_parent      ON accounts (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_accounts_type        ON accounts (tenant_id, account_type);
