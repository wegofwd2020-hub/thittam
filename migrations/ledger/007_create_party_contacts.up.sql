-- 007_create_party_contacts.up.sql
-- Individuals at a party: who to reach about a given matter. The party keeps
-- the organization's main email and phone. Each party has at most one primary
-- contact.
CREATE TABLE party_contacts (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    party_id    UUID        NOT NULL REFERENCES parties(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    designation TEXT        NOT NULL DEFAULT '' CHECK (char_length(designation) <= 100),
    email       TEXT        NOT NULL DEFAULT '' CHECK (char_length(email) <= 254),
    phone       TEXT        NOT NULL DEFAULT '' CHECK (char_length(phone) <= 30),
    is_primary  BOOLEAN     NOT NULL DEFAULT false,
    position    INT         NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_party_contacts_party ON party_contacts (tenant_id, party_id, position);
CREATE UNIQUE INDEX idx_party_contacts_one_primary ON party_contacts (party_id) WHERE is_primary;
