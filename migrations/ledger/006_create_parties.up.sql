-- 006_create_parties.up.sql
-- Customers and vendors. A party is a customer, a supplier, or both. Contact
-- and tax fields are plain text; no monetary values live on this table.
CREATE TABLE parties (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID        NOT NULL,
    name             TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    party_type       TEXT        NOT NULL CHECK (party_type IN ('customer', 'supplier', 'both')),
    email            TEXT        NOT NULL DEFAULT '',
    phone            TEXT        NOT NULL DEFAULT '',
    tax_id           TEXT        NOT NULL DEFAULT '',
    address          TEXT        NOT NULL DEFAULT '',
    default_currency TEXT        CHECK (default_currency IS NULL OR default_currency ~ '^[A-Z]{3}$'),
    is_active        BOOLEAN     NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Names are unique per tenant, ignoring case.
CREATE UNIQUE INDEX idx_parties_tenant_name ON parties (tenant_id, lower(name));
CREATE INDEX idx_parties_tenant_type ON parties (tenant_id, party_type);
