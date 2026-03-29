package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// MimoBaseURL 小米 Mimo API BaseURL
	MimoBaseURL = "https://api.xiaomimimo.com/v1"
)

// MimoProvider 实现小米 Mimo 的 Provider 接口
type MimoProvider struct{ BaseProvider }

// Info 返回小米 Mimo provider 的元数据
func (p *MimoProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderMimo,
		DisplayName: "小米 MiMo",
		Description: "mimo-v2-pro, mimo-v2-flash, mimo-v2-omni, etc.",
		DocURL:      "https://www.mimo-v2.com/zh/docs",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: MimoBaseURL,
			types.ModelTypeVLLM:        MimoBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"xiaomimimo.com"},
		Models: []ModelEntry{
			{ID: "mimo-v2-pro", DisplayName: "MiMo V2 Pro", ModelType: types.ModelTypeKnowledgeQA, Description: "Flagship model with 1M context, deep thinking & tool calling"},
			{ID: "mimo-v2-flash", DisplayName: "MiMo V2 Flash", ModelType: types.ModelTypeKnowledgeQA, Description: "Fast and efficient reasoning model"},
			{ID: "mimo-v2-omni", DisplayName: "MiMo V2 Omni", ModelType: types.ModelTypeVLLM, Description: "Multimodal model with vision and audio understanding"},
		},
	}
}

// ValidateConfig 验证小米 Mimo provider 配置
func (p *MimoProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}
