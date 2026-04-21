-- Pre-migration audit for #91.
-- Run this against the target iam database BEFORE applying
-- migrations/iam/015_tenants_unique_name.up.sql — the UNIQUE INDEX it creates
-- will fail if any rows collide on lower(trim(name)).
--
-- Empty result set means safe to proceed. Any rows returned must be resolved
-- manually: rename the offending tenant(s) with an UPDATE, or archive/delete
-- demo rows that shouldn't own a real tenant name.
--
-- Usage:
--   psql -d thittam_iam -f scripts/audit-tenant-name-collisions.sql

SELECT
    lower(trim(name))       AS canonical_name,
    count(*)                AS collision_count,
    array_agg(id ORDER BY created_at) AS tenant_ids,
    array_agg(name ORDER BY created_at) AS original_names,
    array_agg(created_at ORDER BY created_at) AS created_ats
FROM tenants
GROUP BY lower(trim(name))
HAVING count(*) > 1
ORDER BY canonical_name;
