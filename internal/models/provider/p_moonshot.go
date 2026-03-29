package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	MoonshotBaseURL = "https://api.moonshot.ai/v1"
)

// MoonshotProvider 实现 Moonshot AI (Kimi) 的 Provider 接口
type MoonshotProvider struct{ BaseProvider }

// Info 返回 Moonshot provider 的元数据
func (p *MoonshotProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderMoonshot,
		DisplayName: "月之暗面 Moonshot",
		Description: "kimi-k2.5, kimi-k2-0905-preview, etc.",
		DocURL:      "https://platform.moonshot.cn/docs",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: MoonshotBaseURL,
			types.ModelTypeVLLM:        MoonshotBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"moonshot.ai"},
		Models: []ModelEntry{
			{ID: "kimi-k2.5", DisplayName: "Kimi K2.5", ModelType: types.ModelTypeKnowledgeQA, Description: "Most intelligent Kimi model with vision, thinking & agent support"},
			{ID: "kimi-k2-0905-preview", DisplayName: "Kimi K2 0905 Preview", ModelType: types.ModelTypeKnowledgeQA, Description: "Enhanced agent coding capabilities"},
		},
	}
}

// ValidateConfig 验证 Moonshot provider 配置
func (p *MoonshotProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, true, true)
}
