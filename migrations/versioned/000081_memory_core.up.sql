-- Migration: 000081_memory_core
-- Description: Long-term memory (Memory Wiki) core schema.
--
-- Three tables plus a revision log, mirroring the wiki page model so memory is
-- something a person can read, edit and roll back rather than an opaque vector:
--
--   1. memory_spaces        — one private store per principal per workspace.
--   2. memory_notes         — append-only observations with their evidence.
--   3. memory_pages         — the deduplicated, editable, injectable unit.
--   4. memory_page_revisions— immutable snapshots of superseded versions.
--
-- Scoping note: spaces are keyed by (tenant, principal type, principal id)
-- rather than by sessions.user_id, whose format varies per channel (bare UUID
-- for web, composite strings for embed and IM) and therefore cannot serve as a
-- stable identity. The triple used here is the same one mcp_oauth_tokens
-- already proved out, and it covers web, OIDC, IM, API external users and
-- embed visitors uniformly.
--
-- Portability note: every column type here has a direct SQLite equivalent and
-- the partial unique indexes are supported by both engines, so the Lite schema
-- in migrations/sqlite is a mechanical translation rather than a redesign.

DO $$ BEGIN RAISE NOTICE '[Migration 000081] Applying memory core schema'; END $$;

-- ---------------------------------------------------------------------------
-- 1) memory_spaces
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS memory_spaces (
    id                    VARCHAR(36) PRIMARY KEY,
    tenant_id             BIGINT       NOT NULL,
    scope_type            VARCHAR(16)  NOT NULL DEFAULT 'user',
    owner_principal_type  VARCHAR(32)  NOT NULL DEFAULT '',
    owner_principal_id    VARCHAR(512) NOT NULL DEFAULT '',
    display_name          VARCHAR(255) NOT NULL DEFAULT '',
    status                VARCHAR(16)  NOT NULL DEFAULT 'active',
    config                JSONB        NOT NULL DEFAULT '{}'::JSONB,
    vector_kb_id          VARCHAR(36)  NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMPTZ
);

COMMENT ON TABLE memory_spaces IS 'One long-term memory store per principal per workspace.';
COMMENT ON COLUMN memory_spaces.config IS 'Space-level memory settings patch — the narrowest override layer.';
COMMENT ON COLUMN memory_spaces.vector_kb_id IS 'Hidden knowledge base that indexes this space for semantic recall. Using a hidden KB keeps every vector driver untouched and works unchanged on sqlite-vec.';

-- One live space per (tenant, scope, principal). The partial predicate lets a
-- purged space be recreated later without tripping the constraint.
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_spaces_owner
    ON memory_spaces (tenant_id, scope_type, owner_principal_type, owner_principal_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memory_spaces_tenant ON memory_spaces (tenant_id);

-- ---------------------------------------------------------------------------
-- 2) memory_notes
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS memory_notes (
    id                 VARCHAR(36) PRIMARY KEY,
    tenant_id          BIGINT       NOT NULL,
    space_id           VARCHAR(36)  NOT NULL,
    note_type          VARCHAR(32)  NOT NULL,
    statement          TEXT         NOT NULL,
    subject            VARCHAR(255) NOT NULL DEFAULT '',
    scope              VARCHAR(16)  NOT NULL DEFAULT 'permanent',
    confidence         REAL         NOT NULL DEFAULT 0.5,
    sensitivity        VARCHAR(16)  NOT NULL DEFAULT 'normal',
    source             VARCHAR(16)  NOT NULL DEFAULT 'pipeline',
    origin_role        VARCHAR(16)  NOT NULL DEFAULT 'user',
    session_id         VARCHAR(36)  NOT NULL DEFAULT '',
    source_message_ids JSONB        NOT NULL DEFAULT '[]'::JSONB,
    anchor_candidates  JSONB        NOT NULL DEFAULT '[]'::JSONB,
    normalized_hash    VARCHAR(64)  NOT NULL DEFAULT '',
    status             VARCHAR(16)  NOT NULL DEFAULT 'pending',
    merged_page_id     VARCHAR(36)  NOT NULL DEFAULT '',
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMPTZ
);

COMMENT ON TABLE memory_notes IS 'Append-only extracted observations. Never edited in place so every memory stays traceable to the messages that produced it.';
COMMENT ON COLUMN memory_notes.origin_role IS 'Conversation role the statement came from. Extraction only accepts "user": distilling a memory from a retrieved document or tool output would let a poisoned document implant a durable instruction.';
COMMENT ON COLUMN memory_notes.normalized_hash IS 'Hash of the punctuation-stripped, lowercased statement. Exact duplicates are merged without invoking a model.';

CREATE INDEX IF NOT EXISTS idx_memory_notes_space_status ON memory_notes (space_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_notes_hash        ON memory_notes (space_id, normalized_hash);
CREATE INDEX IF NOT EXISTS idx_memory_notes_session     ON memory_notes (session_id);
CREATE INDEX IF NOT EXISTS idx_memory_notes_tenant      ON memory_notes (tenant_id);

-- ---------------------------------------------------------------------------
-- 3) memory_pages
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS memory_pages (
    id               VARCHAR(36)  PRIMARY KEY,
    tenant_id        BIGINT       NOT NULL,
    space_id         VARCHAR(36)  NOT NULL,
    slug             VARCHAR(255) NOT NULL,
    title            VARCHAR(512) NOT NULL DEFAULT '',
    page_type        VARCHAR(32)  NOT NULL,
    status           VARCHAR(16)  NOT NULL DEFAULT 'active',
    content          TEXT         NOT NULL DEFAULT '',
    summary          TEXT         NOT NULL DEFAULT '',
    structured       JSONB        NOT NULL DEFAULT '{}'::JSONB,
    aliases          JSONB        NOT NULL DEFAULT '[]'::JSONB,
    in_links         JSONB        NOT NULL DEFAULT '[]'::JSONB,
    out_links        JSONB        NOT NULL DEFAULT '[]'::JSONB,
    folder_path      JSONB        NOT NULL DEFAULT '[]'::JSONB,
    strength         REAL         NOT NULL DEFAULT 1.0,
    hit_count        INT          NOT NULL DEFAULT 0,
    confidence       REAL         NOT NULL DEFAULT 0.5,
    pinned           BOOLEAN      NOT NULL DEFAULT FALSE,
    superseded_by    VARCHAR(36)  NOT NULL DEFAULT '',
    note_refs        JSONB        NOT NULL DEFAULT '[]'::JSONB,
    version          INT          NOT NULL DEFAULT 1,
    last_edit_source VARCHAR(16)  NOT NULL DEFAULT 'pipeline',
    last_seen_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ
);

COMMENT ON TABLE memory_pages IS 'The editable, human-readable memory unit. Only pages are ever injected into a prompt.';
COMMENT ON COLUMN memory_pages.structured IS 'Whitelisted preference fields. Only these typed values may steer generation; free text is injected as data, never as instructions.';
COMMENT ON COLUMN memory_pages.pinned IS 'Pinned pages are exempt from decay and archival.';
COMMENT ON COLUMN memory_pages.superseded_by IS 'Set when a newer, conflicting memory replaced this one. Conflict is the normal state of a memory, not an error.';

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_pages_space_slug
    ON memory_pages (space_id, slug) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memory_pages_space_type ON memory_pages (space_id, page_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_pages_tenant     ON memory_pages (tenant_id);
CREATE INDEX IF NOT EXISTS idx_memory_pages_updated    ON memory_pages (space_id, updated_at DESC);

-- ---------------------------------------------------------------------------
-- 4) memory_page_revisions
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS memory_page_revisions (
    id          VARCHAR(36)  PRIMARY KEY,
    tenant_id   BIGINT       NOT NULL,
    space_id    VARCHAR(36)  NOT NULL,
    page_id     VARCHAR(36)  NOT NULL,
    version     INT          NOT NULL,
    title       VARCHAR(512) NOT NULL DEFAULT '',
    content     TEXT         NOT NULL DEFAULT '',
    summary     TEXT         NOT NULL DEFAULT '',
    structured  JSONB        NOT NULL DEFAULT '{}'::JSONB,
    edit_source VARCHAR(16)  NOT NULL DEFAULT '',
    editor_id   VARCHAR(64)  NOT NULL DEFAULT '',
    edited_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_page_revisions_page_version
    ON memory_page_revisions (page_id, version);

CREATE INDEX IF NOT EXISTS idx_memory_page_revisions_space
    ON memory_page_revisions (space_id, page_id);

DO $$ BEGIN RAISE NOTICE '[Migration 000081] memory core schema applied successfully'; END $$;
