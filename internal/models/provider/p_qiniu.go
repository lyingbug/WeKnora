package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// QiniuBaseURL 七牛云 API BaseURL (OpenAI 兼容模式)
	QiniuBaseURL = "https://api.qnaigc.com/v1"
)

// QiniuProvider 实现七牛云的 Provider 接口
type QiniuProvider struct{ BaseProvider }

// Info 返回七牛云 provider 的元数据
func (p *QiniuProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderQiniu,
		DisplayName: "七牛云 Qiniu",
		Description: "deepseek/deepseek-v3.2-251201, z-ai/glm-4.7, etc.",
		DocURL:      "https://developer.qiniu.com/hub/22318/product-overview",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: QiniuBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"qiniuapi.com", "qiniu"},
		Models: []ModelEntry{
			{ID: "deepseek/deepseek-v3.2-251201", DisplayName: "DeepSeek V3.2", ModelType: types.ModelTypeKnowledgeQA, Description: "DeepSeek V3.2 via Qiniu"},
			{ID: "z-ai/glm-4.7", DisplayName: "GLM-4.7", ModelType: types.ModelTypeKnowledgeQA, Description: "GLM-4.7 via Qiniu"},
		},
	}
}

// ValidateConfig 验证七牛云 provider 配置
func (p *QiniuProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, true, true)
}
