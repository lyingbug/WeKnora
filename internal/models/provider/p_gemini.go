package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// GeminiBaseURL Google Gemini API BaseURL
	GeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	// GeminiOpenAICompatBaseURL Gemini OpenAI 兼容模式 BaseURL
	GeminiOpenAICompatBaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
)

// GeminiProvider 实现 Google Gemini 的 Provider 接口
type GeminiProvider struct{ BaseProvider }

// Info 返回 Gemini provider 的元数据
func (p *GeminiProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderGemini,
		DisplayName: "Google Gemini",
		Description: "gemini-2.5-pro, gemini-2.5-flash, gemini-3-flash-preview, etc.",
		DocURL:      "https://ai.google.dev/gemini-api/docs/models",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: GeminiOpenAICompatBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"generativelanguage.googleapis.com"},
		Models: []ModelEntry{
			{ID: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", ModelType: types.ModelTypeKnowledgeQA, Description: "Most advanced model for complex tasks with deep reasoning"},
			{ID: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", ModelType: types.ModelTypeKnowledgeQA, Description: "Best price-performance for low-latency tasks"},
			{ID: "gemini-3-flash-preview", DisplayName: "Gemini 3 Flash Preview", ModelType: types.ModelTypeKnowledgeQA, Description: "Frontier-class performance at reduced cost"},
			{ID: "gemini-3.1-flash-preview", DisplayName: "Gemini 3.1 Flash Preview", ModelType: types.ModelTypeKnowledgeQA, Description: "Latest fast Gemini model"},
		},
	}
}

// ValidateConfig 验证 Gemini provider 配置
func (p *GeminiProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}
