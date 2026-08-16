package plugin

import (
	"fmt"
	"sync"
)

var (
	catalogMu sync.RWMutex
	catalog   = map[string]Factory{}
)

// Register adds a factory to the process-wide catalog. Duplicate names
// keep the first registration (first-wins) so a later import cannot hijack
// a built-in plugin id.
func Register(name string, factory Factory) {
	if name == "" || factory == nil {
		return
	}
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if _, exists := catalog[name]; exists {
		return
	}
	catalog[name] = factory
}

// LookupFactory returns a catalog factory by name.
func LookupFactory(name string) (Factory, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	f, ok := catalog[name]
	return f, ok
}

// CatalogNames returns registered factory names in unspecified order.
func CatalogNames() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	out := make([]string, 0, len(catalog))
	for name := range catalog {
		out = append(out, name)
	}
	return out
}

// MustLookupFactory returns a factory or an error naming the missing id.
func MustLookupFactory(name string) (Factory, error) {
	f, ok := LookupFactory(name)
	if !ok {
		return nil, fmt.Errorf("plugin: factory %q is not registered", name)
	}
	return f, nil
}
