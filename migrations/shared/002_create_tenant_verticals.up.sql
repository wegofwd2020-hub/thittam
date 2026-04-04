-- 002_create_tenant_verticals.up.sql
-- Maps each tenant to its vertical definition + optional config overrides.

CREATE TABLE tenant_verticals (
    tenant_id       UUID        PRIMARY KEY,
    vertical_id     TEXT        NOT NULL REFERENCES vertical_definitions(id),
    config_override JSONB,
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    registered_by   UUID        NOT NULL
);

CREATE INDEX idx_tv_vertical ON tenant_verticals (vertical_id);
