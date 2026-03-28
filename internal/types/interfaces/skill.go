package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/types"
)

// SkillService defines the interface for skill business logic
type SkillService interface {
	// ListPreloadedSkills returns metadata for all preloaded skills
	ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error)

	// GetSkillByName retrieves a skill by its name
	GetSkillByName(ctx context.Context, name string) (*skills.Skill, error)

	// CreateSkill creates a new database-backed skill for the given tenant
	CreateSkill(ctx context.Context, tenantID uint64, req *types.CreateSkillRequest) (*types.SkillRecord, error)

	// GetSkillByID retrieves a skill by its ID, scoped to the given tenant
	GetSkillByID(ctx context.Context, tenantID uint64, skillID uint64) (*types.SkillRecord, error)

	// UpdateSkill updates an existing skill's fields
	UpdateSkill(ctx context.Context, tenantID uint64, skillID uint64, req *types.UpdateSkillRequest) (*types.SkillRecord, error)

	// DeleteSkill soft-deletes a skill by setting its status to disabled
	DeleteSkill(ctx context.Context, tenantID uint64, skillID uint64) error
}
