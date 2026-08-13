package spi

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry holds the registered vendor plugins and resolves one for a
// configured model.
//
// Registration is reversible: Register returns the function that removes the
// descriptor again. That keeps tests from leaking state into one another and
// leaves room for plugins whose lifetime is shorter than the process, which is
// the direction this seam is meant to grow.
type Registry struct {
	mu      sync.RWMutex
	entries []entry
	nextSeq int
}

type entry struct {
	desc Descriptor
	seq  int
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register validates and adds a descriptor, returning the function that
// removes it. A descriptor that fails validation is rejected outright rather
// than half-registered, so a malformed plugin fails at startup instead of
// producing a request the vendor will reject.
func (r *Registry) Register(desc Descriptor) (func(), error) {
	if err := desc.validate(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	seq := r.nextSeq
	r.nextSeq++
	r.entries = append(r.entries, entry{desc: desc, seq: seq})
	return func() { r.remove(seq) }, nil
}

// MustRegister adds a descriptor and panics if it is malformed. Plugin
// packages call it from init, where a descriptor is a compile-time constant of
// the program and a mistake in one is a programming error, not a runtime
// condition worth degrading around.
func (r *Registry) MustRegister(desc Descriptor) func() {
	undo, err := r.Register(desc)
	if err != nil {
		panic(fmt.Sprintf("llm/spi: %v", err))
	}
	return undo
}

func (r *Registry) remove(seq int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.entries {
		if e.seq == seq {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return
		}
	}
}

// Query selects a descriptor. It is a struct rather than a parameter list
// because the selectors grow: model-specific descriptors and protocol choice
// arrived after the first two, and a caller that ignores one should not have
// to name it.
type Query struct {
	// Vendor is the provider identity, matched case-insensitively.
	Vendor string
	// Kind is the capability required.
	Kind ModelKind
	// Model is the model name, matched against each descriptor's matcher.
	Model string
	// Protocol optionally pins the wire protocol, for vendors offering more
	// than one. Empty accepts whichever the vendor registered first, which is
	// its documented default.
	Protocol ProtocolID
}

// Resolve returns the descriptor handling a query.
//
// A vendor may register several descriptors for one kind; the one whose model
// matcher is specific wins over the vendor's catch-all, and among equally
// specific matchers the earliest registration wins. That ordering is what lets
// "OpenAI reasoning models" be a separate declaration from "OpenAI models"
// without either one knowing about the other.
func (r *Registry) Resolve(q Query) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name := strings.ToLower(strings.TrimSpace(q.Vendor))
	var best *entry
	for i := range r.entries {
		e := &r.entries[i]
		if strings.ToLower(e.desc.Vendor) != name || e.desc.Kind != q.Kind {
			continue
		}
		if q.Protocol != "" && e.desc.Protocol != q.Protocol {
			continue
		}
		if !e.desc.Models.Matches(q.Model) {
			continue
		}
		if best == nil || better(e, best) {
			best = e
		}
	}
	if best == nil {
		return Descriptor{}, false
	}
	return best.desc, true
}

// Protocols reports the distinct protocols a vendor offers for a kind, in
// registration order. The model editor uses it to decide whether to show a
// protocol choice at all: one entry means there is nothing to choose.
func (r *Registry) Protocols(vendor string, kind ModelKind) []ProtocolID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name := strings.ToLower(strings.TrimSpace(vendor))
	seen := map[ProtocolID]struct{}{}
	var out []ProtocolID
	for _, e := range r.entries {
		if strings.ToLower(e.desc.Vendor) != name || e.desc.Kind != kind {
			continue
		}
		if _, dup := seen[e.desc.Protocol]; dup {
			continue
		}
		seen[e.desc.Protocol] = struct{}{}
		out = append(out, e.desc.Protocol)
	}
	return out
}

// better reports whether candidate outranks incumbent: specific matchers beat
// catch-alls, then earlier registration wins.
func better(candidate, incumbent *entry) bool {
	candidateSpecific := !candidate.desc.Models.IsCatchAll()
	incumbentSpecific := !incumbent.desc.Models.IsCatchAll()
	if candidateSpecific != incumbentSpecific {
		return candidateSpecific
	}
	return candidate.seq < incumbent.seq
}

// List reports every descriptor registered for a kind, ordered by vendor and
// then registration. It backs the capability catalog the model editor renders.
func (r *Registry) List(kind ModelKind) []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]entry, 0, len(r.entries))
	for _, e := range r.entries {
		if e.desc.Kind == kind {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].desc.Vendor != out[j].desc.Vendor {
			return out[i].desc.Vendor < out[j].desc.Vendor
		}
		return out[i].seq < out[j].seq
	})
	descs := make([]Descriptor, 0, len(out))
	for _, e := range out {
		descs = append(descs, e.desc)
	}
	return descs
}

// Vendors reports the distinct vendors registered for a kind.
func (r *Registry) Vendors(kind ModelKind) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, desc := range r.List(kind) {
		if _, dup := seen[desc.Vendor]; dup {
			continue
		}
		seen[desc.Vendor] = struct{}{}
		out = append(out, desc.Vendor)
	}
	return out
}

// Default is the process-wide registry that vendor packages register into from
// their init functions.
var Default = NewRegistry()

// Register adds a descriptor to the default registry.
func Register(desc Descriptor) (func(), error) { return Default.Register(desc) }

// MustRegister adds a descriptor to the default registry, panicking if it is
// malformed.
func MustRegister(desc Descriptor) func() { return Default.MustRegister(desc) }

// Resolve looks up a descriptor in the default registry.
func Resolve(q Query) (Descriptor, bool) { return Default.Resolve(q) }

// List reports the default registry's descriptors for a kind.
func List(kind ModelKind) []Descriptor { return Default.List(kind) }

// Protocols reports the protocols a vendor offers for a kind in the default
// registry.
func Protocols(vendor string, kind ModelKind) []ProtocolID {
	return Default.Protocols(vendor, kind)
}
