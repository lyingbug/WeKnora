package provider

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	openai "github.com/sashabaranov/go-openai"
)

const (
	// LKEAPBaseURL 腾讯云知识引擎原子能力 (LKEAP) 兼容 OpenAI 协议的 BaseURL
	LKEAPBaseURL = "https://api.lkeap.cloud.tencent.com/v1"
)

// LKEAPProvider 实现腾讯云 LKEAP 的 Provider 接口
// 支持 DeepSeek-R1, DeepSeek-V3 系列模型，具备思维链能力
type LKEAPProvider struct{}

// Info 返回 LKEAP provider 的元数据
func (p *LKEAPProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderLKEAP,
		DisplayName: "腾讯云 LKEAP",
		Description: "DeepSeek-R1, DeepSeek-V3 系列模型，支持思维链",
		DocURL:      "https://cloud.tencent.com/document/product/1772",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: LKEAPBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"lkeap.cloud.tencent.com", "api.lkeap"},
		Models: []ModelEntry{
			{ID: "deepseek-r1", DisplayName: "DeepSeek R1", ModelType: types.ModelTypeKnowledgeQA, Description: "DeepSeek R1 reasoning model"},
			{ID: "deepseek-v3", DisplayName: "DeepSeek V3", ModelType: types.ModelTypeKnowledgeQA, Description: "DeepSeek V3 with thinking support"},
		},
	}
}

// ValidateConfig 验证 LKEAP provider 配置
func (p *LKEAPProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}

// Adapter returns the provider-specific adapter rules for LKEAP.
func (p *LKEAPProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Chat: []ChatAdaptRule{
			{
				ModelMatcher:      IsLKEAPDeepSeekV3Model,
				RequestCustomizer: lkeapRequestCustomizer,
			},
		},
	}
}

// IsLKEAPDeepSeekV3Model 检查是否为 DeepSeek V3.x 系列模型
// V3.x 系列支持通过 Thinking 参数控制思维链开关
func IsLKEAPDeepSeekV3Model(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "deepseek-v3")
}

// IsLKEAPDeepSeekR1Model 检查是否为 DeepSeek R1 系列模型
// R1 系列默认开启思维链
func IsLKEAPDeepSeekR1Model(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "deepseek-r1")
}

// IsLKEAPThinkingModel 检查是否为支持思维链的 LKEAP 模型
func IsLKEAPThinkingModel(modelName string) bool {
	return IsLKEAPDeepSeekR1Model(modelName) || IsLKEAPDeepSeekV3Model(modelName)
}

// --- LKEAP Chat adapter functions ---

// lkeapRequestCustomizer controls the thinking parameter for DeepSeek V3.x on LKEAP.
// Reference: https://cloud.tencent.com/document/product/1772/115963
func lkeapRequestCustomizer(
	req *openai.ChatCompletionRequest, opts any, _ bool,
) (any, bool) {
	accessor, ok := opts.(ChatOptsAccessor)
	if !ok || accessor.GetThinking() == nil {
		return nil, false
	}

	lkeapReq := thinkingChatCompletionRequest{
		ChatCompletionRequest: *req,
	}

	thinkingType := "disabled"
	if *accessor.GetThinking() {
		thinkingType = "enabled"
	}
	lkeapReq.Thinking = &thinkingConfig{Type: thinkingType}

	return lkeapReq, true
}
