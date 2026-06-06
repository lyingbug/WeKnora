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

func TestSkillExecutionRunRepository_CreateAndList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.SkillExecutionRun{}))

	repo := NewSkillExecutionRunRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, repo.CreateSkillExecutionRun(ctx, &types.SkillExecutionRun{
		ID:            "run-1",
		TenantID:      10,
		UserID:        "user-a",
		AgentID:       "agent-a",
		SessionID:     "session-a",
		MessageID:     "message-a",
		ToolCallID:    "call-a",
		SkillID:       "alpha",
		ScriptPath:    "scripts/run.py",
		Status:        "success",
		DurationMS:    15,
		ResourceUsage: types.JSON(`{"exit_code":0}`),
		CreatedAt:     now.Add(-time.Minute),
		UpdatedAt:     now.Add(-time.Minute),
	}))
	require.NoError(t, repo.CreateSkillExecutionRun(ctx, &types.SkillExecutionRun{
		ID:            "run-2",
		TenantID:      10,
		SkillID:       "beta",
		Status:        "failed",
		ResourceUsage: types.JSON(`{"exit_code":1}`),
		CreatedAt:     now,
		UpdatedAt:     now,
	}))
	require.NoError(t, repo.CreateSkillExecutionRun(ctx, &types.SkillExecutionRun{
		ID:        "run-other",
		TenantID:  11,
		SkillID:   "gamma",
		Status:    "success",
		CreatedAt: now,
		UpdatedAt: now,
	}))

	got, err := repo.ListSkillExecutionRuns(ctx, 10, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "run-2", got[0].ID)
	assert.Equal(t, "run-1", got[1].ID)
	assert.JSONEq(t, `{"exit_code":1}`, got[0].ResourceUsage.ToString())
}
