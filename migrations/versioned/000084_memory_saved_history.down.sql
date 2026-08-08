DROP INDEX IF EXISTS idx_memory_pages_space_key;
DROP INDEX IF EXISTS idx_memory_pages_space_saved;
ALTER TABLE memory_notes DROP COLUMN IF EXISTS structured;
ALTER TABLE memory_notes DROP COLUMN IF EXISTS memory_key;
ALTER TABLE memory_pages DROP COLUMN IF EXISTS memory_key;
ALTER TABLE memory_pages DROP COLUMN IF EXISTS saved;
