package embedding

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/ollama"
)

// NewEmbedder creates an embedder based on the configuration.
// Uses provider-driven routing: Ollama provider → OllamaEmbedder, all others → OpenAIEmbedder.
// Backward compatible: Source="local" automatically maps to provider=ollama.
func NewEmbedder(config Config, pooler EmbedderPooler, ollamaService *ollama.OllamaService) (Embedder, error) {
	providerName := provider.ResolveProviderWithSource(
		config.Provider, config.BaseURL, strings.ToLower(string(config.Source)))

	// Ollama (local) provider
	if provider.IsOllama(providerName) {
		return NewOllamaEmbedder(config.BaseURL,
			config.ModelName, config.TruncatePromptTokens, config.Dimensions, config.ModelID, pooler, ollamaService)
	}

	// Remote providers: check if the provider has an adapter with embedding rules
	var rule *provider.EmbeddingAdaptRule
	if adapter := provider.GetAdapter(providerName); adapter != nil {
		rule = adapter.Embedding
	}

	// If the rule indicates multimodal, check if the model name actually is multimodal
	if rule != nil && rule.IsMultimodal {
		if isMultimodalModel(config.ModelName) {
			return NewMultimodalEmbedder(providerName, config, pooler)
		}

		// Non-multimodal model on a multimodal provider (e.g. Aliyun text-embedding-v3):
		// Use OpenAI-compatible endpoint with no adapter rule
		if providerName == provider.ProviderAliyun {
			if config.BaseURL == "" || !strings.Contains(config.BaseURL, "/compatible-mode/") {
				config.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
			}
		}
		return NewOpenAIEmbedder(config, nil, pooler)
	}

	// Non-multimodal rule or no rule: use unified OpenAI embedder with optional adapter
	return NewOpenAIEmbedder(config, rule, pooler)
}

// isMultimodalModel checks if the model name indicates a multimodal embedding model.
func isMultimodalModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	return strings.Contains(lower, "vision") || strings.Contains(lower, "multimodal")
}
