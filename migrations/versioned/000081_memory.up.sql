-- Migration 000081: cross-session long-term memory.
--
-- Two tables only. memory_subjects is one row per (workspace, principal) and
-- caches the resident block that every turn injects; memory_items holds the
-- individual remembered statements.
--
-- Contradictions are resolved by bi-temporal supersede rather than deletion:
-- the outdated row keeps its content and gets invalid_at + superseded_by, so
-- "what did it believe last month" stays answerable and the user can see why a
-- memory changed. Rows are only physically removed when the user forgets them.

CREATE TABLE IF NOT EXISTS memory_subjects (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id INTEGER NOT NULL,
    -- Principal.StorageID(), e.g. "web_user:<uuid>" or "im_user:wecom:<ch>:<u>".
    subject_id VARCHAR(512) NOT NULL,
    -- Per-user opt out. The workspace-level switch lives on tenants.memory_config.
    enabled BOOLEAN NOT NULL DEFAULT true,
    -- Rendered profile/preference block, recomputed on write and read as-is on
    -- every turn so the read path stays a single primary-key lookup.
    block_text TEXT NOT NULL DEFAULT '',
    block_updated_at TIMESTAMP WITH TIME ZONE,
    item_count INTEGER NOT NULL DEFAULT 0,
    last_extracted_at TIMESTAMP WITH TIME ZONE,
    -- Watermark: everything this subject said up to here has been considered
    -- for distillation. Runs walk forward from it, so a message cannot be
    -- skipped by a timer or by a burst of turns.
    extract_cursor TIMESTAMP WITH TIME ZONE,
    -- Sessions with turns past the cursor, recorded when a turn arrives while
    -- a run is already in flight.
    pending_sessions JSONB,
    -- Set while a distillation task is queued or running, so concurrent turns
    -- enqueue one task rather than one per turn.
    extract_scheduled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_subjects_scope
    ON memory_subjects (tenant_id, subject_id);

CREATE TABLE IF NOT EXISTS memory_items (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id INTEGER NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    -- profile | preference | fact | task
    kind VARCHAR(32) NOT NULL,
    content TEXT NOT NULL,
    -- Readable subject the statement is about, kept verbatim because a question
    -- often names the topic while the statement carries only the value.
    topic VARCHAR(255) NOT NULL DEFAULT '',
    -- Normalized topic key used to detect that a new statement contradicts an
    -- existing one. Two items sharing a key are the same fact at different times.
    normalized_key VARCHAR(255) NOT NULL DEFAULT '',
    importance SMALLINT NOT NULL DEFAULT 3,
    -- explicit (user asked) | extracted (background) | manual (memory editor)
    origin VARCHAR(16) NOT NULL DEFAULT 'extracted',
    -- active | superseded | archived
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    source_session_id VARCHAR(36),
    source_message_id VARCHAR(36),
    valid_from TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    invalid_at TIMESTAMP WITH TIME ZONE,
    superseded_by VARCHAR(36),
    last_used_at TIMESTAMP WITH TIME ZONE,
    use_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_memory_items_scope
    ON memory_items (tenant_id, subject_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_items_key
    ON memory_items (tenant_id, subject_id, normalized_key);

-- Workspace-level switch, write mode, extraction model and capacity.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS memory_config JSONB;

-- Which memories were injected into this answer. Persisted rather than only
-- streamed so reopening a conversation still shows what the answer saw.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS used_memories JSONB;
