package rerank

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/httputil"
	"github.com/Tencent/WeKnora/internal/models/provider"
)

// OpenAIReranker implements a reranking system based on OpenAI-compatible models.
// When an adapter rule is provided, it delegates request building and response parsing
// to provider-specific hooks.
type OpenAIReranker struct {
	modelName string
	modelID   string
	apiKey    string
	baseURL   string
	client    *http.Client
	rule      *provider.RerankAdaptRule
}

// RerankRequest represents a standard OpenAI rerank request body.
type RerankRequest struct {
	Model                string   `json:"model"`
	Query                string   `json:"query"`
	Documents            []string `json:"documents"`
	TruncatePromptTokens int      `json:"truncate_prompt_tokens"`
}

// RerankResponse represents the standard OpenAI rerank response.
type RerankResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Usage   UsageInfo    `json:"usage"`
	Results []RankResult `json:"results"`
}

// UsageInfo contains information about token usage in the API request.
type UsageInfo struct {
	TotalTokens int `json:"total_tokens"`
}

// NewOpenAIReranker creates a new OpenAIReranker with optional adapter rule.
func NewOpenAIReranker(config *RerankerConfig, rule *provider.RerankAdaptRule) (*OpenAIReranker, error) {
	baseURL := "https://api.openai.com/v1"
	if config.BaseURL != "" {
		baseURL = config.BaseURL
	}

	return &OpenAIReranker{
		modelName: config.ModelName,
		modelID:   config.ModelID,
		apiKey:    config.APIKey,
		baseURL:   baseURL,
		client:    httputil.TimedClient(httputil.DefaultTimeout),
		rule:      rule,
	}, nil
}

// Rerank performs document reranking based on relevance to the query.
func (r *OpenAIReranker) Rerank(ctx context.Context, query string, documents []string) ([]RankResult, error) {
	// Build request body
	var body any
	if r.rule != nil && r.rule.BuildRequest != nil {
		var err error
		body, err = r.rule.BuildRequest(r.modelName, query, documents)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
	} else {
		body = &RerankRequest{
			Model:                r.modelName,
			Query:                query,
			Documents:            documents,
			TruncatePromptTokens: 511,
		}
	}

	// Determine endpoint URL
	url := fmt.Sprintf("%s/rerank", r.baseURL)
	if r.rule != nil && r.rule.UseFullURL {
		url = r.baseURL
	}

	logger.Debugf(ctx, "%s", buildRerankRequestDebug(r.modelName, url, query, documents))

	// Send request with retry
	respBody, err := httputil.PostJSONWithRetry(ctx, r.client, url, r.apiKey, body, nil, httputil.DefaultMaxRetries)
	if err != nil {
		return nil, fmt.Errorf("rerank API request: %w", err)
	}

	// Parse response
	if r.rule != nil && r.rule.ParseResponse != nil {
		providerResults, err := r.rule.ParseResponse(respBody, documents)
		if err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}
		results := make([]RankResult, len(providerResults))
		for i, pr := range providerResults {
			results[i] = RankResult{
				Index:          pr.Index,
				Document:       DocumentInfo{Text: pr.DocumentText},
				RelevanceScore: pr.RelevanceScore,
			}
		}
		return results, nil
	}

	// Default: standard OpenAI response format
	var response RerankResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return response.Results, nil
}

// GetModelName returns the name of the reranking model.
func (r *OpenAIReranker) GetModelName() string {
	return r.modelName
}

// GetModelID returns the unique identifier of the reranking model.
func (r *OpenAIReranker) GetModelID() string {
	return r.modelID
}
