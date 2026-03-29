package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// NovitaOpenAIBaseURL Novita OpenAI-compatible API BaseURL
	NovitaOpenAIBaseURL = "https://api.novita.ai/openai/v1"
)

// NovitaProvider 实现 Novita AI 的 Provider 接口
type NovitaProvider struct{ BaseProvider }

// Info 返回 Novita provider 的元数据
func (p *NovitaProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderNovita,
		DisplayName: "Novita AI",
		Description: "moonshotai/kimi-k2.5, zai-org/glm-5, minimax/minimax-m2.7, qwen/qwen3-embedding-0.6b, etc.",
		DocURL:      "https://novita.ai/docs",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: NovitaOpenAIBaseURL,
			types.ModelTypeEmbedding:   NovitaOpenAIBaseURL,
			types.ModelTypeVLLM:        NovitaOpenAIBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"api.novita.ai", "novita.ai"},
		Models: []ModelEntry{
			{ID: "moonshotai/kimi-k2.5", DisplayName: "Kimi K2.5", ModelType: types.ModelTypeKnowledgeQA, Description: "Kimi K2.5 via Novita"},
			{ID: "zai-org/glm-5", DisplayName: "GLM-5", ModelType: types.ModelTypeKnowledgeQA, Description: "GLM-5 via Novita"},
			{ID: "minimax/minimax-m2.7", DisplayName: "MiniMax M2.7", ModelType: types.ModelTypeKnowledgeQA, Description: "MiniMax M2.7 via Novita"},
			{ID: "qwen/qwen3-embedding-0.6b", DisplayName: "Qwen3 Embedding 0.6B", ModelType: types.ModelTypeEmbedding, Description: "Qwen3 embedding via Novita"},
		},
	}
}

// ValidateConfig 验证 Novita provider 配置
func (p *NovitaProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}
