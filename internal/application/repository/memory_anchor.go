package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrMemoryAnchorNotFound is returned when no anchor matches the lookup.
var ErrMemoryAnchorNotFound = errors.New("memory anchor not found")

type memoryAnchorRepository struct {
	db *gorm.DB
}

// NewMemoryAnchorRepository creates the memory anchor repository.
func NewMemoryAnchorRepository(db *gorm.DB) interfaces.MemoryAnchorRepository {
	return &memoryAnchorRepository{db: db}
}

// Upsert reinforces an existing anchor or creates it.
//
// Deliberately not an ON CONFLICT clause: the uniqueness constraint is a
// partial index (WHERE deleted_at IS NULL), and naming a partial index as a
// conflict target requires repeating its predicate in a way GORM expresses
// differently on each dialect. Update-then-insert is portable, and the insert
// is retried as an update once so a lost race still reinforces rather than
// erroring out on the chat path.
func (r *memoryAnchorRepository) Upsert(ctx context.Context, req *types.MemoryAnchorUpsert) error {
	if req == nil || req.SpaceID == "" || req.TargetRef == "" || req.Relation == "" {
		return errors.New("memory anchor upsert requires space, target and relation")
	}
	delta := req.Delta
	if delta <= 0 {
		delta = 1
	}
	now := time.Now()

	updated, err := r.reinforce(ctx, req, delta, now)
	if err != nil {
		return err
	}
	if updated {
		return nil
	}

	anchor := &types.MemoryAnchor{
		ID:              uuid.New().String(),
		TenantID:        req.TenantID,
		SpaceID:         req.SpaceID,
		MemoryPageID:    req.MemoryPageID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		TargetKind:      req.TargetKind,
		TargetRef:       req.TargetRef,
		Relation:        req.Relation,
		Strength:        float64(delta),
		HitCount:        delta,
		Confidence:      req.Confidence,
		Source:          req.Source,
		Evidence:        req.Evidence,
		FirstSeenAt:     now,
		LastSeenAt:      now,
	}
	if anchor.Confidence <= 0 {
		anchor.Confidence = 0.5
	}
	if err := r.db.WithContext(ctx).Create(anchor).Error; err == nil {
		return nil
	}
	// Someone inserted the same anchor between our read and our write.
	updated, retryErr := r.reinforce(ctx, req, delta, now)
	if retryErr != nil {
		return retryErr
	}
	if !updated {
		return errors.New("failed to upsert memory anchor")
	}
	return nil
}

func (r *memoryAnchorRepository) reinforce(
	ctx context.Context, req *types.MemoryAnchorUpsert, delta int, now time.Time,
) (bool, error) {
	var existing types.MemoryAnchor
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND knowledge_base_id = ? AND target_kind = ? AND target_ref = ? AND relation = ? AND memory_page_id = ?",
			req.SpaceID, req.KnowledgeBaseID, req.TargetKind, req.TargetRef, req.Relation, req.MemoryPageID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	evidence := existing.Evidence
	evidence.AppendEvidence(req.Evidence)

	updates := map[string]interface{}{
		"hit_count":    gorm.Expr("hit_count + ?", delta),
		"strength":     gorm.Expr("strength + ?", float64(delta)),
		"last_seen_at": now,
		"updated_at":   now,
		"evidence":     evidence,
	}
	if req.Confidence > 0 {
		updates["confidence"] = req.Confidence
	}
	result := r.db.WithContext(ctx).
		Model(&types.MemoryAnchor{}).
		Where("id = ?", existing.ID).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *memoryAnchorRepository) ListBySpace(
	ctx context.Context, spaceID, kbID string,
) ([]*types.MemoryAnchor, error) {
	query := r.db.WithContext(ctx).Where("space_id = ?", spaceID)
	if kbID != "" {
		query = query.Where("knowledge_base_id = ?", kbID)
	}
	var anchors []*types.MemoryAnchor
	err := query.Order("last_seen_at DESC").Find(&anchors).Error
	return anchors, err
}

// ListOverlay selects only the four columns the illumination maths needs.
// Hydrating whole rows for a knowledge base with thousands of anchors would
// pull evidence blobs nobody reads.
func (r *memoryAnchorRepository) ListOverlay(
	ctx context.Context, spaceID, kbID, targetKind string,
) ([]types.MemoryOverlayAnchor, error) {
	query := r.db.WithContext(ctx).
		Model(&types.MemoryAnchor{}).
		Select("target_ref, relation, hit_count, last_seen_at, memory_page_id").
		Where("space_id = ?", spaceID)
	if kbID != "" {
		query = query.Where("knowledge_base_id = ?", kbID)
	}
	if targetKind != "" {
		query = query.Where("target_kind = ?", targetKind)
	}
	var rows []types.MemoryOverlayAnchor
	err := query.Scan(&rows).Error
	return rows, err
}

func (r *memoryAnchorRepository) ListByPage(
	ctx context.Context, spaceID, memoryPageID string,
) ([]*types.MemoryAnchor, error) {
	var anchors []*types.MemoryAnchor
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND memory_page_id = ?", spaceID, memoryPageID).
		Order("last_seen_at DESC").
		Find(&anchors).Error
	return anchors, err
}

func (r *memoryAnchorRepository) ListByTarget(
	ctx context.Context, spaceID, kbID, targetKind, targetRef string,
) ([]*types.MemoryAnchor, error) {
	var anchors []*types.MemoryAnchor
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND knowledge_base_id = ? AND target_kind = ? AND target_ref = ?",
			spaceID, kbID, targetKind, targetRef).
		Order("last_seen_at DESC").
		Find(&anchors).Error
	return anchors, err
}

// AggregateByTarget rolls anchors up across every space in a workspace.
//
// It counts distinct spaces rather than returning them, and exposes no space
// identifier at all: the aggregate exists so a knowledge-base owner can find
// weak pages, not so they can find out what any individual has been reading.
func (r *memoryAnchorRepository) AggregateByTarget(
	ctx context.Context, tenantID uint64, kbID string,
) ([]types.MemoryAnchorAggregate, error) {
	var rows []types.MemoryAnchorAggregate
	err := r.db.WithContext(ctx).
		Model(&types.MemoryAnchor{}).
		Select("target_kind, target_ref, relation, "+
			"SUM(hit_count) AS interactions, COUNT(DISTINCT space_id) AS distinct_spaces").
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, kbID).
		Group("target_kind, target_ref, relation").
		Scan(&rows).Error
	return rows, err
}

func (r *memoryAnchorRepository) Delete(ctx context.Context, spaceID, id string) error {
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND id = ?", spaceID, id).
		Delete(&types.MemoryAnchor{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMemoryAnchorNotFound
	}
	return nil
}

func (r *memoryAnchorRepository) DeleteByPage(
	ctx context.Context, spaceID, memoryPageID string,
) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND memory_page_id = ?", spaceID, memoryPageID).
		Delete(&types.MemoryAnchor{})
	return result.RowsAffected, result.Error
}

func (r *memoryAnchorRepository) DeleteAll(ctx context.Context, spaceID string) (int64, error) {
	result := r.db.WithContext(ctx).Where("space_id = ?", spaceID).Delete(&types.MemoryAnchor{})
	return result.RowsAffected, result.Error
}

func (r *memoryAnchorRepository) Count(ctx context.Context, spaceID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.MemoryAnchor{}).
		Where("space_id = ?", spaceID).
		Count(&count).Error
	return count, err
}

func (r *memoryAnchorRepository) ListAnchoredKBs(
	ctx context.Context, spaceID string,
) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&types.MemoryAnchor{}).
		Distinct("knowledge_base_id").
		Where("space_id = ?", spaceID).
		Pluck("knowledge_base_id", &ids).Error
	return ids, err
}

func (r *memoryAnchorRepository) ListAll(
	ctx context.Context, spaceID string,
) ([]*types.MemoryAnchor, error) {
	var anchors []*types.MemoryAnchor
	err := r.db.WithContext(ctx).
		Where("space_id = ?", spaceID).
		Order("created_at ASC").
		Find(&anchors).Error
	return anchors, err
}
