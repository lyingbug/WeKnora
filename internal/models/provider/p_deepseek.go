package provider

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	openai "github.com/sashabaranov/go-openai"
)

const (
	// DeepSeekBaseURL DeepSeek 官方 API BaseURL
	DeepSeekBaseURL = "https://api.deepseek.com/v1"
)

// DeepSeekProvider 实现 DeepSeek 的 Provider 接口
type DeepSeekProvider struct{}

// Info 返回 DeepSeek provider 的元数据
func (p *DeepSeekProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderDeepSeek,
		DisplayName: "DeepSeek",
		Description: "deepseek-chat, deepseek-reasoner, etc.",
		DocURL:      "https://api-docs.deepseek.com/zh-cn/",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: DeepSeekBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"api.deepseek.com"},
		Models: []ModelEntry{
			{ID: "deepseek-chat", DisplayName: "DeepSeek Chat (V3.2)", ModelType: types.ModelTypeKnowledgeQA, Description: "General chat model (upgraded to V3.2)"},
			{ID: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner (V3.2)", ModelType: types.ModelTypeKnowledgeQA, Description: "Reasoning model with chain-of-thought (V3.2)"},
			{ID: "deepseek-r1", DisplayName: "DeepSeek R1", ModelType: types.ModelTypeKnowledgeQA, Description: "Pure thinking model with automatic reasoning"},
		},
	}
}

// ValidateConfig 验证 DeepSeek provider 配置
func (p *DeepSeekProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}

// Adapter returns the provider-specific adapter rules for DeepSeek.
func (p *DeepSeekProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Chat: []ChatAdaptRule{
			{
				ModelMatcher:      func(modelName string) bool { return true },
				RequestCustomizer: deepseekRequestCustomizer,
			},
		},
	}
}

// --- DeepSeek Chat adapter functions ---

// deepseekRequestCustomizer removes tool_choice which DeepSeek doesn't support.
func deepseekRequestCustomizer(
	req *openai.ChatCompletionRequest, opts any, _ bool,
) (any, bool) {
	if accessor, ok := opts.(ChatOptsAccessor); ok && accessor.GetToolChoice() != "" {
		logger.Infof(context.Background(), "deepseek model, skip tool_choice")
		req.ToolChoice = nil
	}
	return nil, false
}
