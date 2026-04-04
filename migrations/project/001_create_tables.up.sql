-- 001_create_tables.up.sql — project-management service tables
-- These tables live in the tenant-scoped schema (tenant_<uuid>).

CREATE TABLE productions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    title       TEXT        NOT NULL,
    slug        TEXT        NOT NULL,
    description TEXT,
    genre       TEXT,
    language    TEXT        DEFAULT 'en',
    status      TEXT        NOT NULL DEFAULT 'development'
                            CHECK (status IN ('development','pre_production','production','post_production','released','archived')),
    start_date  DATE,
    end_date    DATE,
    created_by  UUID        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, slug)
);

CREATE INDEX idx_productions_tenant ON productions (tenant_id);
CREATE INDEX idx_productions_status ON productions (tenant_id, status);

CREATE TABLE phases (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    production_id UUID        NOT NULL REFERENCES productions(id) ON DELETE CASCADE,
    tenant_id     UUID        NOT NULL,
    phase_type    TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending','active','completed')),
    start_date    DATE,
    end_date      DATE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_phases_production ON phases (production_id);

CREATE TABLE crew_members (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    production_id UUID        NOT NULL REFERENCES productions(id) ON DELETE CASCADE,
    tenant_id     UUID        NOT NULL,
    user_id       UUID,
    name          TEXT        NOT NULL,
    role          TEXT        NOT NULL,
    department    TEXT,
    day_rate      NUMERIC(14,2),
    currency      TEXT        DEFAULT 'INR',
    start_date    DATE,
    end_date      DATE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_crew_production ON crew_members (production_id);
