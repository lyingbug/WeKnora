package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/httputil"
	"github.com/Tencent/WeKnora/internal/models/provider"
)

// OpenAIEmbedder implements text vectorization using OpenAI-compatible APIs.
// When an adapter rule is provided, it delegates request building and/or response
// parsing to provider-specific hooks.
type OpenAIEmbedder struct {
	apiKey               string
	baseURL              string
	modelName            string
	truncatePromptTokens int
	dimensions           int
	modelID              string
	client               *http.Client
	rule                 *provider.EmbeddingAdaptRule
	EmbedderPooler
}

// OpenAIEmbedRequest represents a standard OpenAI embedding request.
type OpenAIEmbedRequest struct {
	Model                string   `json:"model"`
	Input                []string `json:"input"`
	EncodingFormat       string   `json:"encoding_format,omitempty"`
	TruncatePromptTokens int      `json:"truncate_prompt_tokens,omitempty"`
}

// OpenAIEmbedResponse represents a standard OpenAI embedding response.
type OpenAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// NewOpenAIEmbedder creates a new OpenAI-compatible embedder with optional adapter rule.
func NewOpenAIEmbedder(config Config, rule *provider.EmbeddingAdaptRule, pooler EmbedderPooler) (*OpenAIEmbedder, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	truncateTokens := config.TruncatePromptTokens
	if truncateTokens == 0 {
		truncateTokens = 511
	}

	return &OpenAIEmbedder{
		apiKey:               config.APIKey,
		baseURL:              baseURL,
		modelName:            config.ModelName,
		truncatePromptTokens: truncateTokens,
		dimensions:           config.Dimensions,
		modelID:              config.ModelID,
		client:               httputil.TimedClient(httputil.DefaultTimeout),
		rule:                 rule,
		EmbedderPooler:       pooler,
	}, nil
}

// Embed converts a single text to a vector.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return embedSingle(ctx, text, e.BatchEmbed)
}

// BatchEmbed converts multiple texts to vectors in batch.
func (e *OpenAIEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	// Build request body
	var body any
	if e.rule != nil && e.rule.BuildRequest != nil {
		var err error
		body, err = e.rule.BuildRequest(ctx, e.modelName, texts, e.dimensions, e.truncatePromptTokens)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
	} else {
		body = &OpenAIEmbedRequest{
			Model:                e.modelName,
			Input:                texts,
			EncodingFormat:       "float",
			TruncatePromptTokens: e.truncatePromptTokens,
		}
	}

	// Determine endpoint URL
	endpointPath := "/embeddings"
	if e.rule != nil && e.rule.EndpointPath != "" {
		endpointPath = e.rule.EndpointPath
	}
	url := e.baseURL + endpointPath

	// Collect extra headers from the adapter rule
	var extraHeaders map[string]string
	if e.rule != nil {
		extraHeaders = e.rule.ExtraHeaders
	}

	logger.Debugf(ctx, "OpenAIEmbedder BatchEmbed: model=%s, url=%s, input_count=%d",
		e.modelName, url, len(texts))

	// Log input validation warnings
	for i, text := range texts {
		textLen := len(text)
		if textLen == 0 || textLen > 8192 {
			preview := text
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			logger.Errorf(ctx, "OpenAIEmbedder BatchEmbed input[%d]: INVALID length=%d (must be [1, 8192]), preview=%s",
				i, textLen, preview)
		}
	}

	// Send request with retry
	respBody, err := httputil.PostJSONWithRetry(ctx, e.client, url, e.apiKey, body, extraHeaders, httputil.DefaultMaxRetries)
	if err != nil {
		return nil, fmt.Errorf("embedding API request: %w", err)
	}

	// Parse response
	if e.rule != nil && e.rule.ParseResponse != nil {
		return e.rule.ParseResponse(respBody)
	}

	// Default: standard OpenAI response format
	var response OpenAIEmbedResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	embeddings := make([][]float32, 0, len(response.Data))
	for _, data := range response.Data {
		embeddings = append(embeddings, data.Embedding)
	}

	return embeddings, nil
}

// GetModelName returns the model name.
func (e *OpenAIEmbedder) GetModelName() string {
	return e.modelName
}

// GetDimensions returns the vector dimensions.
func (e *OpenAIEmbedder) GetDimensions() int {
	return e.dimensions
}

// GetModelID returns the model ID.
func (e *OpenAIEmbedder) GetModelID() string {
	return e.modelID
}
