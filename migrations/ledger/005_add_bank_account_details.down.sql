-- 005_add_bank_account_details.down.sql
ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_bank_fields_complete,
    DROP CONSTRAINT IF EXISTS accounts_bank_currency_format,
    DROP CONSTRAINT IF EXISTS accounts_bank_last4_format,
    DROP COLUMN IF EXISTS bank_currency,
    DROP COLUMN IF EXISTS account_last4,
    DROP COLUMN IF EXISTS bank_name;
