-- 018_tenants_name_collapse_whitespace.down.sql
-- Restore the trim-only index from migration 015.
DROP INDEX tenants_name_ci_unique;
CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (lower(trim(name)));
