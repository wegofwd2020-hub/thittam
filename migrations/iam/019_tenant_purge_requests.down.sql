-- WARNING: this rollback's ADD CONSTRAINT will FAIL if any tenant row
-- already has status='purged' (which the 'purged' value being removed
-- from the CHECK would then reject). Reconcile those rows first — e.g.
-- move them to 'deactivated' or otherwise handle them out of band — before
-- running this down migration in an environment where PurgeTenant (#92
-- Stage 3) has actually executed.
ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active','suspended','grace','deactivated','purge_eligible'));

ALTER TABLE tenants DROP COLUMN IF EXISTS purged_at;

DROP INDEX IF EXISTS idx_tenant_purge_requests_approved;
DROP INDEX IF EXISTS tenant_purge_requests_one_open;
DROP TABLE IF EXISTS tenant_purge_requests;
