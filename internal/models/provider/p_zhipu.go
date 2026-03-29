package provider

import (
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// ZhipuChatBaseURL 智谱 AI Chat 的默认 BaseURL
	ZhipuChatBaseURL = "https://open.bigmodel.cn/api/paas/v4"
	// ZhipuEmbeddingBaseURL 智谱 AI Embedding 的默认 BaseURL
	ZhipuEmbeddingBaseURL = "https://open.bigmodel.cn/api/paas/v4"
	// ZhipuRerankBaseURL 智谱 AI Rerank 的默认 BaseURL
	ZhipuRerankBaseURL = "https://open.bigmodel.cn/api/paas/v4/rerank"
)

// ZhipuProvider 实现智谱 AI 的 Provider 接口
type ZhipuProvider struct{}

// Info 返回智谱 AI provider 的元数据
func (p *ZhipuProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderZhipu,
		DisplayName: "智谱 BigModel",
		Description: "glm-5, glm-4.7, embedding-3, rerank, etc.",
		DocURL:      "https://docs.bigmodel.cn/cn/guide/start/model-overview",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: ZhipuChatBaseURL,
			types.ModelTypeEmbedding:   ZhipuEmbeddingBaseURL,
			types.ModelTypeRerank:      ZhipuRerankBaseURL,
			types.ModelTypeVLLM:        ZhipuChatBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"open.bigmodel.cn", "zhipu"},
		Models: []ModelEntry{
			{ID: "glm-5", DisplayName: "GLM-5", ModelType: types.ModelTypeKnowledgeQA, Description: "Latest flagship model with 200K context"},
			{ID: "glm-5-turbo", DisplayName: "GLM-5 Turbo", ModelType: types.ModelTypeKnowledgeQA, Description: "Enhanced flagship model"},
			{ID: "glm-4.7", DisplayName: "GLM-4.7", ModelType: types.ModelTypeKnowledgeQA, Description: "High-intelligence model with 200K context"},
			{ID: "glm-4.6v", DisplayName: "GLM-4.6V", ModelType: types.ModelTypeVLLM, Description: "Flagship vision reasoning model with tool calling"},
			{ID: "embedding-3", DisplayName: "Embedding-3", ModelType: types.ModelTypeEmbedding, Description: "Zhipu embedding model"},
			{ID: "rerank", DisplayName: "Rerank", ModelType: types.ModelTypeRerank, Description: "Zhipu reranking model"},
		},
	}
}

// ValidateConfig 验证智谱 AI provider 配置
func (p *ZhipuProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}

// Adapter returns the provider-specific adapter rules for Zhipu.
func (p *ZhipuProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Rerank: &RerankAdaptRule{
			UseFullURL:    true,
			BuildRequest:  zhipuBuildRerankRequest,
			ParseResponse: zhipuParseRerankResponse,
		},
	}
}

// --- Zhipu Rerank adapter functions ---

func zhipuBuildRerankRequest(model, query string, docs []string) (any, error) {
	return map[string]any{
		"model":             model,
		"query":             query,
		"documents":         docs,
		"top_n":             0,
		"return_documents":  true,
		"return_raw_scores": false,
	}, nil
}

func zhipuParseRerankResponse(body []byte, _ []string) ([]RerankResult, error) {
	var resp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
			Document       string  `json:"document"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal zhipu rerank response: %w", err)
	}
	results := make([]RerankResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = RerankResult{
			Index:          r.Index,
			DocumentText:   r.Document,
			RelevanceScore: r.RelevanceScore,
		}
	}
	return results, nil
}
