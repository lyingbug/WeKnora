package provider

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
)

// ProviderAdapter holds adapter rules for chat, embedding, and rerank operations.
type ProviderAdapter struct {
	Chat      []ChatAdaptRule
	Embedding *EmbeddingAdaptRule
	Rerank    *RerankAdaptRule
}

// ChatOptsAccessor is an optional interface that the opts parameter
// passed to ChatAdaptRule.RequestCustomizer may implement.
// It exposes chat option fields that provider adapters commonly need.
type ChatOptsAccessor interface {
	GetThinking() *bool
	GetToolChoice() string
}

// ChatAdaptRule defines how to customize chat requests for a specific provider or model.
type ChatAdaptRule struct {
	// ModelMatcher returns true if this rule applies to the given model name.
	ModelMatcher func(modelName string) bool
	// RequestCustomizer modifies the request before sending. Returns (custom body, useCustomBody).
	RequestCustomizer func(req *openai.ChatCompletionRequest, opts any, isStream bool) (any, bool)
	// EndpointCustomizer returns a custom endpoint URL, or "" to use the default.
	EndpointCustomizer func(baseURL, modelID string, isStream bool) string
}

// EmbeddingAdaptRule defines how to build and parse embedding requests for non-OpenAI APIs.
type EmbeddingAdaptRule struct {
	// BuildRequest creates the provider-specific request body.
	// The context is passed for providers that need request-scoped information (e.g., input_type).
	BuildRequest func(ctx context.Context, model string, texts []string, dims, truncateTokens int) (any, error)
	// ParseResponse parses the provider-specific response body into embeddings.
	ParseResponse func(body []byte) ([][]float32, error)
	// EndpointPath is the API endpoint path (e.g., "/v1/embeddings").
	EndpointPath string
	// ExtraHeaders are additional HTTP headers to include in requests.
	ExtraHeaders map[string]string
	// IsMultimodal indicates whether the embedding model supports multimodal input.
	IsMultimodal bool
}

// RerankResult represents a single reranking result.
type RerankResult struct {
	Index          int
	DocumentText   string
	RelevanceScore float64
}

// RerankAdaptRule defines how to build and parse rerank requests for non-OpenAI APIs.
type RerankAdaptRule struct {
	// BuildRequest creates the provider-specific rerank request body.
	BuildRequest func(model, query string, docs []string) (any, error)
	// ParseResponse parses the provider-specific response body into rerank results.
	ParseResponse func(body []byte, docs []string) ([]RerankResult, error)
	// UseFullURL indicates whether BuildRequest returns a full URL (true) or a path to append (false).
	UseFullURL bool
}

// --- Shared chat adapter types ---

// thinkingConfig is the thinking configuration used by LKEAP, Volcengine, and similar providers.
// Format: { "type": "enabled" } or { "type": "disabled" }
type thinkingConfig struct {
	Type string `json:"type"` // "enabled" or "disabled"
}

// thinkingChatCompletionRequest extends the standard request with a thinking field.
// Used by LKEAP, Volcengine, and other providers that control thinking via this format.
type thinkingChatCompletionRequest struct {
	openai.ChatCompletionRequest
	Thinking *thinkingConfig `json:"thinking,omitempty"`
}
