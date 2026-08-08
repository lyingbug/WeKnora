DROP INDEX IF EXISTS idx_memory_items_key;
DROP INDEX IF EXISTS idx_memory_items_scope;
DROP TABLE IF EXISTS memory_items;
DROP INDEX IF EXISTS idx_memory_subjects_scope;
DROP TABLE IF EXISTS memory_subjects;
ALTER TABLE tenants DROP COLUMN IF EXISTS memory_config;
ALTER TABLE messages DROP COLUMN IF EXISTS used_memories;
