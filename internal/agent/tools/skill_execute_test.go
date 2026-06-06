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
	require.NoError(t, err)

	err = rejectUnsupportedRuntimePermissions(types.JSON(`{"mcp":{"services":["weather"]}}`))
	require.NoError(t, err)
}

func TestApprovedExecutionPolicyRejectsUnconfiguredMCPBindings(t *testing.T) {
	tests := []struct {
		name        string
		permissions types.JSON
		want        string
	}{
		{
			name:        "mcp",
			permissions: types.JSON(`{"mcp":["weather"]}`),
			want:        "approved mcp binding weather is not configured",
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

func TestApprovedExecutionPolicyInjectsApprovedCredentials(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"credentials":["API_KEY"]}`),
		credentials: types.JSON(`{"API_KEY":"secret","OTHER":"hidden"}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	policy, err := tool.approvedExecutionPolicy(ctx, "credential-skill")
	require.NoError(t, err)
	assert.Equal(t, "secret", policy.Env["API_KEY"])
	assert.NotContains(t, policy.Env, "OTHER")
}

func TestApprovedExecutionPolicyRejectsMissingApprovedCredential(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"credentials":["API_KEY"]}`),
		credentials: types.JSON(`{}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	_, err := tool.approvedExecutionPolicy(ctx, "credential-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved credential API_KEY is not configured")
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
	assert.Equal(t, []string{"api.example.com"}, policy.AllowedNetworkDomains)
	assert.Contains(t, policy.Env["HTTP_PROXY"], "http://host.docker.internal:")
	assert.Equal(t, policy.Env["HTTP_PROXY"], policy.Env["HTTPS_PROXY"])
	assert.Equal(t, policy.Env["HTTP_PROXY"], policy.Env["ALL_PROXY"])
	require.NotNil(t, policy.Cleanup)
	policy.Cleanup()
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

func TestApprovedExecutionPolicyInjectsApprovedMCPBindings(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"mcp":["github"]}`),
		mcpBindings: types.JSON(`{"github":"mcp-service-1","other":"hidden"}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	policy, err := tool.approvedExecutionPolicy(ctx, "mcp-skill")
	require.NoError(t, err)
	assert.JSONEq(t, `{"github":"mcp-service-1"}`, policy.Env["WEKNORA_SKILL_MCP_BINDINGS"])
}

func TestApprovedExecutionPolicyInjectsMCPBrokerSession(t *testing.T) {
	broker := NewSkillMCPBroker(
		fakeBrokerMCPServiceGetter{service: &types.MCPService{ID: "mcp-service-1", TenantID: 7, Name: "github", Enabled: true}},
		fakeBrokerMCPClientProvider{client: &fakeBrokerMCPClient{}},
		nil,
	)
	defer broker.Shutdown(context.Background())

	tool := NewExecuteSkillScriptTool(nil)
	tool.SetMCPBroker(broker)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"mcp":["github"]}`),
		mcpBindings: types.JSON(`{"github":"mcp-service-1"}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	policy, err := tool.approvedExecutionPolicy(ctx, "mcp-skill")
	require.NoError(t, err)
	defer policy.Cleanup()
	assert.True(t, policy.AllowNetwork)
	assert.Contains(t, policy.Env["WEKNORA_SKILL_MCP_BROKER_URL"], "http://host.docker.internal:")
	assert.NotEmpty(t, policy.Env["WEKNORA_SKILL_MCP_TOKEN"])
	assert.JSONEq(t, `{"github":"mcp-service-1"}`, policy.Env["WEKNORA_SKILL_MCP_BINDINGS"])
}

func TestApprovedExecutionPolicyRejectsMissingApprovedMCPBinding(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"mcp":["github"]}`),
		mcpBindings: types.JSON(`{}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	_, err := tool.approvedExecutionPolicy(ctx, "mcp-skill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approved mcp binding github is not configured")
}

type fakeSkillPermissionChecker struct {
	permissions types.JSON
	credentials types.JSON
	mcpBindings types.JSON
}

func (f fakeSkillPermissionChecker) ApprovedPermissions(context.Context, uint64, string) (types.JSON, error) {
	return f.permissions, nil
}

func (f fakeSkillPermissionChecker) ApprovedCredentials(context.Context, uint64, string) (types.JSON, error) {
	return f.credentials, nil
}

func (f fakeSkillPermissionChecker) ApprovedMCPBindings(context.Context, uint64, string) (types.JSON, error) {
	return f.mcpBindings, nil
}
