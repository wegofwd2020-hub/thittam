-- 001_create_vertical_definitions.down.sql

DROP TRIGGER IF EXISTS trg_vertical_definitions_updated_at ON vertical_definitions;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP INDEX IF EXISTS idx_vertical_active;
DROP TABLE IF EXISTS vertical_definitions;
