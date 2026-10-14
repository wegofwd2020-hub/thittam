-- 008_journal_line_party.up.sql
-- Optional party (customer or vendor) on a journal line. Existing lines stay
-- untagged. A party with posted lines cannot be deleted (ON DELETE RESTRICT).
ALTER TABLE journal_lines
    ADD COLUMN party_id UUID REFERENCES parties(id) ON DELETE RESTRICT;

CREATE INDEX idx_lines_party ON journal_lines (party_id) WHERE party_id IS NOT NULL;
