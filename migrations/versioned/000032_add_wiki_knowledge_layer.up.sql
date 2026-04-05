-- Wiki Knowledge Layer tables (Karpathy's LLM Wiki pattern)
-- Adds a structured, LLM-maintained wiki layer between raw source documents and queries

-- Wiki pages: LLM-maintained interlinked Markdown knowledge pages
CREATE TABLE IF NOT EXISTS wiki_pages (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    title VARCHAR(512) NOT NULL,
    slug VARCHAR(512) NOT NULL,
    page_type VARCHAR(32) NOT NULL DEFAULT 'concept',
    content TEXT,
    summary TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    tags JSONB,
    out_links JSONB,
    in_links JSONB,
    source_knowledge_ids JSONB,
    version INT NOT NULL DEFAULT 1,
    model_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_tenant ON wiki_pages(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_kb ON wiki_pages(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_title ON wiki_pages(title);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_pages_slug ON wiki_pages(slug);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_type ON wiki_pages(page_type);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_deleted_at ON wiki_pages(deleted_at);

-- Wiki schemas: configuration defining wiki structure and conventions per knowledge base
CREATE TABLE IF NOT EXISTS wiki_schemas (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    schema_content TEXT,
    wiki_model_id VARCHAR(100),
    auto_ingest BOOLEAN NOT NULL DEFAULT TRUE,
    auto_lint BOOLEAN NOT NULL DEFAULT FALSE,
    lint_interval_hours INT NOT NULL DEFAULT 24,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wiki_schemas_tenant ON wiki_schemas(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_schemas_kb ON wiki_schemas(knowledge_base_id);

-- Wiki lint issues: quality issues found during wiki health checks
CREATE TABLE IF NOT EXISTS wiki_lint_issues (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    wiki_page_id VARCHAR(36),
    issue_type VARCHAR(64) NOT NULL,
    severity VARCHAR(32) NOT NULL DEFAULT 'info',
    description TEXT,
    suggested_fix TEXT,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_wiki_lint_tenant ON wiki_lint_issues(tenant_id);
CREATE INDEX IF NOT EXISTS idx_wiki_lint_kb ON wiki_lint_issues(knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_wiki_lint_page ON wiki_lint_issues(wiki_page_id);
