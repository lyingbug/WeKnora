package handler

import (
	"net/http"
	"strconv"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// WikiHandler processes HTTP requests related to wiki knowledge layer resources.
type WikiHandler struct {
	wikiService interfaces.WikiService
}

// NewWikiHandler creates a new wiki handler instance.
func NewWikiHandler(wikiService interfaces.WikiService) *WikiHandler {
	return &WikiHandler{
		wikiService: wikiService,
	}
}

// GetSchema returns the wiki schema for a knowledge base.
// GET /api/v1/knowledge-bases/:id/wiki/schema
func (h *WikiHandler) GetSchema(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	schema, err := h.wikiService.GetOrCreateSchema(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, schema)
}

// UpdateSchema updates the wiki schema configuration.
// PUT /api/v1/knowledge-bases/:id/wiki/schema
func (h *WikiHandler) UpdateSchema(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	var schema types.WikiSchema
	if err := c.ShouldBindJSON(&schema); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	schema.KnowledgeBaseID = kbID

	if err := h.wikiService.UpdateSchema(c.Request.Context(), &schema); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, schema)
}

// IngestKnowledge triggers wiki ingestion for specified knowledge items.
// POST /api/v1/knowledge-bases/:id/wiki/ingest
func (h *WikiHandler) IngestKnowledge(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	var req types.WikiIngestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	req.KnowledgeBaseID = kbID

	if err := h.wikiService.IngestKnowledge(c.Request.Context(), kbID, req.KnowledgeIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "wiki ingestion completed"})
}

// QueryWiki searches the wiki for answers to a query.
// POST /api/v1/knowledge-bases/:id/wiki/query
func (h *WikiHandler) QueryWiki(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	var req types.WikiQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	req.KnowledgeBaseID = kbID

	results, err := h.wikiService.QueryWiki(c.Request.Context(), kbID, req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// LintWiki performs health checks on the wiki.
// POST /api/v1/knowledge-bases/:id/wiki/lint
func (h *WikiHandler) LintWiki(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	issues, err := h.wikiService.LintWiki(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"issues": issues})
}

// ListPages lists wiki pages in a knowledge base.
// GET /api/v1/knowledge-bases/:id/wiki/pages
func (h *WikiHandler) ListPages(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	pageType := c.Query("page_type")
	page, pageSize := parsePagination(c)

	result, err := h.wikiService.ListPages(c.Request.Context(), kbID, pageType, &types.Pagination{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetPage retrieves a single wiki page.
// GET /api/v1/wiki/pages/:page_id
func (h *WikiHandler) GetPage(c *gin.Context) {
	pageID := c.Param("page_id")
	if pageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page ID is required"})
		return
	}

	page, err := h.wikiService.GetPage(c.Request.Context(), pageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	c.JSON(http.StatusOK, page)
}

// GetPageBySlug retrieves a wiki page by slug.
// GET /api/v1/knowledge-bases/:id/wiki/pages/by-slug/:slug
func (h *WikiHandler) GetPageBySlug(c *gin.Context) {
	kbID := c.Param("id")
	slug := c.Param("slug")
	if kbID == "" || slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID and slug are required"})
		return
	}

	page, err := h.wikiService.GetPageBySlug(c.Request.Context(), kbID, slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	c.JSON(http.StatusOK, page)
}

// UpdatePage updates a wiki page.
// PUT /api/v1/wiki/pages/:page_id
func (h *WikiHandler) UpdatePage(c *gin.Context) {
	pageID := c.Param("page_id")
	if pageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page ID is required"})
		return
	}

	// First get the existing page
	existingPage, err := h.wikiService.GetPage(c.Request.Context(), pageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	var update types.WikiPage
	if err := c.ShouldBindJSON(&update); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Apply updates to existing page
	if update.Title != "" {
		existingPage.Title = update.Title
	}
	if update.Content != "" {
		existingPage.Content = update.Content
	}
	if update.Summary != "" {
		existingPage.Summary = update.Summary
	}
	if update.Status != "" {
		existingPage.Status = update.Status
	}

	if err := h.wikiService.UpdatePage(c.Request.Context(), existingPage); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, existingPage)
}

// DeletePage deletes a wiki page.
// DELETE /api/v1/wiki/pages/:page_id
func (h *WikiHandler) DeletePage(c *gin.Context) {
	pageID := c.Param("page_id")
	if pageID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "page ID is required"})
		return
	}

	if err := h.wikiService.DeletePage(c.Request.Context(), pageID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "page deleted"})
}

// GetStats returns wiki statistics for a knowledge base.
// GET /api/v1/knowledge-bases/:id/wiki/stats
func (h *WikiHandler) GetStats(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	stats, err := h.wikiService.GetStats(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ListLintIssues lists unresolved lint issues.
// GET /api/v1/knowledge-bases/:id/wiki/lint/issues
func (h *WikiHandler) ListLintIssues(c *gin.Context) {
	kbID := c.Param("id")
	if kbID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "knowledge base ID is required"})
		return
	}

	issues, err := h.wikiService.ListLintIssues(c.Request.Context(), kbID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"issues": issues})
}

// ResolveLintIssue marks a lint issue as resolved.
// PUT /api/v1/wiki/lint/issues/:issue_id/resolve
func (h *WikiHandler) ResolveLintIssue(c *gin.Context) {
	issueID := c.Param("issue_id")
	if issueID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "issue ID is required"})
		return
	}

	if err := h.wikiService.ResolveLintIssue(c.Request.Context(), issueID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "issue resolved"})
}

// parsePagination extracts page and page_size from query params with defaults.
func parsePagination(c *gin.Context) (int, int) {
	page := 1
	pageSize := 20

	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}

	return page, pageSize
}
