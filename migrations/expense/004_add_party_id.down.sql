-- 004_add_party_id.down.sql
DROP INDEX IF EXISTS idx_expenses_party;
DROP INDEX IF EXISTS idx_po_party;
ALTER TABLE expenses        DROP COLUMN IF EXISTS party_id;
ALTER TABLE purchase_orders DROP COLUMN IF EXISTS party_id;
