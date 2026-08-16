package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const manifestName = "plugin.yaml"

// Discover walks each directory for plugin.yaml (the dir itself or one
// level of children). Results are sorted by id. Invalid files return error.
func Discover(dirs []string) ([]Manifest, error) {
	seen := map[string]string{}
	var out []Manifest
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" || dir == "-" || dir == "none" {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("plugin: stat %s: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("plugin: %s is not a directory", dir)
		}
		found, err := scanDir(dir)
		if err != nil {
			return nil, err
		}
		for _, m := range found {
			if prev, ok := seen[m.ID]; ok {
				return nil, fmt.Errorf("plugin: duplicate id %q (%s and %s)", m.ID, prev, m.Dir)
			}
			seen[m.ID] = m.Dir
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func scanDir(dir string) ([]Manifest, error) {
	var out []Manifest
	root := filepath.Join(dir, manifestName)
	if st, err := os.Stat(root); err == nil && !st.IsDir() {
		m, err := LoadManifest(root)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), manifestName)
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			continue
		}
		m, err := LoadManifest(path)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// BundleFromManifests builds the external bundle. Disabled / auto_enable
// false rows are still listed so a profile patch can turn them on.
func BundleFromManifests(manifests []Manifest) Bundle {
	entries := make([]Entry, 0, len(manifests))
	for _, m := range manifests {
		entries = append(entries, Entry{
			ID:       m.ID,
			Plugin:   m.ID,
			Config:   m.Config,
			Disabled: !m.Enabled(),
		})
	}
	return Bundle{Name: ExternalBundle, Entries: entries}
}

// ParsePluginDirs splits WEKNORA_PLUGIN_DIR (os.PathListSeparator).
func ParsePluginDirs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"plugins.d"}
	}
	var out []string
	for _, p := range filepath.SplitList(raw) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
