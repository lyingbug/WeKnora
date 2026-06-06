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

func TestApprovedExecutionPolicy(t *testing.T) {
	tool := NewExecuteSkillScriptTool(nil)
	tool.SetPermissionChecker(fakeSkillPermissionChecker{
		permissions: types.JSON(`{"network":["api.example.com"],"compute":{"timeout_seconds":3}}`),
	})
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))

	policy, err := tool.approvedExecutionPolicy(ctx, "web-fetcher")
	require.NoError(t, err)
	assert.Equal(t, 3*time.Second, policy.Timeout)
	assert.True(t, policy.AllowNetwork)
}

type fakeSkillPermissionChecker struct {
	permissions types.JSON
}

func (f fakeSkillPermissionChecker) ApprovedPermissions(context.Context, uint64, string) (types.JSON, error) {
	return f.permissions, nil
}
