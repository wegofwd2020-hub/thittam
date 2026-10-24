-- 009_account_cost_behavior.down.sql
ALTER TABLE accounts DROP COLUMN IF EXISTS cost_behavior;
