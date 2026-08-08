package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// ErrMemoryNoteNotFound is returned when no note matches the lookup.
var ErrMemoryNoteNotFound = errors.New("memory note not found")

type memoryNoteRepository struct {
	db *gorm.DB
}

// NewMemoryNoteRepository creates the memory note repository.
func NewMemoryNoteRepository(db *gorm.DB) interfaces.MemoryNoteRepository {
	return &memoryNoteRepository{db: db}
}

// CreateBatch inserts a window's worth of observations in one statement.
// Lite runs with a single writer connection, so batching here is not a
// micro-optimisation: one round trip instead of ten keeps the extraction
// worker from holding the write lock while the chat path waits for it.
func (r *memoryNoteRepository) CreateBatch(ctx context.Context, notes []*types.MemoryNote) error {
	if len(notes) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(notes, 50).Error
}

func (r *memoryNoteRepository) GetByID(
	ctx context.Context, spaceID, id string,
) (*types.MemoryNote, error) {
	var note types.MemoryNote
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND id = ?", spaceID, id).
		First(&note).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemoryNoteNotFound
	}
	if err != nil {
		return nil, err
	}
	return &note, nil
}

func (r *memoryNoteRepository) List(
	ctx context.Context, req *types.MemoryNoteListRequest,
) ([]*types.MemoryNote, int64, error) {
	req.Normalize()

	query := r.db.WithContext(ctx).Model(&types.MemoryNote{}).Where("space_id = ?", req.SpaceID)
	if len(req.Statuses) > 0 {
		query = query.Where("status IN ?", req.Statuses)
	}
	if len(req.Types) > 0 {
		query = query.Where("note_type IN ?", req.Types)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var notes []*types.MemoryNote
	err := query.
		Order("created_at DESC").
		Order("id ASC").
		Limit(req.PageSize).
		Offset((req.Page - 1) * req.PageSize).
		Find(&notes).Error
	return notes, total, err
}

func (r *memoryNoteRepository) ListPending(
	ctx context.Context, spaceID string, limit int,
) ([]*types.MemoryNote, error) {
	if limit <= 0 {
		limit = 50
	}
	var notes []*types.MemoryNote
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND status = ?", spaceID, types.MemoryNoteStatusPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&notes).Error
	return notes, err
}

// ExistsHash is the cheap first line of de-duplication: an identical statement
// never needs a model to decide it is a repeat.
func (r *memoryNoteRepository) ExistsHash(
	ctx context.Context, spaceID, hash string,
) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.MemoryNote{}).
		Where("space_id = ? AND normalized_hash = ?", spaceID, hash).
		Where("status IN ?", []string{types.MemoryNoteStatusPending, types.MemoryNoteStatusMerged}).
		Count(&count).Error
	return count > 0, err
}

func (r *memoryNoteRepository) UpdateStatus(
	ctx context.Context, spaceID, id, status, mergedPageID string,
) error {
	result := r.db.WithContext(ctx).
		Model(&types.MemoryNote{}).
		Where("space_id = ? AND id = ?", spaceID, id).
		Updates(map[string]interface{}{
			"status":         status,
			"merged_page_id": mergedPageID,
			"updated_at":     time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMemoryNoteNotFound
	}
	return nil
}

func (r *memoryNoteRepository) MarkExpired(
	ctx context.Context, spaceID string, before time.Time,
) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&types.MemoryNote{}).
		Where("space_id = ? AND status = ? AND expires_at IS NOT NULL AND expires_at < ?",
			spaceID, types.MemoryNoteStatusPending, before).
		Updates(map[string]interface{}{
			"status":     types.MemoryNoteStatusExpired,
			"updated_at": time.Now(),
		})
	return result.RowsAffected, result.Error
}

func (r *memoryNoteRepository) Count(
	ctx context.Context, spaceID string, statuses []string,
) (int64, error) {
	query := r.db.WithContext(ctx).Model(&types.MemoryNote{}).Where("space_id = ?", spaceID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func (r *memoryNoteRepository) DeleteAll(ctx context.Context, spaceID string) (int64, error) {
	result := r.db.WithContext(ctx).Where("space_id = ?", spaceID).Delete(&types.MemoryNote{})
	return result.RowsAffected, result.Error
}

func (r *memoryNoteRepository) DeleteByPage(
	ctx context.Context, spaceID, pageID string,
) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND merged_page_id = ?", spaceID, pageID).
		Delete(&types.MemoryNote{})
	return result.RowsAffected, result.Error
}

func (r *memoryNoteRepository) ListAll(
	ctx context.Context, spaceID string,
) ([]*types.MemoryNote, error) {
	var notes []*types.MemoryNote
	err := r.db.WithContext(ctx).
		Where("space_id = ?", spaceID).
		Order("created_at ASC").
		Find(&notes).Error
	return notes, err
}
