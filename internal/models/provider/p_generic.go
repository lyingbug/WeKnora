package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
	openai "github.com/sashabaranov/go-openai"
)

// GenericProvider 实现通用 OpenAI 兼容的 Provider 接口
type GenericProvider struct{}

// Info 返回通用 provider 的元数据
func (p *GenericProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderGeneric,
		DisplayName: "自定义 (OpenAI兼容接口)",
		Description: "Generic API endpoint (OpenAI-compatible)",
		DefaultURLs: map[types.ModelType]string{}, // 需要用户自行配置填写
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: false, // 可能需要也可能不需要
		URLPatterns:  nil,   // empty - fallback provider
	}
}

// ValidateConfig 验证通用 provider 配置
func (p *GenericProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, false, true)
}

// Adapter returns the provider-specific adapter rules for Generic.
func (p *GenericProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Chat: []ChatAdaptRule{
			{
				ModelMatcher:      func(modelName string) bool { return true },
				RequestCustomizer: genericRequestCustomizer,
			},
		},
	}
}

// --- Generic Chat adapter functions ---

// genericRequestCustomizer sets ChatTemplateKwargs for thinking control (e.g., vLLM).
func genericRequestCustomizer(
	req *openai.ChatCompletionRequest, opts any, _ bool,
) (any, bool) {
	thinking := false
	if accessor, ok := opts.(ChatOptsAccessor); ok && accessor.GetThinking() != nil {
		thinking = *accessor.GetThinking()
	}
	req.ChatTemplateKwargs = map[string]interface{}{
		"enable_thinking": thinking,
	}
	return req, true
}
