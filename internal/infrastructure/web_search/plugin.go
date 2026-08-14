package web_search

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Web search on the plugin system.
//
// A provider used to be a Go file: build a struct, set two headers, unmarshal
// into another struct, loop over the results, then add a line to the
// dependency container. Adding one meant a release. Now a provider is a YAML
// file in plugins/websearch, and adding one means writing that file.
//
// This package supplies what a file cannot: the meaning of the domain. It
// declares what a plugin may read as input, which output fields it must map,
// and how the mapped records become the results the rest of WeKnora consumes.

// Kind is the capability plugins declare to serve web search.
const Kind = "websearch"

// The input namespace a manifest may read with ${input.…}. This is the
// domain's half of the contract: the kernel does not know what a query is.
const (
	// InputQuery is the search text.
	InputQuery = "query"
	// InputMaxResults is how many results the caller wants.
	InputMaxResults = "max_results"
	// InputIncludeDate asks the provider for publication dates where it can
	// supply them.
	InputIncludeDate = "include_date"
)

// The output fields a manifest maps with runtime.response.fields. Title
// and URL are what make a result usable; the rest improve it.
const (
	// FieldTitle is the result headline. Required.
	FieldTitle = "title"
	// FieldURL is the result link. Required.
	FieldURL = "url"
	// FieldSnippet is the summary shown to the model and the user.
	FieldSnippet = "snippet"
	// FieldContent is the full page text, when a provider returns one.
	FieldContent = "content"
	// FieldPublishedAt is a publication timestamp in RFC 3339.
	FieldPublishedAt = "published_at"
)

//go:embed plugins/*.yaml
var builtinPlugins embed.FS

// Install loads the built-in provider manifests into a registry. A deployment
// that adds its own plugin directory layers it on top, and a file there with
// the same id overrides the built-in one.
func Install(reg *plugin.Registry) int {
	registerNatives(reg)
	return plugin.LoadFS(reg, plugin.SourceBuiltin, builtinPlugins, "plugins")
}

// Open builds a provider from stored parameters.
//
// The configuration is validated against the plugin's own manifest first, so a
// provider never receives a configuration it declared as invalid, and a
// missing credential is reported the same way for every provider instead of
// as whatever error each constructor happened to write.
func Open(ctx context.Context, reg *plugin.Registry, providerType string, params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
	cfg, entry, err := reg.Configure(Kind, providerType, RawFromParams(params))
	if err != nil {
		return nil, err
	}
	if entry.IsNative() {
		instance, err := entry.NewNative(cfg)
		if err != nil {
			return nil, err
		}
		provider, ok := instance.(interfaces.WebSearchProvider)
		if !ok {
			return nil, fmt.Errorf("%s: native implementation does not provide web search", entry.Manifest.Ref())
		}
		return provider, nil
	}
	return &declarativeProvider{entry: entry, config: cfg}, nil
}

// RawFromParams converts the stored parameter struct into the configuration
// map a manifest declares.
//
// Storage is unchanged by the move to plugins: existing rows load as they are,
// with ExtraConfig supplying whatever provider-specific keys a manifest
// declares. That is what makes this a migration rather than a data rewrite.
func RawFromParams(params types.WebSearchProviderParameters) map[string]any {
	raw := map[string]any{}
	if params.APIKey != "" {
		raw["api_key"] = params.APIKey
	}
	if params.EngineID != "" {
		raw["engine_id"] = params.EngineID
	}
	if params.BaseURL != "" {
		raw["base_url"] = params.BaseURL
	}
	if params.ProxyURL != "" {
		raw["proxy_url"] = params.ProxyURL
	}
	for key, value := range params.ExtraConfig {
		if _, taken := raw[key]; !taken && value != "" {
			raw[key] = value
		}
	}
	return raw
}

// declarativeProvider runs a manifest-described provider.
type declarativeProvider struct {
	entry  *plugin.Entry
	config plugin.Config
}

// Name reports the provider id, which is what results are attributed to.
func (p *declarativeProvider) Name() string { return p.entry.Manifest.ID }

// Search performs the call the manifest describes and converts the mapped
// records into web search results.
func (p *declarativeProvider) Search(
	ctx context.Context, query string, maxResults int, includeDate bool,
) ([]*types.WebSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("%s: query is empty", p.entry.Manifest.Ref())
	}
	if maxResults <= 0 {
		maxResults = 10
	}

	records, err := p.entry.Invoke(ctx, p.config, map[string]any{
		InputQuery:       query,
		InputMaxResults:  maxResults,
		InputIncludeDate: includeDate,
	})
	if err != nil {
		return nil, err
	}

	results := make([]*types.WebSearchResult, 0, len(records))
	for _, record := range records {
		// A record without a link is not a usable search result, and silently
		// returning one would put an empty citation in front of a user.
		link := record.Get(FieldURL)
		if link == "" {
			continue
		}
		result := &types.WebSearchResult{
			Title:   record.Get(FieldTitle),
			URL:     link,
			Snippet: record.Get(FieldSnippet),
			Content: record.Get(FieldContent),
			Source:  p.entry.Manifest.ID,
		}
		if stamp := record.Get(FieldPublishedAt); stamp != "" {
			if parsed, err := time.Parse(time.RFC3339, stamp); err == nil {
				result.PublishedAt = &parsed
			}
		}
		results = append(results, result)
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
