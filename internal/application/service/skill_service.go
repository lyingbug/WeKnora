package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// DefaultPreloadedSkillsDir is the default directory for preloaded skills
const DefaultPreloadedSkillsDir = "skills/preloaded"

// skillService implements SkillService interface
type skillService struct {
	loader       *skills.Loader
	db           *gorm.DB
	preloadedDir string
	mu           sync.RWMutex
	initialized  bool
}

// NewSkillService creates a new skill service
func NewSkillService(db *gorm.DB) interfaces.SkillService {
	// Determine the preloaded skills directory
	preloadedDir := getPreloadedSkillsDir()

	return &skillService{
		db:           db,
		preloadedDir: preloadedDir,
		initialized:  false,
	}
}

// getPreloadedSkillsDir returns the path to the preloaded skills directory
func getPreloadedSkillsDir() string {
	// Check if SKILLS_DIR environment variable is set
	if dir := os.Getenv("WEKNORA_SKILLS_DIR"); dir != "" {
		return dir
	}

	// Try to find the skills directory relative to the executable
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		skillsDir := filepath.Join(execDir, DefaultPreloadedSkillsDir)
		if _, err := os.Stat(skillsDir); err == nil {
			return skillsDir
		}
	}

	// Try current working directory
	cwd, err := os.Getwd()
	if err == nil {
		skillsDir := filepath.Join(cwd, DefaultPreloadedSkillsDir)
		if _, err := os.Stat(skillsDir); err == nil {
			return skillsDir
		}
	}

	// Default to relative path (will be created if needed)
	return DefaultPreloadedSkillsDir
}

// ensureInitialized initializes the loader if not already done
func (s *skillService) ensureInitialized(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return nil
	}

	// Check if preloaded directory exists
	if _, err := os.Stat(s.preloadedDir); os.IsNotExist(err) {
		logger.Warnf(ctx, "Preloaded skills directory does not exist: %s", s.preloadedDir)
		// Create the directory to avoid repeated warnings
		if err := os.MkdirAll(s.preloadedDir, 0755); err != nil {
			logger.Warnf(ctx, "Failed to create preloaded skills directory: %v", err)
		}
	}

	// Create loader with preloaded directory
	s.loader = skills.NewLoader([]string{s.preloadedDir})
	s.initialized = true

	logger.Infof(ctx, "Skill service initialized with preloaded directory: %s", s.preloadedDir)

	return nil
}

// ListPreloadedSkills returns metadata for all preloaded skills
func (s *skillService) ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	metadata, err := s.loader.DiscoverSkills()
	if err != nil {
		logger.Errorf(ctx, "Failed to discover preloaded skills: %v", err)
		return nil, fmt.Errorf("failed to discover skills: %w", err)
	}

	logger.Infof(ctx, "Discovered %d preloaded skills", len(metadata))

	return metadata, nil
}

// GetSkillByName retrieves a skill by its name
func (s *skillService) GetSkillByName(ctx context.Context, name string) (*skills.Skill, error) {
	if err := s.ensureInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize skill service: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	skill, err := s.loader.LoadSkillInstructions(name)
	if err != nil {
		logger.Errorf(ctx, "Failed to load skill %s: %v", name, err)
		return nil, fmt.Errorf("failed to load skill: %w", err)
	}

	return skill, nil
}

// GetPreloadedDir returns the configured preloaded skills directory
func (s *skillService) GetPreloadedDir() string {
	return s.preloadedDir
}

// CreateSkill creates a new database-backed skill for the given tenant
func (s *skillService) CreateSkill(ctx context.Context, tenantID uint64, req *types.CreateSkillRequest) (*types.SkillRecord, error) {
	now := time.Now()
	record := &types.SkillRecord{
		TenantID:     tenantID,
		Name:         req.Name,
		Description:  req.Description,
		Instructions: req.Instructions,
		Status:       types.SkillStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	result := s.db.WithContext(ctx).
		Table("skills").
		Create(record)
	if result.Error != nil {
		if isDuplicateSkillKeyError(result.Error) {
			return nil, fmt.Errorf("skill already exists with name: %s", req.Name)
		}
		return nil, fmt.Errorf("failed to create skill: %w", result.Error)
	}

	logger.Infof(ctx, "Created skill ID=%d name=%s for tenant=%d", record.ID, record.Name, tenantID)
	return record, nil
}

// GetSkillByID retrieves a skill by its ID, scoped to the given tenant
func (s *skillService) GetSkillByID(ctx context.Context, tenantID uint64, skillID uint64) (*types.SkillRecord, error) {
	var record types.SkillRecord
	result := s.db.WithContext(ctx).
		Table("skills").
		Where("id = ? AND tenant_id = ?", skillID, tenantID).
		First(&record)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("skill not found")
		}
		return nil, fmt.Errorf("failed to get skill: %w", result.Error)
	}
	return &record, nil
}

// UpdateSkill updates an existing skill's fields
func (s *skillService) UpdateSkill(ctx context.Context, tenantID uint64, skillID uint64, req *types.UpdateSkillRequest) (*types.SkillRecord, error) {
	// Verify the skill exists and belongs to the tenant
	existing, err := s.GetSkillByID(ctx, tenantID, skillID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Instructions != nil {
		updates["instructions"] = *req.Instructions
	}
	if req.Status != nil {
		updates["status"] = string(*req.Status)
	}

	result := s.db.WithContext(ctx).
		Table("skills").
		Where("id = ? AND tenant_id = ?", skillID, tenantID).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to update skill: %w", result.Error)
	}

	// Re-fetch the updated record
	updated, err := s.GetSkillByID(ctx, tenantID, skillID)
	if err != nil {
		return nil, err
	}

	logger.Infof(ctx, "Updated skill ID=%d name=%s for tenant=%d", existing.ID, existing.Name, tenantID)
	return updated, nil
}

// DeleteSkill soft-deletes a skill by setting its status to disabled
func (s *skillService) DeleteSkill(ctx context.Context, tenantID uint64, skillID uint64) error {
	// Verify the skill exists and belongs to the tenant
	existing, err := s.GetSkillByID(ctx, tenantID, skillID)
	if err != nil {
		return err
	}

	result := s.db.WithContext(ctx).
		Table("skills").
		Where("id = ? AND tenant_id = ?", skillID, tenantID).
		Updates(map[string]interface{}{
			"status":     string(types.SkillStatusDisabled),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to delete skill: %w", result.Error)
	}

	logger.Infof(ctx, "Soft-deleted skill ID=%d name=%s for tenant=%d", existing.ID, existing.Name, tenantID)
	return nil
}

// isDuplicateSkillKeyError checks if the error is a duplicate key violation
func isDuplicateSkillKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "UNIQUE constraint failed")
}
