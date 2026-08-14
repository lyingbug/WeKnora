package chat

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveProvider pins the transport-level adapter table. It is short
// because parameter behavior moved to the model plugins: what remains here is
// only what a declaration cannot express — signing, a non-derived endpoint, a
// message rewrite, and tool-call metadata.
func TestResolveProvider(t *testing.T) {
	cases := []struct {
		name  string
		prov  provider.ProviderName
		model string
		want  providerAdapter
	}{
		{"deepseek keeps the raw path for cache counters", provider.ProviderDeepSeek, "deepseek-chat", deepseekProvider{}},
		{"gemini carries thought signatures", provider.ProviderGemini, "gemini-3-flash-preview", geminiProvider{}},
		{"azure authenticates with api-key", provider.ProviderAzureOpenAI, "gpt-4", azureProvider{}},
		{"weknora cloud signs its requests", provider.ProviderWeKnoraCloud, "anything", weKnoraCloudProvider{}},
		{"openai needs no transport adapter", provider.ProviderOpenAI, "gpt-5", baseProvider{}},
		{"aliyun needs no transport adapter", provider.ProviderAliyun, "qwen3-32b", baseProvider{}},
		{"unknown falls back", provider.ProviderName("nope"), "x", baseProvider{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.IsType(t, tc.want, resolveProvider(tc.prov, tc.model))
		})
	}
}

func newOutboundChat(t *testing.T, providerName, model string, extra map[string]string) *RemoteAPIChat {
	t.Helper()
	c, err := NewRemoteAPIChat(&ChatConfig{
		Source:      types.ModelSourceRemote,
		ModelName:   model,
		APIKey:      "k",
		ModelID:     model,
		Provider:    providerName,
		ExtraConfig: extra,
	})
	require.NoError(t, err)
	return c
}

// TestBuildOutbound_Thinking asserts the wire format each vendor's plugin
// produces through the OpenAI-compatible transport. The expectations match the
// ones this path produced before the plugin seam existed, so a regression in
// the new resolution shows up as a changed body rather than as silence.
func TestBuildOutbound_Thinking(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hi"}}

	t.Run("generic deployment uses chat_template_kwargs", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderGeneric), "qwen", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(false)}, true)
		require.NoError(t, err)
		require.True(t, useRaw)
		assert.Contains(t, mustJSON(t, body), "chat_template_kwargs")
	})

	t.Run("stored thinking_type override wins over the plugin default", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderGeneric), "deepseek-v4-flash",
			map[string]string{ExtraConfigThinkingControl: "thinking_type"})
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(false)}, true)
		require.NoError(t, err)
		require.True(t, useRaw)
		js := mustJSON(t, body)
		assert.Contains(t, js, `"thinking"`)
		assert.Contains(t, js, `"disabled"`)
		assert.NotContains(t, js, "chat_template_kwargs")
	})

	t.Run("stored none override sends no thinking field", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderGeneric), "x",
			map[string]string{ExtraConfigThinkingControl: "none"})
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(false)}, true)
		require.NoError(t, err)
		assert.False(t, useRaw)
		_, ok := body.(*openai.ChatCompletionRequest)
		assert.True(t, ok)
	})

	t.Run("qwen non-stream forces disabled", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderAliyun), "qwen3-32b", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(true)}, false)
		require.NoError(t, err)
		require.True(t, useRaw)
		assert.Contains(t, mustJSON(t, body), `"enable_thinking":false`)
	})

	t.Run("qwen stream honors requested true", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderAliyun), "qwen3-32b", nil)
		body, _, _, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(true)}, true)
		require.NoError(t, err)
		assert.Contains(t, mustJSON(t, body), `"enable_thinking":true`)
	})

	t.Run("volcengine thinking enabled", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderVolcengine), "doubao", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(true)}, true)
		require.NoError(t, err)
		require.True(t, useRaw)
		js := mustJSON(t, body)
		assert.Contains(t, js, `"thinking"`)
		assert.Contains(t, js, `"enabled"`)
	})

	t.Run("lkeap deepseek-v3 emits thinking type", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderLKEAP), "deepseek-v3.1", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(false)}, true)
		require.NoError(t, err)
		require.True(t, useRaw)
		assert.Contains(t, mustJSON(t, body), `"thinking"`)
	})

	t.Run("lkeap r1 reasons unconditionally and takes no toggle", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderLKEAP), "deepseek-r1", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Thinking: ptrBool(false)}, true)
		require.NoError(t, err)
		assert.False(t, useRaw)
		_, ok := body.(*openai.ChatCompletionRequest)
		assert.True(t, ok)
	})
}

// TestBuildOutbound_ParameterDispositions covers the vendors whose plugins
// forbid or pin a standard parameter.
func TestBuildOutbound_ParameterDispositions(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hi"}}

	t.Run("deepseek strips tool_choice", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderDeepSeek), "deepseek-chat", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{ToolChoice: "auto"}, false)
		require.NoError(t, err)
		assert.True(t, useRaw)
		request, ok := body.(map[string]any)
		require.True(t, ok)
		assert.NotContains(t, request, "tool_choice")
	})

	t.Run("moonshot pins temperature to 1", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderMoonshot), "moonshot-v1-8k", nil)
		body, _, _, err := c.buildOutbound(msgs, &ChatOptions{Temperature: 0.7, TopP: 0.9}, false)
		require.NoError(t, err)
		js := mustJSON(t, body)
		assert.Contains(t, js, `"temperature":1`)
	})

	t.Run("openai reasoning model drops sampling and renames the ceiling", func(t *testing.T) {
		c := newOutboundChat(t, string(provider.ProviderOpenAI), "o3-mini", nil)
		body, _, useRaw, err := c.buildOutbound(msgs, &ChatOptions{Temperature: 0.7, MaxTokens: 4096}, false)
		require.NoError(t, err)
		require.True(t, useRaw)
		request, ok := body.(map[string]any)
		require.True(t, ok)
		assert.NotContains(t, request, "temperature")
		assert.NotContains(t, request, "max_tokens")
		assert.EqualValues(t, 4096, request["max_completion_tokens"])
	})
}

func TestBuildOutbound_GeminiProviderMetadata(t *testing.T) {
	c := newOutboundChat(t, string(provider.ProviderGemini), "gemini-3-flash-preview", nil)
	messages := []Message{
		{Role: "user", Content: "find docs"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:               "call_1",
				Type:             "function",
				ProviderMetadata: types.ToolCallMetadata{"google": json.RawMessage(`{"thought_signature":"gemini-signature"}`)},
				Function: FunctionCall{
					Name:      "wiki_search",
					Arguments: `{"query":"MACS"}`,
				},
			}},
		},
	}

	body, _, useRaw, err := c.buildOutbound(messages, &ChatOptions{}, false)
	require.NoError(t, err)
	require.True(t, useRaw)

	js := mustJSON(t, body)
	assert.Contains(t, js, `"extra_content"`)
	assert.Contains(t, js, `"thought_signature":"gemini-signature"`)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
