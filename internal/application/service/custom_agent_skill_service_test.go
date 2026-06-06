package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCustomAgentSkillServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.CustomAgent{},
		&types.SkillRegistryEntry{},
		&types.TenantSkillInstall{},
		&types.AgentSkillBinding{},
	))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name_version ON skills(name, version)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_skill_installs_tenant_skill ON tenant_skill_installs(tenant_id, skill_id)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_skill_bindings_tenant_agent_skill ON agent_skill_bindings(tenant_id, agent_id, skill_id)").Error)

	return db
}

func TestCustomAgentService_UpdateAgentSkillConfig_SyncsConfigAndBindings(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(10))
	preloadedDir := t.TempDir()
	writeTestSkill(t, preloadedDir, "alpha-dir", "alpha", "Alpha skill")
	writeTestSkill(t, preloadedDir, "beta-dir", "beta", "Beta skill")

	db := setupCustomAgentSkillServiceTestDB(t)
	agentRepo := repository.NewCustomAgentRepository(db)
	skillRepo := repository.NewSkillRepository(db)
	skillSvc := NewSkillServiceWithRepository(skillRepo, preloadedDir)
	require.NoError(t, skillSvc.EnsureTenantPreloadedSkillInstalls(ctx, 10))

	svc := NewCustomAgentService(agentRepo, nil, nil, nil, nil, skillSvc)
	require.NoError(t, agentRepo.CreateAgent(ctx, &types.CustomAgent{
		ID:       "agent-a",
		Name:     "Agent A",
		TenantID: 10,
		Config: types.CustomAgentConfig{
			AgentMode:           types.AgentModeSmartReasoning,
			SkillsSelectionMode: "none",
		},
	}))

	got, err := svc.UpdateAgentSkillConfig(ctx, "agent-a", "selected", []string{"alpha", "alpha", "missing"})
	require.NoError(t, err)
	assert.Equal(t, "agent-a", got.AgentID)
	assert.Equal(t, "selected", got.Mode)
	assert.Equal(t, []string{"alpha", "missing"}, got.SelectedSkills)

	agent, err := agentRepo.GetAgentByID(ctx, "agent-a", 10)
	require.NoError(t, err)
	assert.Equal(t, "selected", agent.Config.SkillsSelectionMode)
	assert.Equal(t, []string{"alpha", "missing"}, agent.Config.SelectedSkills)

	resolved, err := skillSvc.ResolveAgentSelectedSkills(ctx, 10, "agent-a", "selected", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, resolved)

	got, err = svc.UpdateAgentSkillConfig(ctx, "agent-a", "all", []string{"alpha"})
	require.NoError(t, err)
	assert.Equal(t, "all", got.Mode)
	assert.Empty(t, got.SelectedSkills)

	agent, err = agentRepo.GetAgentByID(ctx, "agent-a", 10)
	require.NoError(t, err)
	assert.Equal(t, "all", agent.Config.SkillsSelectionMode)
	assert.Empty(t, agent.Config.SelectedSkills)

	_, err = svc.UpdateAgentSkillConfig(ctx, "agent-a", "bad-mode", nil)
	require.ErrorIs(t, err, ErrInvalidSkillMode)
}
