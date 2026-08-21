package websearch

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestBundleMatchesBuiltins(t *testing.T) {
	b := Bundle()
	if len(b.Entries) != len(builtins()) {
		t.Fatalf("bundle %d != builtins %d", len(b.Entries), len(builtins()))
	}
	for i, s := range builtins() {
		want := prefix + s.id
		if b.Entries[i].ID != want || b.Entries[i].Plugin != want {
			t.Fatalf("entry[%d] = %+v, want %s", i, b.Entries[i], want)
		}
		if _, ok := plugin.LookupFactory(want); !ok {
			t.Fatalf("factory %s not registered", want)
		}
	}
}

func TestProviderPluginRegistersAndUnregisters(t *testing.T) {
	reg := web_search.NewRegistry()
	h := plugin.NewHost()
	h.Context().Provide(plugin.ServiceWebSearch, reg)
	if err := h.Compose(DefaultProfile(), Bundles(), nil); err != nil {
		t.Fatal(err)
	}
	if !reg.Has("duckduckgo") || !reg.Has("exa") {
		t.Fatalf("list = %v", reg.List())
	}
	if _, err := reg.CreateProvider("duckduckgo", types.WebSearchProviderParameters{}); err != nil {
		t.Fatal(err)
	}
	h.Unload()
	if reg.Has("duckduckgo") {
		t.Fatal("duckduckgo should unload with the plugin")
	}
}

func TestPatchDisablesProvider(t *testing.T) {
	off := true
	reg := web_search.NewRegistry()
	h := plugin.NewHost()
	h.Context().Provide(plugin.ServiceWebSearch, reg)
	profile := DefaultProfile()
	profile.Patch = []plugin.Patch{{ID: "websearch.exa", Disabled: &off}}
	if err := h.Compose(profile, Bundles(), nil); err != nil {
		t.Fatal(err)
	}
	if reg.Has("exa") {
		t.Fatal("exa should be disabled by patch")
	}
	if !reg.Has("bing") {
		t.Fatal("bing should still be mounted")
	}
	h.Unload()
}
