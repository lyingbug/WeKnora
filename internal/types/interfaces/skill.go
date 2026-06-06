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

	// ListTenantSkills returns Skills installed and enabled for a tenant.
	ListTenantSkills(ctx context.Context, tenantID uint64) ([]*skills.SkillMetadata, error)

	// ListTenantSkillInstalls returns all active registry Skills installed for a tenant, including disabled installs.
	ListTenantSkillInstalls(ctx context.Context, tenantID uint64) ([]*types.TenantSkillInstallInfo, error)

	// SetTenantSkillEnabled toggles a tenant Skill install without deleting registry or package data.
	SetTenantSkillEnabled(ctx context.Context, tenantID uint64, skillID string, enabled bool) error

	// ImportPreloadedSkills scans preloaded Skill directories and upserts their metadata into the registry.
	ImportPreloadedSkills(ctx context.Context) error

	// EnsureTenantPreloadedSkillInstalls installs all active preloaded Skills for a tenant.
	EnsureTenantPreloadedSkillInstalls(ctx context.Context, tenantID uint64) error

	// InstallLocalSkillPackage validates and installs a local Skill package for a tenant.
	InstallLocalSkillPackage(ctx context.Context, tenantID uint64, packagePath string, installedBy string) (*types.SkillRegistryEntry, error)

	// SyncAgentSkillBindings synchronizes explicit Agent binding rows from the Agent config.
	SyncAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string, mode string, selectedSkillNames []string) error

	// ResolveAgentSkillAccess resolves the Agent config to installed Skill names and loader search directories.
	ResolveAgentSkillAccess(ctx context.Context, tenantID uint64, agentID string, mode string, selectedSkillNames []string) ([]string, []string, error)

	// ResolveAgentSelectedSkills resolves the Agent config to installed Skill names.
	ResolveAgentSelectedSkills(ctx context.Context, tenantID uint64, agentID string, mode string, selectedSkillNames []string) ([]string, error)

	// GetSkillByName retrieves a Skill's instructions from the current package source.
	GetSkillByName(ctx context.Context, name string) (*skills.Skill, error)
}

// SkillRepository defines registry persistence for installed and built-in Skills.
type SkillRepository interface {
	UpsertSkill(ctx context.Context, skill *types.SkillRegistryEntry) error
	ListActiveSkills(ctx context.Context) ([]*types.SkillRegistryEntry, error)
	GetActiveSkillByNameVersion(ctx context.Context, name, version string) (*types.SkillRegistryEntry, error)
	CountSkills(ctx context.Context) (int64, error)
	UpsertTenantSkillInstall(ctx context.Context, install *types.TenantSkillInstall) error
	ListTenantSkillInstallEntries(ctx context.Context, tenantID uint64) ([]*types.TenantSkillInstallInfo, error)
	SetTenantSkillInstallEnabled(ctx context.Context, tenantID uint64, skillID string, enabled bool) error
	ListTenantInstalledSkills(ctx context.Context, tenantID uint64) ([]*types.SkillRegistryEntry, error)
	ListTenantInstalledSkillNames(ctx context.Context, tenantID uint64) (map[string]*types.SkillRegistryEntry, error)
	ReplaceAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string, skillIDs []string) error
	ListAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string) ([]*types.SkillRegistryEntry, error)
}
