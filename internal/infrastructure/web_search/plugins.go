package web_search

// Provider declarations.
//
// Each entry replaces a line in the dependency container's wiring list plus a
// hand-written credential check inside the constructor. They live together
// here while the domain has a fixed set of providers; a provider that grows
// its own package moves its declaration alongside its implementation, and
// nothing else changes because registration is by init rather than by a
// central list.

func init() {
	// --- Free providers: no credential at all. ---

	Register(Definition{
		ID:          "duckduckgo",
		DisplayName: "DuckDuckGo",
		New:         NewDuckDuckGoProvider,
	})

	// --- Self-hosted: an endpoint instead of a credential. ---

	Register(Definition{
		ID:          "searxng",
		DisplayName: "SearXNG",
		Fields:      []Field{BaseURLField(true)},
		Tags:        []string{"self-hosted"},
		New:         NewSearxngProvider,
		// A schema can require the URL but not check that it is absolute and
		// http(s); the provider already had that check, so Probe reuses it.
		Validate: func(params Params) error { return ValidateSearxngBaseURL(params.BaseURL) },
	})

	// --- Key-only providers. ---

	for _, p := range []struct {
		id, name, doc string
		new           Factory
	}{
		{"bing", "Bing Search", "https://learn.microsoft.com/bing/search-apis", NewBingProvider},
		{"tavily", "Tavily", "https://docs.tavily.com", NewTavilyProvider},
		{"baidu", "Baidu Search", "", NewBaiduProvider},
		{"keenable", "Keenable", "", NewKeenableProvider},
		{"ollama", "Ollama Web Search", "https://docs.ollama.com", NewOllamaProvider},
	} {
		Register(Definition{
			ID:          p.id,
			DisplayName: p.name,
			DocURL:      p.doc,
			Fields:      []Field{APIKeyField(true)},
			New:         p.new,
		})
	}

	// --- Providers with their own options. ---

	Register(Definition{
		ID:          "google",
		DisplayName: "Google Programmable Search",
		DocURL:      "https://developers.google.com/custom-search/v1/overview",
		Fields:      []Field{APIKeyField(true), EngineIDField()},
		New:         NewGoogleProvider,
	})

	Register(Definition{
		ID:          "exa",
		DisplayName: "Exa",
		DocURL:      "https://docs.exa.ai",
		Fields: []Field{
			APIKeyField(true),
			FlagField(FieldIncludeText, 10),
		},
		New: NewExaProvider,
	})

	Register(Definition{
		ID:          "zhipu",
		DisplayName: "Zhipu Web Search",
		DocURL:      "https://docs.bigmodel.cn/cn/guide/tools/web-search",
		Fields: []Field{
			APIKeyField(true),
			OptionField(FieldSearchEngine, 10, "search_std",
				"search_std", "search_pro", "search_pro_sogou", "search_pro_quark"),
			OptionField(FieldContentSize, 20, "medium", "medium", "high"),
		},
		New:      NewZhipuProvider,
		Validate: ValidateZhipuParameters,
	})

	Register(Definition{
		ID:          "metaso",
		DisplayName: "Metaso AI Search",
		Fields: []Field{
			APIKeyField(true),
			OptionField(FieldScope, 10, "webpage",
				"webpage", "document", "scholar", "podcast", "video", "image"),
		},
		New:      NewMetasoProvider,
		Validate: ValidateMetasoParameters,
	})
}
