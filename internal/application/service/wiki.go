// Package service provides business logic implementations for WeKnora application
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Default wiki schema content for new knowledge bases.
const defaultSchemaContent = `# Wiki Schema

## Page Types
- **entity**: Named entity pages (people, organizations, products, technologies)
- **concept**: Abstract concept or methodology pages
- **synthesis**: Cross-document insights and analysis pages
- **index**: Table of contents and navigation pages
- **log**: Ingestion activity log entries

## Conventions
- Use [[Page Title]] for wiki links
- Each page should have a clear, descriptive title
- Entity pages should start with a brief definition
- Concept pages should explain the concept and its relationships
- Synthesis pages should cite their source pages
- Keep pages focused; split if a page exceeds ~2000 words
`

// wikiService implements the WikiService interface.
// It orchestrates the three-layer Wiki Knowledge pattern:
// Raw Sources → Wiki (LLM-maintained) → Schema (conventions).
type wikiService struct {
	wikiRepo      interfaces.WikiRepository
	kgService     interfaces.KnowledgeService
	chunkService  interfaces.ChunkService
	modelService  interfaces.ModelService
}

// NewWikiService creates a new wiki service.
func NewWikiService(
	wikiRepo interfaces.WikiRepository,
	kgService interfaces.KnowledgeService,
	chunkService interfaces.ChunkService,
	modelService interfaces.ModelService,
) interfaces.WikiService {
	return &wikiService{
		wikiRepo:     wikiRepo,
		kgService:    kgService,
		chunkService: chunkService,
		modelService: modelService,
	}
}

// --- Schema Management ---

func (s *wikiService) GetOrCreateSchema(ctx context.Context, kbID string) (*types.WikiSchema, error) {
	tenantID := getTenantIDFromContext(ctx)
	schema, err := s.wikiRepo.GetSchema(ctx, tenantID, kbID)
	if err == nil {
		return schema, nil
	}

	// Create a default schema
	schema = &types.WikiSchema{
		TenantID:          tenantID,
		KnowledgeBaseID:   kbID,
		Enabled:           true,
		SchemaContent:     defaultSchemaContent,
		AutoIngest:        true,
		AutoLint:          false,
		LintIntervalHours: 24,
	}
	if err := s.wikiRepo.CreateSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("failed to create wiki schema: %w", err)
	}
	return schema, nil
}

func (s *wikiService) UpdateSchema(ctx context.Context, schema *types.WikiSchema) error {
	return s.wikiRepo.UpdateSchema(ctx, schema)
}

// --- Ingest ---

// ingestResponse represents the LLM's structured response during wiki ingest.
type ingestResponse struct {
	Pages []ingestPageOp `json:"pages"`
}

type ingestPageOp struct {
	Action   string   `json:"action"`
	Title    string   `json:"title"`
	Slug     string   `json:"slug"`
	PageType string   `json:"page_type"`
	Content  string   `json:"content"`
	Summary  string   `json:"summary"`
	Tags     []string `json:"tags"`
	LinksTo  []string `json:"links_to"`
}

func (s *wikiService) IngestKnowledge(ctx context.Context, kbID string, knowledgeIDs []string) error {
	tenantID := getTenantIDFromContext(ctx)

	// Get wiki schema
	schema, err := s.GetOrCreateSchema(ctx, kbID)
	if err != nil {
		return fmt.Errorf("failed to get wiki schema: %w", err)
	}
	if !schema.Enabled {
		return fmt.Errorf("wiki mode is not enabled for this knowledge base")
	}

	// Load existing wiki page summaries for context
	existingPages, _, err := s.wikiRepo.ListPages(ctx, tenantID, kbID, "", 0, 500)
	if err != nil {
		return fmt.Errorf("failed to list existing pages: %w", err)
	}

	existingPagesContext := buildExistingPagesContext(existingPages)

	// Process each knowledge item
	for _, knowledgeID := range knowledgeIDs {
		knowledge, err := s.kgService.GetKnowledgeByIDOnly(ctx, knowledgeID)
		if err != nil {
			logger.Warnf(ctx, "[Wiki] Failed to get knowledge %s: %v", knowledgeID, err)
			continue
		}

		// Get the text content from chunks
		sourceContent, err := s.getKnowledgeContent(ctx, knowledgeID)
		if err != nil {
			logger.Warnf(ctx, "[Wiki] Failed to get content for knowledge %s: %v", knowledgeID, err)
			continue
		}

		if strings.TrimSpace(sourceContent) == "" {
			logger.Warnf(ctx, "[Wiki] Empty content for knowledge %s, skipping", knowledgeID)
			continue
		}

		// Build the ingest prompt
		prompt := buildIngestPrompt(schema.SchemaContent, existingPagesContext, knowledge.Title, sourceContent)

		// Call LLM to process the document
		llmResponse, err := s.callLLM(ctx, schema.WikiModelID, prompt)
		if err != nil {
			logger.Errorf(ctx, err, "[Wiki] LLM call failed for knowledge %s", knowledgeID)
			continue
		}

		// Parse the LLM response
		pageOps, err := parseIngestResponse(llmResponse)
		if err != nil {
			logger.Warnf(ctx, "[Wiki] Failed to parse LLM response for knowledge %s: %v", knowledgeID, err)
			continue
		}

		// Apply the page operations
		for _, op := range pageOps.Pages {
			if err := s.applyPageOp(ctx, tenantID, kbID, knowledgeID, &op); err != nil {
				logger.Warnf(ctx, "[Wiki] Failed to apply page op %q: %v", op.Title, err)
			}
		}
	}

	return nil
}

// getKnowledgeContent retrieves concatenated text content from knowledge chunks.
func (s *wikiService) getKnowledgeContent(ctx context.Context, knowledgeID string) (string, error) {
	chunks, err := s.chunkService.ListChunksByKnowledgeID(ctx, knowledgeID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, chunk := range chunks {
		if chunk.ChunkType == types.ChunkTypeText || chunk.ChunkType == types.ChunkTypeSummary {
			sb.WriteString(chunk.Content)
			sb.WriteString("\n\n")
		}
	}
	return sb.String(), nil
}

// applyPageOp creates or updates a wiki page based on an LLM-generated operation.
func (s *wikiService) applyPageOp(ctx context.Context, tenantID uint64, kbID, knowledgeID string, op *ingestPageOp) error {
	slug := op.Slug
	if slug == "" {
		slug = titleToSlug(op.Title)
	}

	tagsJSON, _ := json.Marshal(op.Tags)
	linksJSON, _ := json.Marshal(op.LinksTo)
	sourceIDsJSON, _ := json.Marshal([]string{knowledgeID})

	if op.Action == "update" {
		// Try to find existing page by title
		existing, err := s.wikiRepo.GetPageByTitle(ctx, tenantID, kbID, op.Title)
		if err == nil && existing != nil {
			// Merge source knowledge IDs
			var existingSources []string
			if len(existing.SourceKnowledgeIDs) > 0 {
				_ = json.Unmarshal(existing.SourceKnowledgeIDs, &existingSources)
			}
			if !contains(existingSources, knowledgeID) {
				existingSources = append(existingSources, knowledgeID)
			}
			mergedSourcesJSON, _ := json.Marshal(existingSources)

			existing.Content = op.Content
			existing.Summary = op.Summary
			existing.Tags = types.JSON(tagsJSON)
			existing.OutLinks = types.JSON(linksJSON)
			existing.SourceKnowledgeIDs = types.JSON(mergedSourcesJSON)
			existing.Version++
			return s.wikiRepo.UpdatePage(ctx, existing)
		}
		// Fall through to create if not found
	}

	page := &types.WikiPage{
		TenantID:           tenantID,
		KnowledgeBaseID:    kbID,
		Title:              op.Title,
		Slug:               slug,
		PageType:           op.PageType,
		Content:            op.Content,
		Summary:            op.Summary,
		Status:             types.WikiPageStatusActive,
		Tags:               types.JSON(tagsJSON),
		OutLinks:           types.JSON(linksJSON),
		SourceKnowledgeIDs: types.JSON(sourceIDsJSON),
		Version:            1,
	}
	return s.wikiRepo.CreatePage(ctx, page)
}

// --- Query ---

func (s *wikiService) QueryWiki(ctx context.Context, kbID string, query string, topK int) ([]*types.WikiQueryResult, error) {
	tenantID := getTenantIDFromContext(ctx)

	if topK <= 0 {
		topK = 10
	}

	// Search wiki pages by content matching
	pages, err := s.wikiRepo.SearchPages(ctx, tenantID, kbID, query, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to search wiki pages: %w", err)
	}

	results := make([]*types.WikiQueryResult, 0, len(pages))
	for _, page := range pages {
		excerpt := extractExcerpt(page.Content, query, 300)
		results = append(results, &types.WikiQueryResult{
			Page:      page,
			Relevance: 1.0, // Placeholder; real scoring would use embeddings
			Excerpt:   excerpt,
		})
	}

	return results, nil
}

// --- Lint ---

func (s *wikiService) LintWiki(ctx context.Context, kbID string) ([]*types.WikiLintIssue, error) {
	tenantID := getTenantIDFromContext(ctx)

	pages, _, err := s.wikiRepo.ListPages(ctx, tenantID, kbID, "", 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list wiki pages: %w", err)
	}

	var issues []*types.WikiLintIssue

	// Build page title set for link validation
	pageTitles := make(map[string]*types.WikiPage)
	for _, page := range pages {
		pageTitles[page.Title] = page
	}

	for _, page := range pages {
		// Check for broken links
		wikiLinks := extractWikiLinks(page.Content)
		for _, link := range wikiLinks {
			if _, exists := pageTitles[link]; !exists {
				issue := &types.WikiLintIssue{
					TenantID:        tenantID,
					KnowledgeBaseID: kbID,
					WikiPageID:      page.ID,
					IssueType:       "broken_link",
					Severity:        types.WikiLintSeverityWarning,
					Description:     fmt.Sprintf("Page %q contains a wiki link to %q which does not exist", page.Title, link),
					SuggestedFix:    fmt.Sprintf("Create a new page titled %q or remove the broken link", link),
				}
				if err := s.wikiRepo.CreateLintIssue(ctx, issue); err != nil {
					logger.Warnf(ctx, "[Wiki] Failed to create lint issue: %v", err)
				}
				issues = append(issues, issue)
			}
		}

		// Check for orphan pages (no in-links, not an index page)
		var inLinks []string
		if len(page.InLinks) > 0 {
			_ = json.Unmarshal(page.InLinks, &inLinks)
		}
		if len(inLinks) == 0 && page.PageType != types.WikiPageTypeIndex && page.PageType != types.WikiPageTypeLog {
			// Verify it's truly orphaned by checking if any page links to this page
			isOrphan := true
			for _, otherPage := range pages {
				if otherPage.ID == page.ID {
					continue
				}
				otherLinks := extractWikiLinks(otherPage.Content)
				if containsStr(otherLinks, page.Title) {
					isOrphan = false
					break
				}
			}
			if isOrphan {
				issue := &types.WikiLintIssue{
					TenantID:        tenantID,
					KnowledgeBaseID: kbID,
					WikiPageID:      page.ID,
					IssueType:       "orphan_page",
					Severity:        types.WikiLintSeverityInfo,
					Description:     fmt.Sprintf("Page %q has no incoming links from other pages", page.Title),
					SuggestedFix:    "Add references to this page from related pages, or consider merging its content",
				}
				if err := s.wikiRepo.CreateLintIssue(ctx, issue); err != nil {
					logger.Warnf(ctx, "[Wiki] Failed to create lint issue: %v", err)
				}
				issues = append(issues, issue)
			}
		}
	}

	return issues, nil
}

// --- CRUD ---

func (s *wikiService) GetPage(ctx context.Context, pageID string) (*types.WikiPage, error) {
	tenantID := getTenantIDFromContext(ctx)
	return s.wikiRepo.GetPageByID(ctx, tenantID, pageID)
}

func (s *wikiService) GetPageBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error) {
	tenantID := getTenantIDFromContext(ctx)
	return s.wikiRepo.GetPageBySlug(ctx, tenantID, kbID, slug)
}

func (s *wikiService) ListPages(ctx context.Context, kbID string, pageType string, page *types.Pagination) (*types.PageResult, error) {
	tenantID := getTenantIDFromContext(ctx)

	offset := 0
	limit := 20
	if page != nil {
		offset = (page.Page - 1) * page.PageSize
		limit = page.PageSize
	}

	pages, total, err := s.wikiRepo.ListPages(ctx, tenantID, kbID, pageType, offset, limit)
	if err != nil {
		return nil, err
	}

	return &types.PageResult{
		Total: total,
		Data:  pages,
	}, nil
}

func (s *wikiService) UpdatePage(ctx context.Context, page *types.WikiPage) error {
	page.Version++
	return s.wikiRepo.UpdatePage(ctx, page)
}

func (s *wikiService) DeletePage(ctx context.Context, pageID string) error {
	tenantID := getTenantIDFromContext(ctx)
	return s.wikiRepo.DeletePage(ctx, tenantID, pageID)
}

// --- Stats ---

func (s *wikiService) GetStats(ctx context.Context, kbID string) (*types.WikiStats, error) {
	tenantID := getTenantIDFromContext(ctx)

	totalPages, err := s.wikiRepo.CountPages(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}

	pagesByType, err := s.wikiRepo.CountPagesByType(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}

	orphanPages, err := s.wikiRepo.FindOrphanPages(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}

	openIssues, err := s.wikiRepo.CountOpenLintIssues(ctx, tenantID, kbID)
	if err != nil {
		return nil, err
	}

	return &types.WikiStats{
		TotalPages:     totalPages,
		PagesByType:    pagesByType,
		OrphanPages:    int64(len(orphanPages)),
		OpenLintIssues: openIssues,
	}, nil
}

// --- Lint Issues ---

func (s *wikiService) ListLintIssues(ctx context.Context, kbID string) ([]*types.WikiLintIssue, error) {
	tenantID := getTenantIDFromContext(ctx)
	return s.wikiRepo.ListLintIssues(ctx, tenantID, kbID, false)
}

func (s *wikiService) ResolveLintIssue(ctx context.Context, issueID string) error {
	tenantID := getTenantIDFromContext(ctx)
	return s.wikiRepo.ResolveLintIssue(ctx, tenantID, issueID)
}

// --- Helper functions ---

// getTenantIDFromContext extracts tenant ID from context using WeKnora's context helpers.
func getTenantIDFromContext(ctx context.Context) uint64 {
	id, _ := types.TenantIDFromContext(ctx)
	return id
}

// callLLM invokes the LLM with the given prompt. This integrates with
// WeKnora's model service to use the configured wiki model.
func (s *wikiService) callLLM(ctx context.Context, modelID string, prompt string) (string, error) {
	if modelID == "" {
		return "", fmt.Errorf("wiki model ID is not configured")
	}

	chatModel, err := s.modelService.GetChatModel(ctx, modelID)
	if err != nil {
		return "", fmt.Errorf("failed to get chat model %s: %w", modelID, err)
	}

	messages := []chat.Message{
		{Role: "user", Content: prompt},
	}

	response, err := chatModel.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("LLM chat failed: %w", err)
	}

	return response.Content, nil
}

// buildIngestPrompt constructs the full ingest prompt from template components.
func buildIngestPrompt(schemaContent, existingPages, sourceTitle, sourceContent string) string {
	// Truncate source content if too long (keep ~8000 chars for the source, leaving room for schema/pages)
	if len(sourceContent) > 8000 {
		sourceContent = sourceContent[:8000] + "\n\n[Content truncated...]"
	}

	return fmt.Sprintf(`You are a wiki knowledge curator. Your task is to read source documents and maintain a structured wiki of interlinked Markdown pages.

## Wiki Schema
%s

## Existing Wiki Pages (titles and summaries)
%s

## Source Document to Process
Title: %s
Content:
%s

## Your Task
Analyze the source document and produce a JSON response with wiki page operations. For each entity, concept, or topic worth documenting:

1. **Entity Pages**: Create or update pages for named entities (people, organizations, products, technologies, etc.)
2. **Concept Pages**: Create or update pages for abstract concepts, methodologies, or domains
3. **Synthesis Pages**: If this document, combined with existing wiki content, reveals cross-cutting insights, create synthesis pages
4. **Log Entry**: Create a brief ingestion log entry documenting what was processed

For each page, use [[Page Title]] wiki-link syntax to reference other pages.

Respond with ONLY a JSON object (no markdown code fences):
{
  "pages": [
    {
      "action": "create" or "update",
      "title": "Page Title",
      "slug": "page-title",
      "page_type": "entity" | "concept" | "synthesis" | "log",
      "content": "Full Markdown content with [[wiki links]]...",
      "summary": "One-line summary of the page",
      "tags": ["tag1", "tag2"],
      "links_to": ["Other Page Title", "Another Page"]
    }
  ]
}

Important:
- When updating an existing page, merge new information with existing content.
- Use [[Page Title]] syntax consistently for all cross-references.
- Each page should be self-contained yet well-connected.
- Prefer factual, concise language.`, schemaContent, existingPages, sourceTitle, sourceContent)
}

// buildExistingPagesContext creates a summary of existing wiki pages for LLM context.
func buildExistingPagesContext(pages []*types.WikiPage) string {
	if len(pages) == 0 {
		return "(No existing wiki pages)"
	}

	var sb strings.Builder
	for _, page := range pages {
		sb.WriteString(fmt.Sprintf("- **%s** [%s]: %s\n", page.Title, page.PageType, page.Summary))
	}
	return sb.String()
}

// parseIngestResponse extracts structured page operations from the LLM response.
func parseIngestResponse(response string) (*ingestResponse, error) {
	// Try to extract JSON from the response (may be wrapped in markdown code fences)
	jsonStr := response
	if idx := strings.Index(response, "{"); idx >= 0 {
		jsonStr = response[idx:]
	}
	if idx := strings.LastIndex(jsonStr, "}"); idx >= 0 {
		jsonStr = jsonStr[:idx+1]
	}

	var result ingestResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse ingest response JSON: %w", err)
	}
	return &result, nil
}

// titleToSlug converts a page title to a URL-friendly slug.
func titleToSlug(title string) string {
	slug := strings.ToLower(title)
	slug = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, slug)
	// Collapse multiple dashes
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = fmt.Sprintf("page-%d", time.Now().UnixNano())
	}
	return slug
}

// extractWikiLinks finds all [[Page Title]] links in content.
var wikiLinkRegex = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

func extractWikiLinks(content string) []string {
	matches := wikiLinkRegex.FindAllStringSubmatch(content, -1)
	links := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			links = append(links, match[1])
		}
	}
	return links
}

// extractExcerpt extracts a relevant excerpt from content around the query terms.
func extractExcerpt(content, query string, maxLen int) string {
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)

	// Find the first occurrence of any query word
	words := strings.Fields(lowerQuery)
	bestIdx := -1
	for _, word := range words {
		idx := strings.Index(lowerContent, word)
		if idx >= 0 && (bestIdx < 0 || idx < bestIdx) {
			bestIdx = idx
		}
	}

	if bestIdx < 0 {
		// No match found, return the beginning
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}

	// Extract around the match
	start := bestIdx - maxLen/3
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
	}

	excerpt := content[start:end]
	if start > 0 {
		excerpt = "..." + excerpt
	}
	if end < len(content) {
		excerpt = excerpt + "..."
	}
	return excerpt
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsStr(slice []string, item string) bool {
	return contains(slice, item)
}
