package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrFolderNotFound is returned when a folder is absent from the requested scope.
var ErrFolderNotFound = errors.New("folder not found")

// folderSubtreeHeightQuery reports how many levels a folder subtree spans,
// counting the root folder itself as level 1. The recursion carries its own
// depth counter and stops at the caller-supplied bound so a corrupted parent
// cycle terminates instead of spinning inside the database.
const folderSubtreeHeightQuery = `
WITH RECURSIVE folder_subtree(id, depth) AS (
	SELECT id, 1
	FROM folders
	WHERE tenant_id = ?
	  AND knowledge_base_id = ?
	  AND id = ?
	  AND deleted_at IS NULL
	UNION ALL
	SELECT child.id, parent.depth + 1
	FROM folders AS child
	JOIN folder_subtree AS parent ON child.parent_id = parent.id
	WHERE child.tenant_id = ?
	  AND child.knowledge_base_id = ?
	  AND child.deleted_at IS NULL
	  AND parent.depth < ?
)
SELECT COALESCE(MAX(depth), 0) FROM folder_subtree`

type folderRepository struct {
	db *gorm.DB
}

func NewFolderRepository(db *gorm.DB) interfaces.FolderRepository {
	return &folderRepository{db: db}
}

func (r *folderRepository) WithinTransaction(
	ctx context.Context,
	fn func(interfaces.FolderRepository) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&folderRepository{db: tx})
	})
}

func (r *folderRepository) Create(ctx context.Context, folder *types.Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

func (r *folderRepository) GetByID(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
) (*types.Folder, error) {
	return r.getByID(ctx, tenantID, knowledgeBaseID, folderID, false)
}

func (r *folderRepository) ListByIDs(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderIDs []string,
) ([]*types.Folder, error) {
	if len(folderIDs) == 0 {
		return []*types.Folder{}, nil
	}
	ids := append([]string(nil), folderIDs...)
	sort.Strings(ids)
	var folders []*types.Folder
	// Existence checks for a folder scope run on the read path of every
	// folder-scoped question, so they stay lock-free: taking FOR UPDATE here
	// would serialize concurrent questions against the same folder and block
	// renames and moves behind them.
	err := r.db.WithContext(ctx).
		Where(
			"tenant_id = ? AND knowledge_base_id = ? AND id IN ?",
			tenantID,
			knowledgeBaseID,
			ids,
		).
		Order("id ASC").
		Find(&folders).Error
	return folders, err
}

func (r *folderRepository) GetByIDForUpdate(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
) (*types.Folder, error) {
	return r.getByID(ctx, tenantID, knowledgeBaseID, folderID, true)
}

func (r *folderRepository) getByID(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
	forUpdate bool,
) (*types.Folder, error) {
	var folder types.Folder
	query := r.db.WithContext(ctx).Where(
		"tenant_id = ? AND knowledge_base_id = ? AND id = ?",
		tenantID,
		knowledgeBaseID,
		folderID,
	)
	if forUpdate && r.db.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&folder).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

func (r *folderRepository) ListChildren(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	parentID *string,
) ([]*types.Folder, error) {
	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, knowledgeBaseID)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	var folders []*types.Folder
	err := query.
		Order("created_at ASC").
		Order("id ASC").
		Find(&folders).Error
	return folders, err
}

func (r *folderRepository) ListByKnowledgeBase(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
) ([]*types.Folder, error) {
	var folders []*types.Folder
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, knowledgeBaseID).
		Order("created_at ASC").
		Order("id ASC").
		Find(&folders).Error
	return folders, err
}

func (r *folderRepository) UpdateName(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
	name string,
) error {
	result := r.db.WithContext(ctx).
		Model(&types.Folder{}).
		Where(
			"tenant_id = ? AND knowledge_base_id = ? AND id = ?",
			tenantID,
			knowledgeBaseID,
			folderID,
		).
		Updates(map[string]any{
			"name":       name,
			"updated_at": time.Now(),
		})
	return folderMutationResult(result)
}

func (r *folderRepository) UpdateParent(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
	parentID *string,
) error {
	// The service must reject self/descendant moves before this scoped write.
	result := r.db.WithContext(ctx).
		Model(&types.Folder{}).
		Where(
			"tenant_id = ? AND knowledge_base_id = ? AND id = ?",
			tenantID,
			knowledgeBaseID,
			folderID,
		).
		Updates(map[string]any{
			"parent_id":  parentID,
			"updated_at": time.Now(),
		})
	return folderMutationResult(result)
}

func (r *folderRepository) Delete(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
) error {
	result := r.db.WithContext(ctx).
		Where(
			"tenant_id = ? AND knowledge_base_id = ? AND id = ?",
			tenantID,
			knowledgeBaseID,
			folderID,
		).
		Delete(&types.Folder{})
	return folderMutationResult(result)
}

func (r *folderRepository) SubtreeHeight(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
	maxDepth int,
) (int, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}
	var height int
	err := r.db.WithContext(ctx).Raw(
		folderSubtreeHeightQuery,
		tenantID,
		knowledgeBaseID,
		folderID,
		tenantID,
		knowledgeBaseID,
		maxDepth,
	).Scan(&height).Error
	if err != nil {
		return 0, err
	}
	if height == 0 {
		return 0, ErrFolderNotFound
	}
	return height, nil
}

func (r *folderRepository) CountChildren(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.Folder{}).
		Where(
			"tenant_id = ? AND knowledge_base_id = ? AND parent_id = ?",
			tenantID,
			knowledgeBaseID,
			folderID,
		).
		Count(&count).Error
	return count, err
}

func (r *folderRepository) CountKnowledge(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	folderID string,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.Knowledge{}).
		Where(
			"tenant_id = ? AND knowledge_base_id = ? AND folder_id = ?",
			tenantID,
			knowledgeBaseID,
			folderID,
		).
		Count(&count).Error
	return count, err
}

func folderMutationResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrFolderNotFound
	}
	return nil
}
