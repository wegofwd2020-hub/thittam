-- 015_tenants_unique_name.up.sql
-- #91: case-insensitive UNIQUE on tenants.name.
--
-- The index expression lowercases and trims so that "Acme Studios",
-- "acme studios", and "  Acme Studios  " collide at the DB layer. The
-- application normalises at write time as well (trim + collapse runs of
-- spaces) so the stored value is clean; this index is the backstop.
--
-- Run `scripts/audit-tenant-name-collisions.sql` against the target database
-- before applying this migration. Existing duplicates will cause CREATE INDEX
-- to fail.

CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (lower(trim(name)));
