package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WikiPageStatus constants define the lifecycle states of a wiki page.
const (
	WikiPageStatusActive   = "active"
	WikiPageStatusArchived = "archived"
	WikiPageStatusDraft    = "draft"
)

// WikiPageType constants categorize wiki pages by their purpose,
// aligned with Karpathy's LLM Wiki pattern (entity, concept, synthesis, index, log).
const (
	WikiPageTypeEntity    = "entity"    // Named entity pages (person, org, product, etc.)
	WikiPageTypeConcept   = "concept"   // Abstract concept or topic pages
	WikiPageTypeSynthesis = "synthesis" // Cross-document synthesis / insight pages
	WikiPageTypeIndex     = "index"     // Index / table of contents pages
	WikiPageTypeLog       = "log"       // Ingestion activity log pages
)

// WikiLintSeverity constants for lint issue severity levels.
const (
	WikiLintSeverityInfo    = "info"
	WikiLintSeverityWarning = "warning"
	WikiLintSeverityError   = "error"
)

// WikiPage represents an LLM-maintained wiki page in the structured knowledge layer.
// It sits between raw source documents and user queries, providing pre-synthesized,
// interlinked knowledge that compounds over time (Karpathy's LLM Wiki pattern).
type WikiPage struct {
	// Unique identifier of the wiki page
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// Tenant ID for multi-tenant isolation
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// Knowledge base this wiki page belongs to
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	// Page title (used for wiki linking via [[Title]])
	Title string `json:"title" gorm:"type:varchar(512);index"`
	// Page slug for URL-friendly identification
	Slug string `json:"slug" gorm:"type:varchar(512);uniqueIndex"`
	// Page type: entity, concept, synthesis, index, log
	PageType string `json:"page_type" gorm:"type:varchar(32);index;default:'concept'"`
	// Markdown content of the page (LLM-maintained)
	Content string `json:"content" gorm:"type:text"`
	// Summary of the page (auto-generated)
	Summary string `json:"summary" gorm:"type:text"`
	// Page status: active, archived, draft
	Status string `json:"status" gorm:"type:varchar(32);default:'active'"`
	// Tags for categorization (stored as JSON array)
	Tags JSON `json:"tags" gorm:"type:json"`
	// Links to other wiki pages (stored as JSON array of page IDs)
	OutLinks JSON `json:"out_links" gorm:"type:json"`
	// Pages that link to this page (stored as JSON array of page IDs)
	InLinks JSON `json:"in_links" gorm:"type:json"`
	// Source knowledge IDs that contributed to this page (JSON array)
	SourceKnowledgeIDs JSON `json:"source_knowledge_ids" gorm:"type:json"`
	// Version number, incremented on each LLM update
	Version int `json:"version" gorm:"default:1"`
	// ID of the LLM model used for last update
	ModelID string `json:"model_id" gorm:"type:varchar(100)"`
	// Creation time
	CreatedAt time.Time `json:"created_at"`
	// Last updated time
	UpdatedAt time.Time `json:"updated_at"`
	// Soft delete marker
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// BeforeCreate hook generates a UUID for new WikiPage entities.
func (w *WikiPage) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return nil
}

// WikiSchema defines the structure and conventions for the wiki within a knowledge base.
// This is the "Schema" layer in Karpathy's three-layer architecture.
type WikiSchema struct {
	// Unique identifier
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// Tenant ID
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// Knowledge base this schema belongs to
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);uniqueIndex"`
	// Whether wiki mode is enabled for this knowledge base
	Enabled bool `json:"enabled" gorm:"default:false"`
	// Schema content in YAML/Markdown defining wiki conventions
	SchemaContent string `json:"schema_content" gorm:"type:text"`
	// LLM model ID used for wiki operations (ingest, lint)
	WikiModelID string `json:"wiki_model_id" gorm:"type:varchar(100)"`
	// Auto-ingest: automatically update wiki when new knowledge is added
	AutoIngest bool `json:"auto_ingest" gorm:"default:true"`
	// Auto-lint: periodically run lint checks
	AutoLint bool `json:"auto_lint" gorm:"default:false"`
	// Lint interval in hours (default: 24)
	LintIntervalHours int `json:"lint_interval_hours" gorm:"default:24"`
	// Creation time
	CreatedAt time.Time `json:"created_at"`
	// Last updated time
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate hook generates a UUID for new WikiSchema entities.
func (w *WikiSchema) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return nil
}

// WikiLintIssue represents an issue found during wiki health checks.
type WikiLintIssue struct {
	// Unique identifier
	ID string `json:"id" gorm:"type:varchar(36);primaryKey"`
	// Tenant ID
	TenantID uint64 `json:"tenant_id" gorm:"index"`
	// Knowledge base ID
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);index"`
	// Wiki page ID (if applicable)
	WikiPageID string `json:"wiki_page_id" gorm:"type:varchar(36);index"`
	// Issue type: orphan_page, contradiction, stale_claim, data_gap, broken_link
	IssueType string `json:"issue_type" gorm:"type:varchar(64)"`
	// Severity: info, warning, error
	Severity string `json:"severity" gorm:"type:varchar(32)"`
	// Description of the issue
	Description string `json:"description" gorm:"type:text"`
	// Suggested fix
	SuggestedFix string `json:"suggested_fix" gorm:"type:text"`
	// Whether the issue has been resolved
	Resolved bool `json:"resolved" gorm:"default:false"`
	// Creation time
	CreatedAt time.Time `json:"created_at"`
	// Resolution time
	ResolvedAt *time.Time `json:"resolved_at"`
}

// BeforeCreate hook generates a UUID for new WikiLintIssue entities.
func (w *WikiLintIssue) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	return nil
}

// WikiIngestRequest represents a request to ingest knowledge into the wiki layer.
type WikiIngestRequest struct {
	KnowledgeBaseID string   `json:"knowledge_base_id" binding:"required"`
	KnowledgeIDs    []string `json:"knowledge_ids" binding:"required"`
}

// WikiQueryRequest represents a request to query the wiki.
type WikiQueryRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id" binding:"required"`
	Query           string `json:"query" binding:"required"`
	TopK            int    `json:"top_k"`
}

// WikiQueryResult represents a single result from a wiki query.
type WikiQueryResult struct {
	Page      *WikiPage `json:"page"`
	Relevance float64   `json:"relevance"`
	Excerpt   string    `json:"excerpt"`
}

// WikiLintRequest represents a request to lint the wiki.
type WikiLintRequest struct {
	KnowledgeBaseID string `json:"knowledge_base_id" binding:"required"`
}

// WikiStats holds aggregate statistics for a knowledge base's wiki.
type WikiStats struct {
	TotalPages     int64            `json:"total_pages"`
	PagesByType    map[string]int64 `json:"pages_by_type"`
	TotalLinks     int64            `json:"total_links"`
	OrphanPages    int64            `json:"orphan_pages"`
	OpenLintIssues int64            `json:"open_lint_issues"`
	LastIngestAt   *time.Time       `json:"last_ingest_at"`
	LastLintAt     *time.Time       `json:"last_lint_at"`
}
