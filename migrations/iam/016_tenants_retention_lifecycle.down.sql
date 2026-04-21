-- 016_tenants_retention_lifecycle.down.sql

DROP INDEX IF EXISTS idx_tenants_deactivated_at_due;
DROP INDEX IF EXISTS idx_tenants_grace_due;
DROP INDEX IF EXISTS idx_tenants_suspended_at_due;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS deactivated_at,
    DROP COLUMN IF EXISTS suspended_at;

ALTER TABLE tenants
    DROP CONSTRAINT IF EXISTS tenants_status_check;

ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active', 'suspended', 'deactivated'));
