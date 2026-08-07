package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrMemoryPageNotFound is returned when no page matches the lookup.
	ErrMemoryPageNotFound = errors.New("memory page not found")
	// ErrMemoryPageConflict is returned when an optimistic-lock check fails.
	ErrMemoryPageConflict = errors.New("memory page version conflict")
)

type memoryPageRepository struct {
	db *gorm.DB
}

// NewMemoryPageRepository creates the memory page repository.
func NewMemoryPageRepository(db *gorm.DB) interfaces.MemoryPageRepository {
	return &memoryPageRepository{db: db}
}

func (r *memoryPageRepository) Create(ctx context.Context, page *types.MemoryPage) error {
	return r.db.WithContext(ctx).Create(page).Error
}

// Update writes a page. expectVersion > 0 enables optimistic locking for
// interactive edits; background writers pass 0 and accept last-write-wins,
// because a decay sweep losing a race with a user edit is harmless while
// blocking the sweep on it is not.
func (r *memoryPageRepository) Update(
	ctx context.Context, page *types.MemoryPage, expectVersion int,
) error {
	return updateMemoryPageRow(r.db.WithContext(ctx), page, expectVersion)
}

// UpdateWithRevision snapshots the outgoing version and applies the update in
// one transaction, so a failed write can never leave behind a revision for a
// version that is still current.
func (r *memoryPageRepository) UpdateWithRevision(
	ctx context.Context, page *types.MemoryPage, rev *types.MemoryPageRevision, expectVersion int,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if rev != nil {
			// A concurrent writer may already have snapshotted this exact
			// version; its copy is identical, so leaving it alone is correct.
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "page_id"}, {Name: "version"}},
				DoNothing: true,
			}).Create(rev).Error; err != nil {
				return err
			}
		}
		return updateMemoryPageRow(tx, page, expectVersion)
	})
}

func updateMemoryPageRow(db *gorm.DB, page *types.MemoryPage, expectVersion int) error {
	previousVersion := page.Version
	page.Version = previousVersion + 1
	page.UpdatedAt = time.Now()

	query := db.Model(&types.MemoryPage{}).Where("space_id = ? AND id = ?", page.SpaceID, page.ID)
	if expectVersion > 0 {
		query = query.Where("version = ?", expectVersion)
	}

	result := query.Updates(map[string]interface{}{
		"title":            page.Title,
		"page_type":        page.PageType,
		"status":           page.Status,
		"content":          page.Content,
		"summary":          page.Summary,
		"structured":       page.Structured,
		"aliases":          page.Aliases,
		"in_links":         page.InLinks,
		"out_links":        page.OutLinks,
		"folder_path":      page.FolderPath,
		"strength":         page.Strength,
		"confidence":       page.Confidence,
		"pinned":           page.Pinned,
		"superseded_by":    page.SupersededBy,
		"note_refs":        page.NoteRefs,
		"version":          page.Version,
		"last_edit_source": page.LastEditSource,
		"last_seen_at":     page.LastSeenAt,
		"updated_at":       page.UpdatedAt,
	})
	if result.Error != nil {
		page.Version = previousVersion
		return result.Error
	}
	if result.RowsAffected == 0 {
		page.Version = previousVersion
		var count int64
		db.Model(&types.MemoryPage{}).Where("space_id = ? AND id = ?", page.SpaceID, page.ID).Count(&count)
		if count == 0 {
			return ErrMemoryPageNotFound
		}
		return ErrMemoryPageConflict
	}
	return nil
}

// UpdateLinks writes the link arrays without touching version.
//
// Kept separate from Update on purpose. A page gaining a backlink because some
// other memory now points at it is not a revision of that page, and treating it
// as one would both pollute the history and invalidate a version the editor is
// holding.
func (r *memoryPageRepository) UpdateLinks(ctx context.Context, page *types.MemoryPage) error {
	return r.db.WithContext(ctx).
		Model(&types.MemoryPage{}).
		Where("space_id = ? AND id = ?", page.SpaceID, page.ID).
		Updates(map[string]interface{}{
			"in_links":  page.InLinks,
			"out_links": page.OutLinks,
		}).Error
}

func (r *memoryPageRepository) GetByID(
	ctx context.Context, spaceID, id string,
) (*types.MemoryPage, error) {
	var page types.MemoryPage
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND id = ?", spaceID, id).
		First(&page).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemoryPageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *memoryPageRepository) GetBySlug(
	ctx context.Context, spaceID, slug string,
) (*types.MemoryPage, error) {
	var page types.MemoryPage
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND slug = ?", spaceID, slug).
		First(&page).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemoryPageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *memoryPageRepository) GetBySlugs(
	ctx context.Context, spaceID string, slugs []string,
) ([]*types.MemoryPage, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	var pages []*types.MemoryPage
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND slug IN ?", spaceID, slugs).
		Find(&pages).Error
	return pages, err
}

func (r *memoryPageRepository) List(
	ctx context.Context, req *types.MemoryPageListRequest,
) ([]*types.MemoryPage, int64, error) {
	req.Normalize()

	query := r.db.WithContext(ctx).Model(&types.MemoryPage{}).Where("space_id = ?", req.SpaceID)
	if len(req.Types) > 0 {
		query = query.Where("page_type IN ?", req.Types)
	}
	if len(req.Statuses) > 0 {
		query = query.Where("status IN ?", req.Statuses)
	}
	if req.Query != "" {
		term := likeTerm(req.Query)
		query = query.Where(
			"LOWER(title) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(content) LIKE ? OR LOWER(slug) LIKE ?",
			term, term, term, term,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	direction := "ASC"
	if req.Desc {
		direction = "DESC"
	}
	// req.SortBy is constrained to a fixed set by Normalize, so this is not an
	// injection surface; the whitelist lives there rather than here so every
	// caller of the DTO gets it.
	order := fmt.Sprintf("%s %s", req.SortBy, direction)

	var pages []*types.MemoryPage
	err := query.
		// Pinned memories lead regardless of the chosen sort. Pinning is the
		// user saying "this one matters"; letting it sort to the bottom of the
		// list defeats the only thing pinning is for.
		Order("pinned DESC").
		Order(order).
		Order("id ASC").
		Limit(req.PageSize).
		Offset((req.Page - 1) * req.PageSize).
		Find(&pages).Error
	return pages, total, err
}

func (r *memoryPageRepository) ListAll(
	ctx context.Context, spaceID string,
) ([]*types.MemoryPage, error) {
	var pages []*types.MemoryPage
	err := r.db.WithContext(ctx).
		Where("space_id = ?", spaceID).
		Order("slug ASC").
		Find(&pages).Error
	return pages, err
}

func (r *memoryPageRepository) ListByTypes(
	ctx context.Context, spaceID string, pageTypes []string, statuses []string, limit int,
) ([]*types.MemoryPage, error) {
	query := r.db.WithContext(ctx).Where("space_id = ?", spaceID)
	if len(pageTypes) > 0 {
		query = query.Where("page_type IN ?", pageTypes)
	}
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var pages []*types.MemoryPage
	// Pinned first, then strongest: the recall path takes the head of this
	// list, so the ordering is the priority policy.
	err := query.Order("pinned DESC").Order("strength DESC").Order("updated_at DESC").Find(&pages).Error
	return pages, err
}

// Search is a portable keyword match. It is intentionally not a full-text
// search: memory spaces hold hundreds of rows, not millions, and a LIKE scan
// there is cheaper than maintaining a second index on two database engines.
func (r *memoryPageRepository) Search(
	ctx context.Context, spaceID, query string, limit int,
) ([]*types.MemoryPage, error) {
	if limit <= 0 {
		limit = 20
	}
	term := likeTerm(query)
	var pages []*types.MemoryPage
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND status = ?", spaceID, types.MemoryPageStatusActive).
		Where("LOWER(title) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(content) LIKE ? OR LOWER(slug) LIKE ?",
			term, term, term, term).
		Order("strength DESC").
		Order("updated_at DESC").
		Limit(limit).
		Find(&pages).Error
	return pages, err
}

func (r *memoryPageRepository) Delete(ctx context.Context, spaceID, id string) error {
	return r.db.WithContext(ctx).
		Where("space_id = ? AND id = ?", spaceID, id).
		Delete(&types.MemoryPage{}).Error
}

func (r *memoryPageRepository) DeleteBySlugs(
	ctx context.Context, spaceID string, slugs []string,
) (int64, error) {
	if len(slugs) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).
		Where("space_id = ? AND slug IN ?", spaceID, slugs).
		Delete(&types.MemoryPage{})
	return result.RowsAffected, result.Error
}

func (r *memoryPageRepository) DeleteAll(ctx context.Context, spaceID string) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("space_id = ?", spaceID).
		Delete(&types.MemoryPage{})
	return result.RowsAffected, result.Error
}

func (r *memoryPageRepository) Count(
	ctx context.Context, spaceID string, statuses []string,
) (int64, error) {
	query := r.db.WithContext(ctx).Model(&types.MemoryPage{}).Where("space_id = ?", spaceID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

func (r *memoryPageRepository) CountByType(
	ctx context.Context, spaceID string,
) (map[string]int64, error) {
	type row struct {
		PageType string
		Total    int64
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Model(&types.MemoryPage{}).
		Select("page_type, COUNT(*) AS total").
		Where("space_id = ? AND status = ?", spaceID, types.MemoryPageStatusActive).
		Group("page_type").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.PageType] = r.Total
	}
	return out, nil
}

// BumpHits records that these pages were actually injected. Usage is what keeps
// a memory alive through decay, so it is recorded on injection rather than on
// retrieval: a page that was fetched but dropped by the token budget did not
// contribute to the answer and should not be rewarded for it.
func (r *memoryPageRepository) BumpHits(
	ctx context.Context, spaceID string, pageIDs []string, seenAt time.Time,
) error {
	if len(pageIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&types.MemoryPage{}).
		Where("space_id = ? AND id IN ?", spaceID, pageIDs).
		Updates(map[string]interface{}{
			"hit_count":    gorm.Expr("hit_count + 1"),
			"last_seen_at": seenAt,
		}).Error
}

func (r *memoryPageRepository) ListRevisions(
	ctx context.Context, spaceID, pageID string,
) ([]*types.MemoryPageRevision, error) {
	var revisions []*types.MemoryPageRevision
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND page_id = ?", spaceID, pageID).
		Order("version DESC").
		Find(&revisions).Error
	return revisions, err
}

func (r *memoryPageRepository) GetRevision(
	ctx context.Context, spaceID, pageID string, version int,
) (*types.MemoryPageRevision, error) {
	var revision types.MemoryPageRevision
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND page_id = ? AND version = ?", spaceID, pageID, version).
		First(&revision).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemoryPageNotFound
	}
	if err != nil {
		return nil, err
	}
	return &revision, nil
}

// ListForDecay returns unpinned active pages, oldest interaction first, so a
// bounded sweep always makes progress on the stalest memories.
func (r *memoryPageRepository) ListForDecay(
	ctx context.Context, spaceID string, limit int,
) ([]*types.MemoryPage, error) {
	if limit <= 0 {
		limit = 200
	}
	var pages []*types.MemoryPage
	err := r.db.WithContext(ctx).
		Where("space_id = ? AND status = ? AND pinned = ?", spaceID, types.MemoryPageStatusActive, false).
		Order("last_seen_at ASC NULLS FIRST").
		Limit(limit).
		Find(&pages).Error
	if err != nil {
		// NULLS FIRST is not universally supported; retry with a portable
		// ordering rather than failing the sweep.
		var fallback []*types.MemoryPage
		if ferr := r.db.WithContext(ctx).
			Where("space_id = ? AND status = ? AND pinned = ?", spaceID, types.MemoryPageStatusActive, false).
			Order("updated_at ASC").
			Limit(limit).
			Find(&fallback).Error; ferr != nil {
			return nil, err
		}
		return fallback, nil
	}
	return pages, nil
}
