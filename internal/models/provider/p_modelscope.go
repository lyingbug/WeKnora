package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// ModelScopeBaseURL ModelScope API BaseURL (OpenAI 兼容模式)
	ModelScopeBaseURL = "https://api-inference.modelscope.cn/v1"
)

// ModelScopeProvider 实现 ModelScope (魔搭) 的 Provider 接口
type ModelScopeProvider struct{ BaseProvider }

// Info 返回 ModelScope provider 的元数据
func (p *ModelScopeProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderModelScope,
		DisplayName: "魔搭 ModelScope",
		Description: "Qwen/Qwen3-8B, Qwen/Qwen3-Embedding-8B, etc.",
		DocURL:      "https://help.aliyun.com/zh/model-studio/getting-started/models",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: ModelScopeBaseURL,
			types.ModelTypeEmbedding:   ModelScopeBaseURL,
			types.ModelTypeVLLM:        ModelScopeBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"modelscope.cn"},
		Models: []ModelEntry{
			{ID: "Qwen/Qwen3-8B", DisplayName: "Qwen3-8B", ModelType: types.ModelTypeKnowledgeQA, Description: "Qwen3 8B chat model"},
			{ID: "Qwen/Qwen3-Embedding-8B", DisplayName: "Qwen3-Embedding-8B", ModelType: types.ModelTypeEmbedding, Description: "Qwen3 8B embedding model"},
		},
	}
}

// ValidateConfig 验证 ModelScope provider 配置
func (p *ModelScopeProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, true, true)
}
