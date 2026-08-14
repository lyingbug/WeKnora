package chat

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Anthropic now routes through the plugin client, which speaks the full
// Messages protocol — content blocks, tool use, and thinking blocks — instead
// of the text-only implementation this replaced.
func TestNewRemoteChat_AnthropicUsesTheMessagesPlugin(t *testing.T) {
	client, err := NewRemoteChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		ModelName: "claude-sonnet-4-5",
		APIKey:    "test-key",
		Provider:  string(provider.ProviderAnthropic),
	})
	require.NoError(t, err)

	plugin, ok := client.(*PluginChat)
	require.True(t, ok, "expected the plugin client, got %T", client)
	assert.Equal(t, spi.ProtocolAnthropicMessages, plugin.Descriptor().Protocol)
	assert.Equal(t, spi.ReplayAlways, plugin.Descriptor().EffectiveReplay(),
		"thinking blocks must be replayed verbatim")
}
