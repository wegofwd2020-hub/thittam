-- 005_add_bank_account_details.up.sql
-- Bank accounts carry bank name, last 4 digits of the account number, and
-- currency. The full account number is never stored.
ALTER TABLE accounts
    ADD COLUMN bank_name     TEXT,
    ADD COLUMN account_last4 TEXT,
    ADD COLUMN bank_currency TEXT,
    ADD CONSTRAINT accounts_bank_last4_format
        CHECK (account_last4 IS NULL OR account_last4 ~ '^[0-9]{4}$'),
    ADD CONSTRAINT accounts_bank_currency_format
        CHECK (bank_currency IS NULL OR bank_currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT accounts_bank_fields_complete
        CHECK ((bank_name IS NULL) = (account_last4 IS NULL)
           AND (bank_name IS NULL) = (bank_currency IS NULL));
