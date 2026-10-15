-- 004_add_party_id.up.sql
-- Optional customer or vendor on a purchase order or expense. The party lives
-- in the general-ledger service, so there is no foreign key here; the service
-- checks tenant scope on reads. Existing rows stay untagged.
ALTER TABLE purchase_orders ADD COLUMN party_id UUID;
ALTER TABLE expenses        ADD COLUMN party_id UUID;

CREATE INDEX idx_po_party       ON purchase_orders (tenant_id, party_id) WHERE party_id IS NOT NULL;
CREATE INDEX idx_expenses_party ON expenses (tenant_id, party_id)        WHERE party_id IS NOT NULL;
