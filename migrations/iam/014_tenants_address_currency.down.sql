-- 014_tenants_address_currency.down.sql

ALTER TABLE tenants
    DROP CONSTRAINT IF EXISTS tenants_country_code_format,
    DROP CONSTRAINT IF EXISTS tenants_currency_code_format;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS address_line1,
    DROP COLUMN IF EXISTS address_line2,
    DROP COLUMN IF EXISTS city,
    DROP COLUMN IF EXISTS country_code,
    DROP COLUMN IF EXISTS postal_code,
    DROP COLUMN IF EXISTS primary_currency_code;
