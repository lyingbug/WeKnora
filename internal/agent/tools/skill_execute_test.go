package tools

import (
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
