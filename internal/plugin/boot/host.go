// Package boot wires the plugin Host into WeKnora: it publishes existing
// registries as Context services, stacks the base bundle, then applies
// disk plugins from WEKNORA_PLUGIN_DIR, config/plugin_profile.yaml and
// WEKNORA_PLUGINS overlays.
package boot

import (
	"context"
	"fmt"
	"os"
	"strings"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/plugin"
	pluginruntime "github.com/Tencent/WeKnora/internal/plugin/runtime"
	"github.com/Tencent/WeKnora/internal/plugin/websearch"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	_ "github.com/Tencent/WeKnora/plugins/websearch-echo"
)

const (
	envProfile   = "WEKNORA_PLUGIN_PROFILE"
	envPlugins   = "WEKNORA_PLUGINS"
	envPatch     = "WEKNORA_PLUGIN_PATCH"
	envPluginDir = "WEKNORA_PLUGIN_DIR"
)

// NewHost constructs and composes the process plugin tree.
func NewHost(registry *infra_web_search.Registry, cleaner interfaces.ResourceCleaner) (*plugin.Host, error) {
	host := plugin.NewHost()
	host.Context().Provide(plugin.ServiceWebSearch, registry)
	ctxLog := context.Background()

	disk, err := plugin.Discover(plugin.ParsePluginDirs(os.Getenv(envPluginDir)))
	if err != nil {
		return nil, err
	}
	if err := pluginruntime.RegisterManifests(disk); err != nil {
		return nil, err
	}

	profile, err := loadProfile()
	if err != nil {
		return nil, err
	}
	bundles := websearch.Bundles()
	ext := plugin.BundleFromManifests(disk)
	ext.Entries = dropKnownIDs(ext.Entries, bundles[websearch.BundleName], ctxLog)
	bundles[plugin.ExternalBundle] = ext
	if !containsString(profile.Bundles, plugin.ExternalBundle) {
		profile.Bundles = append(profile.Bundles, plugin.ExternalBundle)
	}

	extra, err := extraPatches()
	if err != nil {
		return nil, err
	}
	if err := host.Compose(profile, bundles, extra); err != nil {
		return nil, err
	}

	ctx := context.Background()
	enabled := 0
	for _, m := range host.Mounted() {
		if !m.Disabled {
			enabled++
		}
	}
	logger.Infof(ctx, "[Plugin] mounted %d plugins (%d rows, %d disk)", enabled, len(host.Mounted()), len(disk))
	for _, m := range host.Mounted() {
		if m.Disabled {
			logger.Debugf(ctx, "[Plugin] disabled %s", m.ID)
			continue
		}
		logger.Debugf(ctx, "[Plugin] enabled %s", m.ID)
	}
	logger.Debugf(ctx, "[Plugin] dump:\n%s", host.Dump())

	if cleaner != nil {
		cleaner.RegisterWithName("PluginHost", func() error {
			host.Unload()
			return nil
		})
	}
	return host, nil
}

// Start is a dig Invoke hook so the host is constructed at process boot.
func Start(h *plugin.Host) {
	if h == nil {
		logger.Warnf(context.Background(), "[Plugin] host was not constructed")
	}
}

func loadProfile() (plugin.Profile, error) {
	fallback := websearch.DefaultProfile()
	path := strings.TrimSpace(os.Getenv(envProfile))
	if path == "" {
		path = "config/plugin_profile.yaml"
	}
	loaded, err := plugin.LoadProfile(path, true)
	if err != nil {
		return plugin.Profile{}, err
	}
	if loaded == nil {
		return fallback, nil
	}
	if len(loaded.Bundles) == 0 {
		loaded.Bundles = fallback.Bundles
	}
	if loaded.Name == "" || loaded.Name == "unnamed" {
		loaded.Name = fallback.Name
	}
	return *loaded, nil
}

func extraPatches() ([]plugin.Patch, error) {
	var out []plugin.Patch
	if raw := strings.TrimSpace(os.Getenv(envPlugins)); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			out = append(out, plugin.Patch{ID: name, Plugin: name, Insert: true})
		}
	}
	path := strings.TrimSpace(os.Getenv(envPatch))
	if path == "" {
		return out, nil
	}
	overlay, err := plugin.LoadProfile(path, false)
	if err != nil {
		return nil, fmt.Errorf("plugin overlay: %w", err)
	}
	if overlay != nil {
		out = append(out, overlay.Patch...)
	}
	return out, nil
}

func dropKnownIDs(entries []plugin.Entry, base plugin.Bundle, ctx context.Context) []plugin.Entry {
	known := make(map[string]struct{}, len(base.Entries))
	for _, e := range base.Entries {
		known[e.ID] = struct{}{}
	}
	var out []plugin.Entry
	for _, e := range entries {
		if _, ok := known[e.ID]; ok {
			logger.Warnf(ctx, "[Plugin] skip disk plugin %s: id already in bundle %s", e.ID, base.Name)
			continue
		}
		out = append(out, e)
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
