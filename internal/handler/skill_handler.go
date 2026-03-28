package handler

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

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

// ListSkills godoc
// @Summary      获取预装Skills列表
// @Description  获取所有预装的Agent Skills元数据
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

	skillsMetadata, err := h.skillService.ListPreloadedSkills(ctx)
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

// CreateSkill godoc
// @Summary      Create a new skill
// @Description  Create a new database-backed skill for the current tenant
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        request  body      types.CreateSkillRequest  true  "Skill creation request"
// @Success      201      {object}  map[string]interface{}    "Created skill"
// @Failure      400      {object}  errors.AppError           "Bad request"
// @Failure      409      {object}  errors.AppError           "Conflict"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills [post]
func (h *SkillHandler) CreateSkill(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	var req types.CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse create skill request", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		c.Error(errors.NewValidationError("Skill name must be between 1 and 64 characters"))
		return
	}

	// Validate description
	req.Description = strings.TrimSpace(req.Description)
	if req.Description == "" || len(req.Description) > 1024 {
		c.Error(errors.NewValidationError("Skill description must be between 1 and 1024 characters"))
		return
	}

	// Security: check for prompt injection patterns
	if err := validateSkillContent(req.Instructions); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if err := validateSkillContent(req.Description); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}

	record, err := h.skillService.CreateSkill(ctx, tenantID, &req)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		if strings.Contains(err.Error(), "already exists") {
			c.Error(errors.NewConflictError("Skill with this name already exists"))
			return
		}
		c.Error(errors.NewInternalServerError("Failed to create skill: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Skill created successfully, ID: %d, name: %s", record.ID, record.Name)
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    record,
	})
}

// GetSkill godoc
// @Summary      Get skill details
// @Description  Get details of a specific skill by ID
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id   path      int                       true  "Skill ID"
// @Success      200  {object}  map[string]interface{}    "Skill details"
// @Failure      400  {object}  errors.AppError           "Bad request"
// @Failure      404  {object}  errors.AppError           "Not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{id} [get]
func (h *SkillHandler) GetSkill(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	skillID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid skill ID"))
		return
	}

	record, err := h.skillService.GetSkillByID(ctx, tenantID, skillID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError("Skill not found"))
			return
		}
		c.Error(errors.NewInternalServerError("Failed to get skill: " + err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    record,
	})
}

// UpdateSkill godoc
// @Summary      Update a skill
// @Description  Update an existing skill's description, instructions, or status
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id       path      int                        true  "Skill ID"
// @Param        request  body      types.UpdateSkillRequest   true  "Skill update request"
// @Success      200      {object}  map[string]interface{}     "Updated skill"
// @Failure      400      {object}  errors.AppError            "Bad request"
// @Failure      404      {object}  errors.AppError            "Not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{id} [put]
func (h *SkillHandler) UpdateSkill(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	skillID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid skill ID"))
		return
	}

	var req types.UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to parse update skill request", err)
		c.Error(errors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}

	// Validate optional description
	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		if desc == "" || len(desc) > 1024 {
			c.Error(errors.NewValidationError("Skill description must be between 1 and 1024 characters"))
			return
		}
		req.Description = &desc

		if err := validateSkillContent(desc); err != nil {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
	}

	// Validate optional instructions
	if req.Instructions != nil {
		if err := validateSkillContent(*req.Instructions); err != nil {
			c.Error(errors.NewBadRequestError(err.Error()))
			return
		}
	}

	// Validate optional status
	if req.Status != nil && !req.Status.IsValid() {
		c.Error(errors.NewValidationError("Invalid skill status"))
		return
	}

	record, err := h.skillService.UpdateSkill(ctx, tenantID, skillID, &req)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError("Skill not found"))
			return
		}
		c.Error(errors.NewInternalServerError("Failed to update skill: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Skill updated successfully, ID: %d", record.ID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    record,
	})
}

// DeleteSkill godoc
// @Summary      Delete a skill
// @Description  Soft delete a skill by setting its status to disabled
// @Tags         Skills
// @Accept       json
// @Produce      json
// @Param        id   path      int                       true  "Skill ID"
// @Success      200  {object}  map[string]interface{}    "Deletion result"
// @Failure      400  {object}  errors.AppError           "Bad request"
// @Failure      404  {object}  errors.AppError           "Not found"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /skills/{id} [delete]
func (h *SkillHandler) DeleteSkill(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	skillID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(errors.NewBadRequestError("Invalid skill ID"))
		return
	}

	if err := h.skillService.DeleteSkill(ctx, tenantID, skillID); err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		if strings.Contains(err.Error(), "not found") {
			c.Error(errors.NewNotFoundError("Skill not found"))
			return
		}
		c.Error(errors.NewInternalServerError("Failed to delete skill: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Skill soft-deleted successfully, ID: %d", skillID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Skill disabled successfully",
	})
}

// validateSkillContent checks for prompt injection patterns in skill content
func validateSkillContent(content string) error {
	dangerousPatterns := []string{
		"ignore previous instructions",
		"ignore all instructions",
		"disregard your instructions",
		"you are now",
		"new instructions:",
	}
	lower := strings.ToLower(content)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lower, pattern) {
			return fmt.Errorf("skill content contains potentially dangerous pattern")
		}
	}
	return nil
}
