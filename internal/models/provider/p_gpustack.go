package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// GPUStackBaseURL GPUStack API BaseURL (OpenAI 兼容模式)
	GPUStackBaseURL = "http://your_gpustack_server_url/v1-openai"
	// GPUStackRerankBaseURL GPUStack Rerank API 虽然兼容OpenAI，但路径不同 (/v1/rerank 而非 /v1-openai/rerank)
	GPUStackRerankBaseURL = "http://your_gpustack_server_url/v1"
)

// GPUStackProvider 实现 GPUStack 的 Provider 接口
type GPUStackProvider struct{ BaseProvider }

// Info 返回 GPUStack provider 的元数据
func (p *GPUStackProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderGPUStack,
		DisplayName: "GPUStack",
		Description: "Choose your deployed model on GPUStack",
		DocURL:      "https://docs.gpustack.ai",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: GPUStackBaseURL,
			types.ModelTypeEmbedding:   GPUStackBaseURL,
			types.ModelTypeRerank:      GPUStackRerankBaseURL,
			types.ModelTypeVLLM:        GPUStackBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"gpustack"},
	}
}

// ValidateConfig 验证 GPUStack provider 配置
func (p *GPUStackProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, true, true)
}
