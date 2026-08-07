-- Lite counterpart of migrations/versioned/000081_memory_core,
-- 000082_memory_anchors and 000083_memory_tenant_config.
--
-- Structure, indexes and constraint semantics are identical to the Postgres
-- schema; only the types change, following the mapping already used across
-- 000000_init:
--   BIGINT      -> INTEGER
--   JSONB       -> TEXT (GORM serialises via driver.Valuer / sql.Scanner)
--   TIMESTAMPTZ -> DATETIME
--   BOOLEAN     -> BOOLEAN (SQLite affinity)
--
-- The three partial unique indexes keep their `WHERE deleted_at IS NULL`
-- predicate: SQLite supports partial indexes, so soft-delete-and-recreate
-- behaves the same on both engines. See idx_wiki_pages_kb_slug in 000000_init
-- for the existing precedent.

CREATE TABLE IF NOT EXISTS memory_spaces (
    id                    VARCHAR(36) PRIMARY KEY,
    tenant_id             INTEGER      NOT NULL,
    scope_type            VARCHAR(16)  NOT NULL DEFAULT 'user',
    owner_principal_type  VARCHAR(32)  NOT NULL DEFAULT '',
    owner_principal_id    VARCHAR(512) NOT NULL DEFAULT '',
    display_name          VARCHAR(255) NOT NULL DEFAULT '',
    status                VARCHAR(16)  NOT NULL DEFAULT 'active',
    config                TEXT         NOT NULL DEFAULT '{}',
    vector_kb_id          VARCHAR(36)  NOT NULL DEFAULT '',
    created_at            DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at            DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_spaces_owner
    ON memory_spaces (tenant_id, scope_type, owner_principal_type, owner_principal_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memory_spaces_tenant ON memory_spaces (tenant_id);

CREATE TABLE IF NOT EXISTS memory_notes (
    id                 VARCHAR(36) PRIMARY KEY,
    tenant_id          INTEGER      NOT NULL,
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
    source_message_ids TEXT         NOT NULL DEFAULT '[]',
    anchor_candidates  TEXT         NOT NULL DEFAULT '[]',
    normalized_hash    VARCHAR(64)  NOT NULL DEFAULT '',
    status             VARCHAR(16)  NOT NULL DEFAULT 'pending',
    merged_page_id     VARCHAR(36)  NOT NULL DEFAULT '',
    expires_at         DATETIME,
    created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at         DATETIME
);

CREATE INDEX IF NOT EXISTS idx_memory_notes_space_status ON memory_notes (space_id, status);
CREATE INDEX IF NOT EXISTS idx_memory_notes_hash        ON memory_notes (space_id, normalized_hash);
CREATE INDEX IF NOT EXISTS idx_memory_notes_session     ON memory_notes (session_id);
CREATE INDEX IF NOT EXISTS idx_memory_notes_tenant      ON memory_notes (tenant_id);

CREATE TABLE IF NOT EXISTS memory_pages (
    id               VARCHAR(36)  PRIMARY KEY,
    tenant_id        INTEGER      NOT NULL,
    space_id         VARCHAR(36)  NOT NULL,
    slug             VARCHAR(255) NOT NULL,
    title            VARCHAR(512) NOT NULL DEFAULT '',
    page_type        VARCHAR(32)  NOT NULL,
    status           VARCHAR(16)  NOT NULL DEFAULT 'active',
    content          TEXT         NOT NULL DEFAULT '',
    summary          TEXT         NOT NULL DEFAULT '',
    structured       TEXT         NOT NULL DEFAULT '{}',
    aliases          TEXT         NOT NULL DEFAULT '[]',
    in_links         TEXT         NOT NULL DEFAULT '[]',
    out_links        TEXT         NOT NULL DEFAULT '[]',
    folder_path      TEXT         NOT NULL DEFAULT '[]',
    strength         REAL         NOT NULL DEFAULT 1.0,
    hit_count        INTEGER      NOT NULL DEFAULT 0,
    confidence       REAL         NOT NULL DEFAULT 0.5,
    pinned           BOOLEAN      NOT NULL DEFAULT 0,
    superseded_by    VARCHAR(36)  NOT NULL DEFAULT '',
    note_refs        TEXT         NOT NULL DEFAULT '[]',
    version          INTEGER      NOT NULL DEFAULT 1,
    last_edit_source VARCHAR(16)  NOT NULL DEFAULT 'pipeline',
    last_seen_at     DATETIME,
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at       DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_pages_space_slug
    ON memory_pages (space_id, slug) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memory_pages_space_type ON memory_pages (space_id, page_type, status);
CREATE INDEX IF NOT EXISTS idx_memory_pages_tenant     ON memory_pages (tenant_id);
CREATE INDEX IF NOT EXISTS idx_memory_pages_updated    ON memory_pages (space_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS memory_page_revisions (
    id          VARCHAR(36)  PRIMARY KEY,
    tenant_id   INTEGER      NOT NULL,
    space_id    VARCHAR(36)  NOT NULL,
    page_id     VARCHAR(36)  NOT NULL,
    version     INTEGER      NOT NULL,
    title       VARCHAR(512) NOT NULL DEFAULT '',
    content     TEXT         NOT NULL DEFAULT '',
    summary     TEXT         NOT NULL DEFAULT '',
    structured  TEXT         NOT NULL DEFAULT '{}',
    edit_source VARCHAR(16)  NOT NULL DEFAULT '',
    editor_id   VARCHAR(64)  NOT NULL DEFAULT '',
    edited_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_page_revisions_page_version
    ON memory_page_revisions (page_id, version);

CREATE INDEX IF NOT EXISTS idx_memory_page_revisions_space
    ON memory_page_revisions (space_id, page_id);

CREATE TABLE IF NOT EXISTS memory_anchors (
    id                VARCHAR(36)  PRIMARY KEY,
    tenant_id         INTEGER      NOT NULL,
    space_id          VARCHAR(36)  NOT NULL,
    memory_page_id    VARCHAR(36)  NOT NULL DEFAULT '',
    knowledge_base_id VARCHAR(36)  NOT NULL,
    target_kind       VARCHAR(24)  NOT NULL,
    target_ref        VARCHAR(512) NOT NULL,
    relation          VARCHAR(24)  NOT NULL,
    strength          REAL         NOT NULL DEFAULT 0,
    hit_count         INTEGER      NOT NULL DEFAULT 0,
    confidence        REAL         NOT NULL DEFAULT 0.5,
    source            VARCHAR(16)  NOT NULL DEFAULT 'pipeline',
    evidence          TEXT         NOT NULL DEFAULT '{}',
    first_seen_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at        DATETIME
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_anchors_unique
    ON memory_anchors (space_id, knowledge_base_id, target_kind, target_ref, relation, memory_page_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_memory_anchors_space_kb
    ON memory_anchors (space_id, knowledge_base_id, target_kind);

CREATE INDEX IF NOT EXISTS idx_memory_anchors_kb_target
    ON memory_anchors (knowledge_base_id, target_kind, target_ref);

CREATE INDEX IF NOT EXISTS idx_memory_anchors_page
    ON memory_anchors (space_id, memory_page_id);

CREATE INDEX IF NOT EXISTS idx_memory_anchors_tenant
    ON memory_anchors (tenant_id);

ALTER TABLE tenants ADD COLUMN memory_config TEXT;
