package web_search

import (
	"context"

	"github.com/Tencent/WeKnora/internal/plugin"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Web search on the plugin kernel.
//
// This domain is what the kernel looked like before it existed: a registry
// mapping an id to a factory, with no schema, no health, and no metadata. Each
// provider validated its own configuration inside its constructor with a
// hand-written "API key is required", the settings form was built from a list
// maintained separately, and adding a provider meant editing the dependency
// container.
//
// Declaring the same facts instead gives all three surfaces at once: the
// kernel validates before a constructor runs, renders the form, and reports
// health. What is left per provider is the declaration and the constructor
// that was already there.

// Kind is the web-search capability.
const Kind = plugin.Kind("websearch")

// Plugins is the registry of web search providers.
var Plugins = plugin.NewRegistry[interfaces.WebSearchProvider](Kind)

// Configuration field ids. They match the stored parameter names so an
// existing configuration loads unchanged.
const (
	FieldAPIKey   = "api_key"
	FieldEngineID = "engine_id"
	FieldBaseURL  = "base_url"
	FieldProxyURL = "proxy_url"

	// Provider-specific options. They were previously undocumented keys inside
	// the ExtraConfig map, readable only by finding the constructor that
	// parsed them; declaring them makes each one validated and renderable.
	FieldSearchEngine = "search_engine"
	FieldContentSize  = "content_size"
	FieldScope        = "scope"
	FieldIncludeText  = "include_text"
)

// Factory builds a provider from the stored parameters. It is the signature
// the existing constructors already have, so migrating a provider is a
// declaration rather than a rewrite.
type Factory func(params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error)

// Definition is what one provider declares.
type Definition struct {
	// ID is the stored provider type, e.g. "tavily".
	ID string
	// DisplayName is the human-readable name.
	DisplayName string
	// Fields are the provider-specific inputs. The shared proxy field is
	// appended automatically, since every provider accepts one.
	Fields []plugin.Field
	// Tags mark optional capabilities for callers that route by feature.
	Tags []string
	// DocURL links the provider's API documentation.
	DocURL string
	// New builds the provider.
	New Factory
	// Validate optionally performs the provider-specific checks a schema
	// cannot express, such as verifying a self-hosted URL is absolute. It runs
	// during Probe, so a settings page can report the problem before saving.
	Validate func(params types.WebSearchProviderParameters) error
}

// Register declares a provider and adds it to the registry. Providers call it
// from init in their own file, so adding one no longer means editing a central
// wiring list.
func Register(def Definition) plugin.Registration {
	return Plugins.MustRegister(&providerPlugin{def: def})
}

// APIKeyField declares the credential nearly every provider needs.
func APIKeyField(required bool) plugin.Field {
	return plugin.Field{
		ID:       FieldAPIKey,
		Kind:     plugin.KindString,
		Required: required,
		Secret:   true,
		UI: plugin.FieldUI{
			Group:    "credentials",
			LabelKey: "websearch.field.apiKey.label",
			Order:    10,
		},
	}
}

// BaseURLField declares the endpoint a self-hosted provider needs.
func BaseURLField(required bool) plugin.Field {
	return plugin.Field{
		ID:       FieldBaseURL,
		Kind:     plugin.KindString,
		Required: required,
		UI: plugin.FieldUI{
			Group:    "endpoint",
			LabelKey: "websearch.field.baseUrl.label",
			HelpKey:  "websearch.field.baseUrl.help",
			Order:    10,
		},
	}
}

// EngineIDField declares the Custom Search Engine id Google requires.
func EngineIDField() plugin.Field {
	return plugin.Field{
		ID:       FieldEngineID,
		Kind:     plugin.KindString,
		Required: true,
		UI: plugin.FieldUI{
			Group:    "credentials",
			LabelKey: "websearch.field.engineId.label",
			Order:    20,
		},
	}
}

// OptionField declares a provider-specific option with its vocabulary.
func OptionField(id string, order int, defaultValue string, values ...string) plugin.Field {
	options := make([]plugin.EnumOption, 0, len(values))
	for _, value := range values {
		options = append(options, plugin.EnumOption{
			Value:    value,
			LabelKey: "websearch.option." + id + "." + value,
		})
	}
	field := plugin.Field{
		ID:   id,
		Kind: plugin.KindEnum,
		Enum: options,
		UI: plugin.FieldUI{
			Group:    "options",
			LabelKey: "websearch.field." + id + ".label",
			Order:    order,
		},
	}
	if defaultValue != "" {
		field.Default = plugin.Ptr(plugin.EnumValue(defaultValue))
	}
	return field
}

// FlagField declares a provider-specific boolean option.
func FlagField(id string, order int) plugin.Field {
	return plugin.Field{
		ID:   id,
		Kind: plugin.KindBool,
		UI: plugin.FieldUI{
			Group:    "options",
			LabelKey: "websearch.field." + id + ".label",
			Order:    order,
		},
	}
}

// proxyField is appended to every provider: outbound traffic may need a proxy
// regardless of which API is behind it.
func proxyField() plugin.Field {
	return plugin.Field{
		ID:   FieldProxyURL,
		Kind: plugin.KindString,
		UI: plugin.FieldUI{
			Group:    "endpoint",
			LabelKey: "websearch.field.proxyUrl.label",
			HelpKey:  "websearch.field.proxyUrl.help",
			Order:    90,
		},
	}
}

// providerPlugin adapts a Definition to the kernel's plugin contract.
type providerPlugin struct{ def Definition }

func (p *providerPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		Kind:        Kind,
		ID:          p.def.ID,
		DisplayName: p.def.DisplayName,
		SummaryKey:  "websearch.provider." + p.def.ID + ".summary",
		DocURL:      p.def.DocURL,
		Tags:        p.def.Tags,
	}
}

func (p *providerPlugin) Schema() plugin.Schema {
	fields := make([]plugin.Field, 0, len(p.def.Fields)+1)
	fields = append(fields, p.def.Fields...)
	fields = append(fields, proxyField())
	return plugin.Schema{Fields: fields}
}

func (p *providerPlugin) Probe(_ context.Context, cfg plugin.Config) plugin.Health {
	if p.def.Validate == nil {
		return plugin.Healthy()
	}
	if err := p.def.Validate(ParamsFromConfig(cfg)); err != nil {
		return plugin.Unhealthy("websearch.invalidConfig", err.Error())
	}
	return plugin.Healthy()
}

func (p *providerPlugin) New(_ context.Context, cfg plugin.Config) (interfaces.WebSearchProvider, error) {
	return p.def.New(ParamsFromConfig(cfg))
}

// ParamsFromConfig converts a validated configuration into the stored
// parameter struct the provider constructors already accept.
//
// Provider-specific options travel back through ExtraConfig because that is
// what the constructors read. The difference from before is that they are
// declared fields now: validated against their vocabulary and rendered as
// form controls, instead of undocumented map keys a caller had to know about.
// ExtraConfig has become a transport detail between this adapter and an
// untouched constructor rather than an open passthrough.
func ParamsFromConfig(cfg plugin.Config) types.WebSearchProviderParameters {
	params := types.WebSearchProviderParameters{
		APIKey:   cfg.String(FieldAPIKey),
		EngineID: cfg.String(FieldEngineID),
		BaseURL:  cfg.String(FieldBaseURL),
		ProxyURL: cfg.String(FieldProxyURL),
	}
	for _, id := range optionFieldIDs {
		if !cfg.Has(id) {
			continue
		}
		value, _ := cfg.Value(id)
		if params.ExtraConfig == nil {
			params.ExtraConfig = map[string]string{}
		}
		params.ExtraConfig[id] = value.String()
	}
	return params
}

// optionFieldIDs are the provider-specific option fields that reach a
// constructor through ExtraConfig.
var optionFieldIDs = []string{
	FieldSearchEngine, FieldContentSize, FieldScope, FieldIncludeText,
}

// RawFromParams converts stored parameters into the loosely typed map the
// kernel validates, so an existing configuration reaches the new path without
// a data migration.
func RawFromParams(params types.WebSearchProviderParameters) map[string]any {
	raw := map[string]any{
		FieldAPIKey:   params.APIKey,
		FieldEngineID: params.EngineID,
		FieldBaseURL:  params.BaseURL,
		FieldProxyURL: params.ProxyURL,
	}
	for key, value := range params.ExtraConfig {
		if _, declared := raw[key]; !declared {
			raw[key] = value
		}
	}
	return raw
}

// Open builds a provider of the given type from stored parameters. It is the
// replacement for the old registry's CreateProvider, and unlike it the
// configuration is validated against the provider's schema first.
func Open(ctx context.Context, providerType string, params types.WebSearchProviderParameters) (interfaces.WebSearchProvider, error) {
	return Plugins.Open(ctx, providerType, RawFromParams(params))
}

// Probe reports whether a provider would work with the given parameters,
// without building it.
func Probe(ctx context.Context, providerType string, params types.WebSearchProviderParameters) plugin.Health {
	return Plugins.Probe(ctx, providerType, RawFromParams(params))
}

// Field and Params are local aliases so a provider declaration reads without
// repeating two package qualifiers on every line.
type (
	Field  = plugin.Field
	Params = types.WebSearchProviderParameters
)
