package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	SiliconFlowBaseURL = "https://api.siliconflow.cn/v1"
)

// SiliconFlowProvider 实现硅基流动的 Provider 接口
type SiliconFlowProvider struct{ BaseProvider }

// Info 返回硅基流动 provider 的元数据
func (p *SiliconFlowProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderSiliconFlow,
		DisplayName: "硅基流动 SiliconFlow",
		Description: "deepseek-ai/DeepSeek-V3.1, BAAI/bge-m3, BAAI/bge-reranker-v2-m3, etc.",
		DocURL:      "https://docs.siliconflow.cn/quickstart/models",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: SiliconFlowBaseURL,
			types.ModelTypeEmbedding:   SiliconFlowBaseURL,
			types.ModelTypeRerank:      SiliconFlowBaseURL,
			types.ModelTypeVLLM:        SiliconFlowBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"siliconflow.cn"},
		Models: []ModelEntry{
			{ID: "deepseek-ai/DeepSeek-V3.1", DisplayName: "DeepSeek V3.1", ModelType: types.ModelTypeKnowledgeQA, Description: "DeepSeek V3.1 via SiliconFlow"},
			{ID: "Qwen/Qwen3-32B", DisplayName: "Qwen3-32B", ModelType: types.ModelTypeKnowledgeQA, Description: "Qwen3 32B via SiliconFlow"},
			{ID: "BAAI/bge-m3", DisplayName: "BGE-M3", ModelType: types.ModelTypeEmbedding, Description: "Multilingual embedding model"},
			{ID: "BAAI/bge-reranker-v2-m3", DisplayName: "BGE Reranker V2 M3", ModelType: types.ModelTypeRerank, Description: "Multilingual reranking model"},
		},
	}
}

// ValidateConfig 验证硅基流动 provider 配置
func (p *SiliconFlowProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, false)
}
