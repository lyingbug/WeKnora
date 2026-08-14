package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/plugin"
)

// The kernel is exercised through an invented domain rather than a real one.
// If these tests needed to know what a model or a search provider is, the
// kernel would not be domain-neutral and the whole premise would be wrong.

// Greeter is a made-up domain capability.
type Greeter interface {
	Greet(name string) string
}

type greeter struct{ prefix string }

func (g greeter) Greet(name string) string { return g.prefix + " " + name }

const kindGreeter = plugin.Kind("demo.greeter")

// politePlugin is a well-formed plugin: a required secret, a bounded number,
// a closed vocabulary, and a default.
type politePlugin struct {
	health plugin.Health
	newErr error
}

func (politePlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{
		Kind:        kindGreeter,
		ID:          "polite",
		DisplayName: "Polite greeter",
		Tags:        []string{"formal"},
	}
}

func (politePlugin) Schema() plugin.Schema {
	return plugin.Schema{Fields: []plugin.Field{
		{ID: "token", Kind: plugin.KindString, Required: true, Secret: true},
		{
			ID: "style", Kind: plugin.KindEnum,
			Enum:    []plugin.EnumOption{{Value: "hello"}, {Value: "greetings"}},
			Default: plugin.Ptr(plugin.EnumValue("hello")),
		},
		{
			ID: "volume", Kind: plugin.KindInt,
			Min: plugin.Float64(1), Max: plugin.Float64(10),
			Default: plugin.Ptr(plugin.IntValue(3)),
		},
		{ID: "internal", Kind: plugin.KindBool, UI: plugin.FieldUI{Hidden: true}},
	}}
}

func (p politePlugin) Probe(context.Context, plugin.Config) plugin.Health {
	if p.health.State == "" {
		return plugin.Healthy()
	}
	return p.health
}

func (p politePlugin) New(_ context.Context, cfg plugin.Config) (Greeter, error) {
	if p.newErr != nil {
		return nil, p.newErr
	}
	return greeter{prefix: cfg.String("style")}, nil
}

func newRegistry(t *testing.T) *plugin.Registry[Greeter] {
	t.Helper()
	return plugin.NewRegistryWithCatalog[Greeter](kindGreeter, plugin.NewCatalog())
}

func TestOpenValidatesBeforeBuilding(t *testing.T) {
	reg := newRegistry(t)
	if _, err := reg.Register(politePlugin{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, err := reg.Open(context.Background(), "polite", map[string]any{
		"token": "secret-value",
		"style": "greetings",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if greeting := got.Greet("world"); greeting != "greetings world" {
		t.Errorf("greeting = %q", greeting)
	}
}

// A plugin must never see a configuration its schema would reject, so the
// failure has to happen before New is called.
func TestOpenRejectsAnInvalidConfiguration(t *testing.T) {
	reg := newRegistry(t)
	_, _ = reg.Register(politePlugin{newErr: errors.New("New must not be reached")})

	cases := map[string]map[string]any{
		"missing required secret": {"style": "hello"},
		"value outside the vocabulary": {
			"token": "t", "style": "howdy",
		},
		"number below the minimum": {
			"token": "t", "volume": 0,
		},
		"unparseable number": {
			"token": "t", "volume": "loud",
		},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := reg.Open(context.Background(), "polite", raw); err == nil {
				t.Fatal("expected the configuration to be rejected")
			}
		})
	}
}

// Configuration arrives from JSON bodies and stored string maps, which both
// lose the distinction between 8080 and "8080". Being lenient about the
// representation is what lets stored configurations survive without migration.
func TestCoercionAcceptsTheRepresentationsConfigurationActuallyArrivesIn(t *testing.T) {
	reg := newRegistry(t)
	_, _ = reg.Register(politePlugin{})

	for _, volume := range []any{5, "5", float64(5)} {
		cfg, err := politePlugin{}.Schema().Validate(map[string]any{"token": "t", "volume": volume})
		if err != nil {
			t.Fatalf("volume %#v: %v", volume, err)
		}
		if cfg.Int("volume") != 5 {
			t.Errorf("volume %#v became %d", volume, cfg.Int("volume"))
		}
	}
	_ = reg
}

func TestDefaultsApplyWhenOmitted(t *testing.T) {
	cfg, err := politePlugin{}.Schema().Validate(map[string]any{"token": "t"})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.String("style") != "hello" {
		t.Errorf("style default = %q", cfg.String("style"))
	}
	if cfg.Int("volume") != 3 {
		t.Errorf("volume default = %d", cfg.Int("volume"))
	}
}

// A secret must not travel back out of the kernel, or an introspectable
// plugin becomes a credential leak.
func TestSecretsAreNeverEchoedBack(t *testing.T) {
	cfg, err := politePlugin{}.Schema().Validate(map[string]any{"token": "super-secret"})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	redacted := cfg.Redacted()
	if _, present := redacted["token"]; present {
		t.Error("the secret survived redaction")
	}
	if redacted["style"] != "hello" {
		t.Errorf("redaction dropped a non-secret field: %#v", redacted)
	}

	for _, group := range (politePlugin{}).Schema().Form() {
		for _, field := range group.Fields {
			if field.ID == "token" {
				if field.EffectiveWidget() != plugin.WidgetPassword {
					t.Errorf("a secret should render masked, got %q", field.EffectiveWidget())
				}
				if field.Default != nil {
					t.Error("a secret must not carry a default into the form")
				}
			}
		}
	}
}

// The form is what a settings page renders, so it must omit anything an
// operator cannot set.
func TestFormOmitsWhatAnOperatorCannotSet(t *testing.T) {
	schema := plugin.Schema{Fields: []plugin.Field{
		{ID: "visible", Kind: plugin.KindString},
		{ID: "hidden", Kind: plugin.KindString, UI: plugin.FieldUI{Hidden: true}},
		{
			ID: "pinned", Kind: plugin.KindString,
			Support: plugin.SupportPinned, Pin: plugin.Ptr(plugin.StringValue("fixed")),
		},
		{ID: "forbidden", Kind: plugin.KindString, Support: plugin.SupportForbidden},
	}}

	var rendered []string
	for _, group := range schema.Form() {
		for _, field := range group.Fields {
			rendered = append(rendered, field.ID)
		}
	}
	if len(rendered) != 1 || rendered[0] != "visible" {
		t.Errorf("form rendered %v, want only the visible field", rendered)
	}

	// A pinned field still reaches the plugin; it just is not a control.
	cfg, err := schema.Validate(map[string]any{"pinned": "ignored"})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.String("pinned") != "fixed" {
		t.Errorf("pinned value = %q, want the pin", cfg.String("pinned"))
	}
	if cfg.Has("forbidden") {
		t.Error("a forbidden field must not reach the plugin")
	}
}

// Malformed declarations are a programming error, and startup is the right
// place to find out.
func TestMalformedDeclarationsAreRejectedAtRegistration(t *testing.T) {
	cases := map[string]plugin.Schema{
		"enum without options": {Fields: []plugin.Field{
			{ID: "mode", Kind: plugin.KindEnum},
		}},
		"pin outside the domain": {Fields: []plugin.Field{
			{
				ID: "size", Kind: plugin.KindInt, Max: plugin.Float64(10),
				Support: plugin.SupportPinned, Pin: plugin.Ptr(plugin.IntValue(99)),
			},
		}},
		"required field carrying a default": {Fields: []plugin.Field{
			{
				ID: "name", Kind: plugin.KindString, Required: true,
				Default: plugin.Ptr(plugin.StringValue("x")),
			},
		}},
		"duplicate field": {Fields: []plugin.Field{
			{ID: "a", Kind: plugin.KindString},
			{ID: "a", Kind: plugin.KindString},
		}},
	}

	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			reg := newRegistry(t)
			if _, err := reg.Register(brokenPlugin{schema: schema}); err == nil {
				t.Fatal("expected registration to be rejected")
			}
		})
	}
}

type brokenPlugin struct{ schema plugin.Schema }

func (brokenPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{Kind: kindGreeter, ID: "broken"}
}
func (b brokenPlugin) Schema() plugin.Schema                            { return b.schema }
func (brokenPlugin) Probe(context.Context, plugin.Config) plugin.Health { return plugin.Healthy() }
func (brokenPlugin) New(context.Context, plugin.Config) (Greeter, error) {
	return nil, errors.New("unreachable")
}

// Registration is reversible so tests do not leak into one another and a
// plugin can outlive less than the process.
func TestRegistrationIsReversible(t *testing.T) {
	catalog := plugin.NewCatalog()
	reg := plugin.NewRegistryWithCatalog[Greeter](kindGreeter, catalog)

	undo, err := reg.Register(politePlugin{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(catalog.Entries(kindGreeter)) != 1 {
		t.Fatal("the plugin should appear in the catalog")
	}

	undo()
	if _, ok := reg.Lookup("polite"); ok {
		t.Error("the plugin should be gone from the registry")
	}
	if len(catalog.Entries(kindGreeter)) != 0 {
		t.Error("the plugin should be gone from the catalog")
	}
}

func TestDuplicateIdsAreRejected(t *testing.T) {
	reg := newRegistry(t)
	if _, err := reg.Register(politePlugin{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := reg.Register(politePlugin{}); err == nil {
		t.Error("registering the same id twice should fail")
	}
}

// Probe answers "would this work?" without building anything, and an invalid
// configuration is an unavailable verdict rather than an error, because that
// is what a settings page needs to display.
func TestProbeReportsRatherThanThrows(t *testing.T) {
	reg := newRegistry(t)
	_, _ = reg.Register(politePlugin{health: plugin.Limited("demo.noNetwork", "offline")})

	health := reg.Probe(context.Background(), "polite", map[string]any{"token": "t"})
	if health.State != plugin.Degraded || !health.OK() {
		t.Errorf("health = %+v, want a degraded but usable verdict", health)
	}

	invalid := reg.Probe(context.Background(), "polite", map[string]any{})
	if invalid.State != plugin.Unavailable {
		t.Errorf("an invalid configuration should probe unavailable, got %+v", invalid)
	}

	missing := reg.Probe(context.Background(), "nope", nil)
	if missing.State != plugin.Unavailable {
		t.Errorf("an unregistered id should probe unavailable, got %+v", missing)
	}
}

// One catalog across domains is what lets a single endpoint answer "what can
// this deployment do?" without a listing endpoint per subsystem.
func TestCatalogSpansDomains(t *testing.T) {
	catalog := plugin.NewCatalog()
	greeters := plugin.NewRegistryWithCatalog[Greeter](kindGreeter, catalog)
	if _, err := greeters.Register(politePlugin{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	const kindCounter = plugin.Kind("demo.counter")
	counters := plugin.NewRegistryWithCatalog[Greeter](kindCounter, catalog)
	if _, err := counters.Register(otherDomainPlugin{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if got := catalog.Kinds(); len(got) != 2 {
		t.Errorf("catalog kinds = %v, want both domains", got)
	}
	if got := len(catalog.Entries(kindGreeter)); got != 1 {
		t.Errorf("filtering by kind returned %d entries", got)
	}
	if got := len(catalog.Entries("")); got != 2 {
		t.Errorf("unfiltered catalog returned %d entries", got)
	}
}

type otherDomainPlugin struct{}

func (otherDomainPlugin) Manifest() plugin.Manifest {
	return plugin.Manifest{Kind: plugin.Kind("demo.counter"), ID: "tally"}
}
func (otherDomainPlugin) Schema() plugin.Schema { return plugin.Schema{} }
func (otherDomainPlugin) Probe(context.Context, plugin.Config) plugin.Health {
	return plugin.Healthy()
}
func (otherDomainPlugin) New(context.Context, plugin.Config) (Greeter, error) {
	return greeter{prefix: "count"}, nil
}

// A plugin registered under the wrong registry is a wiring mistake worth
// catching immediately.
func TestKindMismatchIsRejected(t *testing.T) {
	reg := newRegistry(t)
	if _, err := reg.Register(otherDomainPlugin{}); err == nil {
		t.Error("registering a counter into the greeter registry should fail")
	}
}

// An external plugin has a manifest and a schema but no in-process
// implementation, which is how a domain lists engines discovered over RPC in
// the same catalog as local ones.
func TestExternalPluginsJoinTheCatalog(t *testing.T) {
	catalog := plugin.NewCatalog()
	undo, err := catalog.PublishExternal(
		plugin.Manifest{Kind: kindGreeter, ID: "remote-greeter"},
		plugin.Schema{Fields: []plugin.Field{{ID: "endpoint", Kind: plugin.KindString, Required: true}}},
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	entries := catalog.Entries(kindGreeter)
	if len(entries) != 1 || !entries[0].Manifest.External {
		t.Fatalf("entries = %+v, want one external entry", entries)
	}
	undo()
	if len(catalog.Entries(kindGreeter)) != 0 {
		t.Error("withdrawing an external plugin should remove it")
	}
}
