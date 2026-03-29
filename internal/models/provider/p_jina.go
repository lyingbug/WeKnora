package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	JinaBaseURL = "https://api.jina.ai/v1"
)

// JinaProvider 实现 Jina AI 的 Provider 接口
type JinaProvider struct{}

// Info 返回 Jina AI provider 的元数据
func (p *JinaProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderJina,
		DisplayName: "Jina",
		Description: "jina-embeddings-v4, jina-reranker-v3, etc.",
		DocURL:      "https://jina.ai/models",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeEmbedding: JinaBaseURL,
			types.ModelTypeRerank:    JinaBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"api.jina.ai"},
		Models: []ModelEntry{
			{ID: "jina-embeddings-v4", DisplayName: "Jina Embeddings V4", ModelType: types.ModelTypeEmbedding, Description: "Universal multimodal embedding model (3.8B)"},
			{ID: "jina-clip-v1", DisplayName: "Jina CLIP v1", ModelType: types.ModelTypeEmbedding, Description: "Multimodal CLIP embedding model"},
			{ID: "jina-reranker-v3", DisplayName: "Jina Reranker V3", ModelType: types.ModelTypeRerank, Description: "Multilingual reranker with 131K context (0.6B)"},
			{ID: "jina-reranker-v2-base-multilingual", DisplayName: "Jina Reranker V2 Base", ModelType: types.ModelTypeRerank, Description: "Multilingual reranking model"},
		},
	}
}

// ValidateConfig 验证 Jina AI provider 配置
func (p *JinaProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, false)
}

// Adapter returns the provider-specific adapter rules for Jina.
func (p *JinaProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Embedding: &EmbeddingAdaptRule{
			BuildRequest: jinaBuildEmbeddingRequest,
		},
		Rerank: &RerankAdaptRule{
			UseFullURL:    false, // uses baseURL + "/rerank"
			BuildRequest:  jinaBuildRerankRequest,
			ParseResponse: jinaParseRerankResponse,
		},
	}
}

// --- Jina Rerank adapter functions ---

func jinaBuildRerankRequest(model, query string, docs []string) (any, error) {
	return map[string]any{
		"model":            model,
		"query":            query,
		"documents":        docs,
		"top_n":            len(docs),
		"return_documents": true,
	}, nil
}

func jinaParseRerankResponse(body []byte, _ []string) ([]RerankResult, error) {
	var resp struct {
		Results []struct {
			Index          int             `json:"index"`
			RelevanceScore *float64        `json:"relevance_score"`
			Score          *float64        `json:"score"`
			Document       json.RawMessage `json:"document"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal jina rerank response: %w", err)
	}
	results := make([]RerankResult, len(resp.Results))
	for i, r := range resp.Results {
		// Parse document: can be string or {"text": "..."}
		var docText string
		if err := json.Unmarshal(r.Document, &docText); err != nil {
			var docObj struct {
				Text string `json:"text"`
			}
			if err2 := json.Unmarshal(r.Document, &docObj); err2 != nil {
				return nil, fmt.Errorf("unmarshal jina document: %w", err2)
			}
			docText = docObj.Text
		}

		score := 0.0
		if r.RelevanceScore != nil {
			score = *r.RelevanceScore
		} else if r.Score != nil {
			score = *r.Score
		}

		results[i] = RerankResult{
			Index:          r.Index,
			DocumentText:   docText,
			RelevanceScore: score,
		}
	}
	return results, nil
}

// --- Jina Embedding adapter functions ---

func jinaBuildEmbeddingRequest(_ context.Context, model string, texts []string, dims, _ int) (any, error) {
	req := map[string]any{
		"model":    model,
		"input":    texts,
		"truncate": true,
	}
	if dims > 0 {
		req["dimensions"] = dims
	}
	return req, nil
}
