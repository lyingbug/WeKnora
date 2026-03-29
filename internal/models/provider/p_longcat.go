package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	LongCatBaseURL = "https://api.longcat.chat/openai/v1"
)

// LongCatProvider 实现 LongCat AI 的 Provider 接口
type LongCatProvider struct{ BaseProvider }

// Info 返回 LongCat provider 的元数据
func (p *LongCatProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderLongCat,
		DisplayName: "LongCat AI",
		Description: "LongCat-Flash-Chat, LongCat-Flash-Thinking, etc.",
		DocURL:      "https://platform.longcat.chat/docs",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: LongCatBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"longcat.chat"},
		Models: []ModelEntry{
			{ID: "LongCat-Flash-Chat", DisplayName: "LongCat Flash Chat", ModelType: types.ModelTypeKnowledgeQA, Description: "Fast chat model"},
			{ID: "LongCat-Flash-Thinking", DisplayName: "LongCat Flash Thinking", ModelType: types.ModelTypeKnowledgeQA, Description: "Thinking-enabled chat model"},
		},
	}
}

// ValidateConfig 验证 LongCat provider 配置
func (p *LongCatProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, true, true)
}
