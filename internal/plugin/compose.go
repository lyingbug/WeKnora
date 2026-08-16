package plugin

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Entry is one row in a composed plugin tree.
type Entry struct {
	ID       string `yaml:"id"`
	Plugin   string `yaml:"plugin,omitempty"`
	Config   Config `yaml:"config,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty"`
	Isolate  bool   `yaml:"isolate,omitempty"`
}

// FactoryName is the catalog key. Empty Plugin falls back to ID.
func (e Entry) FactoryName() string {
	if e.Plugin != "" {
		return e.Plugin
	}
	return e.ID
}

// Bundle is a named list of entries that a profile can stack.
type Bundle struct {
	Name    string  `yaml:"name"`
	Entries []Entry `yaml:"entries"`
}

// Patch replaces or inserts one entry by id.
type Patch struct {
	ID       string `yaml:"id"`
	Plugin   string `yaml:"plugin,omitempty"`
	Config   Config `yaml:"config,omitempty"`
	Disabled *bool  `yaml:"disabled,omitempty"`
	Isolate  *bool  `yaml:"isolate,omitempty"`
	Insert   bool   `yaml:"insert,omitempty"`
}

// Profile is a named composition: stacked bundles plus optional patches.
type Profile struct {
	Name    string   `yaml:"name"`
	Bundles []string `yaml:"bundles"`
	Patch   []Patch  `yaml:"patch,omitempty"`
}

// ApplyPatches overlays patches onto entries. A patch with Insert appends
// when the id is missing; otherwise the matching row is replaced field-wise.
func ApplyPatches(entries []Entry, patches []Patch) []Entry {
	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.ID] = i
	}
	out := append([]Entry(nil), entries...)
	for _, p := range patches {
		if p.ID == "" {
			continue
		}
		i, ok := index[p.ID]
		if !ok {
			if !p.Insert {
				continue
			}
			e := Entry{ID: p.ID, Plugin: p.Plugin, Config: p.Config}
			if p.Disabled != nil {
				e.Disabled = *p.Disabled
			}
			if p.Isolate != nil {
				e.Isolate = *p.Isolate
			}
			index[p.ID] = len(out)
			out = append(out, e)
			continue
		}
		e := out[i]
		if p.Plugin != "" {
			e.Plugin = p.Plugin
		}
		if p.Config != nil {
			e.Config = p.Config
		}
		if p.Disabled != nil {
			e.Disabled = *p.Disabled
		}
		if p.Isolate != nil {
			e.Isolate = *p.Isolate
		}
		out[i] = e
	}
	return out
}

// StackBundles concatenates named bundles in order. Unknown names error.
func StackBundles(order []string, bundles map[string]Bundle) ([]Entry, error) {
	var out []Entry
	seen := map[string]int{}
	for _, name := range order {
		b, ok := bundles[name]
		if !ok {
			return nil, fmt.Errorf("plugin: unknown bundle %q", name)
		}
		for _, e := range b.Entries {
			if e.ID == "" {
				return nil, fmt.Errorf("plugin: bundle %q has an entry with empty id", name)
			}
			if prev, dup := seen[e.ID]; dup {
				return nil, fmt.Errorf("plugin: duplicate entry id %q (bundle %q and earlier row %d)", e.ID, name, prev)
			}
			seen[e.ID] = len(out)
			out = append(out, e)
		}
	}
	return out, nil
}

// LoadProfile reads a YAML profile from path. A missing file is not an error
// when allowMissing is true; the caller should then use a default profile.
func LoadProfile(path string, allowMissing bool) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if allowMissing && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin: read profile %s: %w", path, err)
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("plugin: parse profile %s: %w", path, err)
	}
	if p.Name == "" {
		p.Name = "unnamed"
	}
	return &p, nil
}
