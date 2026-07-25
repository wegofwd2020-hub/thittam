-- 002_add_expense_rejection.up.sql
-- RejectExpense (#191) records why an expense was rejected. status='rejected'
-- is already in the CHECK; only the reason + timestamp were missing.
ALTER TABLE expenses
    ADD COLUMN rejection_reason TEXT,
    ADD COLUMN rejected_at      TIMESTAMPTZ;
