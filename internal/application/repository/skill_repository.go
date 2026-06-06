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

func skillBindingID(tenantID uint64, agentID, skillID string) string {
	raw := fmt.Sprintf("%d-%s-%s", tenantID, agentID, skillID)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}
