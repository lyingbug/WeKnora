package web_search

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type stubProvider struct{ name string }

func (s stubProvider) Name() string { return s.name }
func (s stubProvider) Search(context.Context, string, int, bool) ([]*types.WebSearchResult, error) {
	return nil, nil
}

func TestRegistryRegisterHasListUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register("echo", func(types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
		return stubProvider{name: "echo"}, nil
	})
	if !r.Has("echo") {
		t.Fatal("expected echo to be registered")
	}
	if got := r.List(); len(got) != 1 || got[0] != "echo" {
		t.Fatalf("list = %v", got)
	}
	p, err := r.CreateProvider("echo", types.WebSearchProviderParameters{})
	if err != nil || p.Name() != "echo" {
		t.Fatalf("create = %v, %v", p, err)
	}
	r.Unregister("echo")
	if r.Has("echo") {
		t.Fatal("echo should be gone")
	}
	if _, err := r.CreateProvider("echo", types.WebSearchProviderParameters{}); err == nil {
		t.Fatal("expected missing provider error")
	}
}
