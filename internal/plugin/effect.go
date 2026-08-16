package plugin

import "sync"

// Disposable undoes a registration. Dispose must be idempotent.
type Disposable interface {
	Dispose()
}

// DisposeFunc adapts a function to Disposable.
type DisposeFunc func()

// Dispose implements Disposable.
func (f DisposeFunc) Dispose() {
	if f != nil {
		f()
	}
}

// EffectStack records disposers and unwinds them in reverse order.
type EffectStack struct {
	mu     sync.Mutex
	items  []Disposable
	closed bool
}

func newEffectStack() *EffectStack {
	return &EffectStack{}
}

// Push records a disposer. After Close, Push is a no-op and immediately
// disposes the new item so a late registration cannot leak.
func (s *EffectStack) Push(d Disposable) {
	if d == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		d.Dispose()
		return
	}
	s.items = append(s.items, d)
	s.mu.Unlock()
}

// Close disposes every recorded effect in reverse order. Safe to call twice.
func (s *EffectStack) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	items := s.items
	s.items = nil
	s.mu.Unlock()

	for i := len(items) - 1; i >= 0; i-- {
		items[i].Dispose()
	}
}
