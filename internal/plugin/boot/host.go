// Package boot wires the plugin Host into WeKnora: it publishes existing
// registries as Context services, stacks the base bundle, then applies
// config/plugin_profile.yaml and WEKNORA_PLUGINS overlays.
package boot

import (
	"context"
	"fmt"
	"os"
	"strings"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/plugin/websearch"
	"github.com/Tencent/WeKnora/internal/types/interfaces"

	_ "github.com/Tencent/WeKnora/plugins/websearch-echo"
)

const (
	envProfile = "WEKNORA_PLUGIN_PROFILE"
	envPlugins = "WEKNORA_PLUGINS"
	envPatch   = "WEKNORA_PLUGIN_PATCH"
)

// NewHost constructs and composes the process plugin tree.
func NewHost(registry *infra_web_search.Registry, cleaner interfaces.ResourceCleaner) (*plugin.Host, error) {
	host := plugin.NewHost()
	host.Context().Provide(plugin.ServiceWebSearch, registry)

	profile, err := loadProfile()
	if err != nil {
		return nil, err
	}
	extra, err := extraPatches()
	if err != nil {
		return nil, err
	}
	if err := host.Compose(profile, websearch.Bundles(), extra); err != nil {
		return nil, err
	}

	ctx := context.Background()
	enabled := 0
	for _, m := range host.Mounted() {
		if !m.Disabled {
			enabled++
		}
	}
	logger.Infof(ctx, "[Plugin] mounted %d plugins (%d rows)", enabled, len(host.Mounted()))
	for _, m := range host.Mounted() {
		if m.Disabled {
			logger.Debugf(ctx, "[Plugin] disabled %s", m.ID)
			continue
		}
		logger.Debugf(ctx, "[Plugin] enabled %s", m.ID)
	}

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
