-- Cross-session long-term memory (Lite). Mirrors migrations/versioned/000081.
-- Row ids are generated in Go, so there is no server-side default here.

CREATE TABLE IF NOT EXISTS memory_subjects (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    block_text TEXT NOT NULL DEFAULT '',
    block_updated_at DATETIME,
    item_count INTEGER NOT NULL DEFAULT 0,
    last_extracted_at DATETIME,
    extract_cursor DATETIME,
    pending_sessions TEXT,
    extract_scheduled_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_subjects_scope
    ON memory_subjects (tenant_id, subject_id);

CREATE TABLE IF NOT EXISTS memory_items (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    kind VARCHAR(32) NOT NULL,
    content TEXT NOT NULL,
    topic VARCHAR(255) NOT NULL DEFAULT '',
    normalized_key VARCHAR(255) NOT NULL DEFAULT '',
    importance INTEGER NOT NULL DEFAULT 3,
    origin VARCHAR(16) NOT NULL DEFAULT 'extracted',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    source_session_id VARCHAR(36),
    source_message_id VARCHAR(36),
    valid_from DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    invalid_at DATETIME,
    superseded_by VARCHAR(36),
    last_used_at DATETIME,
    use_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memory_items_scope
    ON memory_items (tenant_id, subject_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_items_key
    ON memory_items (tenant_id, subject_id, normalized_key);

ALTER TABLE tenants ADD COLUMN memory_config TEXT;
ALTER TABLE messages ADD COLUMN used_memories TEXT;
