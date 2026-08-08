-- Backfill of PostgreSQL migrations that were never mirrored into the SQLite
-- chain, leaving Lite's schema behind the Go models.
--
-- The drift is not cosmetic: tenants.api_principal_config is missing, and the
-- Tenant model has the field, so GORM emits it on every INSERT and creating a
-- workspace fails outright. A fresh Lite database therefore cannot complete
-- auto-setup. The rest cause narrower failures — missing system_settings makes
-- every settings read fall through to env with a warning, missing
-- pending_subtasks_count breaks stuck-task recovery on boot, missing
-- messages.attachments breaks chat attachments.
--
-- Found by building both schemas and diffing their column sets rather than by
-- reading migrations, since reading migrations is how the gap accumulated.
-- Deliberately not backfilled:
--   * embeddings                        — pgvector; Lite uses lite_embeddings.
--   * embed_channels.allow_memory       — dead column, its Go field was removed.
--   * organization_members_pre_plan3    — a one-off migration artefact.
--
-- Type mapping follows 000000_init: BIGSERIAL -> INTEGER PRIMARY KEY
-- AUTOINCREMENT, BIGINT -> INTEGER, JSONB -> TEXT, TIMESTAMPTZ -> DATETIME.

-- Mirrors 000064_principal_model.
ALTER TABLE tenants ADD COLUMN api_principal_config TEXT;

-- Mirrors 000053_system_admin_and_settings.
ALTER TABLE users ADD COLUMN is_system_admin BOOLEAN NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS system_settings (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    key              VARCHAR(128) NOT NULL UNIQUE,
    value            TEXT         NOT NULL,
    value_type       VARCHAR(16)  NOT NULL,
    category         VARCHAR(32)  NOT NULL,
    description      TEXT         NOT NULL DEFAULT '',
    is_secret        BOOLEAN      NOT NULL DEFAULT 0,
    requires_restart BOOLEAN      NOT NULL DEFAULT 0,
    last_modified_by VARCHAR(36)  NOT NULL DEFAULT '',
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_system_settings_category ON system_settings (category);

-- Mirrors 000056_knowledge_pending_subtasks.
ALTER TABLE knowledges ADD COLUMN pending_subtasks_count INTEGER NOT NULL DEFAULT 0;

-- Mirrors 000034_add_attachments.
ALTER TABLE messages ADD COLUMN attachments TEXT DEFAULT '[]';

-- Mirrors 000054_invitation_tokens.
ALTER TABLE tenant_invitations ADD COLUMN token VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE tenant_invitations ADD COLUMN accepted_count INTEGER NOT NULL DEFAULT 0;

-- Mirrors 000064_principal_model. Existing rows are attributed to the web user
-- they already recorded, matching the Postgres backfill.
ALTER TABLE mcp_oauth_tokens ADD COLUMN principal_type VARCHAR(32) NOT NULL DEFAULT 'web_user';
ALTER TABLE mcp_oauth_tokens ADD COLUMN principal_id VARCHAR(512) NOT NULL DEFAULT '';

UPDATE mcp_oauth_tokens SET principal_id = user_id WHERE principal_id = '';

CREATE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_principal
    ON mcp_oauth_tokens (principal_type, principal_id);

-- Mirrors 000063_knowledge_multi_tags.
CREATE TABLE IF NOT EXISTS knowledge_tag_relations (
    knowledge_id VARCHAR(36) NOT NULL,
    tag_id       VARCHAR(36) NOT NULL,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (knowledge_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_ktr_knowledge ON knowledge_tag_relations (knowledge_id);
CREATE INDEX IF NOT EXISTS idx_ktr_tag       ON knowledge_tag_relations (tag_id);

-- Mirrors 000055_knowledge_processing_spans.
CREATE TABLE IF NOT EXISTS knowledge_processing_spans (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    knowledge_id   VARCHAR(64) NOT NULL,
    attempt        INTEGER     NOT NULL DEFAULT 1,
    span_id        VARCHAR(64) NOT NULL,
    parent_span_id VARCHAR(64),
    name           VARCHAR(64) NOT NULL,
    kind           VARCHAR(16) NOT NULL,
    status         VARCHAR(16) NOT NULL,
    input          TEXT,
    output         TEXT,
    metadata       TEXT,
    error_code     VARCHAR(64),
    error_message  TEXT,
    error_detail   TEXT,
    started_at     DATETIME,
    finished_at    DATETIME,
    duration_ms    INTEGER,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (knowledge_id, attempt, span_id)
);

CREATE INDEX IF NOT EXISTS idx_kpspan_knowledge_attempt
    ON knowledge_processing_spans (knowledge_id, attempt);
