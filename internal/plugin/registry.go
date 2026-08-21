package plugin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The registry is what makes plugins hot. It holds the manifests currently in
// effect and lets a loader replace them while requests are being served.
//
// Two properties matter more than anything else here, and both come from the
// same fact: plugin files are hand-authored and will be wrong sometimes.
//
//   - A broken file must never take the registry down. It is reported and
//     skipped, and whatever was working before keeps working.
//   - A reload must be atomic per source. A request in flight sees either the
//     old set or the new one, never a half-applied directory scan.

// NativeFactory builds an instance for a manifest whose runtime is native.
// A domain registers these for the integrations that cannot be expressed
// declaratively, such as one that scrapes HTML instead of reading JSON.
type NativeFactory func(m *Manifest, cfg Config) (any, error)

// Entry is a loaded plugin: its manifest, plus whatever the runtime needs to
// execute it.
type Entry struct {
	// Manifest is the declaration as loaded.
	Manifest *Manifest
	// invoker executes an http-runtime plugin.
	invoker *HTTPInvoker
	// native builds an instance for a native-runtime plugin.
	native NativeFactory
}

// LoadError records a manifest that failed to load, so a settings page can
// show an operator what is wrong with the file they just edited instead of
// leaving them to read server logs.
type LoadError struct {
	// Source is the file the manifest came from.
	Source string `json:"source"`
	// Err is the reason it was rejected.
	Err string `json:"error"`
}

// Registry holds the plugins currently in effect.
type Registry struct {
	mu sync.RWMutex
	// entries is keyed by "kind/id".
	entries map[string]*Entry
	// origin records which source contributed each entry, so replacing one
	// source leaves the others alone.
	origin map[string]string
	// natives are the compiled-in implementations available to native
	// manifests, keyed by "kind/name".
	natives map[string]NativeFactory
	// failures are the manifests that could not be loaded, by source.
	failures map[string][]LoadError
	// observers are notified after every change.
	observers []func()
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		entries:  map[string]*Entry{},
		origin:   map[string]string{},
		natives:  map[string]NativeFactory{},
		failures: map[string][]LoadError{},
	}
}

// Default is the process-wide registry.
var Default = NewRegistry()

// RegisterNative makes a compiled-in implementation available to manifests
// that declare `runtime: {type: native, native: <name>}`.
//
// Native plugins are the escape hatch, not the norm: adding one still requires
// a release, which is exactly what the file-based format exists to avoid. They
// earn their place only where a declarative description genuinely cannot work.
func (r *Registry) RegisterNative(kind, name string, factory NativeFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.natives[kind+"/"+name] = factory
}

// Replace swaps every plugin contributed by one source.
//
// Sources are independent: replacing the contents of the user plugin directory
// does not disturb the built-in plugins, so a bad edit in one place cannot
// remove capabilities that came from another.
func (r *Registry) Replace(source string, manifests []*Manifest, failures []LoadError) {
	prepared := make(map[string]*Entry, len(manifests))
	rejected := append([]LoadError(nil), failures...)

	r.mu.RLock()
	natives := make(map[string]NativeFactory, len(r.natives))
	for name, factory := range r.natives {
		natives[name] = factory
	}
	r.mu.RUnlock()

	for _, manifest := range manifests {
		entry, err := buildEntry(manifest, natives)
		if err != nil {
			rejected = append(rejected, LoadError{Source: manifest.Source, Err: err.Error()})
			continue
		}
		prepared[manifest.Ref()] = entry
	}

	r.mu.Lock()
	for ref, origin := range r.origin {
		if origin == source {
			delete(r.entries, ref)
			delete(r.origin, ref)
		}
	}
	for ref, entry := range prepared {
		// A later source shadowing an earlier one is legitimate: it is how a
		// deployment overrides a built-in plugin by dropping a file with the
		// same id.
		r.entries[ref] = entry
		r.origin[ref] = source
	}
	if len(rejected) > 0 {
		r.failures[source] = rejected
	} else {
		delete(r.failures, source)
	}
	observers := append([]func(){}, r.observers...)
	r.mu.Unlock()

	for _, notify := range observers {
		notify()
	}
}

// buildEntry prepares a manifest for execution.
func buildEntry(m *Manifest, natives map[string]NativeFactory) (*Entry, error) {
	switch m.Runtime.Type {
	case RuntimeHTTP:
		invoker, err := NewHTTPInvoker(m)
		if err != nil {
			return nil, err
		}
		return &Entry{Manifest: m, invoker: invoker}, nil
	case RuntimeNative:
		factory, ok := natives[m.Kind+"/"+m.Runtime.Native]
		if !ok {
			return nil, fmt.Errorf("runtime.native %q: no implementation of that name is built into this server",
				m.Runtime.Native)
		}
		return &Entry{Manifest: m, native: factory}, nil
	default:
		return nil, fmt.Errorf("runtime.type %q: unknown", m.Runtime.Type)
	}
}

// Observe registers a callback invoked after every change, so a domain can
// refresh anything it caches when a plugin appears or disappears.
func (r *Registry) Observe(notify func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observers = append(r.observers, notify)
}

// Lookup reports the plugin for a kind and id.
func (r *Registry) Lookup(kind, id string) (*Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[kind+"/"+strings.TrimSpace(id)]
	return entry, ok
}

// List reports the plugins of a kind, ordered by id. An empty kind lists all.
func (r *Registry) List(kind string) []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Entry, 0, len(r.entries))
	for _, entry := range r.entries {
		if kind == "" || entry.Manifest.Kind == kind {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Manifest.Kind != out[j].Manifest.Kind {
			return out[i].Manifest.Kind < out[j].Manifest.Kind
		}
		return out[i].Manifest.ID < out[j].Manifest.ID
	})
	return out
}

// Kinds reports the capability kinds that currently have a plugin.
func (r *Registry) Kinds() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := map[string]struct{}{}
	var out []string
	for _, entry := range r.entries {
		if _, dup := seen[entry.Manifest.Kind]; dup {
			continue
		}
		seen[entry.Manifest.Kind] = struct{}{}
		out = append(out, entry.Manifest.Kind)
	}
	sort.Strings(out)
	return out
}

// Failures reports the manifests that could not be loaded, across all sources.
func (r *Registry) Failures() []LoadError {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []LoadError
	for _, errs := range r.failures {
		out = append(out, errs...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

// Configure validates a raw configuration against a plugin's manifest.
func (r *Registry) Configure(kind, id string, raw map[string]any) (Config, *Entry, error) {
	entry, ok := r.Lookup(kind, id)
	if !ok {
		return Config{}, nil, fmt.Errorf("no %s plugin named %q is installed", kind, id)
	}
	cfg, err := BuildConfig(entry.Manifest, raw)
	if err != nil {
		return Config{}, nil, err
	}
	return cfg, entry, nil
}

// Invoke runs a declarative plugin and returns its records.
func (e *Entry) Invoke(ctx context.Context, cfg Config, input map[string]any) ([]Record, error) {
	if e.invoker == nil {
		return nil, fmt.Errorf("%s: not a declarative plugin", e.Manifest.Ref())
	}
	return e.invoker.Invoke(ctx, cfg, input)
}

// NewNative builds the compiled-in implementation behind a native plugin.
func (e *Entry) NewNative(cfg Config) (any, error) {
	if e.native == nil {
		return nil, fmt.Errorf("%s: not a native plugin", e.Manifest.Ref())
	}
	return e.native(e.Manifest, cfg)
}

// IsNative reports whether the plugin executes compiled-in code.
func (e *Entry) IsNative() bool { return e.native != nil }
