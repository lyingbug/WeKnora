-- Backfill of migrations/versioned/000041_task_queue_and_wiki_indexes.
--
-- task_pending_ops and task_dead_letters were only ever added to the Postgres
-- migration chain, so a fresh Lite database has never had them. Anything that
-- relies on the durable debounced-queue pattern therefore degrades on Lite:
-- wiki ingest loses its pending rows, and exhausted retries have nowhere to be
-- archived. Long-term memory reuses the same pattern, so the tables are added
-- here first and Lite's wiki ingest is fixed as a side effect.
--
-- Type mapping follows the conventions already used in 000000_init:
--   BIGSERIAL   -> INTEGER PRIMARY KEY AUTOINCREMENT
--   BIGINT      -> INTEGER
--   JSONB       -> TEXT (GORM serialises via driver.Valuer / sql.Scanner)
--   TIMESTAMPTZ -> DATETIME

CREATE TABLE IF NOT EXISTS task_pending_ops (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   INTEGER      NOT NULL,
    task_type   VARCHAR(64)  NOT NULL,
    scope       VARCHAR(32)  NOT NULL,
    scope_id    VARCHAR(64)  NOT NULL,
    op          VARCHAR(32)  NOT NULL,
    dedup_key   VARCHAR(128) NOT NULL DEFAULT '',
    payload     TEXT         NOT NULL DEFAULT '{}',
    fail_count  INTEGER      NOT NULL DEFAULT 0,
    enqueued_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed_at  DATETIME
);

CREATE INDEX IF NOT EXISTS idx_task_pending_ops_scope
    ON task_pending_ops (task_type, scope, scope_id, id);

CREATE INDEX IF NOT EXISTS idx_task_pending_ops_tenant
    ON task_pending_ops (tenant_id);

CREATE TABLE IF NOT EXISTS task_dead_letters (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id   INTEGER     NOT NULL,
    task_type   VARCHAR(64) NOT NULL,
    scope       VARCHAR(32) NOT NULL,
    scope_id    VARCHAR(64) NOT NULL,
    related_id  VARCHAR(64) NOT NULL DEFAULT '',
    payload     TEXT        NOT NULL,
    last_error  TEXT        NOT NULL DEFAULT '',
    fail_count  INTEGER     NOT NULL,
    failed_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task_dead_letters_scope
    ON task_dead_letters (scope, scope_id, failed_at DESC);

CREATE INDEX IF NOT EXISTS idx_task_dead_letters_tenant
    ON task_dead_letters (tenant_id, failed_at DESC);

CREATE INDEX IF NOT EXISTS idx_task_dead_letters_task_type
    ON task_dead_letters (task_type, failed_at DESC);
