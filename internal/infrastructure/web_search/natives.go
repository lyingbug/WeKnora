package web_search

import (
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
)

// Native implementations.
//
// A native plugin is the escape hatch for an integration a manifest genuinely
// cannot describe. DuckDuckGo is the honest example: it has no JSON search
// API on the free tier, so the provider parses an HTML page, and no amount of
// path mapping expresses that.
//
// The escape hatch is deliberately unattractive. A native plugin still needs a
// manifest, so it appears in the catalog and gets a settings form like any
// other, but adding one requires changing Go and shipping a release — which is
// exactly the cost the file format exists to remove. If a new provider is
// reaching for this, the first question is whether the declarative runtime is
// missing something that would serve every future provider too.

// registerNatives makes the compiled-in implementations available to manifests
// that reference them.
func registerNatives(reg *plugin.Registry) {
	reg.RegisterNative(Kind, "duckduckgo", func(_ *plugin.Manifest, cfg plugin.Config) (any, error) {
		return NewDuckDuckGoProvider(types.WebSearchProviderParameters{
			ProxyURL: cfg.String("proxy_url"),
		})
	})
	registerGoogle(reg)
}

// registerGoogle binds the Google provider, which uses the vendor's official
// Go client rather than a plain HTTP call.
func registerGoogle(reg *plugin.Registry) {
	reg.RegisterNative(Kind, "google", func(_ *plugin.Manifest, cfg plugin.Config) (any, error) {
		return NewGoogleProvider(types.WebSearchProviderParameters{
			APIKey:   cfg.String("api_key"),
			EngineID: cfg.String("engine_id"),
			ProxyURL: cfg.String("proxy_url"),
		})
	})
}
