package handler

import (
	"errors"

	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service/memory"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// MemoryHandler serves the personal long-term memory API.
//
// Authorisation here is unusual for this codebase and deliberately so. Every
// other resource asks "what is your role in this workspace"; memory asks only
// "is this your own space". A workspace administrator has no endpoint that
// returns another person's memories — not their pages, not their notes, not
// their anchors. The service derives the space from the request principal and
// never accepts one from the client, so there is no identifier to tamper with.
type MemoryHandler struct {
	memoryService    interfaces.MemoryService
	settingsService  interfaces.MemorySettingsService
	wikiService      interfaces.WikiPageService
	kbService        interfaces.KnowledgeBaseService
	knowledgeService interfaces.KnowledgeService
}

// NewMemoryHandler creates the memory handler.
func NewMemoryHandler(
	memoryService interfaces.MemoryService,
	settingsService interfaces.MemorySettingsService,
	wikiService interfaces.WikiPageService,
	kbService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
) *MemoryHandler {
	return &MemoryHandler{
		memoryService:    memoryService,
		settingsService:  settingsService,
		wikiService:      wikiService,
		kbService:        kbService,
		knowledgeService: knowledgeService,
	}
}

// respondMemoryError maps service errors onto status codes.
//
// "Memory is off for you" is a 404 rather than a 403: from the caller's point
// of view there is no memory space, and saying "forbidden" would imply one
// exists that they are not allowed to see.
func (h *MemoryHandler) respondMemoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, memory.ErrMemoryNotEnabled), errors.Is(err, memory.ErrNoPrincipal):
		c.JSON(http.StatusNotFound, gin.H{"error": "memory is not enabled for this account"})
	case errors.Is(err, memory.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, repository.ErrMemoryPageNotFound),
		errors.Is(err, repository.ErrMemoryNoteNotFound),
		errors.Is(err, repository.ErrMemoryAnchorNotFound),
		errors.Is(err, repository.ErrMemorySpaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, repository.ErrMemoryPageConflict):
		// The editor holds a version that is no longer current. 409 lets the
		// UI reload and show the newer text instead of reporting a fault.
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, memory.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		logger.ErrorWithFields(c.Request.Context(), err, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "memory request failed"})
	}
}

// getSlugParam reads gin's wildcard slug parameter.
func memorySlugParam(c *gin.Context) string {
	return strings.TrimSpace(strings.TrimPrefix(c.Param("slug"), "/"))
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Space and settings
// ---------------------------------------------------------------------------

// GetSpace godoc
// @Summary      Get the caller's memory space
// @Description  Returns the caller's memory space with statistics and capability flags
// @Tags         Memory
// @Produce      json
// @Success      200  {object}  types.MemorySpaceView
// @Failure      404  {object}  errors.AppError
// @Security     Bearer
// @Router       /memory/space [get]
func (h *MemoryHandler) GetSpace(c *gin.Context) {
	view, err := h.memoryService.SpaceView(c.Request.Context())
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}

// GetSettings godoc
// @Summary      Get effective memory settings
// @Description  Returns every setting with its effective value, the layer that decided it and whether a wider layer locks it
// @Tags         Memory
// @Produce      json
// @Success      200  {object}  types.MemorySettingsView
// @Security     Bearer
// @Router       /memory/settings [get]
func (h *MemoryHandler) GetSettings(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := types.TenantIDFromContext(ctx)
	userID, _ := types.UserIDFromContext(ctx)

	view, err := h.settingsService.View(ctx, types.MemorySettingsResolveOptions{
		TenantID: tenantID, UserID: userID,
	}, types.MemoryLayerUser)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}

// UpdateSettings godoc
// @Summary      Update the caller's memory settings
// @Description  Patches the user layer; keys locked by a wider layer are reported back rather than silently ignored
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        request  body      types.MemorySettingsUpdateRequest  true  "Settings patch"
// @Success      200      {object}  types.MemorySettingsUpdateResponse
// @Security     Bearer
// @Router       /memory/settings [put]
func (h *MemoryHandler) UpdateSettings(c *gin.Context) {
	ctx := c.Request.Context()
	var req types.MemorySettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	userID, _ := types.UserIDFromContext(ctx)
	if userID == "" {
		h.respondMemoryError(c, memory.ErrNoPrincipal)
		return
	}

	notes, err := h.settingsService.UpdateUser(ctx, tenantID, userID, req.Settings)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	view, err := h.settingsService.View(ctx, types.MemorySettingsResolveOptions{
		TenantID: tenantID, UserID: userID,
	}, types.MemoryLayerUser)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": types.MemorySettingsUpdateResponse{View: *view, Notes: notes}})
}

// GetTenantSettings godoc
// @Summary      Get workspace memory settings
// @Tags         Memory
// @Produce      json
// @Success      200  {object}  types.MemorySettingsView
// @Security     Bearer
// @Router       /memory/tenant-settings [get]
func (h *MemoryHandler) GetTenantSettings(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := types.TenantIDFromContext(ctx)

	view, err := h.settingsService.View(ctx, types.MemorySettingsResolveOptions{
		TenantID: tenantID,
	}, types.MemoryLayerTenant)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view})
}

// UpdateTenantSettings godoc
// @Summary      Update workspace memory settings
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        request  body      types.MemorySettingsUpdateRequest  true  "Settings patch"
// @Success      200      {object}  types.MemorySettingsUpdateResponse
// @Security     Bearer
// @Router       /memory/tenant-settings [put]
func (h *MemoryHandler) UpdateTenantSettings(c *gin.Context) {
	ctx := c.Request.Context()
	var req types.MemorySettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	tenantID, _ := types.TenantIDFromContext(ctx)

	notes, err := h.settingsService.UpdateTenant(ctx, tenantID, req.Settings)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	view, err := h.settingsService.View(ctx, types.MemorySettingsResolveOptions{
		TenantID: tenantID,
	}, types.MemoryLayerTenant)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": types.MemorySettingsUpdateResponse{View: *view, Notes: notes}})
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

// ListPages godoc
// @Summary      List memories
// @Tags         Memory
// @Produce      json
// @Param        type       query  string  false  "Comma-separated memory types"
// @Param        status     query  string  false  "Comma-separated statuses"
// @Param        query      query  string  false  "Keyword filter"
// @Param        page       query  int     false  "Page number"
// @Param        page_size  query  int     false  "Page size"
// @Success      200  {object}  types.MemoryPageListResponse
// @Security     Bearer
// @Router       /memory/pages [get]
func (h *MemoryHandler) ListPages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	req := &types.MemoryPageListRequest{
		Types:    splitCSV(c.Query("type")),
		Statuses: splitCSV(c.Query("status")),
		Query:    c.Query("query"),
		Page:     page,
		PageSize: pageSize,
		SortBy:   c.Query("sort_by"),
		Desc:     c.DefaultQuery("sort_order", "desc") != "asc",
	}
	resp, err := h.memoryService.ListPages(c.Request.Context(), req)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// GetPage godoc
// @Summary      Get one memory
// @Tags         Memory
// @Produce      json
// @Param        slug  path  string  true  "Memory slug"
// @Success      200  {object}  types.MemoryPage
// @Security     Bearer
// @Router       /memory/pages/{slug} [get]
func (h *MemoryHandler) GetPage(c *gin.Context) {
	page, err := h.memoryService.GetPage(c.Request.Context(), memorySlugParam(c))
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": page})
}

// CreatePage godoc
// @Summary      Create a memory by hand
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        request  body  types.MemoryPageWriteRequest  true  "Memory"
// @Success      200  {object}  types.MemoryPage
// @Security     Bearer
// @Router       /memory/pages [post]
func (h *MemoryHandler) CreatePage(c *gin.Context) {
	var req types.MemoryPageWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	req.EditSource = types.MemoryEditSourceUser
	page, err := h.memoryService.WritePage(c.Request.Context(), &req)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": page})
}

// UpdatePage godoc
// @Summary      Update a memory
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        slug     path  string                        true  "Memory slug"
// @Param        request  body  types.MemoryPageWriteRequest  true  "Memory"
// @Success      200  {object}  types.MemoryPage
// @Security     Bearer
// @Router       /memory/pages/{slug} [put]
func (h *MemoryHandler) UpdatePage(c *gin.Context) {
	var req types.MemoryPageWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	req.Slug = memorySlugParam(c)
	req.EditSource = types.MemoryEditSourceUser

	page, err := h.memoryService.WritePage(c.Request.Context(), &req)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": page})
}

// DeletePage godoc
// @Summary      Delete a memory
// @Tags         Memory
// @Produce      json
// @Param        slug  path  string  true  "Memory slug"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /memory/pages/{slug} [delete]
func (h *MemoryHandler) DeletePage(c *gin.Context) {
	if err := h.memoryService.DeletePage(c.Request.Context(), memorySlugParam(c)); err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SearchPages godoc
// @Summary      Search memories
// @Tags         Memory
// @Produce      json
// @Param        q      query  string  true   "Query"
// @Param        limit  query  int     false  "Maximum results"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /memory/search [get]
func (h *MemoryHandler) SearchPages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	pages, err := h.memoryService.SearchPages(c.Request.Context(), c.Query("q"), limit)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": pages})
}

// ListRevisions godoc
// @Summary      List a memory's revisions
// @Tags         Memory
// @Produce      json
// @Param        slug  path  string  true  "Memory slug"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /memory/revisions/{slug} [get]
func (h *MemoryHandler) ListRevisions(c *gin.Context) {
	revisions, err := h.memoryService.ListRevisions(c.Request.Context(), memorySlugParam(c))
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": revisions})
}

// RevertPage godoc
// @Summary      Revert a memory to an earlier revision
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        request  body  types.MemoryRevertRequest  true  "Revert target"
// @Success      200  {object}  types.MemoryPage
// @Security     Bearer
// @Router       /memory/revert [post]
func (h *MemoryHandler) RevertPage(c *gin.Context) {
	var req types.MemoryRevertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	page, err := h.memoryService.RevertPage(c.Request.Context(), &req)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": page})
}

// ---------------------------------------------------------------------------
// Notes (review inbox)
// ---------------------------------------------------------------------------

// ListNotes godoc
// @Summary      List extracted observations
// @Tags         Memory
// @Produce      json
// @Param        status     query  string  false  "Comma-separated statuses"
// @Param        page       query  int     false  "Page number"
// @Param        page_size  query  int     false  "Page size"
// @Success      200  {object}  types.MemoryNoteListResponse
// @Security     Bearer
// @Router       /memory/notes [get]
func (h *MemoryHandler) ListNotes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	statuses := splitCSV(c.Query("status"))
	if len(statuses) == 0 {
		statuses = []string{types.MemoryNoteStatusPending}
	}
	resp, err := h.memoryService.ListNotes(c.Request.Context(), &types.MemoryNoteListRequest{
		Statuses: statuses,
		Types:    splitCSV(c.Query("type")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// PromoteNote godoc
// @Summary      Accept an observation as a memory
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        id       path  string                          true   "Note ID"
// @Param        request  body  types.MemoryNotePromoteRequest  false  "Edits to apply first"
// @Success      200  {object}  types.MemoryPage
// @Security     Bearer
// @Router       /memory/notes/{id}/promote [post]
func (h *MemoryHandler) PromoteNote(c *gin.Context) {
	var req types.MemoryNotePromoteRequest
	_ = c.ShouldBindJSON(&req)

	page, err := h.memoryService.PromoteNote(
		c.Request.Context(), secutils.SanitizeForLog(c.Param("id")), &req,
	)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": page})
}

// RejectNote godoc
// @Summary      Decline an observation
// @Tags         Memory
// @Produce      json
// @Param        id  path  string  true  "Note ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /memory/notes/{id}/reject [post]
func (h *MemoryHandler) RejectNote(c *gin.Context) {
	if err := h.memoryService.RejectNote(
		c.Request.Context(), secutils.SanitizeForLog(c.Param("id")),
	); err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// Graph, stats, anchors
// ---------------------------------------------------------------------------

// GetGraph godoc
// @Summary      Get the memory graph
// @Tags         Memory
// @Produce      json
// @Param        mode    query  string  false  "personal or bridged"
// @Param        center  query  string  false  "Centre slug for a neighbourhood view"
// @Param        depth   query  int     false  "Neighbourhood depth"
// @Param        limit   query  int     false  "Maximum nodes"
// @Success      200  {object}  types.MemoryGraphData
// @Security     Bearer
// @Router       /memory/graph [get]
func (h *MemoryHandler) GetGraph(c *gin.Context) {
	depth, _ := strconv.Atoi(c.DefaultQuery("depth", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))

	data, err := h.memoryService.Graph(c.Request.Context(), &types.MemoryGraphRequest{
		Mode:   c.DefaultQuery("mode", types.MemoryGraphModePersonal),
		Center: c.Query("center"),
		Depth:  depth,
		Types:  splitCSV(c.Query("types")),
		Limit:  limit,
	})
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	h.resolveDocumentTitles(c, data)
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// resolveDocumentTitles replaces document ids with document titles.
//
// A knowledge-base document is anchored by id, because that is what a retrieval
// result carries, but an id is not a label. The graph service has no knowledge
// service and should not grow one for a display concern, so the substitution
// happens here. A document that has since been deleted keeps its id, which at
// least says something is there.
func (h *MemoryHandler) resolveDocumentTitles(c *gin.Context, data *types.MemoryGraphData) {
	if data == nil || h.knowledgeService == nil {
		return
	}
	ids := make([]string, 0)
	for _, node := range data.Nodes {
		if node.Kind == types.MemoryGraphNodeKnowledge && node.Slug != "" {
			ids = append(ids, node.Slug)
		}
	}
	if len(ids) == 0 {
		return
	}
	tenantID, ok := types.TenantIDFromContext(c.Request.Context())
	if !ok {
		return
	}
	docs, err := h.knowledgeService.GetKnowledgeBatch(c.Request.Context(), tenantID, ids)
	if err != nil {
		return
	}
	titles := make(map[string]string, len(docs))
	for _, doc := range docs {
		if doc != nil && doc.ID != "" && doc.Title != "" {
			titles[doc.ID] = doc.Title
		}
	}
	for i := range data.Nodes {
		if data.Nodes[i].Kind != types.MemoryGraphNodeKnowledge {
			continue
		}
		if title, ok := titles[data.Nodes[i].Slug]; ok {
			data.Nodes[i].Title = title
		}
	}
}

// GetStats godoc
// @Summary      Get memory statistics
// @Tags         Memory
// @Produce      json
// @Success      200  {object}  types.MemoryStats
// @Security     Bearer
// @Router       /memory/stats [get]
func (h *MemoryHandler) GetStats(c *gin.Context) {
	stats, err := h.memoryService.Stats(c.Request.Context())
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// ListAnchors godoc
// @Summary      List the caller's knowledge-base anchors
// @Tags         Memory
// @Produce      json
// @Param        kb_id  query  string  false  "Restrict to one knowledge base"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /memory/anchors [get]
func (h *MemoryHandler) ListAnchors(c *gin.Context) {
	anchors, err := h.memoryService.ListAnchors(c.Request.Context(), c.Query("kb_id"))
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": anchors})
}

// AddAnchor godoc
// @Summary      Record a relationship to a knowledge-base page
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        request  body  types.MemoryAnchorRequest  true  "Anchor"
// @Success      200  {object}  types.MemoryAnchor
// @Security     Bearer
// @Router       /memory/anchors [post]
func (h *MemoryHandler) AddAnchor(c *gin.Context) {
	var req types.MemoryAnchorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	// Anchoring writes into a knowledge base's aggregate view, so the caller has
	// to be able to read that knowledge base. The route cannot check it: the id
	// arrives in the body, not the path, so KBAccessRead has nothing to bind to.
	// Without this, anyone could anchor into any knowledge base in the workspace
	// and colour its coverage and insight aggregates.
	if h.kbService != nil && strings.TrimSpace(req.KnowledgeBaseID) != "" {
		if _, err := h.kbService.GetKnowledgeBaseByID(c.Request.Context(), req.KnowledgeBaseID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": apperrors.NewNotFoundError("knowledge base not found").Error(),
			})
			return
		}
	}

	anchor, err := h.memoryService.AddAnchor(c.Request.Context(), &req)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": anchor})
}

// DeleteAnchor godoc
// @Summary      Remove an anchor
// @Tags         Memory
// @Produce      json
// @Param        id  path  string  true  "Anchor ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /memory/anchors/{id} [delete]
func (h *MemoryHandler) DeleteAnchor(c *gin.Context) {
	if err := h.memoryService.DeleteAnchor(
		c.Request.Context(), secutils.SanitizeForLog(c.Param("id")),
	); err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// Forget and export
// ---------------------------------------------------------------------------

// Forget godoc
// @Summary      Delete memories in bulk
// @Tags         Memory
// @Accept       json
// @Produce      json
// @Param        request  body  types.MemoryForgetRequest  true  "What to forget"
// @Success      200  {object}  types.MemoryForgetResponse
// @Security     Bearer
// @Router       /memory/forget [post]
func (h *MemoryHandler) Forget(c *gin.Context) {
	var req types.MemoryForgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.NewBadRequestError(err.Error()).Error()})
		return
	}
	resp, err := h.memoryService.Forget(c.Request.Context(), &req)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	logger.Infof(c.Request.Context(), "memory: forget scope=%s pages=%d notes=%d anchors=%d",
		req.Scope, resp.PagesDeleted, resp.NotesDeleted, resp.AnchorsDeleted)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Export godoc
// @Summary      Export the caller's memory
// @Description  Returns the full space as JSON for portability
// @Tags         Memory
// @Produce      json
// @Success      200  {object}  types.MemoryExport
// @Security     Bearer
// @Router       /memory/export [get]
func (h *MemoryHandler) Export(c *gin.Context) {
	export, err := h.memoryService.Export(c.Request.Context())
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="weknora-memory.json"`)
	c.JSON(http.StatusOK, export)
}

// ---------------------------------------------------------------------------
// Knowledge-base scoped views
// ---------------------------------------------------------------------------

// GetCoverage godoc
// @Summary      Get the caller's mastery coverage for a knowledge base
// @Tags         Memory
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Success      200  {object}  types.MemoryCoverage
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/memory/coverage [get]
func (h *MemoryHandler) GetCoverage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("kb_id"))
	targetKind := overlayTargetKind(c.Query("target"))

	pages, err := h.coverageUnits(c, kbID, targetKind)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	coverage, err := h.memoryService.Coverage(ctx, kbID, pages, targetKind)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": coverage})
}

// GetOverlay godoc
// @Summary      Get per-item illumination for a knowledge base
// @Description  Returns the caller's engagement state keyed by wiki slug or document id
// @Tags         Memory
// @Produce      json
// @Param        kb_id   path   string  true   "Knowledge base ID"
// @Param        target  query  string  false  "wiki_page (default) or knowledge"
// @Success      200  {object}  map[string]types.MemoryOverlayNode
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/memory/overlay [get]
//
// The wiki graph gets its overlay folded into the graph response, because it is
// drawing nodes anyway. A document list has no such carrier, so it asks here.
func (h *MemoryHandler) GetOverlay(c *gin.Context) {
	kbID := secutils.SanitizeForLog(c.Param("kb_id"))
	overlay, err := h.memoryService.Overlay(
		c.Request.Context(), kbID, overlayTargetKind(c.Query("target")),
	)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	if overlay == nil {
		// Illumination switched off, or no space yet. An empty map says "nothing
		// is lit" without the caller having to special-case a null.
		overlay = map[string]types.MemoryOverlayNode{}
	}
	c.JSON(http.StatusOK, gin.H{"data": overlay})
}

// overlayTargetKind maps the query parameter onto an anchor target kind.
func overlayTargetKind(raw string) string {
	if strings.TrimSpace(raw) == types.MemoryAnchorTargetKnowledge {
		return types.MemoryAnchorTargetKnowledge
	}
	return types.MemoryAnchorTargetWikiPage
}

// insightUnits projects the same units coverage uses, plus the size that decides
// whether something counts as thin.
func (h *MemoryHandler) insightUnits(
	c *gin.Context, kbID, targetKind string,
) ([]types.MemoryInsightPage, error) {
	ctx := c.Request.Context()
	if targetKind == types.MemoryAnchorTargetKnowledge {
		if h.knowledgeService == nil {
			return nil, nil
		}
		all, err := h.knowledgeService.ListKnowledgeByKnowledgeBaseID(ctx, kbID)
		if err != nil {
			return nil, err
		}
		out := make([]types.MemoryInsightPage, 0, len(all))
		for _, knowledge := range all {
			if knowledge == nil || knowledge.ID == "" {
				continue
			}
			// A document's stored size stands in for a wiki page's body length.
			// It is a coarser measure — a scanned PDF is large and may still say
			// little — but "asked about a great deal, and barely anything here"
			// is the signal, and size carries it.
			out = append(out, types.MemoryInsightPage{
				Slug: knowledge.ID, Title: knowledge.Title, ContentLength: int(knowledge.FileSize),
			})
		}
		return out, nil
	}

	all, err := h.wikiService.ListAllPages(ctx, kbID)
	if err != nil {
		return nil, err
	}
	out := make([]types.MemoryInsightPage, 0, len(all))
	for _, page := range all {
		if page.Status != types.WikiPageStatusPublished {
			continue
		}
		out = append(out, types.MemoryInsightPage{
			Slug: page.Slug, Title: page.Title, ContentLength: len(page.Content),
		})
	}
	return out, nil
}

// coverageUnits projects whatever the knowledge base is made of: wiki pages, or
// the documents of an ordinary knowledge base.
//
// Retired items are excluded from the denominator on purpose: taking something
// out of circulation should not make everyone's coverage drop for reasons
// unrelated to what they have read.
func (h *MemoryHandler) coverageUnits(
	c *gin.Context, kbID, targetKind string,
) ([]types.MemoryCoveragePage, error) {
	if targetKind == types.MemoryAnchorTargetKnowledge {
		return h.coverageDocuments(c, kbID)
	}
	return h.coveragePages(c, kbID)
}

// coverageDocuments projects an ordinary knowledge base's documents, keyed by
// id because that is what a retrieval anchor records as its target.
func (h *MemoryHandler) coverageDocuments(
	c *gin.Context, kbID string,
) ([]types.MemoryCoveragePage, error) {
	if h.knowledgeService == nil {
		return nil, nil
	}
	all, err := h.knowledgeService.ListKnowledgeByKnowledgeBaseID(c.Request.Context(), kbID)
	if err != nil {
		return nil, err
	}
	out := make([]types.MemoryCoveragePage, 0, len(all))
	for _, knowledge := range all {
		if knowledge == nil || knowledge.ID == "" {
			continue
		}
		out = append(out, types.MemoryCoveragePage{
			Slug:   knowledge.ID,
			Folder: firstFolderSegment(knowledge.FolderPath),
		})
	}
	return out, nil
}

// firstFolderSegment reduces "/a/b/c" to "a", matching how wiki coverage buckets
// by the first breadcrumb.
func firstFolderSegment(path string) string {
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if segment != "" {
			return segment
		}
	}
	return ""
}

// coveragePages projects the knowledge base's live wiki pages.
func (h *MemoryHandler) coveragePages(c *gin.Context, kbID string) ([]types.MemoryCoveragePage, error) {
	all, err := h.wikiService.ListAllPages(c.Request.Context(), kbID)
	if err != nil {
		return nil, err
	}
	out := make([]types.MemoryCoveragePage, 0, len(all))
	for _, page := range all {
		if page.Status != types.WikiPageStatusPublished {
			continue
		}
		folder := ""
		if len(page.CategoryPath) > 0 {
			folder = page.CategoryPath[0]
		}
		out = append(out, types.MemoryCoveragePage{Slug: page.Slug, Folder: folder})
	}
	return out, nil
}

// GetInsights godoc
// @Summary      Get anonymised memory insights for a knowledge base
// @Description  Aggregates anchors across everyone in the workspace under a k-anonymity threshold
// @Tags         Memory
// @Produce      json
// @Param        kb_id  path  string  true  "Knowledge base ID"
// @Success      200  {object}  types.MemoryInsightsResponse
// @Security     Bearer
// @Router       /knowledgebase/{kb_id}/memory/insights [get]
func (h *MemoryHandler) GetInsights(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("kb_id"))

	targetKind := overlayTargetKind(c.Query("target"))
	pages, err := h.insightUnits(c, kbID, targetKind)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}

	resp, err := h.memoryService.Insights(ctx, kbID, pages, targetKind)
	if err != nil {
		h.respondMemoryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// MemorySpaceIDForRequest is used by other handlers that need the caller's
// space without duplicating the resolution rules.
func (h *MemoryHandler) MemorySpaceIDForRequest(c *gin.Context) string {
	space, err := h.memoryService.GetSpace(c.Request.Context())
	if err != nil || space == nil {
		return ""
	}
	return space.ID
}
