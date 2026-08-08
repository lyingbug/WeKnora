DROP INDEX IF EXISTS idx_memory_pages_space_key;
DROP INDEX IF EXISTS idx_memory_pages_space_saved;
ALTER TABLE memory_notes DROP COLUMN structured;
ALTER TABLE memory_notes DROP COLUMN memory_key;
ALTER TABLE memory_pages DROP COLUMN memory_key;
ALTER TABLE memory_pages DROP COLUMN saved;
