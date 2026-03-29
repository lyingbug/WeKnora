package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// HunyuanBaseURL 腾讯混元 API BaseURL (OpenAI 兼容模式)
	HunyuanBaseURL = "https://api.hunyuan.cloud.tencent.com/v1"
)

// HunyuanProvider 实现腾讯混元的 Provider 接口
type HunyuanProvider struct{ BaseProvider }

// Info 返回腾讯混元 provider 的元数据
func (p *HunyuanProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderHunyuan,
		DisplayName: "腾讯混元 Hunyuan",
		Description: "hunyuan-t1-latest, hunyuan-turbos-latest, hunyuan-embedding, etc.",
		DocURL:      "https://cloud.tencent.com/document/product/1729/101848",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: HunyuanBaseURL,
			types.ModelTypeEmbedding:   HunyuanBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"hunyuan.cloud.tencent.com"},
		Models: []ModelEntry{
			{ID: "hunyuan-t1-latest", DisplayName: "Hunyuan T1", ModelType: types.ModelTypeKnowledgeQA, Description: "Hybrid-Transformer-Mamba reasoning model"},
			{ID: "hunyuan-turbos-latest", DisplayName: "Hunyuan TurboS", ModelType: types.ModelTypeKnowledgeQA, Description: "Flagship model with enhanced thinking"},
			{ID: "hunyuan-a13b", DisplayName: "Hunyuan A13B", ModelType: types.ModelTypeKnowledgeQA, Description: "80B total / 13B active MoE model"},
			{ID: "hunyuan-lite", DisplayName: "Hunyuan Lite", ModelType: types.ModelTypeKnowledgeQA, Description: "Lightweight MoE model with 256K context"},
			{ID: "hunyuan-embedding", DisplayName: "Hunyuan Embedding", ModelType: types.ModelTypeEmbedding, Description: "Hunyuan embedding model"},
		},
	}
}

// ValidateConfig 验证腾讯混元 provider 配置
func (p *HunyuanProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, true, true)
}
