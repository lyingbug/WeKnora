package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	QianfanBaseURL = "https://qianfan.baidubce.com/v2"
)

// QianfanProvider 实现百度千帆的 Provider 接口
type QianfanProvider struct{ BaseProvider }

// Info 返回百度千帆 provider 的元数据
func (p *QianfanProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderQianfan,
		DisplayName: "百度千帆 Baidu Cloud",
		Description: "ernie-5.0, ernie-5.0-thinking-latest, embedding-v1, bce-reranker-base, etc.",
		DocURL:      "https://cloud.baidu.com/doc/qianfan-docs/s/7m95lyy43",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: QianfanBaseURL,
			types.ModelTypeEmbedding:   QianfanBaseURL,
			types.ModelTypeRerank:      QianfanBaseURL,
			types.ModelTypeVLLM:        QianfanBaseURL,
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeRerank,
			types.ModelTypeVLLM,
		},
		RequiresAuth: true,
		URLPatterns:  []string{"qianfan.baidubce.com", "baidubce.com"},
		Models: []ModelEntry{
			{ID: "ernie-5.0", DisplayName: "ERNIE 5.0", ModelType: types.ModelTypeKnowledgeQA, Description: "Latest ERNIE flagship model with 128K context"},
			{ID: "ernie-5.0-thinking-latest", DisplayName: "ERNIE 5.0 Thinking", ModelType: types.ModelTypeKnowledgeQA, Description: "ERNIE thinking model with reasoning capabilities"},
			{ID: "ernie-4.5-turbo-128k", DisplayName: "ERNIE 4.5 Turbo 128K", ModelType: types.ModelTypeKnowledgeQA, Description: "Fast ERNIE model with 128K context"},
			{ID: "embedding-v1", DisplayName: "Embedding V1", ModelType: types.ModelTypeEmbedding, Description: "Baidu embedding model"},
			{ID: "bce-reranker-base", DisplayName: "BCE Reranker Base", ModelType: types.ModelTypeRerank, Description: "Baidu reranking model"},
		},
	}
}

// ValidateConfig 验证百度千帆 provider 配置
func (p *QianfanProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, true, true, true)
}
