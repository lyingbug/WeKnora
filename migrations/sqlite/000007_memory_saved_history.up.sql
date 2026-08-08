ALTER TABLE memory_pages ADD COLUMN saved BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE memory_pages ADD COLUMN memory_key VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE memory_notes ADD COLUMN memory_key VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE memory_notes ADD COLUMN structured TEXT NOT NULL DEFAULT '{}';

UPDATE memory_pages
SET saved = 1
WHERE last_edit_source IN ('user', 'agent', 'revert');

CREATE INDEX IF NOT EXISTS idx_memory_pages_space_saved
    ON memory_pages (space_id, saved, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_pages_space_key
    ON memory_pages (space_id, memory_key) WHERE memory_key <> '' AND deleted_at IS NULL;
