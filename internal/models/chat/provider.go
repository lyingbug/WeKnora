package chat

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/provider"
	modelutils "github.com/Tencent/WeKnora/internal/models/utils"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

// This file holds the transport-level provider behavior that a declarative
// descriptor cannot express: request signing, an endpoint that is not derived
// from the base URL, a message rewrite, and vendor state that must survive a
// tool-call round trip.
//
// Everything else that used to live here — which field carries the thinking
// toggle, which sampling parameters a reasoning model rejects, which model
// pins its temperature — is now declared in internal/models/llm/vendors and
// applied through the resolved plan. Those were the parts that had to be kept
// in sync with a frontend copy, and they no longer exist in two places.

// authCreds carries the credentials a providerAdapter needs to authenticate a
// raw HTTP request. APIKey covers the common Bearer / api-key cases; AppID and
// AppSecret are only used by signing providers (WeKnoraCloud).
type authCreds struct {
	APIKey    string
	AppID     string
	AppSecret string
}

// providerAdapter captures the transport-level behavior of an
// OpenAI-compatible chat backend. Every method has a default on baseProvider,
// so an adapter overrides only what genuinely differs.
type providerAdapter interface {
	// Name is the provider this adapter handles.
	Name() provider.ProviderName
	// Matches reports whether this adapter applies to the given model name.
	// Default: true.
	Matches(model string) bool
	// TransformMessages rewrites the converted messages (e.g. downgrading
	// multi-content to plain text). Default: identity.
	TransformMessages(msgs []openai.ChatCompletionMessage) []openai.ChatCompletionMessage
	// Endpoint overrides the request URL. Empty string means the standard
	// "<baseURL>/chat/completions" handled by the caller. Default: "".
	Endpoint(baseURL, modelID string, isStream bool) string
	// Auth sets authentication headers on a raw HTTP request. Default: Bearer.
	Auth(req *http.Request, creds authCreds, body []byte)
	// ForceRawHTTP forces the raw HTTP path even when the body is standard
	// (needed by providers that must sign the exact request bytes, or whose
	// response carries counters the SDK type cannot represent). Default: false.
	ForceRawHTTP() bool
	// ExtractToolCallMetadata captures provider-specific state from a raw
	// OpenAI-compatible tool_call object. Default: nil.
	ExtractToolCallMetadata(raw json.RawMessage) types.ToolCallMetadata
	// InjectToolCallMetadata writes provider-specific state back into an outbound
	// OpenAI-compatible tool_call object. Default: noop.
	InjectToolCallMetadata(toolCall map[string]any, metadata types.ToolCallMetadata)
}

// baseProvider supplies the default behavior for every providerAdapter method.
// It is also the fallback returned by resolveProvider for unknown providers:
// Bearer auth, standard endpoint, no request shaping.
type baseProvider struct{}

func (baseProvider) Name() provider.ProviderName { return "" }
func (baseProvider) Matches(string) bool         { return true }
func (baseProvider) TransformMessages(msgs []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	return msgs
}
func (baseProvider) Endpoint(string, string, bool) string { return "" }
func (baseProvider) Auth(req *http.Request, creds authCreds, _ []byte) {
	req.Header.Set("Authorization", "Bearer "+creds.APIKey)
}
func (baseProvider) ForceRawHTTP() bool { return false }
func (baseProvider) ExtractToolCallMetadata(json.RawMessage) types.ToolCallMetadata {
	return nil
}
func (baseProvider) InjectToolCallMetadata(map[string]any, types.ToolCallMetadata) {}

// --- WeKnoraCloud: custom endpoint + request signing + multi-content downgrade ---

type weKnoraCloudProvider struct{ baseProvider }

func (weKnoraCloudProvider) Name() provider.ProviderName { return provider.ProviderWeKnoraCloud }

func (weKnoraCloudProvider) Endpoint(baseURL, _ string, _ bool) string {
	return strings.TrimRight(baseURL, "/") + "/api/v1/chat/completions"
}

func (weKnoraCloudProvider) ForceRawHTTP() bool { return true }

func (weKnoraCloudProvider) Auth(req *http.Request, creds authCreds, body []byte) {
	requestID := uuid.NewString()
	headers := modelutils.Sign(creds.AppID, creds.AppSecret, requestID, string(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}

// TransformMessages downgrades MultiContent to plain text while preserving
// tool_calls / tool_call_id / name so the function-calling protocol keeps working.
func (weKnoraCloudProvider) TransformMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		msg := m
		if msg.Content == "" && len(msg.MultiContent) > 0 {
			var textParts []string
			for _, part := range msg.MultiContent {
				if part.Type == openai.ChatMessagePartTypeText && part.Text != "" {
					textParts = append(textParts, part.Text)
				}
			}
			msg.Content = strings.Join(textParts, "\n")
			msg.MultiContent = nil
		}
		result = append(result, msg)
	}
	return result
}

// --- DeepSeek: native cache counters the SDK response type cannot represent ---

type deepseekProvider struct{ baseProvider }

func (deepseekProvider) Name() provider.ProviderName { return provider.ProviderDeepSeek }

// Native DeepSeek cache counters are not represented by go-openai v1.41.2;
// use the raw path so prompt_cache_hit_tokens/miss_tokens remain observable.
func (deepseekProvider) ForceRawHTTP() bool { return true }

// --- Gemini OpenAI compatibility: tool thought signatures live in extra_content ---

type geminiProvider struct{ baseProvider }

func (geminiProvider) Name() provider.ProviderName { return provider.ProviderGemini }
func (geminiProvider) ForceRawHTTP() bool          { return true }
func (geminiProvider) ExtractToolCallMetadata(raw json.RawMessage) types.ToolCallMetadata {
	var tc struct {
		ExtraContent map[string]json.RawMessage `json:"extra_content,omitempty"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil
	}
	google, ok := tc.ExtraContent["google"]
	if !ok || len(google) == 0 {
		return nil
	}
	return types.ToolCallMetadata{"google": google}
}

func (geminiProvider) InjectToolCallMetadata(toolCall map[string]any, metadata types.ToolCallMetadata) {
	if len(metadata) == 0 {
		return
	}
	google, ok := metadata["google"]
	if !ok || len(google) == 0 {
		return
	}
	var googleValue any
	if err := json.Unmarshal(google, &googleValue); err != nil {
		return
	}
	toolCall["extra_content"] = map[string]any{"google": googleValue}
}

// --- Azure OpenAI: api-key auth ---

type azureProvider struct{ baseProvider }

func (azureProvider) Name() provider.ProviderName { return provider.ProviderAzureOpenAI }
func (azureProvider) Auth(req *http.Request, creds authCreds, _ []byte) {
	req.Header.Set("api-key", creds.APIKey)
}

// providerRegistry is ordered: more specific adapters (those with a real
// Matches predicate) must precede the generic catch-all for the same provider.
var providerRegistry = []providerAdapter{
	weKnoraCloudProvider{},
	deepseekProvider{},
	geminiProvider{},
	azureProvider{},
}

// resolveProvider returns the adapter handling the given provider+model, or
// baseProvider{} (Bearer auth, standard endpoint, no shaping) when none matches.
func resolveProvider(name provider.ProviderName, model string) providerAdapter {
	for _, p := range providerRegistry {
		if p.Name() == name && p.Matches(model) {
			return p
		}
	}
	return baseProvider{}
}
