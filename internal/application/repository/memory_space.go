package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// ErrMemorySpaceNotFound is returned when no space matches the lookup.
var ErrMemorySpaceNotFound = errors.New("memory space not found")

type memorySpaceRepository struct {
	db *gorm.DB
}

// NewMemorySpaceRepository creates the memory space repository.
func NewMemorySpaceRepository(db *gorm.DB) interfaces.MemorySpaceRepository {
	return &memorySpaceRepository{db: db}
}

func (r *memorySpaceRepository) Create(ctx context.Context, space *types.MemorySpace) error {
	return r.db.WithContext(ctx).Create(space).Error
}

func (r *memorySpaceRepository) GetByID(
	ctx context.Context, tenantID uint64, id string,
) (*types.MemorySpace, error) {
	var space types.MemorySpace
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		First(&space).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemorySpaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// GetByOwner resolves the space for one principal. The principal triple is the
// only stable cross-channel identity available: sessions.user_id encodes web
// UUIDs, embed composites and IM composites in the same column, so it cannot
// be used as a key here.
func (r *memorySpaceRepository) GetByOwner(
	ctx context.Context, tenantID uint64, scopeType string, principal types.Principal,
) (*types.MemorySpace, error) {
	principal = principal.Normalize()
	if !principal.Valid() {
		return nil, ErrMemorySpaceNotFound
	}
	var space types.MemorySpace
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND scope_type = ? AND owner_principal_type = ? AND owner_principal_id = ?",
			tenantID, scopeType, principal.Type, principal.ID).
		First(&space).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemorySpaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// Update writes the mutable columns through an explicit map. A struct update
// would skip zero values, which would make "clear the display name" or "reset
// the config to empty" silently not stick.
func (r *memorySpaceRepository) Update(ctx context.Context, space *types.MemorySpace) error {
	result := r.db.WithContext(ctx).
		Model(&types.MemorySpace{}).
		Where("tenant_id = ? AND id = ?", space.TenantID, space.ID).
		Updates(map[string]interface{}{
			"display_name": space.DisplayName,
			"status":       space.Status,
			"config":       space.Config,
			"vector_kb_id": space.VectorKBID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMemorySpaceNotFound
	}
	return nil
}

func (r *memorySpaceRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&types.MemorySpace{}).Error
}

func (r *memorySpaceRepository) ListByTenant(
	ctx context.Context, tenantID uint64,
) ([]*types.MemorySpace, error) {
	var spaces []*types.MemorySpace
	err := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at ASC").
		Find(&spaces).Error
	return spaces, err
}

// ListActiveIDs pages through active spaces for background sweeps. Ordering by
// id keeps the pagination stable while rows are being written concurrently.
func (r *memorySpaceRepository) ListActiveIDs(
	ctx context.Context, limit, offset int,
) ([]*types.MemorySpace, error) {
	if limit <= 0 {
		limit = 100
	}
	var spaces []*types.MemorySpace
	err := r.db.WithContext(ctx).
		Where("status = ?", types.MemorySpaceStatusActive).
		Order("id ASC").
		Limit(limit).Offset(offset).
		Find(&spaces).Error
	return spaces, err
}

// likeTerm builds a portable, case-insensitive LIKE pattern. ILIKE is
// PostgreSQL-only, so every text search in this package lowers both sides
// instead — identical behaviour on SQLite.
func likeTerm(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	q = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(q)
	return "%" + q + "%"
}
