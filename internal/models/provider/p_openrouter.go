package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
)

// OpenRouterProvider 实现 OpenRouter 的 Provider 接口
type OpenRouterProvider struct{ BaseProvider }

// Info 返回 OpenRouter provider 的元数据
func (p *OpenRouterProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderOpenRouter,
		DisplayName: "OpenRouter",
		Description: "openai/gpt-5.2-chat, google/gemini-3-flash-preview, etc.",
		DocURL:      "https://openrouter.ai/docs/overview/models",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: OpenRouterBaseURL,
			types.ModelTypeEmbedding:   OpenRouterBaseURL,
			types.ModelTypeVLLM:        OpenRouterBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"openrouter.ai"},
		Models: []ModelEntry{
			{ID: "openai/gpt-5.2-chat", DisplayName: "OpenAI GPT-5.2 Chat", ModelType: types.ModelTypeKnowledgeQA, Description: "GPT-5.2 via OpenRouter"},
			{ID: "google/gemini-3-flash-preview", DisplayName: "Google Gemini 3 Flash Preview", ModelType: types.ModelTypeKnowledgeQA, Description: "Gemini 3 Flash via OpenRouter"},
		},
	}
}

// ValidateConfig 验证 OpenRouter provider 配置
func (p *OpenRouterProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, false)
}
