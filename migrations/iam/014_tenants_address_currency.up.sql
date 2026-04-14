-- 014_tenants_address_currency.up.sql
-- #61: company address + country-driven primary currency.
--
-- Address fields are nullable (legacy demo tenants don't have them); country
-- and currency are NOT NULL once any row post-migration carries a value, so
-- we backfill existing rows to IN/INR first.

ALTER TABLE tenants
    ADD COLUMN address_line1         TEXT,
    ADD COLUMN address_line2         TEXT,
    ADD COLUMN city                  TEXT,
    ADD COLUMN country_code          CHAR(2),
    ADD COLUMN postal_code           TEXT,
    ADD COLUMN primary_currency_code CHAR(3);

-- Backfill existing rows. XYZ_CBA Productions is the only seeded tenant today;
-- give every existing row sensible Indian defaults so NOT NULL can be enforced.
UPDATE tenants SET country_code = 'IN' WHERE country_code IS NULL;
UPDATE tenants SET primary_currency_code = 'INR' WHERE primary_currency_code IS NULL;

ALTER TABLE tenants
    ALTER COLUMN country_code          SET NOT NULL,
    ALTER COLUMN primary_currency_code SET NOT NULL;

-- ISO 3166-1 alpha-2 / 4217 formats — enforce case + length at the DB edge.
ALTER TABLE tenants
    ADD CONSTRAINT tenants_country_code_format
        CHECK (country_code ~ '^[A-Z]{2}$'),
    ADD CONSTRAINT tenants_currency_code_format
        CHECK (primary_currency_code ~ '^[A-Z]{3}$');
