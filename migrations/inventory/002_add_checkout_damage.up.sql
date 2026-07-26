-- CheckInAsset (#192) records damage reported at check-in.
ALTER TABLE asset_checkouts
    ADD COLUMN notes              TEXT,
    ADD COLUMN report_damage      BOOLEAN       NOT NULL DEFAULT false,
    ADD COLUMN damage_severity    TEXT,
    ADD COLUMN damage_description TEXT,
    ADD COLUMN repair_cost        NUMERIC(14,2);
