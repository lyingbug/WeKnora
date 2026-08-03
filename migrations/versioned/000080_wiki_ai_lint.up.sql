-- Migration: 000080_wiki_ai_lint
-- Adds the AI health review to the wiki lint lifecycle: runs declare a mode
-- (static rules, model review, or both) and a scope (whole wiki, or a named
-- set of pages), and each reviewed page keeps a ledger row so an unchanged
-- page is never paid for twice.

ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS mode VARCHAR(16) NOT NULL DEFAULT 'static';
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'kb';
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS scope_key VARCHAR(280) NOT NULL DEFAULT 'kb';
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS target_slugs JSONB NOT NULL DEFAULT '[]'::JSONB;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_pages_scanned INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_pages_skipped INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_calls INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_finding_count INT NOT NULL DEFAULT 0;

-- One active run per scope rather than per knowledge base, so checking a
-- single page never reports that the whole wiki is busy.
DROP INDEX IF EXISTS idx_wiki_lint_runs_one_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_lint_runs_one_active_scope
    ON wiki_lint_runs(knowledge_base_id, scope_key) WHERE status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS wiki_page_ai_reviews (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    page_id VARCHAR(36) NOT NULL,
    slug VARCHAR(255) NOT NULL DEFAULT '',
    content_hash VARCHAR(64) NOT NULL DEFAULT '',
    reviewer_version VARCHAR(32) NOT NULL DEFAULT '',
    reviewed_version INT NOT NULL DEFAULT 0,
    finding_count INT NOT NULL DEFAULT 0,
    run_id VARCHAR(36) NOT NULL DEFAULT '',
    model_id VARCHAR(36) NOT NULL DEFAULT '',
    reviewed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_page_ai_reviews_page
    ON wiki_page_ai_reviews(knowledge_base_id, page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_ai_reviews_run ON wiki_page_ai_reviews(run_id);
