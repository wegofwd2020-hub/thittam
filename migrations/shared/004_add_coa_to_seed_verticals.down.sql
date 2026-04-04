-- 004_add_coa_to_seed_verticals.down.sql
-- Removes default_chart_of_accounts from all seeded vertical configs.

UPDATE vertical_definitions
SET config = config - 'default_chart_of_accounts',
    updated_at = now()
WHERE id IN ('movie-production', 'software-development', 'construction', 'events-management');
