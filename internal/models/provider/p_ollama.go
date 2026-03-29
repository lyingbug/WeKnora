package provider

import (
	"github.com/Tencent/WeKnora/internal/types"
)

// OllamaProvider implements the Provider interface for local Ollama models.
type OllamaProvider struct{ BaseProvider }

func (p *OllamaProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:        ProviderOllama,
		DisplayName: "Ollama (本地模型)",
		Description: "Run open-source models locally via Ollama",
		DocURL:      "https://ollama.com/library",
		DefaultURLs: map[types.ModelType]string{
			types.ModelTypeKnowledgeQA: "http://localhost:11434",
			types.ModelTypeEmbedding:   "http://localhost:11434",
			types.ModelTypeVLLM:        "http://localhost:11434",
		},
		ModelTypes: []types.ModelType{
			types.ModelTypeKnowledgeQA,
			types.ModelTypeEmbedding,
			types.ModelTypeVLLM,
		},
		RequiresAuth: false,
		URLPatterns:  []string{"localhost:11434", "host.docker.internal:11434"},
		Models: []ModelEntry{
			{ID: "qwen3:32b", DisplayName: "Qwen3 32B", ModelType: types.ModelTypeKnowledgeQA, Description: "Qwen3 32B local model", Tags: []string{"thinking", "tool-calling"}},
			{ID: "llama3.3:latest", DisplayName: "Llama 3.3", ModelType: types.ModelTypeKnowledgeQA, Description: "Meta Llama 3.3"},
			{ID: "deepseek-r1:32b", DisplayName: "DeepSeek R1 32B", ModelType: types.ModelTypeKnowledgeQA, Description: "DeepSeek R1 reasoning model", Tags: []string{"thinking"}},
			{ID: "nomic-embed-text", DisplayName: "Nomic Embed Text", ModelType: types.ModelTypeEmbedding, Description: "Text embedding model for Ollama"},
			{ID: "llava:latest", DisplayName: "LLaVA", ModelType: types.ModelTypeVLLM, Description: "Vision-language model"},
		},
	}
}

func (p *OllamaProvider) ValidateConfig(config *Config) error {
	return validateRequired(config, false, false, true)
}
