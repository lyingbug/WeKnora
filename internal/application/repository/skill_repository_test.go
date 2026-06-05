package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSkillRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.SkillRegistryEntry{}))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name_version ON skills(name, version)").Error)

	return db
}

func testSkillEntry(name, version, status string) *types.SkillRegistryEntry {
	now := time.Now().UTC()

	return &types.SkillRegistryEntry{
		ID:          name + "-" + version,
		Name:        name,
		Version:     version,
		Description: "original description",
		SourceType:  types.SkillSourceTypePreloaded,
		SourceURI:   "skills/preloaded/" + name,
		Digest:      "digest-original",
		Manifest:    types.JSON(`{"kind":"original"}`),
		Status:      status,
		IsBuiltin:   true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestSkillRepository_UpsertSkill_IdempotentByNameVersion(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	first := testSkillEntry("code-review", "1.0.0", types.SkillStatusActive)
	require.NoError(t, repo.UpsertSkill(ctx, first))

	updated := testSkillEntry("code-review", "1.0.0", types.SkillStatusActive)
	updated.ID = "replacement-id-ignored"
	updated.Description = "updated description"
	updated.SourceType = "marketplace"
	updated.SourceURI = "registry://code-review"
	updated.Digest = "digest-updated"
	updated.Manifest = types.JSON(`{"kind":"updated","permissions":["read"]}`)
	updated.IsBuiltin = false
	updated.UpdatedAt = first.UpdatedAt.Add(time.Hour)
	require.NoError(t, repo.UpsertSkill(ctx, updated))

	count, err := repo.CountSkills(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	got, err := repo.GetActiveSkillByName(ctx, "code-review")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, first.ID, got.ID, "upsert should not replace the registry row identity")
	assert.Equal(t, "updated description", got.Description)
	assert.Equal(t, "marketplace", got.SourceType)
	assert.Equal(t, "registry://code-review", got.SourceURI)
	assert.Equal(t, "digest-updated", got.Digest)
	assert.JSONEq(t, `{"kind":"updated","permissions":["read"]}`, got.Manifest.ToString())
	assert.False(t, got.IsBuiltin)
	assert.True(t, got.UpdatedAt.Equal(updated.UpdatedAt))
}

func TestSkillRepository_ListActiveSkills_FiltersAndOrders(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	for _, skill := range []*types.SkillRegistryEntry{
		testSkillEntry("beta", "2.0.0", types.SkillStatusActive),
		testSkillEntry("alpha", "2.0.0", types.SkillStatusActive),
		testSkillEntry("alpha", "1.0.0", types.SkillStatusActive),
		testSkillEntry("alpha", "3.0.0", types.SkillStatusDisabled),
		testSkillEntry("gamma", "1.0.0", types.SkillStatusDisabled),
	} {
		require.NoError(t, repo.UpsertSkill(ctx, skill))
	}

	got, err := repo.ListActiveSkills(ctx)
	require.NoError(t, err)

	require.Len(t, got, 3)
	assert.Equal(t, "alpha", got[0].Name)
	assert.Equal(t, "1.0.0", got[0].Version)
	assert.Equal(t, "alpha", got[1].Name)
	assert.Equal(t, "2.0.0", got[1].Version)
	assert.Equal(t, "beta", got[2].Name)
	assert.Equal(t, "2.0.0", got[2].Version)
}
