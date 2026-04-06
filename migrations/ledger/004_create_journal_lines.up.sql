-- 004_create_journal_lines.up.sql
-- Journal entry line items. Each line is either a debit or a credit — never both.
-- The CHECK constraint enforces this at the DB level; the service enforces
-- that sum(debit) = sum(credit) across all lines for a given entry before posting.

CREATE TABLE journal_lines (
    id            UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    journal_id    UUID           NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id    UUID           NOT NULL REFERENCES accounts(id),
    debit_amount  NUMERIC(14,2)  NOT NULL DEFAULT 0,
    credit_amount NUMERIC(14,2)  NOT NULL DEFAULT 0,
    currency      CHAR(3)        NOT NULL DEFAULT 'INR',
    description   TEXT,

    CONSTRAINT dr_or_cr CHECK (
        (debit_amount > 0 AND credit_amount = 0) OR
        (credit_amount > 0 AND debit_amount = 0)
    )
);

CREATE INDEX idx_lines_journal  ON journal_lines (journal_id);
CREATE INDEX idx_lines_account  ON journal_lines (account_id);
