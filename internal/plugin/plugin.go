// Package plugin is a Cordis-inspired composition kernel for WeKnora.
//
// A plugin contributes services, typed events, and reversible registrations
// to a shared Context. There is no privileged core to patch: new behavior
// mounts beside existing plugins. See docs/dev/plugin-architecture.md.
package plugin

// Plugin is a unit of composition. Name identifies the plugin in dumps and
// logs. Inject lists service keys that must exist before Apply runs. Apply
// registers services, event listeners, and other reversible effects.
type Plugin interface {
	Name() string
	Inject() []string
	Apply(ctx *Context) error
}

// Func is a function-shaped plugin. Use it for small in-tree adapters that
// do not need their own type.
type Func struct {
	ID         string
	InjectKeys []string
	ApplyFn    func(ctx *Context) error
}

// Name implements Plugin.
func (f Func) Name() string { return f.ID }

// Inject implements Plugin.
func (f Func) Inject() []string { return f.InjectKeys }

// Apply implements Plugin.
func (f Func) Apply(ctx *Context) error {
	if f.ApplyFn == nil {
		return nil
	}
	return f.ApplyFn(ctx)
}

// Factory constructs a plugin from declarative config (YAML / env overlay).
type Factory func(cfg Config) (Plugin, error)

// Config is a plugin's declarative configuration bag.
type Config map[string]any

// String returns a string config value, or empty if missing / not a string.
func (c Config) String(key string) string {
	if c == nil {
		return ""
	}
	v, ok := c[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// Bool returns a bool config value, or false if missing / not a bool.
func (c Config) Bool(key string) bool {
	if c == nil {
		return false
	}
	v, ok := c[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// Merge returns a new Config with overlay keys winning.
func (c Config) Merge(overlay Config) Config {
	out := Config{}
	for k, v := range c {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}
