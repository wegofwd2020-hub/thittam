-- 001_create_vertical_definitions.up.sql
-- Shared infrastructure table: stores base vertical YAML configs as JSONB.

CREATE TABLE vertical_definitions (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL,
    version     TEXT        NOT NULL,
    description TEXT,
    config      JSONB       NOT NULL,
    is_active   BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vertical_active ON vertical_definitions (is_active) WHERE is_active = true;

-- Auto-update updated_at on row modification.
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_vertical_definitions_updated_at
    BEFORE UPDATE ON vertical_definitions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
