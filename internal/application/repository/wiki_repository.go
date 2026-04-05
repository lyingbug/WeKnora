package repository

import (
	"context"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// WikiRepositoryImpl implements interfaces.WikiRepository using GORM.
type WikiRepositoryImpl struct {
	db *gorm.DB
}

// NewWikiRepository creates a new WikiRepositoryImpl.
func NewWikiRepository(db *gorm.DB) interfaces.WikiRepository {
	return &WikiRepositoryImpl{db: db}
}

// --- WikiPage CRUD ---

func (r *WikiRepositoryImpl) CreatePage(ctx context.Context, page *types.WikiPage) error {
	return r.db.WithContext(ctx).Create(page).Error
}

func (r *WikiRepositoryImpl) GetPageByID(ctx context.Context, tenantID uint64, pageID string) (*types.WikiPage, error) {
	var page types.WikiPage
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", pageID, tenantID).
		First(&page).Error
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *WikiRepositoryImpl) GetPageBySlug(ctx context.Context, tenantID uint64, kbID string, slug string) (*types.WikiPage, error) {
	var page types.WikiPage
	err := r.db.WithContext(ctx).
		Where("slug = ? AND knowledge_base_id = ? AND tenant_id = ?", slug, kbID, tenantID).
		First(&page).Error
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *WikiRepositoryImpl) GetPageByTitle(ctx context.Context, tenantID uint64, kbID string, title string) (*types.WikiPage, error) {
	var page types.WikiPage
	err := r.db.WithContext(ctx).
		Where("title = ? AND knowledge_base_id = ? AND tenant_id = ?", title, kbID, tenantID).
		First(&page).Error
	if err != nil {
		return nil, err
	}
	return &page, nil
}

func (r *WikiRepositoryImpl) ListPages(ctx context.Context, tenantID uint64, kbID string, pageType string, offset, limit int) ([]*types.WikiPage, int64, error) {
	var pages []*types.WikiPage
	var total int64

	query := r.db.WithContext(ctx).Model(&types.WikiPage{}).
		Where("knowledge_base_id = ? AND tenant_id = ?", kbID, tenantID)

	if pageType != "" {
		query = query.Where("page_type = ?", pageType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("updated_at DESC").Offset(offset).Limit(limit).Find(&pages).Error; err != nil {
		return nil, 0, err
	}

	return pages, total, nil
}

func (r *WikiRepositoryImpl) UpdatePage(ctx context.Context, page *types.WikiPage) error {
	return r.db.WithContext(ctx).Save(page).Error
}

func (r *WikiRepositoryImpl) DeletePage(ctx context.Context, tenantID uint64, pageID string) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", pageID, tenantID).
		Delete(&types.WikiPage{}).Error
}

func (r *WikiRepositoryImpl) SearchPages(ctx context.Context, tenantID uint64, kbID string, query string, limit int) ([]*types.WikiPage, error) {
	var pages []*types.WikiPage
	searchPattern := "%" + strings.ToLower(query) + "%"

	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND tenant_id = ? AND (LOWER(title) LIKE ? OR LOWER(content) LIKE ?)",
			kbID, tenantID, searchPattern, searchPattern).
		Order("updated_at DESC").
		Limit(limit).
		Find(&pages).Error
	if err != nil {
		return nil, err
	}
	return pages, nil
}

func (r *WikiRepositoryImpl) CountPages(ctx context.Context, tenantID uint64, kbID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.WikiPage{}).
		Where("knowledge_base_id = ? AND tenant_id = ?", kbID, tenantID).
		Count(&count).Error
	return count, err
}

func (r *WikiRepositoryImpl) CountPagesByType(ctx context.Context, tenantID uint64, kbID string) (map[string]int64, error) {
	type result struct {
		PageType string
		Count    int64
	}
	var results []result

	err := r.db.WithContext(ctx).Model(&types.WikiPage{}).
		Select("page_type, COUNT(*) as count").
		Where("knowledge_base_id = ? AND tenant_id = ?", kbID, tenantID).
		Group("page_type").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.PageType] = r.Count
	}
	return counts, nil
}

func (r *WikiRepositoryImpl) FindOrphanPages(ctx context.Context, tenantID uint64, kbID string) ([]*types.WikiPage, error) {
	var pages []*types.WikiPage
	// Orphan pages have no inbound links (in_links is null or empty JSON array)
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND tenant_id = ? AND (in_links IS NULL OR in_links = '[]' OR in_links = 'null')",
			kbID, tenantID).
		Find(&pages).Error
	if err != nil {
		return nil, err
	}
	return pages, nil
}

// --- WikiSchema CRUD ---

func (r *WikiRepositoryImpl) GetSchema(ctx context.Context, tenantID uint64, kbID string) (*types.WikiSchema, error) {
	var schema types.WikiSchema
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND tenant_id = ?", kbID, tenantID).
		First(&schema).Error
	if err != nil {
		return nil, err
	}
	return &schema, nil
}

func (r *WikiRepositoryImpl) CreateSchema(ctx context.Context, schema *types.WikiSchema) error {
	return r.db.WithContext(ctx).Create(schema).Error
}

func (r *WikiRepositoryImpl) UpdateSchema(ctx context.Context, schema *types.WikiSchema) error {
	return r.db.WithContext(ctx).Save(schema).Error
}

// --- WikiLintIssue CRUD ---

func (r *WikiRepositoryImpl) CreateLintIssue(ctx context.Context, issue *types.WikiLintIssue) error {
	return r.db.WithContext(ctx).Create(issue).Error
}

func (r *WikiRepositoryImpl) ListLintIssues(ctx context.Context, tenantID uint64, kbID string, resolved bool) ([]*types.WikiLintIssue, error) {
	var issues []*types.WikiLintIssue
	err := r.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND tenant_id = ? AND resolved = ?", kbID, tenantID, resolved).
		Order("created_at DESC").
		Find(&issues).Error
	if err != nil {
		return nil, err
	}
	return issues, nil
}

func (r *WikiRepositoryImpl) CountOpenLintIssues(ctx context.Context, tenantID uint64, kbID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&types.WikiLintIssue{}).
		Where("knowledge_base_id = ? AND tenant_id = ? AND resolved = ?", kbID, tenantID, false).
		Count(&count).Error
	return count, err
}

func (r *WikiRepositoryImpl) ResolveLintIssue(ctx context.Context, tenantID uint64, issueID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&types.WikiLintIssue{}).
		Where("id = ? AND tenant_id = ?", issueID, tenantID).
		Updates(map[string]interface{}{"resolved": true, "resolved_at": &now}).Error
}
