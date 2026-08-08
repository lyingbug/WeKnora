package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type memoryRepository struct {
	db *gorm.DB
}

// NewMemoryRepository creates the long-term memory repository.
func NewMemoryRepository(db *gorm.DB) interfaces.MemoryRepository {
	return &memoryRepository{db: db}
}

// scoped starts every query already filtered by workspace and subject. All
// reads and writes go through it so a missing scope predicate is impossible.
func (r *memoryRepository) scoped(ctx context.Context, scope interfaces.MemoryScope) *gorm.DB {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID)
}

func (r *memoryRepository) GetSubject(
	ctx context.Context, scope interfaces.MemoryScope,
) (*types.MemorySubject, error) {
	var subject types.MemorySubject
	err := r.scoped(ctx, scope).First(&subject).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &subject, nil
}

func (r *memoryRepository) EnsureSubject(
	ctx context.Context, scope interfaces.MemoryScope,
) (*types.MemorySubject, error) {
	subject := &types.MemorySubject{
		ID:        uuid.New().String(),
		TenantID:  scope.TenantID,
		SubjectID: scope.SubjectID,
		Enabled:   true,
	}
	// DoNothing plus a re-read keeps concurrent first turns from racing into a
	// unique-violation. The insert is a no-op whenever the row already exists.
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "subject_id"}},
			DoNothing: true,
		}).
		Create(subject).Error
	if err != nil {
		return nil, err
	}
	existing, err := r.GetSubject(ctx, scope)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("memory subject vanished after upsert")
	}
	return existing, nil
}

func (r *memoryRepository) UpdateSubjectEnabled(
	ctx context.Context, scope interfaces.MemoryScope, enabled bool,
) error {
	if _, err := r.EnsureSubject(ctx, scope); err != nil {
		return err
	}
	return r.scoped(ctx, scope).
		Model(&types.MemorySubject{}).
		Updates(map[string]interface{}{"enabled": enabled, "updated_at": time.Now()}).Error
}

func (r *memoryRepository) UpdateSubjectBlock(
	ctx context.Context, scope interfaces.MemoryScope, block string, itemCount int,
) error {
	now := time.Now()
	return r.scoped(ctx, scope).
		Model(&types.MemorySubject{}).
		Updates(map[string]interface{}{
			"block_text":       block,
			"block_updated_at": now,
			"item_count":       itemCount,
			"updated_at":       now,
		}).Error
}

func (r *memoryRepository) MarkExtracted(ctx context.Context, scope interfaces.MemoryScope) error {
	now := time.Now()
	return r.scoped(ctx, scope).
		Model(&types.MemorySubject{}).
		Updates(map[string]interface{}{"last_extracted_at": now, "updated_at": now}).Error
}

func (r *memoryRepository) CreateItem(ctx context.Context, item *types.MemoryItem) error {
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	if item.ValidFrom.IsZero() {
		item.ValidFrom = time.Now()
	}
	if item.Status == "" {
		item.Status = types.MemoryStatusActive
	}
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *memoryRepository) GetItem(
	ctx context.Context, scope interfaces.MemoryScope, id string,
) (*types.MemoryItem, error) {
	var item types.MemoryItem
	err := r.scoped(ctx, scope).Where("id = ?", id).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *memoryRepository) ListActiveByKinds(
	ctx context.Context, scope interfaces.MemoryScope, kinds []string, limit int,
) ([]*types.MemoryItem, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	var items []*types.MemoryItem
	query := r.scoped(ctx, scope).
		Where("status = ?", types.MemoryStatusActive).
		Where("kind IN ?", kinds).
		Order("importance DESC, valid_from DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *memoryRepository) ListItems(
	ctx context.Context, scope interfaces.MemoryScope, status string, limit, offset int,
) ([]*types.MemoryItem, int64, error) {
	query := r.scoped(ctx, scope).Model(&types.MemoryItem{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []*types.MemoryItem
	err := query.Order("valid_from DESC").Limit(limit).Offset(offset).Find(&items).Error
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *memoryRepository) FindActiveByKey(
	ctx context.Context, scope interfaces.MemoryScope, normalizedKey string,
) (*types.MemoryItem, error) {
	if normalizedKey == "" {
		return nil, nil
	}
	var item types.MemoryItem
	err := r.scoped(ctx, scope).
		Where("status = ? AND normalized_key = ?", types.MemoryStatusActive, normalizedKey).
		Order("valid_from DESC").
		First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *memoryRepository) UpdateItemContent(
	ctx context.Context, scope interfaces.MemoryScope, id, content, normalizedKey string, importance int,
) error {
	return r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"content":        content,
			"normalized_key": normalizedKey,
			"importance":     importance,
			"origin":         types.MemoryOriginManual,
			"updated_at":     time.Now(),
		}).Error
}

func (r *memoryRepository) SupersedeItem(
	ctx context.Context, scope interfaces.MemoryScope, id, supersededBy string,
) error {
	now := time.Now()
	return r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("id = ? AND status = ?", id, types.MemoryStatusActive).
		Updates(map[string]interface{}{
			"status":        types.MemoryStatusSuperseded,
			"invalid_at":    now,
			"superseded_by": supersededBy,
			"updated_at":    now,
		}).Error
}

func (r *memoryRepository) DeleteItem(
	ctx context.Context, scope interfaces.MemoryScope, id string,
) error {
	return r.scoped(ctx, scope).Where("id = ?", id).Delete(&types.MemoryItem{}).Error
}

func (r *memoryRepository) DeleteAll(
	ctx context.Context, scope interfaces.MemoryScope,
) (int64, error) {
	result := r.scoped(ctx, scope).Delete(&types.MemoryItem{})
	return result.RowsAffected, result.Error
}

func (r *memoryRepository) TouchUsed(
	ctx context.Context, scope interfaces.MemoryScope, ids []string,
) error {
	if len(ids) == 0 {
		return nil
	}
	return r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"last_used_at": time.Now(),
			"use_count":    gorm.Expr("use_count + 1"),
		}).Error
}

// ArchiveLowestRanked keeps the `keep` best active items and archives the
// rest. Ranking is importance first, then recency of use, then recency of
// creation — no decay curve, because a half-life that silently buries a
// correct memory is worse than a hard cap the user can see in the list.
func (r *memoryRepository) ArchiveLowestRanked(
	ctx context.Context, scope interfaces.MemoryScope, keep int,
) (int64, error) {
	if keep <= 0 {
		return 0, nil
	}
	var survivors []string
	err := r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("status = ?", types.MemoryStatusActive).
		Order("importance DESC, COALESCE(last_used_at, valid_from) DESC, valid_from DESC").
		Limit(keep).
		Pluck("id", &survivors).Error
	if err != nil {
		return 0, err
	}
	query := r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("status = ?", types.MemoryStatusActive)
	if len(survivors) > 0 {
		query = query.Where("id NOT IN ?", survivors)
	}
	result := query.Updates(map[string]interface{}{
		"status":     types.MemoryStatusArchived,
		"updated_at": time.Now(),
	})
	return result.RowsAffected, result.Error
}

func (r *memoryRepository) CountActive(
	ctx context.Context, scope interfaces.MemoryScope,
) (int64, error) {
	var count int64
	err := r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("status = ?", types.MemoryStatusActive).
		Count(&count).Error
	return count, err
}
