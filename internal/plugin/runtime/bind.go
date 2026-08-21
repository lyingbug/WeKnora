package runtime

import (
	"fmt"
	"os"
	"strings"

	infra_web_search "github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// RegisterManifests adds a factory for each disk plugin. First-wins so a
// dropped file cannot hijack a built-in id.
func RegisterManifests(manifests []plugin.Manifest) error {
	for _, m := range manifests {
		m := m
		plugin.Register(m.ID, func(cfg plugin.Config) (plugin.Plugin, error) {
			return newDiskPlugin(m, m.Config.Merge(cfg))
		})
	}
	return nil
}

func newDiskPlugin(m plugin.Manifest, cfg plugin.Config) (plugin.Plugin, error) {
	if m.Seam != plugin.ServiceWebSearch {
		return nil, fmt.Errorf("plugin %s: seam %q is not wired for disk load yet", m.ID, m.Seam)
	}
	return plugin.Func{
		ID:         m.ID,
		InjectKeys: []string{plugin.ServiceWebSearch},
		ApplyFn: func(ctx *plugin.Context) error {
			reg, err := plugin.Service[*infra_web_search.Registry](ctx, plugin.ServiceWebSearch)
			if err != nil {
				return err
			}
			factory, closer, err := providerFactory(m, cfg)
			if err != nil {
				return err
			}
			id := m.ProviderID()
			info := types.WebSearchProviderTypeInfo{
				ID:             id,
				Name:           m.DisplayName(),
				RequiresAPIKey: m.RequiresKey,
				Description:    strings.TrimSpace(m.Description),
				DocsURL:        m.DocsURL,
			}
			return ctx.Effect(func() plugin.Disposable {
				reg.Register(id, factory)
				types.RegisterWebSearchProviderType(info)
				return plugin.DisposeFunc(func() {
					if closer != nil {
						_ = closer.Close()
					}
					reg.Unregister(id)
					types.UnregisterWebSearchProviderType(id)
				})
			})
		},
	}, nil
}

func providerFactory(m plugin.Manifest, cfg plugin.Config) (infra_web_search.ProviderFactory, ioCloser, error) {
	timeout := clampTimeout(m.Timeout())
	switch m.Runtime {
	case plugin.RuntimeStdio:
		session, err := newStdioSession(m)
		if err != nil {
			return nil, nil, err
		}
		return func(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
			return session.withParams(params), nil
		}, session, nil
	case plugin.RuntimeHTTP:
		endpoint := cfg.String("endpoint")
		if endpoint == "" {
			endpoint = m.Endpoint
		}
		base, err := newHTTPProvider(m.ProviderID(), endpoint, timeout)
		if err != nil {
			return nil, nil, err
		}
		return func(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
			return base.withParams(params), nil
		}, nil, nil
	case plugin.RuntimeJS:
		path := m.EntryPath()
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("plugin %s: read %s: %w", m.ID, path, err)
		}
		base, err := newJSProvider(m.ProviderID(), string(src), timeout)
		if err != nil {
			return nil, nil, err
		}
		return func(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
			return base.withParams(params), nil
		}, nil, nil
	default:
		return nil, nil, fmt.Errorf("plugin %s: runtime %q", m.ID, m.Runtime)
	}
}

type ioCloser interface {
	Close() error
}
