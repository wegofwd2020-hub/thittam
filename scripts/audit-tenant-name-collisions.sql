-- Detect tenant-name collisions under the migration-018 normalisation
-- (case-insensitive, trimmed, internal whitespace collapsed). Run before
-- applying 018 — any rows returned would break CREATE UNIQUE INDEX.
SELECT regexp_replace(lower(trim(name)), '\s+', ' ', 'g') AS normalized_name,
       count(*)                                            AS n,
       array_agg(id)                                       AS tenant_ids
FROM tenants
GROUP BY regexp_replace(lower(trim(name)), '\s+', ' ', 'g')
HAVING count(*) > 1;
