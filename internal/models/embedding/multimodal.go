package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/httputil"
	"github.com/Tencent/WeKnora/internal/models/provider"
)

// --- Aliyun Multimodal Embedder ---

const (
	// AliyunMultimodalEmbeddingEndpoint is the DashScope multimodal embedding API endpoint.
	AliyunMultimodalEmbeddingEndpoint = "/api/v1/services/embeddings/multimodal-embedding/multimodal-embedding"
)

// AliyunMultimodalEmbedder implements multimodal text embedding via Aliyun DashScope API.
type AliyunMultimodalEmbedder struct {
	apiKey               string
	baseURL              string
	modelName            string
	truncatePromptTokens int
	dimensions           int
	modelID              string
	client               *http.Client
	EmbedderPooler
}

// aliyunEmbedRequest represents an Aliyun DashScope multimodal embedding request.
type aliyunEmbedRequest struct {
	Model string           `json:"model"`
	Input aliyunEmbedInput `json:"input"`
}

type aliyunEmbedInput struct {
	Contents []aliyunContent `json:"contents"`
}

type aliyunContent struct {
	Text string `json:"text,omitempty"`
}

// aliyunEmbedResponse represents an Aliyun DashScope multimodal embedding response.
type aliyunEmbedResponse struct {
	Output struct {
		Embeddings []struct {
			Embedding []float32 `json:"embedding"`
			TextIndex int       `json:"text_index"`
		} `json:"embeddings"`
	} `json:"output"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
}

func newAliyunMultimodalEmbedder(config Config, pooler EmbedderPooler) (*AliyunMultimodalEmbedder, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com"
	}
	// Remove trailing slash and compatible-mode path for multimodal API
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.Contains(baseURL, "/compatible-mode/v1") {
		baseURL = strings.Replace(baseURL, "/compatible-mode/v1", "", 1)
	}
	if strings.Contains(baseURL, "/compatible-mode") {
		baseURL = strings.Replace(baseURL, "/compatible-mode", "", 1)
	}

	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	truncateTokens := config.TruncatePromptTokens
	if truncateTokens == 0 {
		truncateTokens = 511
	}

	return &AliyunMultimodalEmbedder{
		apiKey:               config.APIKey,
		baseURL:              baseURL,
		modelName:            config.ModelName,
		truncatePromptTokens: truncateTokens,
		dimensions:           config.Dimensions,
		modelID:              config.ModelID,
		client:               httputil.TimedClient(httputil.DefaultTimeout),
		EmbedderPooler:       pooler,
	}, nil
}

// Embed converts a single text to a vector.
func (e *AliyunMultimodalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return embedSingle(ctx, text, e.BatchEmbed)
}

// BatchEmbed converts multiple texts to vectors in batch.
func (e *AliyunMultimodalEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	contents := make([]aliyunContent, 0, len(texts))
	for _, text := range texts {
		contents = append(contents, aliyunContent{Text: text})
	}

	reqBody := aliyunEmbedRequest{
		Model: e.modelName,
		Input: aliyunEmbedInput{Contents: contents},
	}

	url := e.baseURL + AliyunMultimodalEmbeddingEndpoint

	logger.Debugf(ctx, "AliyunMultimodalEmbedder BatchEmbed: model=%s, url=%s, input_count=%d",
		e.modelName, url, len(texts))

	respBody, err := httputil.PostJSONWithRetry(ctx, e.client, url, e.apiKey, reqBody, nil, httputil.DefaultMaxRetries)
	if err != nil {
		return nil, fmt.Errorf("aliyun embedding API request: %w", err)
	}

	var response aliyunEmbedResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Extract embedding vectors, preserving order by text_index
	embeddings := make([][]float32, len(texts))
	for _, emb := range response.Output.Embeddings {
		if emb.TextIndex >= 0 && emb.TextIndex < len(embeddings) {
			embeddings[emb.TextIndex] = emb.Embedding
		}
	}

	return embeddings, nil
}

func (e *AliyunMultimodalEmbedder) GetModelName() string { return e.modelName }
func (e *AliyunMultimodalEmbedder) GetDimensions() int   { return e.dimensions }
func (e *AliyunMultimodalEmbedder) GetModelID() string   { return e.modelID }

// --- Volcengine Multimodal Embedder ---

const (
	// VolcengineMultimodalEmbeddingPath is the Volcengine Ark multimodal embedding API path.
	VolcengineMultimodalEmbeddingPath = "/api/v3/embeddings/multimodal"
)

// VolcengineMultimodalEmbedder implements multimodal text embedding via Volcengine Ark API.
// Note: Volcengine multimodal API returns a single combined embedding per call,
// so BatchEmbed makes one API call per text.
type VolcengineMultimodalEmbedder struct {
	apiKey               string
	baseURL              string
	modelName            string
	truncatePromptTokens int
	dimensions           int
	modelID              string
	client               *http.Client
	EmbedderPooler
}

// volcengineEmbedRequest represents a Volcengine Ark multimodal embedding request.
type volcengineEmbedRequest struct {
	Model string                   `json:"model"`
	Input []volcengineInputContent `json:"input"`
}

type volcengineInputContent struct {
	Type     string              `json:"type"`
	Text     string              `json:"text,omitempty"`
	ImageURL *volcengineImageURL `json:"image_url,omitempty"`
}

type volcengineImageURL struct {
	URL string `json:"url"`
}

// volcengineEmbedResponse represents a Volcengine Ark multimodal embedding response.
// The multimodal API returns data as an object with a single embedding array.
type volcengineEmbedResponse struct {
	Object string `json:"object"`
	Data   struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func newVolcengineMultimodalEmbedder(config Config, pooler EmbedderPooler) (*VolcengineMultimodalEmbedder, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com"
	}
	// Normalize the base URL: strip trailing slash and known path suffixes
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.Contains(baseURL, "/embeddings/multimodal") {
		if idx := strings.Index(baseURL, "/api/"); idx != -1 {
			baseURL = baseURL[:idx]
		}
	} else if strings.HasSuffix(baseURL, "/api/v3") {
		baseURL = strings.TrimSuffix(baseURL, "/api/v3")
	}

	if config.ModelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	truncateTokens := config.TruncatePromptTokens
	if truncateTokens == 0 {
		truncateTokens = 511
	}

	return &VolcengineMultimodalEmbedder{
		apiKey:               config.APIKey,
		baseURL:              baseURL,
		modelName:            config.ModelName,
		truncatePromptTokens: truncateTokens,
		dimensions:           config.Dimensions,
		modelID:              config.ModelID,
		client:               httputil.TimedClient(httputil.DefaultTimeout),
		EmbedderPooler:       pooler,
	}, nil
}

// Embed converts a single text to a vector.
func (e *VolcengineMultimodalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return embedSingle(ctx, text, e.BatchEmbed)
}

// BatchEmbed converts multiple texts to vectors. Because Volcengine's multimodal API
// returns a single combined embedding for all inputs, we call the API once per text.
func (e *VolcengineMultimodalEmbedder) BatchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	url := e.baseURL + VolcengineMultimodalEmbeddingPath
	embeddings := make([][]float32, len(texts))

	for i, text := range texts {
		reqBody := volcengineEmbedRequest{
			Model: e.modelName,
			Input: []volcengineInputContent{
				{Type: "text", Text: text},
			},
		}

		logger.Debugf(ctx, "VolcengineMultimodalEmbedder BatchEmbed[%d/%d]: model=%s",
			i+1, len(texts), e.modelName)

		respBody, err := httputil.PostJSONWithRetry(ctx, e.client, url, e.apiKey, reqBody, nil, httputil.DefaultMaxRetries)
		if err != nil {
			return nil, fmt.Errorf("volcengine embedding API request: %w", err)
		}

		var response volcengineEmbedResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}

		embeddings[i] = response.Data.Embedding
	}

	return embeddings, nil
}

func (e *VolcengineMultimodalEmbedder) GetModelName() string { return e.modelName }
func (e *VolcengineMultimodalEmbedder) GetDimensions() int   { return e.dimensions }
func (e *VolcengineMultimodalEmbedder) GetModelID() string   { return e.modelID }

// --- Factory ---

// NewMultimodalEmbedder creates the appropriate multimodal embedder for the given provider.
func NewMultimodalEmbedder(providerName provider.ProviderName, config Config, pooler EmbedderPooler) (Embedder, error) {
	switch providerName {
	case provider.ProviderAliyun:
		return newAliyunMultimodalEmbedder(config, pooler)
	case provider.ProviderVolcengine:
		return newVolcengineMultimodalEmbedder(config, pooler)
	default:
		return nil, fmt.Errorf("no multimodal embedder for provider: %s", providerName)
	}
}
