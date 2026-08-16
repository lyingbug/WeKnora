package plugin

import (
	"context"
	"sync"
	"sync/atomic"
)

// Handler is around-middleware for a named event. Call next to delegate;
// return without next to short-circuit (waterfall / serial).
type Handler func(ctx context.Context, payload any, next func(any) (any, error)) (any, error)

type listener struct {
	id      uint64
	handler Handler
	prepend bool
}

// EventBus dispatches named events. Mode is chosen by the caller and is
// part of each event's public contract.
type EventBus struct {
	mu        sync.RWMutex
	seq       atomic.Uint64
	listeners map[string][]listener
}

// NewEventBus creates an empty bus.
func NewEventBus() *EventBus {
	return &EventBus{listeners: make(map[string][]listener)}
}

// On registers a listener. The returned disposer removes it.
func (b *EventBus) On(name string, h Handler) Disposable {
	return b.on(name, h, false)
}

// Prepend registers a listener that runs before ordinary registrations.
func (b *EventBus) Prepend(name string, h Handler) Disposable {
	return b.on(name, h, true)
}

func (b *EventBus) on(name string, h Handler, prepend bool) Disposable {
	if name == "" || h == nil {
		return DisposeFunc(nil)
	}
	item := listener{id: b.seq.Add(1), handler: h, prepend: prepend}
	b.mu.Lock()
	b.listeners[name] = append(b.listeners[name], item)
	b.mu.Unlock()
	return DisposeFunc(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		cur := b.listeners[name]
		out := make([]listener, 0, len(cur))
		for _, l := range cur {
			if l.id == item.id {
				continue
			}
			out = append(out, l)
		}
		if len(out) == 0 {
			delete(b.listeners, name)
		} else {
			b.listeners[name] = out
		}
	})
}

// snapshot copies listeners so dispatch does not hold the lock.
func (b *EventBus) snapshot(name string) []Handler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	raw := b.listeners[name]
	prepend := make([]Handler, 0, len(raw))
	normal := make([]Handler, 0, len(raw))
	for _, l := range raw {
		if l.prepend {
			prepend = append(prepend, l.handler)
		} else {
			normal = append(normal, l.handler)
		}
	}
	return append(prepend, normal...)
}

// Emit notifies listeners in registration order and ignores return values.
func (b *EventBus) Emit(ctx context.Context, name string, payload any) {
	for _, h := range b.snapshot(name) {
		_, _ = h(ctx, payload, func(p any) (any, error) { return p, nil })
	}
}

// Waterfall is around-middleware. Each listener receives next and must call
// it to delegate. The first listener that returns without next short-circuits.
func (b *EventBus) Waterfall(ctx context.Context, name string, payload any) (any, error) {
	handlers := b.snapshot(name)
	var run func(int, any) (any, error)
	run = func(i int, p any) (any, error) {
		if i >= len(handlers) {
			return p, nil
		}
		return handlers[i](ctx, p, func(nextPayload any) (any, error) {
			return run(i+1, nextPayload)
		})
	}
	return run(0, payload)
}

// Parallel runs every listener concurrently. next is a no-op identity.
func (b *EventBus) Parallel(ctx context.Context, name string, payload any) error {
	handlers := b.snapshot(name)
	if len(handlers) == 0 {
		return nil
	}
	errCh := make(chan error, len(handlers))
	for _, h := range handlers {
		go func(handler Handler) {
			_, err := handler(ctx, payload, func(p any) (any, error) { return p, nil })
			errCh <- err
		}(h)
	}
	var first error
	for range handlers {
		if err := <-errCh; err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Serial runs listeners in order. A listener that returns without next
// replaces the payload seen by later listeners.
func (b *EventBus) Serial(ctx context.Context, name string, payload any) (any, error) {
	cur := payload
	for _, h := range b.snapshot(name) {
		out, err := h(ctx, cur, func(p any) (any, error) {
			return p, nil
		})
		if err != nil {
			return cur, err
		}
		cur = out
	}
	return cur, nil
}
