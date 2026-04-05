package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// WikiService defines the interface for wiki knowledge layer operations.
// It implements Karpathy's LLM Wiki pattern: Ingest (process sources into wiki pages),
// Query (search the wiki), and Lint (health-check the wiki).
type WikiService interface {
	// --- Schema Management ---

	// GetOrCreateSchema returns the wiki schema for a knowledge base, creating a default one if absent.
	GetOrCreateSchema(ctx context.Context, kbID string) (*types.WikiSchema, error)
	// UpdateSchema updates the wiki schema configuration.
	UpdateSchema(ctx context.Context, schema *types.WikiSchema) error

	// --- Ingest ---

	// IngestKnowledge processes one or more knowledge items and creates/updates wiki pages.
	// A single source document may touch 10-15 wiki pages (entity, concept, synthesis, log).
	IngestKnowledge(ctx context.Context, kbID string, knowledgeIDs []string) error

	// --- Query ---

	// QueryWiki searches the wiki pages for answers to a query.
	QueryWiki(ctx context.Context, kbID string, query string, topK int) ([]*types.WikiQueryResult, error)

	// --- Lint ---

	// LintWiki performs health checks on the wiki, identifying orphan pages,
	// contradictions, stale claims, data gaps, and broken links.
	LintWiki(ctx context.Context, kbID string) ([]*types.WikiLintIssue, error)

	// --- CRUD ---

	// GetPage retrieves a wiki page by ID.
	GetPage(ctx context.Context, pageID string) (*types.WikiPage, error)
	// GetPageBySlug retrieves a wiki page by slug.
	GetPageBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error)
	// ListPages lists wiki pages in a knowledge base with optional type filter.
	ListPages(ctx context.Context, kbID string, pageType string, page *types.Pagination) (*types.PageResult, error)
	// UpdatePage updates a wiki page's content (manual edit).
	UpdatePage(ctx context.Context, page *types.WikiPage) error
	// DeletePage deletes a wiki page.
	DeletePage(ctx context.Context, pageID string) error

	// --- Stats ---

	// GetStats returns aggregate statistics for a knowledge base's wiki.
	GetStats(ctx context.Context, kbID string) (*types.WikiStats, error)

	// --- Lint Issues ---

	// ListLintIssues lists unresolved lint issues for a knowledge base.
	ListLintIssues(ctx context.Context, kbID string) ([]*types.WikiLintIssue, error)
	// ResolveLintIssue marks a lint issue as resolved.
	ResolveLintIssue(ctx context.Context, issueID string) error
}

// WikiRepository defines the data access interface for wiki pages.
type WikiRepository interface {
	// --- WikiPage CRUD ---
	CreatePage(ctx context.Context, page *types.WikiPage) error
	GetPageByID(ctx context.Context, tenantID uint64, pageID string) (*types.WikiPage, error)
	GetPageBySlug(ctx context.Context, tenantID uint64, kbID string, slug string) (*types.WikiPage, error)
	GetPageByTitle(ctx context.Context, tenantID uint64, kbID string, title string) (*types.WikiPage, error)
	ListPages(ctx context.Context, tenantID uint64, kbID string, pageType string, offset, limit int) ([]*types.WikiPage, int64, error)
	UpdatePage(ctx context.Context, page *types.WikiPage) error
	DeletePage(ctx context.Context, tenantID uint64, pageID string) error
	SearchPages(ctx context.Context, tenantID uint64, kbID string, query string, limit int) ([]*types.WikiPage, error)
	CountPages(ctx context.Context, tenantID uint64, kbID string) (int64, error)
	CountPagesByType(ctx context.Context, tenantID uint64, kbID string) (map[string]int64, error)
	FindOrphanPages(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikiPage, error)

	// --- WikiSchema CRUD ---
	GetSchema(ctx context.Context, tenantID uint64, kbID string) (*types.WikiSchema, error)
	CreateSchema(ctx context.Context, schema *types.WikiSchema) error
	UpdateSchema(ctx context.Context, schema *types.WikiSchema) error

	// --- WikiLintIssue CRUD ---
	CreateLintIssue(ctx context.Context, issue *types.WikiLintIssue) error
	ListLintIssues(ctx context.Context, tenantID uint64, kbID string, resolved bool) ([]*types.WikiLintIssue, error)
	CountOpenLintIssues(ctx context.Context, tenantID uint64, kbID string) (int64, error)
	ResolveLintIssue(ctx context.Context, tenantID uint64, issueID string) error
}
