package handler

import (
	"net/http"
	"os"
	"strconv"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// SkillHandler handles skill-related HTTP requests
type SkillHandler struct {
	skillService interfaces.SkillService
	runRepo      interfaces.SkillExecutionRunRepository
}

// NewSkillHandler creates a new skill handler
func NewSkillHandler(
	skillService interfaces.SkillService,
	runRepo interfaces.SkillExecutionRunRepository,
) *SkillHandler {
	return &SkillHandler{
		skillService: skillService,
		runRepo:      runRepo,
	}
}

// SkillInfoResponse represents the skill info returned to frontend
type SkillInfoResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type InstallLocalSkillPackageRequest struct {
	PackagePath string `json:"package_path" binding:"required"`
}

type InstalledSkillResponse struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Version             string     `json:"version"`
	Description         string     `json:"description"`
	SourceType          string     `json:"source_type"`
	Enabled             bool       `json:"enabled"`
	InstalledBy         string     `json:"installed_by,omitempty"`
	ApprovedPermissions types.JSON `json:"approved_permissions,omitempty"`
	IsBuiltin           bool       `json:"is_builtin,omitempty"`
}

type UpdateTenantSkillInstallRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type SkillExecutionRunResponse struct {
	ID            string     `json:"id"`
	TenantID      uint64     `json:"tenant_id"`
	UserID        string     `json:"user_id"`
	AgentID       string     `json:"agent_id"`
	SessionID     string     `json:"session_id"`
	MessageID     string     `json:"message_id"`
	ToolCallID    string     `json:"tool_call_id"`
	SkillID       string     `json:"skill_id"`
	ScriptPath    string     `json:"script_path"`
	Status        string     `json:"status"`
	DurationMS    int64      `json:"duration_ms"`
	ResourceUsage types.JSON `json:"resource_usage"`
	ErrorSummary  string     `json:"error_summary,omitempty"`
	CreatedAt     string     `json:"created_at"`
}

// ListSkills godoc
// @Summary      获取可用 Skills 列表
// @Description  获取当前部署中可用的 Agent Skills 元数据。预装 Skills 会在启动时同步到注册表。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "Skills列表"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills [get]
func (h *SkillHandler) ListSkills(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	skillsMetadata, err := h.skillService.ListTenantSkills(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list skills: " + err.Error()))
		return
	}

	// Convert to response format
	var response []SkillInfoResponse
	for _, meta := range skillsMetadata {
		response = append(response, SkillInfoResponse{
			Name:        meta.Name,
			Description: meta.Description,
		})
	}

	// skills_available: true only when sandbox is enabled (docker or local), so frontend can hide/disable Skills UI
	sandboxMode := os.Getenv("WEKNORA_SANDBOX_MODE")
	skillsAvailable := sandboxMode != "" && sandboxMode != "disabled"

	logger.Infof(ctx, "skills_available: %v, sandboxMode: %s", skillsAvailable, sandboxMode)

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"data":             response,
		"skills_available": skillsAvailable,
	})
}

// InstallLocalSkillPackage godoc
// @Summary      安装本地 Skill 包
// @Description  从服务端配置的本地 Skill packages 目录安装一个 Skill 到当前租户。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        request  body      InstallLocalSkillPackageRequest  true  "本地 Skill 包路径"
// @Success      200      {object}  map[string]interface{}           "安装结果"
// @Failure      400      {object}  map[string]interface{}           "请求参数错误"
// @Failure      500      {object}  errors.AppError                  "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/install-local [post]
func (h *SkillHandler) InstallLocalSkillPackage(c *gin.Context) {
	ctx := c.Request.Context()

	var req InstallLocalSkillPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "package_path is required",
		})
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		userID, _ = types.UserIDFromContext(ctx)
	}

	entry, err := h.skillService.InstallLocalSkillPackage(ctx, tenantID, req.PackagePath, userID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to install local skill package: " + err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": InstalledSkillResponse{
			ID:          entry.ID,
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			SourceType:  entry.SourceType,
			Enabled:     true,
			IsBuiltin:   entry.IsBuiltin,
		},
	})
}

// ListInstalledSkills godoc
// @Summary      获取租户已安装 Skills
// @Description  获取当前租户的 Skill 安装记录，包含启用和禁用状态。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "已安装 Skills"
// @Failure      500  {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/installed [get]
func (h *SkillHandler) ListInstalledSkills(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())

	installs, err := h.skillService.ListTenantSkillInstalls(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list installed skills: " + err.Error()))
		return
	}

	response := make([]InstalledSkillResponse, 0, len(installs))
	for _, install := range installs {
		response = append(response, InstalledSkillResponse{
			ID:                  install.SkillID,
			Name:                install.Name,
			Version:             install.Version,
			Description:         install.Description,
			SourceType:          install.SourceType,
			Enabled:             install.Enabled,
			InstalledBy:         install.InstalledBy,
			ApprovedPermissions: install.ApprovedPermissions,
			IsBuiltin:           install.IsBuiltin,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// UpdateTenantSkillInstall godoc
// @Summary      启用或禁用租户 Skill
// @Description  更新当前租户某个 Skill 安装记录的 enabled 状态。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        skill_id  path      string                           true  "Skill ID"
// @Param        request   body      UpdateTenantSkillInstallRequest  true  "启停状态"
// @Success      200       {object}  map[string]interface{}           "更新结果"
// @Failure      400       {object}  map[string]interface{}           "请求参数错误"
// @Failure      500       {object}  errors.AppError                  "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{skill_id} [patch]
func (h *SkillHandler) UpdateTenantSkillInstall(c *gin.Context) {
	ctx := c.Request.Context()
	skillID := c.Param("skill_id")

	var req UpdateTenantSkillInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "enabled is required",
		})
		return
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if err := h.skillService.SetTenantSkillEnabled(ctx, tenantID, skillID, *req.Enabled); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to update tenant skill install: " + err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// ListSkillExecutionRuns godoc
// @Summary      获取 Skill 执行审计记录
// @Description  获取当前租户最近的 Skill 脚本执行记录。
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        limit  query     int  false  "返回数量，默认50，最大100"
// @Success      200    {object}  map[string]interface{}  "Skill执行记录"
// @Failure      500    {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/runs [get]
func (h *SkillHandler) ListSkillExecutionRuns(c *gin.Context) {
	ctx := c.Request.Context()
	if h.runRepo == nil {
		c.Error(errors.NewInternalServerError("Skill execution run repository is not configured"))
		return
	}

	limit := 50
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "limit must be a positive integer",
			})
			return
		}
		limit = parsed
	}

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	runs, err := h.runRepo.ListSkillExecutionRuns(ctx, tenantID, limit)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError("Failed to list skill execution runs: " + err.Error()))
		return
	}

	response := make([]SkillExecutionRunResponse, 0, len(runs))
	for _, run := range runs {
		response = append(response, SkillExecutionRunResponse{
			ID:            run.ID,
			TenantID:      run.TenantID,
			UserID:        run.UserID,
			AgentID:       run.AgentID,
			SessionID:     run.SessionID,
			MessageID:     run.MessageID,
			ToolCallID:    run.ToolCallID,
			SkillID:       run.SkillID,
			ScriptPath:    run.ScriptPath,
			Status:        run.Status,
			DurationMS:    run.DurationMS,
			ResourceUsage: run.ResourceUsage,
			ErrorSummary:  run.ErrorSummary,
			CreatedAt:     run.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}
