// Package plugin is WeKnora's hot-pluggable extension system.
//
// A plugin is a YAML file, not Go code. Dropping one into the plugin
// directory adds a capability to a running server; deleting the file removes
// it. No recompilation, no restart, no fork of this repository.
//
// That constraint is the whole design. An extension point that requires
// writing Go, rebuilding the binary, and redeploying is not a plugin system —
// it is organized hardcoding, and only the people who can ship a release can
// participate. Making the unit of extension a file means anyone who can read
// a vendor's API documentation can add support for it.
//
// # What a plugin looks like
//
//	apiVersion: weknora/v1
//	kind: websearch
//	id: tavily
//	name: Tavily
//	config:
//	  - id: api_key
//	    type: string
//	    required: true
//	    secret: true
//	runtime:
//	  type: http
//	  request:
//	    method: POST
//	    url: https://api.tavily.com/search
//	    body:
//	      api_key: ${config.api_key}
//	      query: ${input.query}
//	      max_results: ${input.max_results}
//	  response:
//	    items: results
//	    fields:
//	      title: title
//	      url: url
//	      snippet: content
//
// # Why declarative HTTP is the primary format
//
// Nearly every integration in this codebase is the same shape: send an HTTP
// request built from configuration and input, read values out of a JSON
// response. Web search providers are all of that shape, and so are most model
// vendors. Expressing that shape as data rather than code covers the majority
// case with no compilation at all.
//
// The minority that needs real logic — scraping HTML, signing requests with a
// custom algorithm — declares runtime type `native` and binds to an
// implementation compiled into the binary. Those still ship a manifest, so the
// registry, the settings form, and the catalog treat every plugin the same way
// regardless of how it executes.
package plugin

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// APIVersion is the manifest format version. A manifest declaring an unknown
// version is rejected rather than guessed at, so a future format change cannot
// be silently misread as the current one.
const APIVersion = "weknora/v1"

// Manifest is one plugin, as authored in a YAML file.
type Manifest struct {
	// APIVersion pins the manifest format.
	APIVersion string `yaml:"apiVersion" json:"api_version"`
	// Kind is the capability domain, such as "websearch". A domain defines
	// what the plugin's input and output mean; the kernel does not.
	Kind string `yaml:"kind" json:"kind"`
	// ID is unique within a kind and is what a stored configuration
	// references. It is explicit, never inferred from a URL or a name.
	ID string `yaml:"id" json:"id"`
	// Name is the human-readable label.
	Name string `yaml:"name" json:"name"`
	// Description explains the plugin in one line.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// DocURL links the upstream API documentation the manifest encodes, so a
	// reviewer can check the mapping against the vendor's own reference.
	DocURL string `yaml:"docURL,omitempty" json:"doc_url,omitempty"`
	// Config declares what an operator must supply. It drives validation and
	// the settings form from one declaration.
	Config []ConfigField `yaml:"config,omitempty" json:"config,omitempty"`
	// Runtime declares how the plugin executes.
	Runtime Runtime `yaml:"runtime" json:"runtime"`

	// Source records where the manifest was loaded from, for diagnostics. It
	// is set by the loader and ignored in the file.
	Source string `yaml:"-" json:"source,omitempty"`
}

// Runtime selects how a plugin executes.
//
// The HTTP fields sit directly on the runtime rather than under a nested
// `http:` key. `type: http` already says which fields apply, and a manifest is
// read and written by hand often enough that one less level of indentation is
// worth more than the symmetry.
type Runtime struct {
	// Type is "http" for a declarative HTTP integration, or "native" for one
	// compiled into the binary.
	Type string `yaml:"type" json:"type"`
	// Request describes the outbound call of an http runtime.
	Request HTTPRequest `yaml:"request,omitempty" json:"request,omitempty"`
	// Response describes how to read the reply of an http runtime.
	Response HTTPResponse `yaml:"response,omitempty" json:"response,omitempty"`
	// Native names a compiled-in implementation registered under this kind.
	Native string `yaml:"native,omitempty" json:"native,omitempty"`
}

// Runtime type values.
const (
	// RuntimeHTTP is a declarative HTTP integration, described entirely by
	// the manifest.
	RuntimeHTTP = "http"
	// RuntimeNative binds to an implementation compiled into the binary, for
	// the integrations that need real logic. It is the escape hatch, not the
	// norm: a native plugin cannot be added without a release.
	RuntimeNative = "native"
)

// HTTPRequest is the outbound call. Every string may contain ${...}
// expressions referring to config and input values.
type HTTPRequest struct {
	// Method defaults to GET.
	Method string `yaml:"method,omitempty" json:"method,omitempty"`
	// URL is the endpoint.
	URL string `yaml:"url" json:"url"`
	// Headers are sent as-is after expression resolution.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	// Query are URL query parameters. A parameter resolving to an empty value
	// is omitted, so an optional input does not become "?scope=".
	Query map[string]string `yaml:"query,omitempty" json:"query,omitempty"`
	// Body is a structured JSON body.
	//
	// It is structured rather than a text template on purpose. A template
	// splices text, so a search query containing a quote would break the JSON
	// or, worse, inject into it. Resolving expressions into typed values and
	// marshalling the result makes that class of bug impossible, which matters
	// when the author of a manifest is not necessarily a Go programmer.
	Body any `yaml:"body,omitempty" json:"body,omitempty"`
	// Timeout bounds the call, e.g. "15s". Defaults to DefaultTimeout.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// HTTPResponse describes how to read a reply.
type HTTPResponse struct {
	// Items is a path to the array of results. An empty path treats the whole
	// response as a single result.
	Items string `yaml:"items,omitempty" json:"items,omitempty"`
	// Fields maps an output name the domain understands to a path within one
	// item. Which names a domain expects is the domain's contract.
	Fields map[string]string `yaml:"fields,omitempty" json:"fields,omitempty"`
	// ErrorPath optionally points at an error message in the reply. A vendor
	// that answers HTTP 200 with an error object is common enough to deserve
	// a declaration rather than a per-plugin workaround.
	ErrorPath string `yaml:"errorPath,omitempty" json:"error_path,omitempty"`
}

// DefaultTimeout bounds an HTTP plugin call that declares no timeout.
const DefaultTimeout = 15 * time.Second

// ResolvedTimeout reports the declared timeout, or the default.
func (r HTTPRequest) ResolvedTimeout() time.Duration {
	if r.Timeout == "" {
		return DefaultTimeout
	}
	d, err := time.ParseDuration(r.Timeout)
	if err != nil || d <= 0 {
		return DefaultTimeout
	}
	return d
}

// ResolvedMethod reports the HTTP method, defaulting to GET.
func (r HTTPRequest) ResolvedMethod() string {
	if r.Method == "" {
		return "GET"
	}
	return strings.ToUpper(r.Method)
}

// ParseManifest reads a manifest from YAML.
//
// Parsing is strict: an unknown field is an error rather than silence, because
// a typo in a hand-written plugin file should say so instead of quietly
// disabling the setting it was meant to configure.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks a manifest for the mistakes a plugin author actually makes.
//
// Every message names the field and says what was expected, because the reader
// of these errors is someone editing YAML without a compiler to help them.
func (m *Manifest) Validate() error {
	if m.APIVersion != APIVersion {
		return fmt.Errorf("apiVersion: expected %q, got %q", APIVersion, m.APIVersion)
	}
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("kind: required (the capability this plugin provides, e.g. websearch)")
	}
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("id: required (a unique identifier within kind %q)", m.Kind)
	}
	if strings.ContainsAny(m.ID, " \t/\\") {
		return fmt.Errorf("id %q: must not contain spaces or slashes", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name: required (the label shown in the interface)")
	}

	seen := make(map[string]struct{}, len(m.Config))
	for i, field := range m.Config {
		if _, dup := seen[field.ID]; dup {
			return fmt.Errorf("config[%d]: duplicate id %q", i, field.ID)
		}
		seen[field.ID] = struct{}{}
		if err := field.validate(); err != nil {
			return fmt.Errorf("config[%d] (%s): %w", i, field.ID, err)
		}
	}

	switch m.Runtime.Type {
	case RuntimeHTTP:
		if strings.TrimSpace(m.Runtime.Request.URL) == "" {
			return fmt.Errorf("runtime.request.url: required when runtime.type is %q", RuntimeHTTP)
		}
		if m.Runtime.Native != "" {
			return fmt.Errorf("runtime.native: must be empty when runtime.type is %q", RuntimeHTTP)
		}
	case RuntimeNative:
		if strings.TrimSpace(m.Runtime.Native) == "" {
			return fmt.Errorf("runtime.native: required when runtime.type is %q", RuntimeNative)
		}
		if m.Runtime.Request.URL != "" {
			return fmt.Errorf("runtime.request: must be empty when runtime.type is %q", RuntimeNative)
		}
	case "":
		return fmt.Errorf("runtime.type: required (%q or %q)", RuntimeHTTP, RuntimeNative)
	default:
		return fmt.Errorf("runtime.type %q: unknown (expected %q or %q)",
			m.Runtime.Type, RuntimeHTTP, RuntimeNative)
	}

	// Expressions are checked here rather than at call time so a typo is
	// reported when the file is loaded, not on the first request that uses it.
	return m.validateExpressions()
}

// Field reports a declared configuration field.
func (m *Manifest) Field(id string) (ConfigField, bool) {
	for _, f := range m.Config {
		if f.ID == id {
			return f, true
		}
	}
	return ConfigField{}, false
}

// Ref is a plugin's fully qualified identity.
func (m *Manifest) Ref() string { return m.Kind + "/" + m.ID }
