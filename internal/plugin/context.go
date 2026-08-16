package plugin

import (
	"fmt"
	"sync"
)

// Context is a repository of services plus an event bus and an effect stack.
// Child contexts created by Isolate inherit parent lookups and the shared
// event bus, but own their own effects so one plugin can unload alone.
type Context struct {
	mu       sync.RWMutex
	parent   *Context
	services map[string]any
	events   *EventBus
	effects  *EffectStack
}

// NewContext creates a root context.
func NewContext() *Context {
	return &Context{
		services: make(map[string]any),
		events:   NewEventBus(),
		effects:  newEffectStack(),
	}
}

// Isolate returns a child context. Services are shared on the root so peer
// plugins can inject them; Effect stays local so Unload only unwinds this
// child. A later per-agent realm can add a true isolated service map.
func (c *Context) Isolate() *Context {
	return &Context{
		parent:   c,
		services: make(map[string]any),
		events:   c.events,
		effects:  newEffectStack(),
	}
}

func (c *Context) root() *Context {
	for c.parent != nil {
		c = c.parent
	}
	return c
}

// Provide installs a service under key on the root context. The previous
// value (if any) is restored when the current isolate unloads.
func (c *Context) Provide(key string, svc any) {
	if key == "" {
		return
	}
	root := c.root()
	c.Effect(func() Disposable {
		root.mu.Lock()
		prev, had := root.services[key]
		root.services[key] = svc
		root.mu.Unlock()
		return DisposeFunc(func() {
			root.mu.Lock()
			defer root.mu.Unlock()
			if had {
				root.services[key] = prev
			} else {
				delete(root.services, key)
			}
		})
	})
}

// Get looks up a service on this context, then parents.
func (c *Context) Get(key string) (any, bool) {
	for cur := c; cur != nil; cur = cur.parent {
		cur.mu.RLock()
		v, ok := cur.services[key]
		cur.mu.RUnlock()
		if ok {
			return v, true
		}
	}
	return nil, false
}

// Service returns a typed service or an error if missing / wrong type.
func Service[T any](ctx *Context, key string) (T, error) {
	var zero T
	v, ok := ctx.Get(key)
	if !ok {
		return zero, fmt.Errorf("plugin: missing service %q", key)
	}
	t, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("plugin: service %q has type %T, want %T", key, v, zero)
	}
	return t, nil
}

// Effect records a reversible registration. fn runs immediately and its
// disposer is unwound on Unload.
func (c *Context) Effect(fn func() Disposable) error {
	if fn == nil {
		return nil
	}
	c.effects.Push(fn())
	return nil
}

// Events returns the shared event bus.
func (c *Context) Events() *EventBus { return c.events }

// Unload disposes every effect recorded on this context (not parents).
func (c *Context) Unload() {
	c.effects.Close()
}
