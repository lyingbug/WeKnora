package websearchecho

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestEchoPluginSearchAndUnload(t *testing.T) {
	reg := web_search.NewRegistry()
	h := plugin.NewHost()
	h.Context().Provide(plugin.ServiceWebSearch, reg)

	on := false
	profile := plugin.Profile{Name: "t", Bundles: []string{"empty"}}
	bundles := map[string]plugin.Bundle{"empty": {Name: "empty"}}
	patch := []plugin.Patch{{
		ID: FactoryName, Plugin: FactoryName, Insert: true, Disabled: &on,
		Config: plugin.Config{"title": "from-config"},
	}}
	if err := h.Compose(profile, bundles, patch); err != nil {
		t.Fatal(err)
	}
	p, err := reg.CreateProvider(ProviderID, types.WebSearchProviderParameters{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Search(context.Background(), "  hello  ", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "from-config" || got[0].Content != "hello" {
		t.Fatalf("result = %+v", got)
	}
	h.Unload()
	if reg.Has(ProviderID) {
		t.Fatal("echo should unload with the plugin")
	}
}

func TestEchoEmptyQuery(t *testing.T) {
	p := &echoProvider{title: "echo"}
	got, err := p.Search(context.Background(), "  ", 3, false)
	if err != nil || got != nil {
		t.Fatalf("empty query = %v, %v", got, err)
	}
}
