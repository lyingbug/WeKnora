package web_search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// allowLoopback lets a test reach an httptest server past the SSRF guard.
// The parsed whitelist is process-cached, so it has to be reset on both sides
// or the setting leaks into tests that expect loopback to be refused.
func allowLoopback(t *testing.T) {
	t.Helper()
	utils.ResetSSRFWhitelistForTest()
	t.Setenv("SSRF_WHITELIST", "127.0.0.1,localhost")
	t.Cleanup(utils.ResetSSRFWhitelistForTest)
}

// These tests are about the shipped plugin files, not about the kernel. A
// manifest that parses but describes the wrong request is the failure mode
// this format introduces, so the files themselves need coverage.

func installed(t *testing.T) *plugin.Registry {
	t.Helper()
	reg := plugin.NewRegistry()
	if count := Install(reg); count == 0 {
		t.Fatal("no built-in web search plugins were installed")
	}
	return reg
}

// Every shipped file must load. A typo here ships a silently missing provider.
func TestBuiltinPluginsAllLoad(t *testing.T) {
	reg := installed(t)

	if failures := reg.Failures(); len(failures) > 0 {
		t.Fatalf("built-in plugins failed to load: %+v", failures)
	}

	for _, id := range []string{"tavily", "bing", "searxng", "exa", "zhipu", "duckduckgo"} {
		entry, ok := reg.Lookup(Kind, id)
		if !ok {
			t.Errorf("provider %s is not installed", id)
			continue
		}
		if entry.Manifest.Name == "" {
			t.Errorf("provider %s has no display name", id)
		}
	}
}

// The credential a provider needs is declared, so it is refused before any
// request happens and reported the same way for every provider.
func TestMissingCredentialIsRefusedUniformly(t *testing.T) {
	reg := installed(t)

	for _, id := range []string{"tavily", "bing", "exa", "zhipu"} {
		if _, err := Open(context.Background(), reg, id, types.WebSearchProviderParameters{}); err == nil {
			t.Errorf("%s should refuse an empty configuration", id)
		}
	}

	// DuckDuckGo needs nothing, and the manifest is what says so.
	if _, err := Open(context.Background(), reg, "duckduckgo", types.WebSearchProviderParameters{}); err != nil {
		t.Errorf("duckduckgo should not require configuration: %v", err)
	}
}

// SearXNG is configured with an endpoint rather than a key, and the manifest
// composes it into the URL.
func TestSelfHostedProviderRequiresItsEndpoint(t *testing.T) {
	reg := installed(t)

	if _, err := Open(context.Background(), reg, "searxng", types.WebSearchProviderParameters{}); err == nil {
		t.Error("searxng should refuse a configuration with no instance URL")
	}
	if _, err := Open(context.Background(), reg, "searxng", types.WebSearchProviderParameters{
		BaseURL: "https://searx.example.com",
	}); err != nil {
		t.Errorf("searxng should accept an instance URL: %v", err)
	}
}

// An end-to-end run of a shipped manifest against a stub standing in for the
// vendor: the request must match what the vendor documents, and the reply must
// map onto results.
func TestTavilyManifestProducesTheDocumentedCall(t *testing.T) {
	allowLoopback(t)

	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"results":[
			{"title":"WeKnora","url":"https://example.test/weknora","content":"A knowledge base."}
		]}`))
	}))
	defer server.Close()

	// Point the shipped manifest at the stub by overriding it from a user
	// directory — which is itself the override mechanism a deployment uses.
	dir := t.TempDir()
	override := `apiVersion: weknora/v1
kind: websearch
id: tavily
name: Tavily (test)
config:
  - id: api_key
    type: string
    required: true
    secret: true
runtime:
  type: http
  request:
    method: POST
    url: ` + server.URL + `
    body:
      api_key: ${config.api_key}
      query: ${input.query}
      max_results: ${input.max_results}
      search_depth: basic
  response:
    items: results
    fields:
      title: title
      url: url
      snippet: content
`
	if err := os.WriteFile(filepath.Join(dir, "tavily.yaml"), []byte(override), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}

	reg := installed(t)
	plugin.LoadDir(reg, "user", dir)

	provider, err := Open(context.Background(), reg, "tavily", types.WebSearchProviderParameters{
		APIKey: "tvly-test",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	results, err := provider.Search(context.Background(), "what is weknora", 3, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if body["api_key"] != "tvly-test" {
		t.Errorf("api_key = %#v", body["api_key"])
	}
	if body["query"] != "what is weknora" {
		t.Errorf("query = %#v", body["query"])
	}
	if count, ok := body["max_results"].(float64); !ok || count != 3 {
		t.Errorf("max_results = %#v, want the number 3", body["max_results"])
	}

	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Title != "WeKnora" || results[0].URL != "https://example.test/weknora" {
		t.Errorf("result = %+v", results[0])
	}
	if results[0].Snippet != "A knowledge base." {
		t.Errorf("snippet = %q", results[0].Snippet)
	}
	if results[0].Source != "tavily" {
		t.Errorf("source = %q", results[0].Source)
	}
}

// A deployment adds a provider the project has never heard of by writing a
// file. This is the whole point, so it is a test rather than a claim.
func TestADeploymentCanAddAProviderWeHaveNeverSeen(t *testing.T) {
	allowLoopback(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "hello" {
			t.Errorf("q = %q", got)
		}
		if got := r.Header.Get("X-Company-Token"); got != "internal-token" {
			t.Errorf("token header = %q", got)
		}
		_, _ = w.Write([]byte(`{"hits":[{"heading":"Intranet page","href":"https://intranet.test/p"}]}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	manifest := `apiVersion: weknora/v1
kind: websearch
id: company-intranet
name: Company intranet
description: Searches our internal wiki.
config:
  - id: token
    type: string
    label: Service token
    required: true
    secret: true
runtime:
  type: http
  request:
    method: GET
    url: ` + server.URL + `/search
    query:
      q: ${input.query}
      limit: ${input.max_results}
    headers:
      X-Company-Token: ${config.token}
  response:
    items: hits
    fields:
      title: heading
      url: href
`
	if err := os.WriteFile(filepath.Join(dir, "intranet.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	reg := installed(t)
	plugin.LoadDir(reg, "user", dir)

	provider, err := Open(context.Background(), reg, "company-intranet", types.WebSearchProviderParameters{
		ExtraConfig: map[string]string{"token": "internal-token"},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	results, err := provider.Search(context.Background(), "hello", 5, false)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Intranet page" {
		t.Fatalf("results = %+v", results)
	}
}
