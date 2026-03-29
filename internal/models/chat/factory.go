package chat

import (
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/ollama"
)

// NewChat creates a chat instance based on the configuration.
// Uses provider-driven routing: Ollama provider → OllamaChat, all others → RemoteAPIChat.
// Backward compatible: Source="local" automatically maps to provider=ollama.
func NewChat(config *ChatConfig, ollamaService *ollama.OllamaService) (Chat, error) {
	providerName := provider.ResolveProviderWithSource(
		config.Provider, config.BaseURL, strings.ToLower(string(config.Source)))

	if provider.IsOllama(providerName) {
		return NewOllamaChat(config, ollamaService)
	}
	return newRemoteChatWithProvider(config, providerName)
}

// newRemoteChatWithProvider creates a remote chat with adapter rules from the resolved provider.
func newRemoteChatWithProvider(config *ChatConfig, providerName provider.ProviderName) (Chat, error) {
	remoteChat, err := NewRemoteAPIChat(config)
	if err != nil {
		return nil, fmt.Errorf("create remote chat: %w", err)
	}

	// Query provider adapter for chat rules
	if adapter := provider.GetAdapter(providerName); adapter != nil {
		for _, rule := range adapter.Chat {
			if rule.ModelMatcher == nil || rule.ModelMatcher(config.ModelName) {
				if rule.RequestCustomizer != nil {
					remoteChat.SetRequestCustomizer(rule.RequestCustomizer)
				}
				if rule.EndpointCustomizer != nil {
					remoteChat.SetEndpointCustomizer(rule.EndpointCustomizer)
				}
				break
			}
		}
	}

	return remoteChat, nil
}
