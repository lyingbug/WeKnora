-- Split user-saved memories from facts inferred from chat history, and give
-- observations a fine-grained identity plus typed preference payload.
ALTER TABLE memory_pages ADD COLUMN IF NOT EXISTS saved BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE memory_pages ADD COLUMN IF NOT EXISTS memory_key VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE memory_notes ADD COLUMN IF NOT EXISTS memory_key VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE memory_notes ADD COLUMN IF NOT EXISTS structured JSONB NOT NULL DEFAULT '{}'::JSONB;

-- Existing human-authored pages were deliberate saves. Pipeline pages remain
-- history-derived so they can be recalled selectively and decay normally.
UPDATE memory_pages
SET saved = TRUE
WHERE last_edit_source IN ('user', 'agent', 'revert');

CREATE INDEX IF NOT EXISTS idx_memory_pages_space_saved
    ON memory_pages (space_id, saved, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_pages_space_key
    ON memory_pages (space_id, memory_key) WHERE memory_key <> '' AND deleted_at IS NULL;
