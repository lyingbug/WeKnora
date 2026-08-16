package plugin

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestEmitAndDispose(t *testing.T) {
	bus := NewEventBus()
	var n atomic.Int32
	d := bus.On("ping", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		n.Add(1)
		return next(payload)
	})
	bus.Emit(context.Background(), "ping", nil)
	d.Dispose()
	bus.Emit(context.Background(), "ping", nil)
	if n.Load() != 1 {
		t.Fatalf("n = %d, want 1", n.Load())
	}
}

func TestWaterfallShortCircuit(t *testing.T) {
	bus := NewEventBus()
	var seen []string
	bus.On("wf", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		seen = append(seen, "outer")
		return next(payload)
	})
	bus.On("wf", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		seen = append(seen, "block")
		return "stopped", nil
	})
	bus.On("wf", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		seen = append(seen, "inner")
		return next(payload)
	})
	out, err := bus.Waterfall(context.Background(), "wf", "start")
	if err != nil {
		t.Fatal(err)
	}
	if out != "stopped" {
		t.Fatalf("out = %v", out)
	}
	if len(seen) != 2 || seen[0] != "outer" || seen[1] != "block" {
		t.Fatalf("seen = %v", seen)
	}
}

func TestWaterfallRewrite(t *testing.T) {
	bus := NewEventBus()
	bus.On("wf", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		return next(payload.(string) + "-a")
	})
	bus.On("wf", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		return next(payload.(string) + "-b")
	})
	out, err := bus.Waterfall(context.Background(), "wf", "x")
	if err != nil {
		t.Fatal(err)
	}
	if out != "x-a-b" {
		t.Fatalf("out = %v", out)
	}
}

func TestPrependRunsFirst(t *testing.T) {
	bus := NewEventBus()
	var seen []string
	bus.On("e", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		seen = append(seen, "normal")
		return next(payload)
	})
	bus.Prepend("e", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		seen = append(seen, "pre")
		return next(payload)
	})
	_, _ = bus.Waterfall(context.Background(), "e", nil)
	if len(seen) != 2 || seen[0] != "pre" || seen[1] != "normal" {
		t.Fatalf("seen = %v", seen)
	}
}

func TestParallelCollectsError(t *testing.T) {
	bus := NewEventBus()
	want := errors.New("boom")
	bus.On("p", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		return nil, want
	})
	bus.On("p", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		return next(payload)
	})
	if err := bus.Parallel(context.Background(), "p", nil); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestSerialReplacesPayload(t *testing.T) {
	bus := NewEventBus()
	bus.On("s", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		return payload.(int) + 1, nil
	})
	bus.On("s", func(ctx context.Context, payload any, next func(any) (any, error)) (any, error) {
		return payload.(int) * 10, nil
	})
	out, err := bus.Serial(context.Background(), "s", 2)
	if err != nil {
		t.Fatal(err)
	}
	if out != 30 {
		t.Fatalf("out = %v, want 30", out)
	}
}

func TestEmptyDispatch(t *testing.T) {
	bus := NewEventBus()
	bus.Emit(context.Background(), "none", nil)
	out, err := bus.Waterfall(context.Background(), "none", "keep")
	if err != nil || out != "keep" {
		t.Fatalf("waterfall empty = %v %v", out, err)
	}
	if err := bus.Parallel(context.Background(), "none", nil); err != nil {
		t.Fatal(err)
	}
	out, err = bus.Serial(context.Background(), "none", "keep")
	if err != nil || out != "keep" {
		t.Fatalf("serial empty = %v %v", out, err)
	}
}
