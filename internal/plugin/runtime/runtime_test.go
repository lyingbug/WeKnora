package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

func TestHTTPProviderSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Query != "hello" || req.Parameters.APIKey != "k" {
			t.Fatalf("req = %+v", req)
		}
		_ = json.NewEncoder(w).Encode(SearchResponse{Results: []*types.WebSearchResult{
			{Title: "T", URL: "https://example.com", Snippet: req.Query, Source: "http-echo"},
		}})
	}))
	defer srv.Close()

	p, err := newHTTPProvider("http-echo", srv.URL, clampTimeout(2000))
	if err != nil {
		t.Fatal(err)
	}
	p = p.withParams(types.WebSearchProviderParameters{APIKey: "k"})
	got, err := p.Search(context.Background(), "hello", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Snippet != "hello" {
		t.Fatalf("got = %+v", got)
	}
}

func TestJSProviderSearch(t *testing.T) {
	src := `
function search(query, maxResults, includeDate, params) {
  if (!query) return [];
  return [{
    title: params.api_key || "js",
    url: "https://weknora.local/js",
    snippet: query,
    source: "js-echo"
  }];
}
`
	p, err := newJSProvider("js-echo", src, clampTimeout(2000))
	if err != nil {
		t.Fatal(err)
	}
	p = p.withParams(types.WebSearchProviderParameters{APIKey: "from-params"})
	got, err := p.Search(context.Background(), "  q  ", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "from-params" || got[0].Snippet != "  q  " {
		t.Fatalf("got = %+v", got)
	}
}

func TestJSProviderHTTPRequest(t *testing.T) {
	utils.ResetSSRFWhitelistForTest()
	t.Setenv("SSRF_WHITELIST", "127.0.0.1,localhost")
	defer utils.ResetSSRFWhitelistForTest()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"q":"` + r.URL.Query().Get("q") + `"}`))
	}))
	defer srv.Close()

	src := fmt.Sprintf(`
function search(query, maxResults) {
  var resp = httpRequest({ method: "GET", url: %q + "?q=" + encodeURIComponent(query) });
  if (resp.status !== 200) return [];
  var data = JSON.parse(resp.body);
  return [{ title: "remote", url: "https://example.com", snippet: data.q, source: "js-http" }];
}
`, srv.URL)
	p, err := newJSProvider("js-http", src, clampTimeout(3000))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Search(context.Background(), "hi", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Snippet != "hi" {
		t.Fatalf("got = %+v", got)
	}
}

func TestRegisterManifestsMountsJS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "search.js"), []byte(`
function search(query) {
  return [{ title: "disk", url: "https://weknora.local/disk", snippet: query, source: "disk" }];
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(`
id: websearch.disk
name: Disk JS
seam: web_search
runtime: js
entry: search.js
description: runtime disk plugin
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := plugin.Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterManifests(ms); err != nil {
		t.Fatal(err)
	}
	reg := infra_web_search.NewRegistry()
	h := plugin.NewHost()
	h.Context().Provide(plugin.ServiceWebSearch, reg)
	if err := h.Compose(plugin.Profile{Bundles: []string{plugin.ExternalBundle}}, map[string]plugin.Bundle{
		plugin.ExternalBundle: plugin.BundleFromManifests(ms),
	}, nil); err != nil {
		t.Fatal(err)
	}
	if !reg.Has("disk") {
		t.Fatalf("list = %v", reg.List())
	}
	if !types.IsKnownWebSearchProviderType("disk") {
		t.Fatal("type catalog missing disk")
	}
	p, err := reg.CreateProvider("disk", types.WebSearchProviderParameters{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Search(context.Background(), "ping", 1, false)
	if err != nil || len(got) != 1 || got[0].Snippet != "ping" {
		t.Fatalf("search = %+v, %v", got, err)
	}
	h.Unload()
	if types.IsKnownWebSearchProviderType("disk") {
		t.Fatal("type should unload with the plugin")
	}
}

func TestHTTPEndpointRejectsBadScheme(t *testing.T) {
	if _, err := newHTTPProvider("x", "file:///etc/passwd", clampTimeout(1)); err == nil {
		t.Fatal("expected scheme error")
	}
}
