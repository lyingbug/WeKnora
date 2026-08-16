package plugin

import (
	"context"
	"fmt"
)

// Mounted describes one mounted (or disabled) row for dumps and logs.
type Mounted struct {
	ID       string
	Plugin   string
	Disabled bool
	Inject   []string
}

type liveScope struct {
	id    string
	scope *Context
}

// Host mounts plugins onto a root Context. Each plugin gets its own isolate
// so Unload of the host (or a future per-plugin unload) only unwinds that
// plugin's effects.
type Host struct {
	ctx     *Context
	mounted []Mounted
	scopes  []liveScope
}

// NewHost creates an empty host with a root context.
func NewHost() *Host {
	return &Host{ctx: NewContext()}
}

// Context returns the root context. Call Provide here for host-owned services
// (existing WeKnora registries) before Compose.
func (h *Host) Context() *Context { return h.ctx }

// Mounted returns a snapshot of composed rows.
func (h *Host) Mounted() []Mounted {
	out := make([]Mounted, len(h.mounted))
	copy(out, h.mounted)
	return out
}

// Compose stacks bundles, applies patches, then mounts enabled entries.
func (h *Host) Compose(profile Profile, bundles map[string]Bundle, extra []Patch) error {
	entries, err := StackBundles(profile.Bundles, bundles)
	if err != nil {
		return err
	}
	entries = ApplyPatches(entries, profile.Patch)
	entries = ApplyPatches(entries, extra)
	return h.mountEntries(entries)
}

func (h *Host) mountEntries(entries []Entry) error {
	pending := append([]Entry(nil), entries...)
	for len(pending) > 0 {
		progress := false
		var next []Entry
		for _, e := range pending {
			if e.Disabled {
				h.mounted = append(h.mounted, Mounted{
					ID: e.ID, Plugin: e.FactoryName(), Disabled: true,
				})
				progress = true
				continue
			}
			ok, err := h.tryMount(e)
			if err != nil {
				return err
			}
			if ok {
				progress = true
				continue
			}
			next = append(next, e)
		}
		if !progress {
			return fmt.Errorf("plugin: unsatisfied inject for %s", describePending(next))
		}
		pending = next
	}
	return nil
}

func (h *Host) tryMount(e Entry) (bool, error) {
	factory, err := MustLookupFactory(e.FactoryName())
	if err != nil {
		return false, err
	}
	p, err := factory(e.Config)
	if err != nil {
		return false, fmt.Errorf("plugin: construct %s: %w", e.ID, err)
	}
	if !h.injectSatisfied(p.Inject()) {
		return false, nil
	}
	scope := h.ctx.Isolate()
	if err := p.Apply(scope); err != nil {
		scope.Unload()
		return false, fmt.Errorf("plugin: apply %s: %w", e.ID, err)
	}
	h.scopes = append(h.scopes, liveScope{id: e.ID, scope: scope})
	h.mounted = append(h.mounted, Mounted{
		ID: e.ID, Plugin: e.FactoryName(), Inject: append([]string(nil), p.Inject()...),
	})
	h.ctx.Events().Emit(context.Background(), EventPluginMounted, e.ID)
	return true, nil
}

func (h *Host) injectSatisfied(keys []string) bool {
	for _, key := range keys {
		if _, ok := h.ctx.Get(key); !ok {
			return false
		}
	}
	return true
}

// Unload closes every plugin scope and then the root context.
func (h *Host) Unload() {
	for i := len(h.scopes) - 1; i >= 0; i-- {
		id := h.scopes[i].id
		h.scopes[i].scope.Unload()
		h.ctx.Events().Emit(context.Background(), EventPluginUnloaded, id)
	}
	h.scopes = nil
	h.ctx.Unload()
}

func describePending(entries []Entry) string {
	if len(entries) == 0 {
		return "(none)"
	}
	out := entries[0].ID
	for i := 1; i < len(entries) && i < 5; i++ {
		out += ", " + entries[i].ID
	}
	if len(entries) > 5 {
		out += ", ..."
	}
	return out
}
