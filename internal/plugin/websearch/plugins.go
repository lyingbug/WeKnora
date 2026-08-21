// Package websearch registers in-tree web search engines as plugins.
// Adding a new in-tree engine means a factory here plus an Entry in Bundle;
// container.go no longer lists providers.
package websearch

import (
	"github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
)

const prefix = "websearch."

// BundleName is the profile bundle that mounts built-in search engines.
const BundleName = "base"

type spec struct {
	id      string
	factory web_search.ProviderFactory
}

func builtins() []spec {
	return []spec{
		{"duckduckgo", web_search.NewDuckDuckGoProvider},
		{"google", web_search.NewGoogleProvider},
		{"bing", web_search.NewBingProvider},
		{"tavily", web_search.NewTavilyProvider},
		{"ollama", web_search.NewOllamaProvider},
		{"baidu", web_search.NewBaiduProvider},
		{"searxng", web_search.NewSearxngProvider},
		{"keenable", web_search.NewKeenableProvider},
		{"zhipu", web_search.NewZhipuProvider},
		{"exa", web_search.NewExaProvider},
		{"metaso", web_search.NewMetasoProvider},
	}
}

func init() {
	for _, s := range builtins() {
		id, factory := prefix+s.id, s.factory
		providerID := s.id
		plugin.Register(id, func(plugin.Config) (plugin.Plugin, error) {
			return providerPlugin(id, providerID, factory), nil
		})
	}
}

func providerPlugin(pluginID, providerID string, factory web_search.ProviderFactory) plugin.Plugin {
	return plugin.Func{
		ID:         pluginID,
		InjectKeys: []string{plugin.ServiceWebSearch},
		ApplyFn: func(ctx *plugin.Context) error {
			reg, err := plugin.Service[*web_search.Registry](ctx, plugin.ServiceWebSearch)
			if err != nil {
				return err
			}
			return ctx.Effect(func() plugin.Disposable {
				reg.Register(providerID, factory)
				return plugin.DisposeFunc(func() { reg.Unregister(providerID) })
			})
		},
	}
}

// Bundle returns the base bundle of in-tree web search plugins.
func Bundle() plugin.Bundle {
	entries := make([]plugin.Entry, 0, len(builtins()))
	for _, s := range builtins() {
		id := prefix + s.id
		entries = append(entries, plugin.Entry{ID: id, Plugin: id})
	}
	return plugin.Bundle{Name: BundleName, Entries: entries}
}

// Bundles is the default bundle map used by the WeKnora plugin host.
func Bundles() map[string]plugin.Bundle {
	b := Bundle()
	return map[string]plugin.Bundle{b.Name: b}
}

// DefaultProfile stacks the base bundle. YAML / env overlays patch it.
func DefaultProfile() plugin.Profile {
	return plugin.Profile{Name: "standard", Bundles: []string{BundleName}}
}
