-- 003_add_expense_rejected_by.up.sql
-- #201: record WHO rejected an expense (mirrors approved_by). Plain UUID, no FK,
-- matching approved_by / submitted_by precedent in 001.
ALTER TABLE expenses
    ADD COLUMN rejected_by UUID;
