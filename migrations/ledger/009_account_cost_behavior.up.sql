-- 009_account_cost_behavior.up.sql
-- Fixed/variable classification for expense accounts. NULL means unclassified;
-- the monthly cost-behavior report buckets those separately so totals reconcile.
ALTER TABLE accounts
    ADD COLUMN cost_behavior TEXT NULL CHECK (cost_behavior IN ('fixed', 'variable'));
