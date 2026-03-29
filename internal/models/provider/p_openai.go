package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	OpenAIBaseURL = "https://api.openai.com/v1"
)

// OpenAIProvider 实现 OpenAI 的 Provider 接口
type OpenAIProvider struct{ BaseProvider }

// Info 返回 OpenAI provider 的元数据
func (p *OpenAIProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderOpenAI,
		DisplayName: "OpenAI",
		Description: "gpt-5.2, gpt-5-mini, etc.",
		DocURL:      "https://platform.openai.com/docs/api-reference",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: OpenAIBaseURL,
			types.ModelTypeEmbedding:   OpenAIBaseURL,
			types.ModelTypeRerank:      OpenAIBaseURL,
			types.ModelTypeVLLM:        OpenAIBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"api.openai.com"},
		Models: []ModelEntry{
			{ID: "gpt-5.2", DisplayName: "GPT-5.2", ModelType: types.ModelTypeKnowledgeQA, Description: "Most capable GPT model"},
			{ID: "gpt-5-mini", DisplayName: "GPT-5 Mini", ModelType: types.ModelTypeKnowledgeQA, Description: "Fast and efficient GPT model"},
			{ID: "gpt-4.1", DisplayName: "GPT-4.1", ModelType: types.ModelTypeKnowledgeQA, Description: "High-performance GPT model"},
			{ID: "gpt-4.1-mini", DisplayName: "GPT-4.1 Mini", ModelType: types.ModelTypeKnowledgeQA, Description: "Compact and cost-effective GPT model"},
			{ID: "gpt-4.1-nano", DisplayName: "GPT-4.1 Nano", ModelType: types.ModelTypeKnowledgeQA, Description: "Ultra-fast lightweight GPT model"},
			{ID: "text-embedding-3-large", DisplayName: "Text Embedding 3 Large", ModelType: types.ModelTypeEmbedding, Description: "Best quality embedding model"},
			{ID: "text-embedding-3-small", DisplayName: "Text Embedding 3 Small", ModelType: types.ModelTypeEmbedding, Description: "Efficient embedding model"},
		},
	}
}

// ValidateConfig 验证 OpenAI provider 配置
func (p *OpenAIProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}
