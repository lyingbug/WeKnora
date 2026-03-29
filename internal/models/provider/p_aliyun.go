package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	openai "github.com/sashabaranov/go-openai"
)

const (
	// AliyunChatBaseURL 阿里云 DashScope Chat/Embedding 的默认 BaseURL
	AliyunChatBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	// AliyunRerankBaseURL 阿里云 DashScope Rerank 的默认 BaseURL
	AliyunRerankBaseURL = "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"
)

// AliyunProvider 实现阿里云 DashScope 的 Provider 接口
type AliyunProvider struct{}

// Info 返回阿里云 provider 的元数据
func (p *AliyunProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderAliyun,
		DisplayName: "阿里云 DashScope",
		Description: "qwen-plus, tongyi-embedding-vision-plus, qwen3-rerank, etc.",
		DocURL:      "https://help.aliyun.com/zh/model-studio/getting-started/models",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: AliyunChatBaseURL,
			types.ModelTypeEmbedding:   AliyunChatBaseURL,
			types.ModelTypeRerank:      AliyunRerankBaseURL,
			types.ModelTypeVLLM:        AliyunChatBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"dashscope.aliyuncs.com"},
		Models: []ModelEntry{
			{ID: "qwen-max", DisplayName: "Qwen Max", ModelType: types.ModelTypeKnowledgeQA, Description: "Most capable Qwen model"},
			{ID: "qwen-plus", DisplayName: "Qwen Plus", ModelType: types.ModelTypeKnowledgeQA, Description: "Balanced Qwen model for general chat"},
			{ID: "qwen3-235b-a22b", DisplayName: "Qwen3-235B-A22B", ModelType: types.ModelTypeKnowledgeQA, Description: "Qwen3 MoE flagship model with thinking support"},
			{ID: "qwen3-32b", DisplayName: "Qwen3-32B", ModelType: types.ModelTypeKnowledgeQA, Description: "Qwen3 32B with thinking support"},
			{ID: "text-embedding-v3", DisplayName: "Text Embedding V3", ModelType: types.ModelTypeEmbedding, Description: "Latest text embedding model"},
			{ID: "tongyi-embedding-vision-plus", DisplayName: "Tongyi Embedding Vision Plus", ModelType: types.ModelTypeEmbedding, Description: "Multimodal embedding model"},
			{ID: "qwen3-rerank", DisplayName: "Qwen3 Rerank", ModelType: types.ModelTypeRerank, Description: "Qwen3 reranking model"},
		},
	}
}

// ValidateConfig 验证阿里云 provider 配置
func (p *AliyunProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}

// Adapter returns the provider-specific adapter rules for Aliyun.
func (p *AliyunProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Chat: []ChatAdaptRule{
			{
				ModelMatcher:      IsQwen3Model,
				RequestCustomizer: aliyunQwen3RequestCustomizer,
			},
		},
		Embedding: &EmbeddingAdaptRule{
			IsMultimodal: true,
		},
		Rerank: &RerankAdaptRule{
			UseFullURL:    true,
			BuildRequest:  aliyunBuildRerankRequest,
			ParseResponse: aliyunParseRerankResponse,
		},
	}
}

// IsQwen3Model 检查模型名是否为 Qwen3 模型
// Qwen3 模型需要特殊处理 enable_thinking 参数
func IsQwen3Model(modelName string) bool {
	return strings.HasPrefix(modelName, "qwen3-")
}

// IsDeepSeekModel 检查模型名是否为 DeepSeek 模型
// DeepSeek 模型不支持 tool_choice 参数
func IsDeepSeekModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "deepseek")
}

// --- Aliyun Rerank adapter functions ---

func aliyunBuildRerankRequest(model, query string, docs []string) (any, error) {
	return map[string]any{
		"model": model,
		"input": map[string]any{
			"query":     query,
			"documents": docs,
		},
		"parameters": map[string]any{
			"return_documents": true,
			"top_n":            len(docs),
		},
	}, nil
}

func aliyunParseRerankResponse(body []byte, _ []string) ([]RerankResult, error) {
	var resp struct {
		Output struct {
			Results []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       struct {
					Text string `json:"text"`
				} `json:"document"`
			} `json:"results"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal aliyun rerank response: %w", err)
	}
	results := make([]RerankResult, len(resp.Output.Results))
	for i, r := range resp.Output.Results {
		results[i] = RerankResult{
			Index:          r.Index,
			DocumentText:   r.Document.Text,
			RelevanceScore: r.RelevanceScore,
		}
	}
	return results, nil
}

// --- Aliyun Chat adapter functions ---

// qwenChatCompletionRequest is the Qwen-specific request structure with enable_thinking field.
type qwenChatCompletionRequest struct {
	openai.ChatCompletionRequest
	EnableThinking *bool `json:"enable_thinking,omitempty"`
}

// aliyunQwen3RequestCustomizer disables thinking for Qwen3 in non-stream mode.
func aliyunQwen3RequestCustomizer(
	req *openai.ChatCompletionRequest, _ any, isStream bool,
) (any, bool) {
	if !isStream {
		qwenReq := qwenChatCompletionRequest{
			ChatCompletionRequest: *req,
		}
		enableThinking := false
		qwenReq.EnableThinking = &enableThinking
		return qwenReq, true
	}
	return nil, false
}
