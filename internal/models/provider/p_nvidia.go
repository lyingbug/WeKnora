package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// NvidiaChatBaseURL NVIDIA Chat 的默认 BaseURL
	NvidiaChatBaseURL = "https://integrate.api.nvidia.com/v1/chat/completions"
	// NvidiaVLMBaseURL NVIDIA VLM 的默认 BaseURL
	NvidiaVLMBaseURL = "https://integrate.api.nvidia.com/v1"
	// NvidiaRerankBaseURL NVIDIA Rerank 的默认 BaseURL
	NvidiaRerankBaseURL = "https://ai.api.nvidia.com/v1/retrieval/nvidia/reranking"
)

// NvidiaProvider 实现NVIDIA AI 的 Provider 接口
type NvidiaProvider struct{}

// Info 返回NVIDIA provider 的元数据
func (p *NvidiaProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderNvidia,
		DisplayName: "NVIDIA",
		Description: "deepseek-ai-deepseek-v3_1, nv-embed-v1, nvidia/llama-3.2-nv-rerankqa-1b-v2, etc.",
		DocURL:      "https://docs.nvidia.com/nim/",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: NvidiaChatBaseURL,
			types.ModelTypeEmbedding:   NvidiaChatBaseURL,
			types.ModelTypeRerank:      NvidiaRerankBaseURL,
			types.ModelTypeVLLM:        NvidiaVLMBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"nvidia.com"},
		Models: []ModelEntry{
			{ID: "deepseek-ai-deepseek-v3_1", DisplayName: "DeepSeek V3.1", ModelType: types.ModelTypeKnowledgeQA, Description: "DeepSeek V3.1 via NVIDIA"},
			{ID: "nv-embed-v1", DisplayName: "NV-Embed V1", ModelType: types.ModelTypeEmbedding, Description: "NVIDIA embedding model"},
			{ID: "nvidia/llama-3.2-nv-rerankqa-1b-v2", DisplayName: "NV-RerankQA 1B V2", ModelType: types.ModelTypeRerank, Description: "Llama-based reranking model"},
		},
	}
}

// ValidateConfig 验证NVIDIA provider 配置
func (p *NvidiaProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}

// Adapter returns the provider-specific adapter rules for NVIDIA.
func (p *NvidiaProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Chat: []ChatAdaptRule{
			{
				ModelMatcher:       func(modelName string) bool { return true },
				EndpointCustomizer: nvidiaEndpointCustomizer,
			},
		},
		Embedding: &EmbeddingAdaptRule{
			BuildRequest: nvidiaBuildEmbeddingRequest,
		},
		Rerank: &RerankAdaptRule{
			UseFullURL:    true,
			BuildRequest:  nvidiaBuildRerankRequest,
			ParseResponse: nvidiaParseRerankResponse,
		},
	}
}

// --- NVIDIA Chat adapter functions ---

// nvidiaEndpointCustomizer uses the BaseURL as the complete endpoint.
func nvidiaEndpointCustomizer(baseURL string, _ string, _ bool) string {
	return baseURL
}

// --- NVIDIA Rerank adapter functions ---

func nvidiaBuildRerankRequest(model, query string, docs []string) (any, error) {
	passages := make([]map[string]string, len(docs))
	for i, d := range docs {
		passages[i] = map[string]string{"text": d}
	}
	return map[string]any{
		"model":    model,
		"query":    map[string]string{"text": query},
		"passages": passages,
	}, nil
}

func nvidiaParseRerankResponse(body []byte, docs []string) ([]RerankResult, error) {
	var resp struct {
		Rankings []struct {
			Index int     `json:"index"`
			Logit float64 `json:"logit"`
		} `json:"rankings"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal nvidia rerank response: %w", err)
	}
	results := make([]RerankResult, len(resp.Rankings))
	for i, r := range resp.Rankings {
		docText := ""
		if r.Index >= 0 && r.Index < len(docs) {
			docText = docs[r.Index]
		}
		results[i] = RerankResult{
			Index:          r.Index,
			DocumentText:   docText,
			RelevanceScore: r.Logit,
		}
	}
	return results, nil
}

// --- NVIDIA Embedding adapter functions ---

func nvidiaBuildEmbeddingRequest(ctx context.Context, model string, texts []string, _, _ int) (any, error) {
	inputType := "passage"
	if isQuery, _ := ctx.Value(types.EmbedQueryContextKey).(bool); isQuery {
		inputType = "query"
	}
	return map[string]any{
		"model":           model,
		"input":           texts,
		"encoding_format": "float",
		"input_type":      inputType,
	}, nil
}
