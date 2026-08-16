// Package websearchecho is a template out-of-tree web search plugin.
// It registers as type "echo" and returns the query as a single result.
// Enable it with WEKNORA_PLUGINS=websearch.echo or a profile patch.
package websearchecho

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/infrastructure/web_search"
	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const (
	FactoryName = "websearch.echo"
	ProviderID  = "echo"
)

func init() {
	plugin.Register(FactoryName, func(cfg plugin.Config) (plugin.Plugin, error) {
		return New(cfg), nil
	})
}

// New constructs the echo plugin. Config key "title" customizes the result.
func New(cfg plugin.Config) plugin.Plugin {
	title := cfg.String("title")
	if title == "" {
		title = "echo"
	}
	return plugin.Func{
		ID:         FactoryName,
		InjectKeys: []string{plugin.ServiceWebSearch},
		ApplyFn: func(ctx *plugin.Context) error {
			reg, err := plugin.Service[*web_search.Registry](ctx, plugin.ServiceWebSearch)
			if err != nil {
				return err
			}
			return ctx.Effect(func() plugin.Disposable {
				reg.Register(ProviderID, func(types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
					return &echoProvider{title: title}, nil
				})
				return plugin.DisposeFunc(func() { reg.Unregister(ProviderID) })
			})
		},
	}
}

type echoProvider struct {
	title string
}

func (p *echoProvider) Name() string { return ProviderID }

func (p *echoProvider) Search(
	_ context.Context, query string, maxResults int, _ bool,
) ([]*types.WebSearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" || maxResults == 0 {
		return nil, nil
	}
	return []*types.WebSearchResult{{
		Title:   p.title,
		URL:     "https://weknora.local/plugin/echo",
		Content: q,
	}}, nil
}
