// Package plugin is WeKnora's plugin kernel: the identity, configuration,
// health, and registration machinery every extensible subsystem shares.
//
// The kernel is deliberately domain-neutral. It does not know what a model is,
// what a search provider is, or what a parser engine is. A domain defines its
// own capability interface and registers plugins that implement it; the kernel
// supplies only what all of them turned out to need independently.
//
// That "independently" is the justification. Before this package existed the
// codebase had four separate registries — web search providers, document
// parser engines, datasource connectors, and model vendors — each with its own
// answer to the same five questions:
//
//   - What is this thing called, and what can it do?
//   - What configuration does it need, and is a given configuration valid?
//   - Is it usable right now, and if not, why not?
//   - How do I build a working instance from a configuration?
//   - How does a UI render a form for it?
//
// Four answers to five questions is four times the surface to keep correct,
// and in practice they diverged: some registries validated configuration, some
// did not; one reported availability with a reason, the rest left callers to
// guess; the model registry's rules were duplicated in the frontend by hand.
// One kernel makes those answers uniform, and makes a new pluggable subsystem
// a matter of declaring a capability interface rather than writing a fifth
// registry.
package plugin

import (
	"context"
	"fmt"
	"strings"
)

// Kind namespaces a capability domain. It is a dotted string so a domain can
// carve out sub-domains without a second registry: "llm.chat" and
// "llm.embedding" are distinct kinds sharing a prefix.
type Kind string

// String reports the kind for logging and diagnostics.
func (k Kind) String() string { return string(k) }

// Domain reports the portion of the kind before the first dot, so a UI can
// group "llm.chat" and "llm.embedding" without knowing either.
func (k Kind) Domain() string {
	if idx := strings.IndexByte(string(k), '.'); idx >= 0 {
		return string(k)[:idx]
	}
	return string(k)
}

// Manifest is the identity and capability declaration every plugin publishes.
//
// It is serializable on purpose: the same structure describes an in-tree Go
// plugin and a plugin running behind an RPC, which is what lets a subsystem
// discover remote implementations without a second description format. The
// document parser already merges locally registered engines with engines
// discovered over RPC, and had to invent its own merge because there was no
// shared manifest to merge.
type Manifest struct {
	// Kind is the capability domain this plugin serves.
	Kind Kind `json:"kind"`
	// ID is the stable identifier, unique within a kind. It is explicit
	// configuration, never inferred: a plugin chosen by sniffing a URL is a
	// plugin nobody can reliably select, and the model layer's URL detection
	// is the cautionary example.
	ID string `json:"id"`
	// DisplayName is a human-readable name for diagnostics and logs. A UI
	// prefers SummaryKey so it can translate.
	DisplayName string `json:"display_name,omitempty"`
	// SummaryKey is an i18n key describing the plugin. The kernel carries no
	// display text: language belongs to the frontend.
	SummaryKey string `json:"summary_key,omitempty"`
	// DocURL links the upstream documentation this plugin implements, so a
	// reviewer can check a claim rather than trust it.
	DocURL string `json:"doc_url,omitempty"`
	// Tags are capability markers the owning domain interprets — supported
	// file extensions for a parser, supported features for a search provider.
	// The kernel treats them as opaque so a domain can add one without
	// changing this package.
	Tags []string `json:"tags,omitempty"`
	// Deprecated, when non-empty, explains what to use instead. A deprecated
	// plugin still resolves, so existing configurations keep working.
	Deprecated string `json:"deprecated,omitempty"`
	// External marks a plugin whose implementation lives outside this process,
	// discovered from a remote catalog. It has a manifest and a schema but no
	// Go factory, and the owning domain decides how to drive it.
	External bool `json:"external,omitempty"`
}

// HasTag reports whether the manifest carries a capability tag.
func (m Manifest) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// validate checks a manifest at registration time.
func (m Manifest) validate() error {
	if strings.TrimSpace(string(m.Kind)) == "" {
		return fmt.Errorf("plugin manifest: kind is required")
	}
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("plugin manifest %s: id is required", m.Kind)
	}
	if strings.ContainsAny(m.ID, " \t/\\") {
		return fmt.Errorf("plugin manifest %s/%s: id must not contain spaces or slashes", m.Kind, m.ID)
	}
	return nil
}

// HealthState is how usable a plugin is under a given configuration.
type HealthState string

const (
	// Ready means the plugin is fully usable.
	Ready HealthState = "ready"
	// Degraded means it works with a documented limitation. A caller that
	// needs the full capability must treat this as a refusal rather than
	// rounding it up to ready.
	Degraded HealthState = "degraded"
	// Unavailable means it cannot serve requests as configured.
	Unavailable HealthState = "unavailable"
)

// Health is a plugin's own report on whether it can work right now.
//
// Reporting it as a fact, with a reason, is the difference between a settings
// page that explains why an engine is greyed out and one that silently omits
// it. The document parser registry already learned this and returns
// "(available, reason)"; making it part of the kernel means every subsystem
// gets the same honesty instead of rediscovering the need.
type Health struct {
	// State is the usability verdict.
	State HealthState `json:"state"`
	// ReasonKey is an i18n key explaining a non-ready state.
	ReasonKey string `json:"reason_key,omitempty"`
	// Detail carries specifics for an operator: the endpoint that refused,
	// the credential that is missing. It is not translated.
	Detail string `json:"detail,omitempty"`
}

// OK reports whether the plugin can serve requests at all.
func (h Health) OK() bool { return h.State == Ready || h.State == Degraded }

// Healthy returns a ready verdict.
func Healthy() Health { return Health{State: Ready} }

// Unhealthy returns an unavailable verdict with a reason.
func Unhealthy(reasonKey, detail string) Health {
	return Health{State: Unavailable, ReasonKey: reasonKey, Detail: detail}
}

// Limited returns a degraded verdict with a reason.
func Limited(reasonKey, detail string) Health {
	return Health{State: Degraded, ReasonKey: reasonKey, Detail: detail}
}

// Plugin is one implementation of a domain capability T.
//
// The three methods are the contract every subsystem needed anyway. Probe is
// separate from New because a settings page must be able to ask "would this
// work?" without building a live instance, and because the answer depends on
// the configuration rather than on the plugin alone.
type Plugin[T any] interface {
	// Manifest declares identity and capabilities.
	Manifest() Manifest
	// Schema declares the configuration this plugin accepts. It drives
	// validation and the settings form from one declaration, so a form cannot
	// offer a field the plugin will reject.
	Schema() Schema
	// Probe reports whether the plugin can work with a configuration, without
	// building an instance. Implementations that cannot check cheaply should
	// report Ready and let New fail.
	Probe(ctx context.Context, cfg Config) Health
	// New builds a configured instance. It receives a configuration the kernel
	// has already validated against Schema, so implementations do not repeat
	// the domain checks.
	New(ctx context.Context, cfg Config) (T, error)
}
