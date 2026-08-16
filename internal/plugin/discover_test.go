package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverNestedAndRoot(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "bravo")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, filepath.Join(nested, "plugin.yaml"), `
id: websearch.bravo
seam: web_search
runtime: js
entry: search.js
`)
	writeManifest(t, filepath.Join(dir, "plugin.yaml"), `
id: websearch.alpha
seam: web_search
runtime: http
endpoint: http://127.0.0.1:9/search
`)
	got, err := Discover([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "websearch.alpha" || got[1].ID != "websearch.bravo" {
		t.Fatalf("got = %+v", got)
	}
	if !got[0].Enabled() || got[1].ProviderID() != "bravo" {
		t.Fatalf("fields = %+v %+v", got[0], got[1])
	}
}

func TestDiscoverMissingDir(t *testing.T) {
	got, err := Discover([]string{filepath.Join(t.TempDir(), "nope"), "none", "-"})
	if err != nil || len(got) != 0 {
		t.Fatalf("got = %v, %v", got, err)
	}
}

func TestDiscoverDuplicateID(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	writeManifest(t, filepath.Join(a, "plugin.yaml"), `
id: websearch.same
seam: web_search
runtime: http
endpoint: http://127.0.0.1:9/a
`)
	writeManifest(t, filepath.Join(b, "plugin.yaml"), `
id: websearch.same
seam: web_search
runtime: http
endpoint: http://127.0.0.1:9/b
`)
	if _, err := Discover([]string{a, b}); err == nil {
		t.Fatal("expected duplicate id")
	}
}

func TestParsePluginDirs(t *testing.T) {
	if got := ParsePluginDirs(""); len(got) != 1 || got[0] != "plugins.d" {
		t.Fatalf("default = %v", got)
	}
	got := ParsePluginDirs("a" + string(os.PathListSeparator) + " b ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("split = %v", got)
	}
}

func TestBundleFromManifestsHonorsAutoEnable(t *testing.T) {
	off := false
	b := BundleFromManifests([]Manifest{
		{ID: "on", Runtime: RuntimeHTTP, Endpoint: "http://x", Seam: ServiceWebSearch},
		{ID: "off", Runtime: RuntimeHTTP, Endpoint: "http://x", Seam: ServiceWebSearch, AutoEnable: &off},
	})
	if len(b.Entries) != 2 || b.Entries[0].Disabled || !b.Entries[1].Disabled {
		t.Fatalf("entries = %+v", b.Entries)
	}
}

func TestConfigMerge(t *testing.T) {
	got := Config{"a": "1", "b": "2"}.Merge(Config{"b": "3", "c": "4"})
	if got.String("a") != "1" || got.String("b") != "3" || got.String("c") != "4" {
		t.Fatalf("merge = %+v", got)
	}
}

func TestHostDump(t *testing.T) {
	Register("dump.alpha", func(Config) (Plugin, error) {
		return Func{ID: "dump.alpha"}, nil
	})
	h := NewHost()
	if err := h.Compose(Profile{Bundles: []string{"b"}}, map[string]Bundle{
		"b": {Entries: []Entry{{ID: "dump.alpha"}, {ID: "skip", Disabled: true}}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	dump := h.Dump()
	if dump == "" || !strings.Contains(dump, "dump.alpha") || !strings.Contains(dump, "skip") {
		t.Fatalf("dump = %s", dump)
	}
	h.Unload()
}

func writeManifest(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
