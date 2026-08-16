package plugin

import (
	"errors"
	"testing"
)

func TestHostComposeMountAndUnload(t *testing.T) {
	var applied, disposed int
	Register("host.alpha", func(Config) (Plugin, error) {
		return Func{
			ID:         "host.alpha",
			InjectKeys: []string{"core"},
			ApplyFn: func(ctx *Context) error {
				applied++
				ctx.Provide("alpha", true)
				return ctx.Effect(func() Disposable {
					return DisposeFunc(func() { disposed++ })
				})
			},
		}, nil
	})
	Register("host.beta", func(Config) (Plugin, error) {
		return Func{
			ID:         "host.beta",
			InjectKeys: []string{"alpha"},
			ApplyFn: func(ctx *Context) error {
				applied++
				if _, err := Service[bool](ctx, "alpha"); err != nil {
					return err
				}
				return nil
			},
		}, nil
	})

	h := NewHost()
	h.Context().Provide("core", struct{}{})
	err := h.Compose(Profile{Name: "t", Bundles: []string{"base"}}, map[string]Bundle{
		"base": {Name: "base", Entries: []Entry{
			{ID: "host.beta", Plugin: "host.beta"},
			{ID: "host.alpha", Plugin: "host.alpha"},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("applied = %d", applied)
	}
	mounted := h.Mounted()
	if len(mounted) != 2 {
		t.Fatalf("mounted = %+v", mounted)
	}
	h.Unload()
	if disposed != 1 {
		t.Fatalf("disposed = %d", disposed)
	}
}

func TestHostDisabledAndMissingInject(t *testing.T) {
	Register("host.needs", func(Config) (Plugin, error) {
		return Func{ID: "host.needs", InjectKeys: []string{"missing-svc"}, ApplyFn: func(*Context) error {
			return nil
		}}, nil
	})
	h := NewHost()
	err := h.Compose(Profile{Bundles: []string{"base"}}, map[string]Bundle{
		"base": {Entries: []Entry{
			{ID: "skip.me", Plugin: "host.needs", Disabled: true},
			{ID: "host.needs", Plugin: "host.needs"},
		}},
	}, nil)
	if err == nil {
		t.Fatal("expected unsatisfied inject")
	}
	h2 := NewHost()
	if err := h2.Compose(Profile{Bundles: []string{"base"}}, map[string]Bundle{
		"base": {Entries: []Entry{{ID: "skip.me", Plugin: "host.needs", Disabled: true}}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if got := h2.Mounted(); len(got) != 1 || !got[0].Disabled {
		t.Fatalf("mounted = %+v", got)
	}
}

func TestHostApplyErrorUnwinds(t *testing.T) {
	var disposed int
	Register("host.fail", func(Config) (Plugin, error) {
		return Func{ID: "host.fail", ApplyFn: func(ctx *Context) error {
			_ = ctx.Effect(func() Disposable {
				return DisposeFunc(func() { disposed++ })
			})
			return errors.New("apply failed")
		}}, nil
	})
	h := NewHost()
	err := h.Compose(Profile{Bundles: []string{"base"}}, map[string]Bundle{
		"base": {Entries: []Entry{{ID: "host.fail"}}},
	}, nil)
	if err == nil {
		t.Fatal("expected apply error")
	}
	if disposed != 1 {
		t.Fatalf("disposed = %d, want 1 (partial apply unwound)", disposed)
	}
}

func TestHostUnknownFactory(t *testing.T) {
	h := NewHost()
	err := h.Compose(Profile{Bundles: []string{"base"}}, map[string]Bundle{
		"base": {Entries: []Entry{{ID: "no.such.plugin"}}},
	}, nil)
	if err == nil {
		t.Fatal("expected missing factory")
	}
}
