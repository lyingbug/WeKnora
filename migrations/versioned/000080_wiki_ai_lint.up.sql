-- Migration: 000080_wiki_ai_lint
-- Adds the AI health review to the wiki lint lifecycle.
--
-- Runs declare a mode (static rules, model review, or both) and a scope (the
-- whole wiki, or a named set of pages). The review ledger records every unit a
-- detector has already judged, keyed by the detector and the unit rather than
-- by page, because the units are not all pages: a page-content review judges
-- one page, a grounding review judges a page against its source document, and
-- a duplicate review judges a pair of pages.

ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS mode VARCHAR(16) NOT NULL DEFAULT 'static';
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'kb';
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS scope_key VARCHAR(280) NOT NULL DEFAULT 'kb';
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS target_slugs JSONB NOT NULL DEFAULT '[]'::JSONB;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_units_reviewed INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_units_skipped INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_calls INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_finding_count INT NOT NULL DEFAULT 0;
ALTER TABLE wiki_lint_runs ADD COLUMN IF NOT EXISTS ai_detectors JSONB NOT NULL DEFAULT '[]'::JSONB;

-- One active run per scope rather than per knowledge base, so checking a
-- single page never reports that the whole wiki is busy.
DROP INDEX IF EXISTS idx_wiki_lint_runs_one_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_lint_runs_one_active_scope
    ON wiki_lint_runs(knowledge_base_id, scope_key) WHERE status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS wiki_review_ledger (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    detector_id VARCHAR(48) NOT NULL,
    -- unit_key identifies what was judged: a page id, or a canonical pair of
    -- page ids. unit_hash covers every input the judgement depended on, so a
    -- unit is re-reviewed exactly when one of its inputs changed.
    unit_key VARCHAR(160) NOT NULL,
    unit_hash VARCHAR(64) NOT NULL DEFAULT '',
    reviewer_version VARCHAR(32) NOT NULL DEFAULT '',
    primary_slug VARCHAR(255) NOT NULL DEFAULT '',
    finding_count INT NOT NULL DEFAULT 0,
    run_id VARCHAR(36) NOT NULL DEFAULT '',
    model_id VARCHAR(36) NOT NULL DEFAULT '',
    reviewed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_review_ledger_unit
    ON wiki_review_ledger(knowledge_base_id, detector_id, unit_key);
CREATE INDEX IF NOT EXISTS idx_wiki_review_ledger_slug
    ON wiki_review_ledger(knowledge_base_id, primary_slug);
CREATE INDEX IF NOT EXISTS idx_wiki_review_ledger_run ON wiki_review_ledger(run_id);
