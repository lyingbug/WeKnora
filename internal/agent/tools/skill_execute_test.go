package tools

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovedComputeTimeout(t *testing.T) {
	got, err := approvedComputeTimeout(types.JSON(`{"compute":{"timeout_seconds":2}}`))
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, got)

	got, err = approvedComputeTimeout(types.JSON(`{"network":[]}`))
	require.NoError(t, err)
	assert.Zero(t, got)

	_, err = approvedComputeTimeout(types.JSON(`{"compute":{"timeout_seconds":0}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "greater than zero")

	_, err = approvedComputeTimeout(types.JSON(`{"compute":"bad"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compute must be an object")
}

func TestApprovedComputeResourceLimits(t *testing.T) {
	memory, err := approvedComputeMemoryLimit(types.JSON(`{"compute":{"memory_mb":256}}`))
	require.NoError(t, err)
	assert.Equal(t, int64(256*1024*1024), memory)

	cpu, err := approvedComputeCPULimit(types.JSON(`{"compute":{"cpu":0.5}}`))
	require.NoError(t, err)
	assert.Equal(t, 0.5, cpu)

	memory, err = approvedComputeMemoryLimit(types.JSON(`{"compute":{"timeout_seconds":2}}`))
	require.NoError(t, err)
	assert.Zero(t, memory)

	_, err = approvedComputeMemoryLimit(types.JSON(`{"compute":{"memory_mb":0}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compute.memory_mb must be greater than zero")

	_, err = approvedComputeCPULimit(types.JSON(`{"compute":{"cpu":"bad"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compute.cpu must be a number")
}

func TestApprovedNetworkAllowed(t *testing.T) {
	got, err := approvedNetworkAllowed(types.JSON(`{"network":["api.example.com"]}`))
	require.NoError(t, err)
	assert.True(t, got)

	got, err = approvedNetworkAllowed(types.JSON(`{"network":[]}`))
	require.NoError(t, err)
	assert.False(t, got)

	got, err = approvedNetworkAllowed(types.JSON(`{"network":["   "]}`))
	require.NoError(t, err)
	assert.False(t, got)

	got, err = approvedNetworkAllowed(types.JSON(`{"compute":{"timeout_seconds":2}}`))
	require.NoError(t, err)
	assert.False(t, got)

	_, err = approvedNetworkAllowed(types.JSON(`{"network":"api.example.com"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network must be an array")

	_, err = approvedNetworkAllowed(types.JSON(`{"network":[123]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network entries must be strings")
}

func TestRejectUnsupportedRuntimePermissions(t *testing.T) {
	err := rejectUnsupportedRuntimePermissions(types.JSON(`{"files":[]}`))
	require.NoError(t, err)

	err = rejectUnsupportedRuntimePermissions(types.JSON(`{"files":["   "]}`))
	require.NoError(t, err)

	err = rejectUnsupportedRuntimePermissions(types.JSON(`{"compute":{"timeout_seconds":2}}`))
	require.NoError(t, err)

	err = rejectUnsupportedRuntimePermissions(types.JSON(`{"files":["session-temp"]}`))
	require.NoError(t, err)

	err = rejectUnsupportedRuntimePermissions(types.JSON(`{"credentials":{"api_key":"secret"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials permissions are not supported at runtime")

	err = rejectUnsupportedRuntimePermissions(types.JSON(`{"mcp":{"services":["weather"]}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcp permissions are not supported at runtime")
}

func TestApprovedExecutionPolicyRejectsUnsupportedRuntimePermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions types.JSON
		want        string
	}{
		{
			name:        "credentials",
			permissions: types.JSON(`{"credentials":["OPENAI_API_KEY"]}`),
			want:        "credentials permissions are not supported at runtime",
		},
		{
			name:        "mcp",
			permissions: types.JSON(`{"mcp":["weather"]}`),
			want:        "mcp permissions are not supported at runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewExecuteSkillScriptTool(nil)
			tool.SetPermissionChecker(fakeSkillPermissionChecker{permissions: tt.permissions})
			ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

			_, err := tool.approvedExecutionPolicy(ctx, "runtime-bound-skill")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestApprovedExecutionPolicy(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"network":["api.example.com"],"compute":{"timeout_seconds":3,"memory_mb":128,"cpu":0.75}}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	policy, err := tool.approvedExecutionPolicy(ctx, "web-fetcher")
	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, policy.Timeout)
	assert.True(t, policy.AllowNetwork)
	assert.Equal(t, int64(128*1024*1024), policy.MemoryLimit)
	assert.Equal(t, 0.75, policy.CPULimit)
}

func TestApprovedExecutionPolicyCreatesSessionTempFileMount(t *testing.T) {
	t.Setenv("WEKNORA_SKILL_SESSION_DIR", t.TempDir())
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"files":["session-temp"]}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = context.WithValue(ctx, types.UserIDContextKey, "user-a")
	ctx = context.WithValue(ctx, types.SessionIDContextKey, "session-a")

	policy, err := tool.approvedExecutionPolicy(ctx, "file-processor")
	require.NoError(t, err)
	require.Len(t, policy.Mounts, 1)
	assert.Contains(t, policy.Mounts[0].HostPath, "tenant-7")
	assert.Contains(t, policy.Mounts[0].HostPath, "user-a")
	assert.Contains(t, policy.Mounts[0].HostPath, "session-a")
	assert.Equal(t, "/mnt/weknora/session", policy.Mounts[0].ContainerPath)
	assert.False(t, policy.Mounts[0].ReadOnly)
}

func TestApprovedExecutionPolicyRejectsUnsupportedFileScope(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"files":["workspace-read"]}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	_, err := tool.approvedExecutionPolicy(ctx, "file-processor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported files permission scope")
}

type fakeSkillPermissionChecker struct {
	permissions types.JSON
}

func (f fakeSkillPermissionChecker) ApprovedPermissions(context.Context, uint64, string) (types.JSON, error) {
	return f.permissions, nil
}
