package handler

import (
	"net/http"
	"os"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// SkillHandler handles skill-related HTTP requests
type SkillHandler struct {
	skillService interfaces.SkillService
}

// NewSkillHandler creates a new skill handler
func NewSkillHandler(skillService interfaces.SkillService) *SkillHandler {
	return &SkillHandler{
		skillService: skillService,
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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	SourceType  string `json:"source_type"`
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
		},
	})
}
