ALTER TABLE asset_checkouts
    DROP COLUMN IF EXISTS repair_cost,
    DROP COLUMN IF EXISTS damage_description,
    DROP COLUMN IF EXISTS damage_severity,
    DROP COLUMN IF EXISTS report_damage,
    DROP COLUMN IF EXISTS notes;
