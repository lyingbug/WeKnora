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
	require.NoError(t, db.AutoMigrate(
		&types.SkillRegistryEntry{},
		&types.TenantSkillInstall{},
		&types.AgentSkillBinding{},
	))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name_version ON skills(name, version)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_installs_tenant_skill ON tenant_skill_installs(tenant_id, skill_id)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_skill_bindings_tenant_agent_skill ON agent_skill_bindings(tenant_id, agent_id, skill_id)").Error)

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

	got, err := repo.GetActiveSkillByNameVersion(ctx, "code-review", "1.0.0")
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

func TestSkillRepository_GetActiveSkillByNameVersion_RequiresExactVersion(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.UpsertSkill(ctx, testSkillEntry("code-review", "1.2.0", types.SkillStatusActive)))
	require.NoError(t, repo.UpsertSkill(ctx, testSkillEntry("code-review", "1.10.0", types.SkillStatusActive)))
	require.NoError(t, repo.UpsertSkill(ctx, testSkillEntry("code-review", "2.0.0", types.SkillStatusDisabled)))

	got, err := repo.GetActiveSkillByNameVersion(ctx, "code-review", "1.10.0")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "1.10.0", got.Version)

	_, err = repo.GetActiveSkillByNameVersion(ctx, "code-review", "2.0.0")
	require.Error(t, err)
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

func TestSkillRepository_TenantInstalls_ListEnabledActiveSkills(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	for _, skill := range []*types.SkillRegistryEntry{
		testSkillEntry("beta", "1.0.0", types.SkillStatusActive),
		testSkillEntry("alpha", "1.0.0", types.SkillStatusActive),
		testSkillEntry("disabled-skill", "1.0.0", types.SkillStatusDisabled),
	} {
		require.NoError(t, repo.UpsertSkill(ctx, skill))
	}

	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-10-alpha",
		TenantID: 10,
		SkillID:  "alpha-1.0.0",
		Enabled:  true,
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-10-beta-disabled",
		TenantID: 10,
		SkillID:  "beta-1.0.0",
		Enabled:  false,
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-10-disabled-install",
		TenantID: 10,
		SkillID:  "disabled-skill-1.0.0",
		Enabled:  true,
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-11-alpha",
		TenantID: 11,
		SkillID:  "alpha-1.0.0",
		Enabled:  true,
	}))
	got, err := repo.ListTenantInstalledSkills(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alpha", got[0].Name)

	byName, err := repo.ListTenantInstalledSkillNames(ctx, 10)
	require.NoError(t, err)
	require.Len(t, byName, 1)
	assert.Equal(t, "alpha-1.0.0", byName["alpha"].ID)
}

func TestSkillRepository_TenantSkillInstallLifecycle(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	for _, skill := range []*types.SkillRegistryEntry{
		testSkillEntry("alpha", "1.0.0", types.SkillStatusActive),
		testSkillEntry("beta", "1.0.0", types.SkillStatusActive),
		testSkillEntry("disabled-skill", "1.0.0", types.SkillStatusDisabled),
	} {
		require.NoError(t, repo.UpsertSkill(ctx, skill))
	}

	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:                  "tenant-10-alpha",
		TenantID:            10,
		SkillID:             "alpha-1.0.0",
		Enabled:             true,
		InstalledBy:         "user-a",
		ApprovedPermissions: types.JSON(`{"network":[]}`),
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:                  "tenant-10-beta",
		TenantID:            10,
		SkillID:             "beta-1.0.0",
		Enabled:             false,
		InstalledBy:         "user-b",
		ApprovedPermissions: types.JSON(`{"files":["session-temp"]}`),
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-10-disabled",
		TenantID: 10,
		SkillID:  "disabled-skill-1.0.0",
		Enabled:  true,
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-11-alpha",
		TenantID: 11,
		SkillID:  "alpha-1.0.0",
		Enabled:  true,
	}))

	installs, err := repo.ListTenantSkillInstallEntries(ctx, 10)
	require.NoError(t, err)
	require.Len(t, installs, 2)
	assert.Equal(t, "alpha", installs[0].Name)
	assert.True(t, installs[0].Enabled)
	assert.Equal(t, "user-a", installs[0].InstalledBy)
	assert.JSONEq(t, `{"network":[]}`, installs[0].ApprovedPermissions.ToString())
	assert.Equal(t, "beta", installs[1].Name)
	assert.False(t, installs[1].Enabled)

	require.NoError(t, repo.SetTenantSkillInstallEnabled(ctx, 10, "alpha-1.0.0", false))
	installs, err = repo.ListTenantSkillInstallEntries(ctx, 10)
	require.NoError(t, err)
	require.Len(t, installs, 2)
	assert.False(t, installs[0].Enabled)

	err = repo.SetTenantSkillInstallEnabled(ctx, 10, "missing-skill", true)
	require.Error(t, err)
}

func TestSkillRepository_GetTenantSkillInstallEntryByName(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.UpsertSkill(ctx, testSkillEntry("alpha", "1.0.0", types.SkillStatusActive)))
	require.NoError(t, repo.UpsertSkill(ctx, testSkillEntry("beta", "1.0.0", types.SkillStatusActive)))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:                  "tenant-10-alpha",
		TenantID:            10,
		SkillID:             "alpha-1.0.0",
		Enabled:             true,
		ApprovedPermissions: types.JSON(`{"compute":{"timeout_seconds":5}}`),
	}))
	require.NoError(t, repo.UpsertTenantSkillInstall(ctx, &types.TenantSkillInstall{
		ID:       "tenant-10-beta",
		TenantID: 10,
		SkillID:  "beta-1.0.0",
		Enabled:  false,
	}))

	got, err := repo.GetTenantSkillInstallEntryByName(ctx, 10, "alpha")
	require.NoError(t, err)
	assert.Equal(t, "alpha", got.Name)
	assert.JSONEq(t, `{"compute":{"timeout_seconds":5}}`, got.ApprovedPermissions.ToString())

	_, err = repo.GetTenantSkillInstallEntryByName(ctx, 10, "beta")
	require.Error(t, err)
}

func TestSkillRepository_AgentBindings_ReplaceAndList(t *testing.T) {
	db := setupSkillRepositoryTestDB(t)
	repo := NewSkillRepository(db)
	ctx := context.Background()

	for _, skill := range []*types.SkillRegistryEntry{
		testSkillEntry("alpha", "1.0.0", types.SkillStatusActive),
		testSkillEntry("beta", "1.0.0", types.SkillStatusActive),
		testSkillEntry("disabled-skill", "1.0.0", types.SkillStatusDisabled),
	} {
		require.NoError(t, repo.UpsertSkill(ctx, skill))
	}

	require.NoError(t, repo.ReplaceAgentSkillBindings(ctx, 10, "agent-a", []string{"alpha-1.0.0", "beta-1.0.0"}))
	got, err := repo.ListAgentSkillBindings(ctx, 10, "agent-a")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "alpha", got[0].Name)
	assert.Equal(t, "beta", got[1].Name)

	require.NoError(t, repo.ReplaceAgentSkillBindings(ctx, 10, "agent-a", []string{"beta-1.0.0", "disabled-skill-1.0.0"}))
	got, err = repo.ListAgentSkillBindings(ctx, 10, "agent-a")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta", got[0].Name)

	otherTenant, err := repo.ListAgentSkillBindings(ctx, 11, "agent-a")
	require.NoError(t, err)
	assert.Empty(t, otherTenant)
}
