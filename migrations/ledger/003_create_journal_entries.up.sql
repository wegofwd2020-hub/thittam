-- 003_create_journal_entries.up.sql
-- Journal entry headers. Once status = 'posted' the row is immutable.
-- Corrections are made via a new reversing entry; the original is marked 'void'.
-- entry_number follows the format JE-{YEAR}-{5-digit-sequence}, e.g. JE-2024-00001.

CREATE TABLE journal_entries (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL,
    production_id UUID,
    period_id     UUID        NOT NULL REFERENCES accounting_periods(id),
    entry_number  TEXT        NOT NULL UNIQUE,
    reference     TEXT,
    narration     TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'draft'
                              CHECK (status IN ('draft', 'posted', 'void')),
    posted_by     UUID,
    posted_at     TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_je_tenant        ON journal_entries (tenant_id);
CREATE INDEX idx_je_period        ON journal_entries (period_id);
CREATE INDEX idx_je_production    ON journal_entries (production_id) WHERE production_id IS NOT NULL;
CREATE INDEX idx_je_status        ON journal_entries (tenant_id, status);
CREATE INDEX idx_je_reference     ON journal_entries (reference) WHERE reference IS NOT NULL;

-- Per-tenant per-year entry counter for generating JE-YYYY-NNNNN numbers atomically.
CREATE TABLE journal_entry_counters (
    tenant_id UUID NOT NULL,
    year      INT  NOT NULL,
    next_seq  INT  NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, year)
);
