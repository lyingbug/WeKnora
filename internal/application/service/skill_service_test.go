package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSkillServiceTestDB(t *testing.T) *gorm.DB {
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

func writeTestSkill(t *testing.T, root, dir, name, description string) {
	t.Helper()

	skillDir := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))
}

func TestSkillService_ImportPreloadedSkills_ImportsIntoRegistryAndListsRegistryEntries(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	require.NoError(t, svc.ImportPreloadedSkills(ctx))

	entry, err := repo.GetActiveSkillByNameVersion(ctx, "alpha", types.DefaultSkillVersion)
	require.NoError(t, err)
	assert.Equal(t, "preloaded-alpha-0-0-0", entry.ID)
	assert.Equal(t, types.SkillSourceTypePreloaded, entry.SourceType)
	assert.Equal(t, types.SkillStatusActive, entry.Status)
	assert.True(t, entry.IsBuiltin)
	assert.Equal(t, filepath.Join(tempDir, "alpha-dir"), entry.SourceURI)
	assert.NotEmpty(t, entry.Digest)
	assert.JSONEq(t, `{}`, entry.Manifest.ToString())

	require.NoError(t, os.RemoveAll(filepath.Join(tempDir, "alpha-dir")))

	got, err := svc.ListPreloadedSkills(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alpha", got[0].Name)
	assert.Equal(t, "Alpha skill", got[0].Description)
}

func TestSkillService_ListPreloadedSkills_FallsBackToFilesystemWhenRegistryIsEmpty(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	got, err := svc.ListPreloadedSkills(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta", got[0].Name)
	assert.Equal(t, "Beta skill", got[0].Description)
}

func TestSkillRegistryID_FitsDatabaseColumn(t *testing.T) {
	id := skillRegistryID(
		types.SkillSourceTypePreloaded,
		"very-long-skill-name-that-can-still-be-valid-but-would-overflow-the-id-column",
		types.DefaultSkillVersion,
	)

	require.LessOrEqual(t, len(id), 64)
	assert.Contains(t, id, "-")
}

func TestSkillService_EnsureTenantPreloadedSkillInstalls_ListsTenantSkills(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)

	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))

	got, err := svc.ListTenantSkills(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "alpha", got[0].Name)
	assert.Equal(t, "beta", got[1].Name)
}

func TestSkillService_SyncAndResolveAgentSelectedSkills(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	writeTestSkill(t, tempDir, "alpha-dir", "alpha", "Alpha skill")
	writeTestSkill(t, tempDir, "beta-dir", "beta", "Beta skill")

	repo := repository.NewSkillRepository(setupSkillServiceTestDB(t))
	svc := NewSkillServiceWithRepository(repo, tempDir)
	require.NoError(t, svc.EnsureTenantPreloadedSkillInstalls(ctx, 10))

	require.NoError(t, svc.SyncAgentSkillBindings(ctx, 10, "agent-a", "selected", []string{"beta", "missing"}))
	selected, err := svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "selected", []string{"beta", "missing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta"}, selected)

	all, err := svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "all", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, all)

	require.NoError(t, svc.SyncAgentSkillBindings(ctx, 10, "agent-a", "none", []string{"beta"}))
	selected, err = svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "selected", []string{"beta"})
	require.NoError(t, err)
	assert.Equal(t, []string{"beta"}, selected)

	none, err := svc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "none", nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
