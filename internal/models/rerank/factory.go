package rerank

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/models/provider"
)

// NewReranker creates a reranker based on the configuration.
// It resolves the provider (including source for Ollama compatibility),
// checks for adapter rules, and creates an OpenAIReranker
// with optional provider-specific request/response customization.
func NewReranker(config *RerankerConfig) (Reranker, error) {
	providerName := provider.ResolveProviderWithSource(
		config.Provider, config.BaseURL, strings.ToLower(string(config.Source)))

	var rule *provider.RerankAdaptRule
	if adapter := provider.GetAdapter(providerName); adapter != nil {
		rule = adapter.Rerank
	}

	return NewOpenAIReranker(config, rule)
}
