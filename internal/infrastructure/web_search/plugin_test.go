package web_search

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
)

// These tests are about what the migration bought, not about the search
// providers themselves. Before it, a missing credential surfaced as an error
// from inside a constructor, provider-specific options were undocumented map
// keys, and the settings form was a separate list someone had to remember to
// update.

// A configuration is validated before a constructor runs, so a provider never
// has to repeat the check and a caller gets the same error shape everywhere.
func TestMissingCredentialIsRejectedBeforeTheProviderIsBuilt(t *testing.T) {
	_, err := Open(context.Background(), "tavily", types.WebSearchProviderParameters{})
	if err == nil {
		t.Fatal("a provider requiring an API key should refuse an empty configuration")
	}

	if _, err := Open(context.Background(), "tavily", types.WebSearchProviderParameters{
		APIKey: "tvly-test",
	}); err != nil {
		t.Fatalf("a complete configuration should build: %v", err)
	}
}

// DuckDuckGo needs no credential, and the schema is what says so.
func TestAFreeProviderNeedsNoCredential(t *testing.T) {
	if _, err := Open(context.Background(), "duckduckgo", types.WebSearchProviderParameters{}); err != nil {
		t.Fatalf("duckduckgo should build without configuration: %v", err)
	}
}

// Google needs two inputs, and the second one used to be discoverable only by
// reading the constructor.
func TestGoogleDeclaresBothInputs(t *testing.T) {
	if _, err := Open(context.Background(), "google", types.WebSearchProviderParameters{
		APIKey: "key",
	}); err == nil {
		t.Fatal("google should refuse a configuration without an engine id")
	}
	if _, err := Open(context.Background(), "google", types.WebSearchProviderParameters{
		APIKey: "key", EngineID: "cx",
	}); err != nil {
		t.Fatalf("google should build with both inputs: %v", err)
	}
}

// Provider-specific options were map keys with no vocabulary. Declaring them
// means a wrong value is refused locally rather than sent upstream.
func TestProviderOptionsAreValidatedAgainstTheirVocabulary(t *testing.T) {
	valid := types.WebSearchProviderParameters{
		APIKey:      "key",
		ExtraConfig: map[string]string{FieldSearchEngine: "search_pro"},
	}
	if _, err := Open(context.Background(), "zhipu", valid); err != nil {
		t.Fatalf("a documented search engine should be accepted: %v", err)
	}

	invalid := types.WebSearchProviderParameters{
		APIKey:      "key",
		ExtraConfig: map[string]string{FieldSearchEngine: "search_turbo"},
	}
	if _, err := Open(context.Background(), "zhipu", invalid); err == nil {
		t.Fatal("an undocumented search engine should be refused")
	}
}

// An option the caller omits still reaches the provider as its documented
// default, which is what the constructor used to do by hand.
func TestOptionDefaultsSurvive(t *testing.T) {
	p, ok := Plugins.Lookup("zhipu")
	if !ok {
		t.Fatal("zhipu should be registered")
	}
	cfg, err := p.Schema().Validate(map[string]any{FieldAPIKey: "key"})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	params := ParamsFromConfig(cfg)
	if got := params.ExtraConfig[FieldSearchEngine]; got != "search_std" {
		t.Errorf("search_engine default = %q, want search_std", got)
	}
	if got := params.ExtraConfig[FieldContentSize]; got != "medium" {
		t.Errorf("content_size default = %q, want medium", got)
	}
}

// Probe answers "would this work?" without building anything, and reuses the
// provider-specific check a schema cannot express.
//
// A probe may do real work — SearXNG's validator resolves the host to reject
// an SSRF target — which is exactly why it is separate from schema validation
// rather than folded into it. The assertions here stay on the two answers that
// need no network, because a probe that reaches the network is the caller's
// choice to make, not something a unit test should depend on.
func TestProbeReportsWhySelfHostedConfigurationFails(t *testing.T) {
	missing := Probe(context.Background(), "searxng", types.WebSearchProviderParameters{})
	if missing.State != plugin.Unavailable {
		t.Errorf("an empty configuration should probe unavailable, got %+v", missing)
	}

	malformed := Probe(context.Background(), "searxng", types.WebSearchProviderParameters{
		BaseURL: "not-a-url",
	})
	if malformed.State != plugin.Unavailable || malformed.Detail == "" {
		t.Errorf("a relative URL should probe unavailable with a reason, got %+v", malformed)
	}

	// The point of a reported reason is that it distinguishes the failures.
	// "You left it blank" and "that is not a URL" are different problems, and
	// a settings page needs to say which one it is.
	if missing.Detail == malformed.Detail {
		t.Errorf("both failures reported the same reason: %q", missing.Detail)
	}
}

// The credential must not travel back out through the catalog, which is what
// a settings page reads.
func TestCredentialsAreNotExposedThroughTheCatalog(t *testing.T) {
	for _, entry := range plugin.DefaultCatalog.Entries(Kind) {
		for _, group := range entry.Groups {
			for _, field := range group.Fields {
				if field.ID != FieldAPIKey {
					continue
				}
				if field.EffectiveWidget() != plugin.WidgetPassword {
					t.Errorf("%s renders its key as %q", entry.Manifest.ID, field.EffectiveWidget())
				}
				if field.Default != nil {
					t.Errorf("%s leaks a key default into the form", entry.Manifest.ID)
				}
			}
		}
	}
}

// Every provider that used to be wired in the dependency container must still
// be reachable; registration moved, the catalog did not shrink.
func TestEveryProviderIsStillRegistered(t *testing.T) {
	want := []string{
		"duckduckgo", "google", "bing", "tavily", "ollama",
		"baidu", "searxng", "keenable", "zhipu", "exa", "metaso",
	}
	for _, id := range want {
		if _, ok := Plugins.Lookup(id); !ok {
			t.Errorf("provider %s is no longer registered", id)
		}
	}
	if got := len(Plugins.List()); got != len(want) {
		t.Errorf("registry holds %d providers, want %d", got, len(want))
	}
}
