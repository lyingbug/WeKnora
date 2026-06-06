package agent

import (
	"context"
	"errors"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSkillExecutionRecorder struct {
	run *types.SkillExecutionRun
	err error
}

func (r *fakeSkillExecutionRecorder) RecordSkillExecution(_ context.Context, run *types.SkillExecutionRun) error {
	r.run = run
	return r.err
}

func TestRecordSkillExecutionRun_RecordsExecuteSkillScript(t *testing.T) {
	recorder := &fakeSkillExecutionRecorder{}
	engine := &AgentEngine{
		config: &types.AgentConfig{AgentID: "agent-a"},
	}
	engine.SetSkillExecutionRecorder(recorder)

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(10))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-a")
	engine.recordSkillExecutionRun(ctx, types.LLMToolCall{
		ID: "call-a",
		Function: types.FunctionCall{
			Name: agenttools.ToolExecuteSkillScript,
		},
	}, types.ToolCall{
		ID:   "call-a",
		Name: agenttools.ToolExecuteSkillScript,
		Args: map[string]interface{}{
			"skill_name":  "alpha",
			"script_path": "scripts/run.py",
		},
		Result: &types.ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"skill_name":  "alpha",
				"script_path": "scripts/run.py",
				"exit_code":   0,
				"duration_ms": float64(42),
			},
		},
	}, 99, "session-a", "message-a")

	require.NotNil(t, recorder.run)
	assert.NotEmpty(t, recorder.run.ID)
	assert.Equal(t, uint64(10), recorder.run.TenantID)
	assert.Equal(t, "user-a", recorder.run.UserID)
	assert.Equal(t, "agent-a", recorder.run.AgentID)
	assert.Equal(t, "session-a", recorder.run.SessionID)
	assert.Equal(t, "message-a", recorder.run.MessageID)
	assert.Equal(t, "call-a", recorder.run.ToolCallID)
	assert.Equal(t, "alpha", recorder.run.SkillID)
	assert.Equal(t, "scripts/run.py", recorder.run.ScriptPath)
	assert.Equal(t, "success", recorder.run.Status)
	assert.Equal(t, int64(42), recorder.run.DurationMS)
	assert.JSONEq(t, `{"duration_ms":42,"exit_code":0,"script_path":"scripts/run.py","skill_name":"alpha"}`, recorder.run.ResourceUsage.ToString())
}

func TestRecordSkillExecutionRun_IgnoresNonSkillToolAndRecorderErrors(t *testing.T) {
	recorder := &fakeSkillExecutionRecorder{err: errors.New("db down")}
	engine := &AgentEngine{}
	engine.SetSkillExecutionRecorder(recorder)

	engine.recordSkillExecutionRun(context.Background(), types.LLMToolCall{
		ID: "call-search",
		Function: types.FunctionCall{
			Name: agenttools.ToolKnowledgeSearch,
		},
	}, types.ToolCall{}, 1, "session-a", "message-a")
	assert.Nil(t, recorder.run)

	engine.recordSkillExecutionRun(context.Background(), types.LLMToolCall{
		ID: "call-skill",
		Function: types.FunctionCall{
			Name: agenttools.ToolExecuteSkillScript,
		},
	}, types.ToolCall{
		Result: &types.ToolResult{
			Success: false,
			Error:   "script failed",
		},
	}, 7, "session-a", "message-a")

	require.NotNil(t, recorder.run)
	assert.Equal(t, "failed", recorder.run.Status)
	assert.Equal(t, "script failed", recorder.run.ErrorSummary)
}
