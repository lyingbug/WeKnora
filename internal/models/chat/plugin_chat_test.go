package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the plugin client end to end against a stub server, so
// they cover what a unit test of either half would miss: that the descriptor's
// auth and parameter dispositions, the protocol driver's body, and the HTTP
// layer agree on one request.

// capture records the request a client sends and replies with a canned body.
type capture struct {
	body    map[string]any
	headers http.Header
	path    string
}

func stubServer(t *testing.T, reply string, contentType string) (*httptest.Server, *capture) {
	t.Helper()
	// The client validates every outbound URL against the SSRF guard, which
	// blocks loopback by default; the stub server lives there.
	t.Setenv("SSRF_WHITELIST", "127.0.0.1")
	got := &capture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if len(raw) > 0 {
			require.NoError(t, json.Unmarshal(raw, &got.body))
		}
		got.headers = r.Header.Clone()
		got.path = r.URL.Path
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return server, got
}

func newPluginChat(t *testing.T, config *ChatConfig) *PluginChat {
	t.Helper()
	client, ok, err := NewPluginChat(config)
	require.NoError(t, err)
	require.True(t, ok, "expected a registered plugin for provider %q", config.Provider)
	return client
}

const anthropicReply = `{
  "content": [
    {"type": "thinking", "thinking": "27 * 453"},
    {"type": "text", "text": "12231"}
  ],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 10, "output_tokens": 4,
            "cache_read_input_tokens": 6, "cache_creation_input_tokens": 2}
}`

func TestPluginChat_AnthropicRequestShape(t *testing.T) {
	server, got := stubServer(t, anthropicReply, "application/json")

	client := newPluginChat(t, &ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   server.URL,
		ModelName: "claude-sonnet-4-5",
		APIKey:    "test-key",
		Provider:  "anthropic",
	})

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "27 * 453?"},
	}, &ChatOptions{Thinking: ptrBool(true), ThinkingBudget: 2000})
	require.NoError(t, err)

	// The system prompt is a top-level field here, not a message.
	assert.Equal(t, "be brief", got.body["system"])
	assert.Equal(t, "/v1/messages", got.path)
	assert.Equal(t, "test-key", got.headers.Get("x-api-key"))
	assert.Equal(t, "2023-06-01", got.headers.Get("anthropic-version"))
	assert.Empty(t, got.headers.Get("Authorization"), "the key must not also travel as a bearer token")

	thinking, ok := got.body["thinking"].(map[string]any)
	require.True(t, ok, "body: %v", got.body)
	assert.Equal(t, "enabled", thinking["type"])
	assert.EqualValues(t, 2000, thinking["budget_tokens"])
	// The budget must fit under the ceiling, which the plugin raises for it.
	assert.Greater(t, got.body["max_tokens"], float64(2000))

	messages, ok := got.body["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 1)
	first := messages[0].(map[string]any)
	assert.Equal(t, "user", first["role"])
	blocks := first["content"].([]any)
	assert.Equal(t, "text", blocks[0].(map[string]any)["type"])

	assert.Equal(t, "12231", resp.Content)
	assert.Equal(t, "27 * 453", resp.ReasoningContent)
	assert.Equal(t, "end_turn", resp.FinishReason)
	// Cache counters are additional input tokens in this protocol, not a
	// subset of them.
	assert.Equal(t, 18, resp.Usage.PromptTokens)
	assert.Equal(t, 6, resp.Usage.CacheReadTokens)
	assert.True(t, resp.Usage.CacheReported)
}

// A tool result is a user-role block referencing its call, and a replayed
// thinking block must lead the assistant turn with its signature intact.
func TestPluginChat_AnthropicReplaysThinkingAndToolResults(t *testing.T) {
	server, got := stubServer(t, anthropicReply, "application/json")

	client := newPluginChat(t, &ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   server.URL,
		ModelName: "claude-sonnet-4-5",
		APIKey:    "k",
		Provider:  "anthropic",
	})

	_, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "weather?"},
		{
			Role:               "assistant",
			ReasoningContent:   "need the tool",
			ReasoningSignature: "sig-abc",
			ToolCalls: []ToolCall{{
				ID:       "call_1",
				Type:     "function",
				Function: FunctionCall{Name: "get_weather", Arguments: `{"city":"SF"}`},
			}},
		},
		{Role: "tool", ToolCallID: "call_1", Content: "sunny"},
	}, &ChatOptions{})
	require.NoError(t, err)

	messages := got.body["messages"].([]any)
	require.Len(t, messages, 3)

	assistant := messages[1].(map[string]any)
	assert.Equal(t, "assistant", assistant["role"])
	blocks := assistant["content"].([]any)
	lead := blocks[0].(map[string]any)
	assert.Equal(t, "thinking", lead["type"], "thinking must lead the turn")
	assert.Equal(t, "sig-abc", lead["signature"], "the signature must return unmodified")
	assert.Equal(t, "tool_use", blocks[1].(map[string]any)["type"])

	result := messages[2].(map[string]any)
	assert.Equal(t, "user", result["role"])
	block := result["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_result", block["type"])
	assert.Equal(t, "call_1", block["tool_use_id"])
}

// Users paste the base URL in several shapes; all of them must reach the same
// endpoint rather than a doubled or truncated path.
func TestPluginChat_AnthropicEndpointSpellings(t *testing.T) {
	for _, suffix := range []string{"", "/", "/v1", "/v1/messages"} {
		t.Run("base"+suffix, func(t *testing.T) {
			server, got := stubServer(t, anthropicReply, "application/json")
			client := newPluginChat(t, &ChatConfig{
				Source:    types.ModelSourceRemote,
				BaseURL:   server.URL + suffix,
				ModelName: "claude-sonnet-4-5",
				APIKey:    "k",
				Provider:  "anthropic",
			})
			_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, &ChatOptions{})
			require.NoError(t, err)
			assert.Equal(t, "/v1/messages", got.path)
		})
	}
}

func TestPluginChat_AnthropicStream(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":0}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"hmm"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":3}}`,
		``,
	}, "\n")
	server, got := stubServer(t, stream, "text/event-stream")

	client := newPluginChat(t, &ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   server.URL,
		ModelName: "claude-sonnet-4-5",
		APIKey:    "k",
		Provider:  "anthropic",
	})

	ch, err := client.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, &ChatOptions{})
	require.NoError(t, err)

	var thinking, answer strings.Builder
	var final types.StreamResponse
	for chunk := range ch {
		switch chunk.ResponseType {
		case types.ResponseTypeThinking:
			thinking.WriteString(chunk.Content)
		case types.ResponseTypeAnswer:
			answer.WriteString(chunk.Content)
			if chunk.Done {
				final = chunk
			}
		case types.ResponseTypeError:
			t.Fatalf("stream error: %s", chunk.Content)
		}
	}

	assert.Equal(t, true, got.body["stream"])
	assert.Equal(t, "hmm", thinking.String())
	assert.Equal(t, "hello", answer.String())
	assert.Equal(t, "end_turn", final.FinishReason)
	require.NotNil(t, final.Usage)
	assert.Equal(t, 3, final.Usage.CompletionTokens)
}

// The Responses protocol is reachable by pinning it on a vendor that offers
// more than one, which is the case the protocol selector exists for.
func TestPluginChat_ResponsesProtocol(t *testing.T) {
	reply := `{
      "status": "completed",
      "output": [
        {"type": "reasoning", "summary": [{"type": "summary_text", "text": "brief"}]},
        {"type": "message", "content": [{"type": "output_text", "text": "Paris"}]}
      ],
      "usage": {"input_tokens": 9, "output_tokens": 2, "total_tokens": 11}
    }`
	server, got := stubServer(t, reply, "application/json")

	client := newPluginChat(t, &ChatConfig{
		Source:      types.ModelSourceRemote,
		BaseURL:     server.URL,
		ModelName:   "gpt-5",
		APIKey:      "k",
		Provider:    "openai",
		ExtraConfig: map[string]string{ExtraConfigProtocol: string(spi.ProtocolOpenAIResponses)},
	})
	require.Equal(t, spi.ProtocolOpenAIResponses, client.Descriptor().Protocol)

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "capital of France?"},
	}, &ChatOptions{Thinking: ptrBool(true), ThinkingEffort: "low", MaxTokens: 300})
	require.NoError(t, err)

	assert.Equal(t, "/responses", got.path)
	assert.Equal(t, "be brief", got.body["instructions"], "the system prompt becomes instructions")
	assert.EqualValues(t, 300, got.body["max_output_tokens"])
	assert.NotContains(t, got.body, "max_tokens")
	reasoning := got.body["reasoning"].(map[string]any)
	assert.Equal(t, "low", reasoning["effort"])

	assert.Equal(t, "Paris", resp.Content)
	assert.Equal(t, "brief", resp.ReasoningContent)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Equal(t, 11, resp.Usage.TotalTokens)
}

// An unregistered provider must report "not resolved" rather than erroring, so
// the caller can fall back to the generic transport.
func TestNewPluginChat_UnregisteredProvider(t *testing.T) {
	client, ok, err := NewPluginChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   "https://example.com",
		ModelName: "x",
		Provider:  "not-a-registered-vendor",
	})
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, client)
}
