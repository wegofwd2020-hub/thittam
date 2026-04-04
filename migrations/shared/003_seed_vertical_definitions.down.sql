-- 003_seed_vertical_definitions.down.sql

DELETE FROM vertical_definitions
WHERE id IN ('movie-production', 'software-development', 'construction', 'events-management');
