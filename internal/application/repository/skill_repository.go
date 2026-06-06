package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type skillRepository struct {
	db *gorm.DB
}

func NewSkillRepository(db *gorm.DB) interfaces.SkillRepository {
	return &skillRepository{db: db}
}

func (r *skillRepository) UpsertSkill(ctx context.Context, skill *types.SkillRegistryEntry) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}, {Name: "version"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"description",
				"source_type",
				"source_uri",
				"digest",
				"manifest",
				"status",
				"is_builtin",
				"updated_at",
			}),
		}).
		Create(skill).Error
}

func (r *skillRepository) ListActiveSkills(ctx context.Context) ([]*types.SkillRegistryEntry, error) {
	var skills []*types.SkillRegistryEntry
	err := r.db.WithContext(ctx).
		Where("status = ?", types.SkillStatusActive).
		Order("name ASC, version ASC").
		Find(&skills).Error
	if err != nil {
		return nil, err
	}
	return skills, nil
}

func (r *skillRepository) GetActiveSkillByNameVersion(
	ctx context.Context,
	name string,
	version string,
) (*types.SkillRegistryEntry, error) {
	var skill types.SkillRegistryEntry
	err := r.db.WithContext(ctx).
		Where("name = ? AND version = ? AND status = ?", name, version, types.SkillStatusActive).
		First(&skill).Error
	if err != nil {
		return nil, err
	}
	return &skill, nil
}

func (r *skillRepository) CountSkills(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.SkillRegistryEntry{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *skillRepository) UpsertTenantSkillInstall(ctx context.Context, install *types.TenantSkillInstall) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "skill_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"installed_by",
				"approved_permissions",
				"updated_at",
			}),
		}).
		Create(install).Error
}

func (r *skillRepository) ListTenantSkillInstallEntries(ctx context.Context, tenantID uint64) ([]*types.TenantSkillInstallInfo, error) {
	var installs []*types.TenantSkillInstallInfo
	err := r.db.WithContext(ctx).
		Table("tenant_skill_installs AS tsi").
		Select(`
			tsi.id AS install_id,
			tsi.tenant_id,
			tsi.skill_id,
			tsi.enabled,
			tsi.installed_by,
			tsi.approved_permissions,
			tsi.created_at AS installed_at,
			tsi.updated_at AS install_updated_at,
			skills.name,
			skills.version,
			skills.description,
			skills.source_type,
			skills.source_uri,
			skills.digest,
			skills.manifest,
			skills.status,
			skills.is_builtin
		`).
		Joins("JOIN skills ON skills.id = tsi.skill_id").
		Where("tsi.tenant_id = ? AND skills.status = ?", tenantID, types.SkillStatusActive).
		Order("skills.name ASC, skills.version ASC").
		Find(&installs).Error
	if err != nil {
		return nil, err
	}
	return installs, nil
}

func (r *skillRepository) GetTenantSkillInstallEntryByName(
	ctx context.Context,
	tenantID uint64,
	skillName string,
) (*types.TenantSkillInstallInfo, error) {
	var install types.TenantSkillInstallInfo
	result := r.db.WithContext(ctx).
		Table("tenant_skill_installs AS tsi").
		Select(`
			tsi.id AS install_id,
			tsi.tenant_id,
			tsi.skill_id,
			tsi.enabled,
			tsi.installed_by,
			tsi.approved_permissions,
			tsi.created_at AS installed_at,
			tsi.updated_at AS install_updated_at,
			skills.name,
			skills.version,
			skills.description,
			skills.source_type,
			skills.source_uri,
			skills.digest,
			skills.manifest,
			skills.status,
			skills.is_builtin
		`).
		Joins("JOIN skills ON skills.id = tsi.skill_id").
		Where(
			"tsi.tenant_id = ? AND tsi.enabled = ? AND skills.name = ? AND skills.status = ?",
			tenantID,
			true,
			skillName,
			types.SkillStatusActive,
		).
		Order("skills.version DESC").
		Limit(1).
		Find(&install)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &install, nil
}

func (r *skillRepository) SetTenantSkillInstallEnabled(ctx context.Context, tenantID uint64, skillID string, enabled bool) error {
	result := r.db.WithContext(ctx).
		Model(&types.TenantSkillInstall{}).
		Where("tenant_id = ? AND skill_id = ?", tenantID, skillID).
		Update("enabled", enabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *skillRepository) ListTenantInstalledSkills(ctx context.Context, tenantID uint64) ([]*types.SkillRegistryEntry, error) {
	var skills []*types.SkillRegistryEntry
	err := r.db.WithContext(ctx).
		Table("skills").
		Select("skills.*").
		Joins("JOIN tenant_skill_installs tsi ON tsi.skill_id = skills.id").
		Where("tsi.tenant_id = ? AND tsi.enabled = ? AND skills.status = ?", tenantID, true, types.SkillStatusActive).
		Order("skills.name ASC, skills.version ASC").
		Find(&skills).Error
	if err != nil {
		return nil, err
	}
	return skills, nil
}

func (r *skillRepository) ListTenantInstalledSkillNames(ctx context.Context, tenantID uint64) (map[string]*types.SkillRegistryEntry, error) {
	skills, err := r.ListTenantInstalledSkills(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]*types.SkillRegistryEntry, len(skills))
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	return byName, nil
}

func (r *skillRepository) ReplaceAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string, skillIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).
			Delete(&types.AgentSkillBinding{}).Error; err != nil {
			return err
		}

		for _, skillID := range skillIDs {
			binding := &types.AgentSkillBinding{
				ID:       skillBindingID(tenantID, agentID, skillID),
				TenantID: tenantID,
				AgentID:  agentID,
				SkillID:  skillID,
				Enabled:  true,
				Config:   types.JSON("{}"),
			}
			if err := tx.Create(binding).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *skillRepository) ListAgentSkillBindings(ctx context.Context, tenantID uint64, agentID string) ([]*types.SkillRegistryEntry, error) {
	var skills []*types.SkillRegistryEntry
	err := r.db.WithContext(ctx).
		Table("skills").
		Select("skills.*").
		Joins("JOIN agent_skill_bindings asb ON asb.skill_id = skills.id").
		Where("asb.tenant_id = ? AND asb.agent_id = ? AND asb.enabled = ? AND skills.status = ?", tenantID, agentID, true, types.SkillStatusActive).
		Order("skills.name ASC, skills.version ASC").
		Find(&skills).Error
	if err != nil {
		return nil, err
	}
	return skills, nil
}

func (r *skillRepository) UpsertTenantSkillCredential(ctx context.Context, credential *types.TenantSkillCredential) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "skill_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"credentials",
				"updated_by",
				"updated_at",
			}),
		}).
		Create(credential).Error
}

func (r *skillRepository) GetTenantSkillCredentialByName(
	ctx context.Context,
	tenantID uint64,
	skillName string,
) (*types.TenantSkillCredential, error) {
	var credential types.TenantSkillCredential
	result := r.db.WithContext(ctx).
		Table("tenant_skill_credentials AS tsc").
		Select("tsc.*").
		Joins("JOIN skills ON skills.id = tsc.skill_id").
		Joins("JOIN tenant_skill_installs tsi ON tsi.tenant_id = tsc.tenant_id AND tsi.skill_id = tsc.skill_id").
		Where(
			"tsc.tenant_id = ? AND tsi.enabled = ? AND skills.name = ? AND skills.status = ?",
			tenantID,
			true,
			skillName,
			types.SkillStatusActive,
		).
		Limit(1).
		Find(&credential)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &credential, nil
}

func (r *skillRepository) ReplaceTenantSkillMCPBindings(
	ctx context.Context,
	tenantID uint64,
	skillID string,
	bindings []*types.TenantSkillMCPBinding,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("tenant_id = ? AND skill_id = ?", tenantID, skillID).
			Delete(&types.TenantSkillMCPBinding{}).Error; err != nil {
			return err
		}
		if len(bindings) == 0 {
			return nil
		}
		return tx.Create(&bindings).Error
	})
}

func (r *skillRepository) ListTenantSkillMCPBindingsByName(
	ctx context.Context,
	tenantID uint64,
	skillName string,
) ([]*types.TenantSkillMCPBinding, error) {
	var bindings []*types.TenantSkillMCPBinding
	err := r.db.WithContext(ctx).
		Table("tenant_skill_mcp_bindings AS tsmb").
		Select("tsmb.*").
		Joins("JOIN skills ON skills.id = tsmb.skill_id").
		Joins("JOIN tenant_skill_installs tsi ON tsi.tenant_id = tsmb.tenant_id AND tsi.skill_id = tsmb.skill_id").
		Joins("JOIN mcp_services ms ON ms.id = tsmb.service_id AND ms.tenant_id = tsmb.tenant_id").
		Where(
			"tsmb.tenant_id = ? AND tsi.enabled = ? AND skills.name = ? AND skills.status = ? AND ms.enabled = ? AND ms.deleted_at IS NULL",
			tenantID,
			true,
			skillName,
			types.SkillStatusActive,
			true,
		).
		Order("tsmb.mcp_name ASC").
		Find(&bindings).Error
	if err != nil {
		return nil, err
	}
	return bindings, nil
}

func (r *skillRepository) ListEnabledTenantMCPServiceIDs(
	ctx context.Context,
	tenantID uint64,
	serviceIDs []string,
) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(serviceIDs))
	if len(serviceIDs) == 0 {
		return result, nil
	}
	var ids []string
	if err := r.db.WithContext(ctx).
		Table("mcp_services").
		Where("tenant_id = ? AND enabled = ? AND deleted_at IS NULL AND id IN ?", tenantID, true, serviceIDs).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result, nil
}

func skillBindingID(tenantID uint64, agentID, skillID string) string {
	raw := fmt.Sprintf("%d-%s-%s", tenantID, agentID, skillID)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}
