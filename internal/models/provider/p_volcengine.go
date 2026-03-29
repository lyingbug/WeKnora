package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
	openai "github.com/sashabaranov/go-openai"
)

const (
	// VolcengineChatBaseURL 火山引擎 Ark Chat API BaseURL (OpenAI 兼容模式)
	VolcengineChatBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	// VolcengineEmbeddingBaseURL 火山引擎 Ark Multimodal Embedding API BaseURL
	VolcengineEmbeddingBaseURL = "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"
)

// VolcengineProvider 实现火山引擎 Ark 的 Provider 接口
type VolcengineProvider struct{}

// Info 返回火山引擎 provider 的元数据
func (p *VolcengineProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderVolcengine,
		DisplayName: "火山引擎 Volcengine",
		Description: "doubao-seed-2.0-pro, doubao-1-5-pro-32k, doubao-embedding-vision, etc.",
		DocURL:      "https://www.volcengine.com/docs/82379/2106519",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: VolcengineChatBaseURL,
			types.ModelTypeEmbedding:   VolcengineEmbeddingBaseURL,
			types.ModelTypeVLLM:        VolcengineChatBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"volces.com", "volcengine"},
		Models: []ModelEntry{
			{ID: "doubao-seed-2.0-pro", DisplayName: "Doubao Seed 2.0 Pro", ModelType: types.ModelTypeKnowledgeQA, Description: "Flagship model for complex reasoning and agent tasks"},
			{ID: "doubao-seed-2.0-lite", DisplayName: "Doubao Seed 2.0 Lite", ModelType: types.ModelTypeKnowledgeQA, Description: "Cost-effective model with balanced performance"},
			{ID: "doubao-1-5-pro-32k-250115", DisplayName: "Doubao 1.5 Pro 32K", ModelType: types.ModelTypeKnowledgeQA, Description: "Doubao 1.5 pro chat model"},
			{ID: "doubao-embedding-vision-250615", DisplayName: "Doubao Embedding Vision", ModelType: types.ModelTypeEmbedding, Description: "Multimodal embedding model"},
		},
	}
}

// ValidateConfig 验证火山引擎 provider 配置
func (p *VolcengineProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}

// Adapter returns the provider-specific adapter rules for Volcengine.
func (p *VolcengineProvider) Adapter() *ProviderAdapter {
	return &ProviderAdapter{
		Chat: []ChatAdaptRule{
			{
				ModelMatcher:      func(modelName string) bool { return true },
				RequestCustomizer: volcengineRequestCustomizer,
			},
		},
		Embedding: &EmbeddingAdaptRule{
			IsMultimodal: true,
		},
	}
}

// --- Volcengine Chat adapter functions ---

// volcengineRequestCustomizer controls the thinking parameter for Volcengine Ark.
// Format is the same as LKEAP: { "thinking": { "type": "enabled"/"disabled" } }
func volcengineRequestCustomizer(
	req *openai.ChatCompletionRequest, opts any, _ bool,
) (any, bool) {
	accessor, ok := opts.(ChatOptsAccessor)
	if !ok || accessor.GetThinking() == nil {
		return nil, false
	}

	vcReq := thinkingChatCompletionRequest{
		ChatCompletionRequest: *req,
	}

	thinkingType := "disabled"
	if *accessor.GetThinking() {
		thinkingType = "enabled"
	}
	vcReq.Thinking = &thinkingConfig{Type: thinkingType}

	return vcReq, true
}
