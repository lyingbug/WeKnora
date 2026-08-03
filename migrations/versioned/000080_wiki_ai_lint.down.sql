-- Rollback: 000080_wiki_ai_lint

DROP TABLE IF EXISTS wiki_review_ledger;

DROP INDEX IF EXISTS idx_wiki_lint_runs_one_active_scope;
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_lint_runs_one_active
    ON wiki_lint_runs(knowledge_base_id) WHERE status IN ('queued', 'running');

ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS ai_detectors;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS ai_finding_count;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS ai_calls;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS ai_units_skipped;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS ai_units_reviewed;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS target_slugs;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS scope_key;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS scope;
ALTER TABLE wiki_lint_runs DROP COLUMN IF EXISTS mode;
