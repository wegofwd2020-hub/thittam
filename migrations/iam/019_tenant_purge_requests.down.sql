ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active','suspended','grace','deactivated','purge_eligible'));

ALTER TABLE tenants DROP COLUMN IF EXISTS purged_at;

DROP INDEX IF EXISTS idx_tenant_purge_requests_approved;
DROP INDEX IF EXISTS tenant_purge_requests_one_open;
DROP TABLE IF EXISTS tenant_purge_requests;
