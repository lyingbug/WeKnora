package plugin

import (
	"testing"
)

func TestContextProvideOverrideAndRestore(t *testing.T) {
	root := NewContext()
	root.Provide("reg", "root-value")

	child := root.Isolate()
	child.Provide("reg", "child-value")
	child.Provide("local", 42)

	if v, err := Service[string](root, "reg"); err != nil || v != "child-value" {
		t.Fatalf("shared override = %q, %v", v, err)
	}
	if v, err := Service[int](root, "local"); err != nil || v != 42 {
		t.Fatalf("shared local = %d, %v", v, err)
	}

	child.Unload()
	if v, err := Service[string](root, "reg"); err != nil || v != "root-value" {
		t.Fatalf("after unload, reg = %q, %v", v, err)
	}
	if _, err := Service[int](root, "local"); err == nil {
		t.Fatal("child-provided service should be gone after unload")
	}
}

func TestContextEffectUnwinds(t *testing.T) {
	ctx := NewContext()
	var order []string
	_ = ctx.Effect(func() Disposable {
		order = append(order, "a")
		return DisposeFunc(func() { order = append(order, "dispose-a") })
	})
	_ = ctx.Effect(func() Disposable {
		order = append(order, "b")
		return DisposeFunc(func() { order = append(order, "dispose-b") })
	})
	ctx.Unload()
	want := []string{"a", "b", "dispose-b", "dispose-a"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestServiceWrongType(t *testing.T) {
	ctx := NewContext()
	ctx.Provide("n", 1)
	if _, err := Service[string](ctx, "n"); err == nil {
		t.Fatal("expected type error")
	}
}

func TestRegisterFirstWins(t *testing.T) {
	name := "test.first-wins"
	Register(name, func(Config) (Plugin, error) {
		return Func{ID: "first"}, nil
	})
	Register(name, func(Config) (Plugin, error) {
		return Func{ID: "second"}, nil
	})
	f, err := MustLookupFactory(name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f(nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "first" {
		t.Fatalf("name = %q, want first", p.Name())
	}
}

func TestMustLookupFactoryMissing(t *testing.T) {
	if _, err := MustLookupFactory("does-not-exist"); err == nil {
		t.Fatal("expected error")
	}
}

func TestConfigHelpers(t *testing.T) {
	var empty Config
	if empty.String("x") != "" || empty.Bool("y") {
		t.Fatal("empty config should return zero values")
	}
	c := Config{"name": "echo", "on": true, "n": 1}
	if c.String("name") != "echo" || !c.Bool("on") || c.String("n") != "" {
		t.Fatalf("helpers = %q %v %q", c.String("name"), c.Bool("on"), c.String("n"))
	}
}

func TestFuncPluginNilApply(t *testing.T) {
	p := Func{ID: "noop"}
	if err := p.Apply(NewContext()); err != nil {
		t.Fatal(err)
	}
	if p.Name() != "noop" || p.Inject() != nil {
		t.Fatalf("unexpected func plugin fields")
	}
}

func TestServiceMissing(t *testing.T) {
	_, err := Service[string](NewContext(), "missing")
	if err == nil {
		t.Fatal("expected missing service error")
	}
}
