-- 018_tenants_name_collapse_whitespace.up.sql
-- #89: strengthen tenants_name_ci_unique to collapse *internal* whitespace.
--
-- Migration 015 indexed lower(trim(name)) — trim only. That lets
-- "Acme  Corp" (two spaces) and "Acme Corp" (one space) coexist, which
-- violates #89's intent. Rebuild the index (same name, so the repo-layer
-- isUniqueViolationOn helper keeps matching) on a fully normalised
-- expression that also collapses runs of internal whitespace to a single
-- space — matching the application's strings.Fields normalisation.
--
-- Run scripts/audit-tenant-name-collisions.sql against the target database
-- before applying: any pre-existing internal-whitespace duplicates will make
-- CREATE UNIQUE INDEX fail.

DROP INDEX IF EXISTS tenants_name_ci_unique;
CREATE UNIQUE INDEX tenants_name_ci_unique
    ON tenants (regexp_replace(lower(trim(name)), '\s+', ' ', 'g'));
