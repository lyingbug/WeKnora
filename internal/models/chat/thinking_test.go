package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func ptrBool(b bool) *bool { return &b }

// TestEffectiveThinkingControl asserts that the reported thinking field is the
// one the request path will actually use.
//
// The value is now the wire field itself rather than a category name, because
// that is the question the setting was always standing in for, and because it
// is read out of the resolved plugin instead of predicted by a second table.
// The frontend copy of those predictions is what this replaces.
func TestEffectiveThinkingControl(t *testing.T) {
	cases := []struct {
		name   string
		config *ChatConfig
		want   string
	}{
		{
			name:   "aliyun qwen uses the boolean",
			config: &ChatConfig{Provider: "aliyun", ModelName: "qwen3-32b"},
			want:   "enable_thinking",
		},
		{
			name:   "deepseek uses the thinking object",
			config: &ChatConfig{Provider: "deepseek", ModelName: "deepseek-chat"},
			want:   "thinking.type",
		},
		{
			name:   "self-hosted deployments use the chat template",
			config: &ChatConfig{Provider: "generic", ModelName: "qwen3"},
			want:   "chat_template_kwargs.enable_thinking",
		},
		{
			name:   "openai reasoning models toggle through the effort ladder",
			config: &ChatConfig{Provider: "openai", ModelName: "gpt-5"},
			want:   "reasoning_effort",
		},
		{
			name:   "anthropic uses the thinking object",
			config: &ChatConfig{Provider: "anthropic", ModelName: "claude-sonnet-4-6"},
			want:   "thinking.type",
		},
		{
			name:   "a model without a toggle reports none",
			config: &ChatConfig{Provider: "openai", ModelName: "gpt-4o"},
			want:   ThinkingControlNone,
		},
		{
			name:   "an unregistered provider reports none",
			config: &ChatConfig{Provider: "not-a-vendor", ModelName: "x"},
			want:   ThinkingControlNone,
		},
		{
			name:   "nil config reports none",
			config: nil,
			want:   ThinkingControlNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EffectiveThinkingControl(tc.config))
		})
	}
}

// TestLegacyThinkingControlOverride pins the compatibility path: a stored
// thinking_control still forces the wire format it names, so an operator who
// pinned one against a misdetected provider keeps the behavior they chose.
func TestLegacyThinkingControlOverride(t *testing.T) {
	cases := []struct {
		control string
		want    string
	}{
		{"enable_thinking", "enable_thinking"},
		{"thinking_type", "thinking.type"},
		{"chat_template_kwargs", "chat_template_kwargs.enable_thinking"},
		{"none", ThinkingControlNone},
	}

	for _, tc := range cases {
		t.Run(tc.control, func(t *testing.T) {
			// Aliyun's own default is the boolean, so anything else proves the
			// override took effect rather than the vendor default surviving.
			got := EffectiveThinkingControl(&ChatConfig{
				Provider:    "aliyun",
				ModelName:   "qwen3-32b",
				ExtraConfig: map[string]string{ExtraConfigThinkingControl: tc.control},
			})
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestSupportsThinking covers the predicate the UI asks before offering a
// toggle at all.
func TestSupportsThinking(t *testing.T) {
	assert.True(t, SupportsThinking(&ChatConfig{Provider: "aliyun", ModelName: "qwen3-32b"}))
	assert.True(t, SupportsThinking(&ChatConfig{Provider: "volcengine", ModelName: "doubao-seed-1-6"}))
	assert.False(t, SupportsThinking(&ChatConfig{Provider: "openai", ModelName: "gpt-4o"}))
	assert.False(t, SupportsThinking(nil))
}
