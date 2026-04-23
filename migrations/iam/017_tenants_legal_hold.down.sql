-- 017_tenants_legal_hold.down.sql

ALTER TABLE tenants
    DROP COLUMN IF EXISTS freeze_reason,
    DROP COLUMN IF EXISTS hold_until;
