package plugin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registration is a reversible handle. Registering returns the function that
// undoes it, which keeps tests from leaking state into one another and leaves
// room for plugins whose lifetime is shorter than the process — a tenant
// enabling an integration, a remote catalog being refreshed.
type Registration func()

// Registry holds the plugins implementing one domain capability.
//
// It is generic so each domain keeps compile-time type safety: a
// Registry[WebSearchProvider] cannot hand back a chat model. The shared
// Catalog below gives the non-generic view a settings page needs, so
// genericity costs nothing at the UI boundary.
type Registry[T any] struct {
	kind    Kind
	mu      sync.RWMutex
	entries []registryEntry[T]
	nextSeq int
	catalog *Catalog
}

type registryEntry[T any] struct {
	plugin Plugin[T]
	seq    int
}

// NewRegistry returns a registry for one capability kind, publishing its
// manifests into the shared catalog.
func NewRegistry[T any](kind Kind) *Registry[T] {
	return &Registry[T]{kind: kind, catalog: DefaultCatalog}
}

// NewRegistryWithCatalog returns a registry publishing into a specific
// catalog, for tests that must not touch the process-wide one.
func NewRegistryWithCatalog[T any](kind Kind, catalog *Catalog) *Registry[T] {
	return &Registry[T]{kind: kind, catalog: catalog}
}

// Kind reports the capability this registry serves.
func (r *Registry[T]) Kind() Kind { return r.kind }

// Register validates and adds a plugin, returning the function that removes it.
//
// Validation is total: a plugin whose manifest or schema is malformed is
// rejected rather than half-registered, so a mistake surfaces at startup
// instead of as a broken settings page in production.
func (r *Registry[T]) Register(p Plugin[T]) (Registration, error) {
	manifest := p.Manifest()
	if manifest.Kind == "" {
		manifest.Kind = r.kind
	}
	if manifest.Kind != r.kind {
		return nil, fmt.Errorf("plugin %s declares kind %s but was registered under %s",
			manifest.ID, manifest.Kind, r.kind)
	}
	if err := manifest.validate(); err != nil {
		return nil, err
	}
	where := fmt.Sprintf("plugin %s/%s", manifest.Kind, manifest.ID)
	if err := p.Schema().validate(where); err != nil {
		return nil, err
	}

	r.mu.Lock()
	for _, existing := range r.entries {
		if strings.EqualFold(existing.plugin.Manifest().ID, manifest.ID) {
			r.mu.Unlock()
			return nil, fmt.Errorf("%s is already registered", where)
		}
	}
	seq := r.nextSeq
	r.nextSeq++
	r.entries = append(r.entries, registryEntry[T]{plugin: p, seq: seq})
	r.mu.Unlock()

	undoCatalog := r.catalog.publish(manifest, p.Schema())
	return func() {
		r.remove(seq)
		undoCatalog()
	}, nil
}

// MustRegister adds a plugin and panics if it is malformed. Plugin packages
// call it from init, where a declaration is a constant of the program and a
// mistake in one is a programming error rather than a runtime condition.
func (r *Registry[T]) MustRegister(p Plugin[T]) Registration {
	undo, err := r.Register(p)
	if err != nil {
		panic(fmt.Sprintf("plugin: %v", err))
	}
	return undo
}

func (r *Registry[T]) remove(seq int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.entries {
		if e.seq == seq {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return
		}
	}
}

// Lookup reports the plugin with an id.
func (r *Registry[T]) Lookup(id string) (Plugin[T], bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries {
		if strings.EqualFold(e.plugin.Manifest().ID, id) {
			return e.plugin, true
		}
	}
	return nil, false
}

// List reports every registered plugin in registration order.
func (r *Registry[T]) List() []Plugin[T] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]registryEntry[T], len(r.entries))
	copy(entries, r.entries)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].seq < entries[j].seq })

	out := make([]Plugin[T], 0, len(entries))
	for _, e := range entries {
		out = append(out, e.plugin)
	}
	return out
}

// Select reports the plugins whose manifest satisfies a predicate, for domains
// that route by capability rather than by id — a parser engine chosen by file
// extension, for instance.
func (r *Registry[T]) Select(match func(Manifest) bool) []Plugin[T] {
	var out []Plugin[T]
	for _, p := range r.List() {
		if match(p.Manifest()) {
			out = append(out, p)
		}
	}
	return out
}

// Open resolves a plugin by id, validates the configuration against its
// schema, and builds an instance.
//
// It is the one path a consumer needs, and going through it is what guarantees
// that no plugin ever receives a configuration its schema would reject. A
// consumer that reaches for Lookup and builds by hand has opted out of that
// guarantee, which is why this is the documented entry point.
func (r *Registry[T]) Open(ctx context.Context, id string, raw map[string]any) (T, error) {
	var zero T
	p, ok := r.Lookup(id)
	if !ok {
		return zero, fmt.Errorf("no %s plugin registered with id %q", r.kind, id)
	}
	cfg, err := p.Schema().Validate(raw)
	if err != nil {
		return zero, fmt.Errorf("%s/%s: %w", r.kind, id, err)
	}
	instance, err := p.New(ctx, cfg)
	if err != nil {
		return zero, fmt.Errorf("%s/%s: %w", r.kind, id, err)
	}
	return instance, nil
}

// Probe reports whether a plugin would work with a configuration, without
// building an instance. An invalid configuration is itself an unavailable
// verdict rather than an error, because that is what a settings page wants to
// display next to the field the operator got wrong.
func (r *Registry[T]) Probe(ctx context.Context, id string, raw map[string]any) Health {
	p, ok := r.Lookup(id)
	if !ok {
		return Unhealthy("plugin.notRegistered", fmt.Sprintf("no %s plugin with id %q", r.kind, id))
	}
	cfg, err := p.Schema().Validate(raw)
	if err != nil {
		return Unhealthy("plugin.invalidConfig", err.Error())
	}
	return p.Probe(ctx, cfg)
}

// CatalogEntry is the non-generic view of one registered plugin: everything a
// settings page needs, and nothing that requires knowing the capability type.
type CatalogEntry struct {
	Manifest Manifest `json:"manifest"`
	Groups   []Group  `json:"groups"`
}

// Catalog is the process-wide index of registered plugins across every domain.
//
// It exists so one endpoint can answer "what can this deployment do?" without
// a per-domain listing endpoint, and so a new pluggable subsystem appears in
// that answer by registering rather than by also touching the API layer.
type Catalog struct {
	mu      sync.RWMutex
	entries []catalogEntry
	nextSeq int
}

type catalogEntry struct {
	entry CatalogEntry
	seq   int
}

// NewCatalog returns an empty catalog.
func NewCatalog() *Catalog { return &Catalog{} }

// DefaultCatalog is the catalog registries publish into by default.
var DefaultCatalog = NewCatalog()

func (c *Catalog) publish(manifest Manifest, schema Schema) Registration {
	c.mu.Lock()
	defer c.mu.Unlock()
	seq := c.nextSeq
	c.nextSeq++
	c.entries = append(c.entries, catalogEntry{
		entry: CatalogEntry{Manifest: manifest, Groups: schema.Form()},
		seq:   seq,
	})
	return func() { c.withdraw(seq) }
}

func (c *Catalog) withdraw(seq int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, e := range c.entries {
		if e.seq == seq {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			return
		}
	}
}

// PublishExternal records a plugin discovered from a remote catalog, which has
// a manifest and a schema but no in-process implementation.
//
// The document parser already merges locally registered engines with engines
// discovered over RPC and had to hand-roll that merge; expressing an external
// plugin in the same catalog is what makes such a merge a kernel feature
// rather than a per-domain one.
func (c *Catalog) PublishExternal(manifest Manifest, schema Schema) (Registration, error) {
	manifest.External = true
	if err := manifest.validate(); err != nil {
		return nil, err
	}
	if err := schema.validate(fmt.Sprintf("external plugin %s/%s", manifest.Kind, manifest.ID)); err != nil {
		return nil, err
	}
	return c.publish(manifest, schema), nil
}

// Entries reports the catalog in registration order, optionally filtered to
// one kind. An empty kind reports everything.
func (c *Catalog) Entries(kind Kind) []CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	filtered := make([]catalogEntry, 0, len(c.entries))
	for _, e := range c.entries {
		if kind == "" || e.entry.Manifest.Kind == kind {
			filtered = append(filtered, e)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].seq < filtered[j].seq })

	out := make([]CatalogEntry, 0, len(filtered))
	for _, e := range filtered {
		out = append(out, e.entry)
	}
	return out
}

// Kinds reports the capability kinds that have at least one plugin.
func (c *Catalog) Kinds() []Kind {
	c.mu.RLock()
	defer c.mu.RUnlock()

	seen := map[Kind]struct{}{}
	var out []Kind
	for _, e := range c.entries {
		if _, dup := seen[e.entry.Manifest.Kind]; dup {
			continue
		}
		seen[e.entry.Manifest.Kind] = struct{}{}
		out = append(out, e.entry.Manifest.Kind)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
