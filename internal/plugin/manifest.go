package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Runtime identifiers for disk-loaded plugins. native stays compile-time.
const (
	RuntimeJS   = "js"
	RuntimeHTTP = "http"
)

// ExternalBundle is the profile bundle assembled from WEKNORA_PLUGIN_DIR.
const ExternalBundle = "external"

// Manifest is the on-disk contract (package.json analogue). Drop a folder
// with plugin.yaml and WeKnora will register it without a blank import.
type Manifest struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name,omitempty"`
	Version     string `yaml:"version,omitempty"`
	Description string `yaml:"description,omitempty"`
	Seam        string `yaml:"seam"`
	Runtime     string `yaml:"runtime"`
	Entry       string `yaml:"entry,omitempty"`
	Endpoint    string `yaml:"endpoint,omitempty"`
	Provider    string `yaml:"provider,omitempty"`
	DocsURL     string `yaml:"docs_url,omitempty"`
	RequiresKey bool   `yaml:"requires_api_key,omitempty"`
	AutoEnable  *bool  `yaml:"auto_enable,omitempty"`
	Disabled    bool   `yaml:"disabled,omitempty"`
	TimeoutMS   int    `yaml:"timeout_ms,omitempty"`
	Config      Config `yaml:"config,omitempty"`
	Dir         string `yaml:"-"`
}

// Enabled reports whether the manifest should be mounted by default.
func (m Manifest) Enabled() bool {
	if m.Disabled {
		return false
	}
	if m.AutoEnable == nil {
		return true
	}
	return *m.AutoEnable
}

// ProviderID is the seam-specific registration key.
func (m Manifest) ProviderID() string {
	if m.Provider != "" {
		return m.Provider
	}
	id := m.ID
	if i := strings.LastIndex(id, "."); i >= 0 {
		return id[i+1:]
	}
	return id
}

// DisplayName is the UI label.
func (m Manifest) DisplayName() string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// Timeout returns the request timeout, default 10s.
func (m Manifest) Timeout() int {
	if m.TimeoutMS > 0 {
		return m.TimeoutMS
	}
	return 10000
}

// EntryPath resolves Entry against the plugin directory.
func (m Manifest) EntryPath() string {
	if m.Entry == "" || m.Dir == "" {
		return m.Entry
	}
	if filepath.IsAbs(m.Entry) {
		return m.Entry
	}
	return filepath.Join(m.Dir, m.Entry)
}

// Validate checks required fields for a disk plugin.
func (m Manifest) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("plugin.yaml: id is required")
	}
	if m.Seam == "" {
		return fmt.Errorf("plugin %s: seam is required", m.ID)
	}
	switch m.Runtime {
	case RuntimeJS:
		if m.Entry == "" {
			return fmt.Errorf("plugin %s: js runtime requires entry", m.ID)
		}
	case RuntimeHTTP:
		if strings.TrimSpace(m.Endpoint) == "" {
			return fmt.Errorf("plugin %s: http runtime requires endpoint", m.ID)
		}
	default:
		return fmt.Errorf("plugin %s: unsupported runtime %q (js|http)", m.ID, m.Runtime)
	}
	return nil
}

// LoadManifest reads one plugin.yaml.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin: read %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("plugin: parse %s: %w", path, err)
	}
	m.Dir = filepath.Dir(path)
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
