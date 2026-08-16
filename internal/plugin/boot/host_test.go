package boot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type nopCleaner struct {
	n int
}

func (c *nopCleaner) Register(cleanup types.CleanupFunc) {
	if cleanup != nil {
		c.n++
	}
}

func (c *nopCleaner) RegisterWithName(_ string, cleanup types.CleanupFunc) {
	c.Register(cleanup)
}

func (c *nopCleaner) Cleanup(context.Context) []error { return nil }

var _ interfaces.ResourceCleaner = (*nopCleaner)(nil)

func TestNewHostMountsBuiltinSearch(t *testing.T) {
	t.Setenv(envProfile, filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv(envPlugins, "")
	t.Setenv(envPatch, "")

	reg := infra_web_search.NewRegistry()
	cleaner := &nopCleaner{}
	host, err := NewHost(reg, cleaner)
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Has("duckduckgo") || !reg.Has("metaso") {
		t.Fatalf("builtins = %v", reg.List())
	}
	if reg.Has("echo") {
		t.Fatal("echo should stay off unless WEKNORA_PLUGINS enables it")
	}
	if cleaner.n != 1 {
		t.Fatalf("cleaner registrations = %d", cleaner.n)
	}
	Start(host)
	host.Unload()
	if reg.Has("duckduckgo") {
		t.Fatal("unload should remove builtins")
	}
}

func TestNewHostEnablesEchoViaEnv(t *testing.T) {
	t.Setenv(envProfile, filepath.Join(t.TempDir(), "missing.yaml"))
	t.Setenv(envPlugins, "websearch.echo")
	t.Setenv(envPatch, "")

	reg := infra_web_search.NewRegistry()
	host, err := NewHost(reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Has("echo") {
		t.Fatalf("echo not mounted, list=%v", reg.List())
	}
	p, err := reg.CreateProvider("echo", types.WebSearchProviderParameters{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "echo" {
		t.Fatalf("name = %s", p.Name())
	}
	host.Unload()
}

func TestLoadProfileUsesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	data := []byte("name: lab\nbundles: [base]\npatch:\n  - id: websearch.exa\n    disabled: true\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envProfile, path)
	t.Setenv(envPlugins, "")
	t.Setenv(envPatch, "")

	reg := infra_web_search.NewRegistry()
	host, err := NewHost(reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Has("exa") {
		t.Fatal("yaml patch should disable exa")
	}
	if !reg.Has("bing") {
		t.Fatal("bing should remain")
	}
	host.Unload()
}
