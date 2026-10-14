-- 008_journal_line_party.down.sql
DROP INDEX IF EXISTS idx_lines_party;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS party_id;
