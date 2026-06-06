package repository

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type skillExecutionRunRepository struct {
	db *gorm.DB
}

func NewSkillExecutionRunRepository(db *gorm.DB) interfaces.SkillExecutionRunRepository {
	return &skillExecutionRunRepository{db: db}
}

func (r *skillExecutionRunRepository) CreateSkillExecutionRun(ctx context.Context, run *types.SkillExecutionRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *skillExecutionRunRepository) ListSkillExecutionRuns(
	ctx context.Context,
	tenantID uint64,
	limit int,
) ([]*types.SkillExecutionRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var runs []*types.SkillExecutionRun
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&runs).Error
	if err != nil {
		return nil, err
	}
	return runs, nil
}
