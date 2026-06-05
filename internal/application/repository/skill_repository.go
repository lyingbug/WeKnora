package repository

import (
	"context"

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
