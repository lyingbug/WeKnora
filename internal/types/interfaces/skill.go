package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/agent/skills"
	"github.com/Tencent/WeKnora/internal/types"
)

// SkillService defines the interface for skill business logic.
type SkillService interface {
	// ListPreloadedSkills returns metadata for Skills available to the current deployment.
	ListPreloadedSkills(ctx context.Context) ([]*skills.SkillMetadata, error)

	// ImportPreloadedSkills scans preloaded Skill directories and upserts their metadata into the registry.
	ImportPreloadedSkills(ctx context.Context) error

	// GetSkillByName retrieves a Skill's instructions from the current package source.
	GetSkillByName(ctx context.Context, name string) (*skills.Skill, error)
}

// SkillRepository defines registry persistence for installed and built-in Skills.
type SkillRepository interface {
	UpsertSkill(ctx context.Context, skill *types.SkillRegistryEntry) error
	ListActiveSkills(ctx context.Context) ([]*types.SkillRegistryEntry, error)
	GetSkillByName(ctx context.Context, name string) (*types.SkillRegistryEntry, error)
	CountSkills(ctx context.Context) (int64, error)
}
