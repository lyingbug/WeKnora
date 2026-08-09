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

// EnqueuePendingSession is the whole "never drop a turn" mechanism, so it runs
// inside a transaction: reading the subject, appending the session and claiming
// the in-flight slot must not interleave with a concurrent turn, or two turns
// could both decide nobody is scheduled (two tasks) or both decide someone is
// (a turn recorded against a run that already read the queue).
func (r *memoryRepository) EnqueuePendingSession(
	ctx context.Context, scope interfaces.MemoryScope, sessionID string, inFlightTimeout time.Duration,
) (*types.MemorySubject, bool, error) {
	var (
		snapshot   types.MemorySubject
		shouldSend bool
	)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var subject types.MemorySubject
		if err := tx.Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
			Clauses(forUpdateClause()).
			First(&subject).Error; err != nil {
			return err
		}
		snapshot = subject

		now := time.Now()
		updates := map[string]interface{}{"updated_at": now}
		if pending := subject.PendingSessions.Append(sessionID); len(pending) != len(subject.PendingSessions) {
			updates["pending_sessions"] = pending
		}
		// A stale marker (worker crashed, task lost) must not wedge the subject
		// forever, so the claim expires.
		inFlight := subject.ExtractScheduledAt != nil &&
			now.Sub(*subject.ExtractScheduledAt) < inFlightTimeout
		if !inFlight {
			updates["extract_scheduled_at"] = now
			shouldSend = true
		}
		return tx.Model(&types.MemorySubject{}).
			Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
			Updates(updates).Error
	})
	if err != nil {
		return nil, false, err
	}
	return &snapshot, shouldSend, nil
}

// ClaimPendingSessions empties the queue and returns it together with the
// watermark to walk forward from. Emptying it here (rather than after the run)
// is deliberate: turns arriving during the run land in a fresh queue and
// trigger a follow-up, instead of being erased by the run that never saw them.
func (r *memoryRepository) ClaimPendingSessions(
	ctx context.Context, scope interfaces.MemoryScope,
) ([]string, time.Time, error) {
	var (
		pending []string
		cursor  time.Time
	)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var subject types.MemorySubject
		if err := tx.Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
			Clauses(forUpdateClause()).
			First(&subject).Error; err != nil {
			return err
		}
		pending = append(pending, subject.PendingSessions...)
		if subject.ExtractCursor != nil {
			cursor = *subject.ExtractCursor
		}
		return tx.Model(&types.MemorySubject{}).
			Where("tenant_id = ? AND subject_id = ?", scope.TenantID, scope.SubjectID).
			Updates(map[string]interface{}{
				"pending_sessions": types.MemoryPendingSessions{},
				"updated_at":       time.Now(),
			}).Error
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	return pending, cursor, nil
}

func (r *memoryRepository) FinishExtraction(
	ctx context.Context, scope interfaces.MemoryScope, cursor time.Time,
) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_extracted_at":    now,
		"extract_scheduled_at": nil,
		"updated_at":           now,
	}
	if !cursor.IsZero() {
		updates["extract_cursor"] = cursor
	}
	return r.scoped(ctx, scope).Model(&types.MemorySubject{}).Updates(updates).Error
}

func (r *memoryRepository) ReleaseExtractionSlot(
	ctx context.Context, scope interfaces.MemoryScope,
) error {
	return r.scoped(ctx, scope).
		Model(&types.MemorySubject{}).
		Updates(map[string]interface{}{"extract_scheduled_at": nil, "updated_at": time.Now()}).Error
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

// notExpired excludes items whose usefulness has a stated end. Applying it in
// the query rather than after the fact means an expired task cannot slip into
// a prompt through a code path that forgot to filter.
func notExpired(query *gorm.DB) *gorm.DB {
	return query.Where("expires_at IS NULL OR expires_at > ?", time.Now())
}

func (r *memoryRepository) ListActiveByKinds(
	ctx context.Context, scope interfaces.MemoryScope, kinds []string, limit int,
) ([]*types.MemoryItem, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	var items []*types.MemoryItem
	query := notExpired(r.scoped(ctx, scope).
		Where("status = ?", types.MemoryStatusActive).
		Where("kind IN ?", kinds)).
		Order("importance DESC, valid_from DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListActiveResident returns what the resident block is built from.
//
// Stable traits qualify by kind. An explicitly requested memory qualifies
// regardless of kind: the user said "remember this", and making that depend on
// their later question happening to share words with it is the fastest way to
// lose their trust in the feature.
func (r *memoryRepository) ListActiveResident(
	ctx context.Context, scope interfaces.MemoryScope, limit int,
) ([]*types.MemoryItem, error) {
	var items []*types.MemoryItem
	query := notExpired(r.scoped(ctx, scope).
		Where("status = ?", types.MemoryStatusActive).
		Where("kind IN ? OR origin = ?",
			[]string{types.MemoryKindProfile, types.MemoryKindPreference},
			types.MemoryOriginExplicit)).
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

func (r *memoryRepository) AddTombstone(
	ctx context.Context, scope interfaces.MemoryScope, topic, fingerprint, sourceMessageID string,
) error {
	if fingerprint == "" {
		return nil
	}
	tombstone := &types.MemoryTombstone{
		ID:              uuid.New().String(),
		TenantID:        scope.TenantID,
		SubjectID:       scope.SubjectID,
		Topic:           topic,
		Fingerprint:     fingerprint,
		SourceMessageID: sourceMessageID,
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"}, {Name: "subject_id"}, {Name: "fingerprint"},
			},
			DoNothing: true,
		}).
		Create(tombstone).Error
	if err != nil {
		return err
	}
	return r.trimTombstones(ctx, scope)
}

// trimTombstones keeps the list bounded. A rejection from long ago matters less
// than this table growing without limit.
func (r *memoryRepository) trimTombstones(ctx context.Context, scope interfaces.MemoryScope) error {
	var keep []string
	err := r.scoped(ctx, scope).
		Model(&types.MemoryTombstone{}).
		Order("created_at DESC").
		Limit(types.MaxMemoryTombstones).
		Pluck("id", &keep).Error
	if err != nil {
		return err
	}
	if len(keep) < types.MaxMemoryTombstones {
		return nil
	}
	return r.scoped(ctx, scope).
		Where("id NOT IN ?", keep).
		Delete(&types.MemoryTombstone{}).Error
}

func (r *memoryRepository) ListTombstones(
	ctx context.Context, scope interfaces.MemoryScope, limit int,
) ([]*types.MemoryTombstone, error) {
	var tombstones []*types.MemoryTombstone
	query := r.scoped(ctx, scope).
		Model(&types.MemoryTombstone{}).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&tombstones).Error; err != nil {
		return nil, err
	}
	return tombstones, nil
}

func (r *memoryRepository) HasTombstone(
	ctx context.Context, scope interfaces.MemoryScope, fingerprint string,
) (bool, error) {
	if fingerprint == "" {
		return false, nil
	}
	var count int64
	err := r.scoped(ctx, scope).
		Model(&types.MemoryTombstone{}).
		Where("fingerprint = ?", fingerprint).
		Count(&count).Error
	return count > 0, err
}

func (r *memoryRepository) HasTombstoneForMessage(
	ctx context.Context, scope interfaces.MemoryScope, sourceMessageID string, within time.Duration,
) (bool, error) {
	if sourceMessageID == "" {
		return false, nil
	}
	query := r.scoped(ctx, scope).
		Model(&types.MemoryTombstone{}).
		Where("source_message_id = ?", sourceMessageID)
	if within > 0 {
		query = query.Where("created_at > ?", time.Now().Add(-within))
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (r *memoryRepository) ExpireOverdue(
	ctx context.Context, scope interfaces.MemoryScope,
) (int64, error) {
	now := time.Now()
	result := r.scoped(ctx, scope).
		Model(&types.MemoryItem{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", types.MemoryStatusActive, now).
		Updates(map[string]interface{}{
			"status":     types.MemoryStatusArchived,
			"updated_at": now,
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
