package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPatchesReplaceAndInsert(t *testing.T) {
	entries := []Entry{
		{ID: "websearch.duckduckgo", Plugin: "websearch.duckduckgo"},
		{ID: "websearch.exa", Plugin: "websearch.exa"},
	}
	off := true
	patched := ApplyPatches(entries, []Patch{
		{ID: "websearch.exa", Disabled: &off},
		{ID: "websearch.echo", Plugin: "websearch.echo", Insert: true},
		{ID: "missing", Plugin: "nope"},
	})
	if len(patched) != 3 {
		t.Fatalf("len = %d", len(patched))
	}
	if !patched[1].Disabled {
		t.Fatal("exa should be disabled")
	}
	if patched[2].ID != "websearch.echo" {
		t.Fatalf("insert = %+v", patched[2])
	}
}

func TestStackBundlesDuplicateAndUnknown(t *testing.T) {
	bundles := map[string]Bundle{
		"base":  {Name: "base", Entries: []Entry{{ID: "a"}}},
		"extra": {Name: "extra", Entries: []Entry{{ID: "a"}}},
	}
	if _, err := StackBundles([]string{"nope"}, bundles); err == nil {
		t.Fatal("expected unknown bundle")
	}
	if _, err := StackBundles([]string{"base", "extra"}, bundles); err == nil {
		t.Fatal("expected duplicate id")
	}
	got, err := StackBundles([]string{"base"}, bundles)
	if err != nil || len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got = %v, %v", got, err)
	}
}

func TestLoadProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	if err := os.WriteFile(path, []byte("name: lab\nbundles: [base]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadProfile(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "lab" || len(p.Bundles) != 1 || p.Bundles[0] != "base" {
		t.Fatalf("profile = %+v", p)
	}
	missing := filepath.Join(dir, "absent.yaml")
	p, err = LoadProfile(missing, true)
	if err != nil || p != nil {
		t.Fatalf("missing allow = %+v, %v", p, err)
	}
	if _, err = LoadProfile(missing, false); err == nil {
		t.Fatal("expected read error")
	}
}

func TestEntryFactoryName(t *testing.T) {
	if (Entry{ID: "a"}).FactoryName() != "a" {
		t.Fatal("fallback")
	}
	if (Entry{ID: "a", Plugin: "b"}).FactoryName() != "b" {
		t.Fatal("plugin field")
	}
}
