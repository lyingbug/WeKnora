package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// MiniMaxBaseURL MiniMax 国际版 API BaseURL
	MiniMaxBaseURL = "https://api.minimax.io/v1"
	// MiniMaxCNBaseURL MiniMax 国内版 API BaseURL
	MiniMaxCNBaseURL = "https://api.minimaxi.com/v1"
)

// MiniMaxProvider 实现 MiniMax 的 Provider 接口
type MiniMaxProvider struct{ BaseProvider }

// Info 返回 MiniMax provider 的元数据
func (p *MiniMaxProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderMiniMax,
		DisplayName: "MiniMax",
		Description: "MiniMax-M2.7, MiniMax-M2.7-highspeed, MiniMax-M2.5, etc.",
		DocURL:      "https://platform.minimaxi.com/document/Announcement",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: MiniMaxCNBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"minimax.io", "minimaxi.com"},
		Models: []ModelEntry{
			{ID: "MiniMax-M2.7", DisplayName: "MiniMax M2.7", ModelType: types.ModelTypeKnowledgeQA, Description: "Latest MiniMax model"},
			{ID: "MiniMax-M2.7-highspeed", DisplayName: "MiniMax M2.7 Highspeed", ModelType: types.ModelTypeKnowledgeQA, Description: "Fast MiniMax model"},
			{ID: "MiniMax-M2.5", DisplayName: "MiniMax M2.5", ModelType: types.ModelTypeKnowledgeQA, Description: "Previous generation MiniMax model"},
		},
	}
}

// ValidateConfig 验证 MiniMax provider 配置
func (p *MiniMaxProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}
