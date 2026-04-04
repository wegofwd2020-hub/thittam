-- 005_add_is_demo_to_tenants.down.sql
DROP INDEX IF EXISTS idx_tenants_demo;
ALTER TABLE tenants DROP COLUMN IF EXISTS is_demo;
