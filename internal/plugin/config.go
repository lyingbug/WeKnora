package plugin

import (
	"fmt"
	"strconv"
	"strings"
)

// Configuration is declared in the manifest, which means one declaration both
// validates what an operator saves and tells the interface what to render.
// A plugin author never writes a validation branch, and the settings form
// cannot offer a field the plugin will reject, because there is only one
// description of the field.

// FieldType is the domain of a configuration value.
type FieldType string

const (
	// TypeString is free text.
	TypeString FieldType = "string"
	// TypeInt is a whole number.
	TypeInt FieldType = "int"
	// TypeFloat is a fractional number.
	TypeFloat FieldType = "float"
	// TypeBool is a flag.
	TypeBool FieldType = "bool"
	// TypeEnum is a closed set of allowed values.
	TypeEnum FieldType = "enum"
)

// ConfigField declares one configuration input.
type ConfigField struct {
	// ID is the key the value is stored and referenced under. A manifest
	// reads it as ${config.<id>}.
	ID string `yaml:"id" json:"id"`
	// Type is the value domain; defaults to string.
	Type FieldType `yaml:"type,omitempty" json:"type,omitempty"`
	// Label is the form label. A plugin file is authored per deployment and
	// per vendor, so it carries its own text rather than an i18n key that
	// would have to be added to the application's translation files.
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
	// Help explains the field under the control.
	Help string `yaml:"help,omitempty" json:"help,omitempty"`
	// Placeholder hints at the expected shape.
	Placeholder string `yaml:"placeholder,omitempty" json:"placeholder,omitempty"`
	// Required rejects a configuration that omits the field.
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`
	// Secret marks a credential: rendered masked, never echoed back, and
	// redacted from logs.
	Secret bool `yaml:"secret,omitempty" json:"secret,omitempty"`
	// Options is the allowed set for an enum.
	Options []Option `yaml:"options,omitempty" json:"options,omitempty"`
	// Min and Max bound a numeric field.
	Min *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	// Default is used when a configuration omits the field.
	Default any `yaml:"default,omitempty" json:"default,omitempty"`
}

// Option is one allowed value of an enum field.
type Option struct {
	Value string `yaml:"value" json:"value"`
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
}

// ResolvedType reports the field type, defaulting to string.
func (f ConfigField) ResolvedType() FieldType {
	if f.Type == "" {
		return TypeString
	}
	return f.Type
}

func (f ConfigField) validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("id is required")
	}
	switch f.ResolvedType() {
	case TypeString, TypeInt, TypeFloat, TypeBool:
		if len(f.Options) > 0 {
			return fmt.Errorf("options are only valid for type %q", TypeEnum)
		}
	case TypeEnum:
		if len(f.Options) == 0 {
			return fmt.Errorf("type %q requires at least one option", TypeEnum)
		}
	default:
		return fmt.Errorf("unknown type %q", f.Type)
	}
	if f.Required && f.Default != nil {
		return fmt.Errorf("a required field must not also declare a default")
	}
	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		return fmt.Errorf("min %v exceeds max %v", *f.Min, *f.Max)
	}
	if f.Default != nil {
		if _, err := f.coerce(f.Default); err != nil {
			return fmt.Errorf("default: %w", err)
		}
	}
	return nil
}

// coerce converts a loosely typed value to this field's type.
//
// It is lenient about representation and strict about domain. Configuration
// arrives from YAML, JSON bodies, and string maps in a database, all of which
// blur 8080 and "8080"; refusing one of those spellings would only produce
// bug reports. Refusing a value outside the declared range is different — that
// is the operator being told something useful.
func (f ConfigField) coerce(raw any) (any, error) {
	switch f.ResolvedType() {
	case TypeString:
		return toString(raw)
	case TypeEnum:
		text, err := toString(raw)
		if err != nil {
			return nil, err
		}
		for _, opt := range f.Options {
			if opt.Value == text {
				return text, nil
			}
		}
		return nil, fmt.Errorf("%q is not one of %s", text, f.optionList())
	case TypeBool:
		return toBool(raw)
	case TypeInt:
		n, err := toFloat(raw)
		if err != nil {
			return nil, err
		}
		if err := f.checkRange(n); err != nil {
			return nil, err
		}
		return int(n), nil
	case TypeFloat:
		n, err := toFloat(raw)
		if err != nil {
			return nil, err
		}
		if err := f.checkRange(n); err != nil {
			return nil, err
		}
		return n, nil
	default:
		return nil, fmt.Errorf("unknown type %q", f.Type)
	}
}

func (f ConfigField) checkRange(n float64) error {
	if f.Min != nil && n < *f.Min {
		return fmt.Errorf("%v is below the minimum of %v", n, *f.Min)
	}
	if f.Max != nil && n > *f.Max {
		return fmt.Errorf("%v is above the maximum of %v", n, *f.Max)
	}
	return nil
}

func (f ConfigField) optionList() string {
	values := make([]string, 0, len(f.Options))
	for _, opt := range f.Options {
		values = append(values, strconv.Quote(opt.Value))
	}
	return strings.Join(values, ", ")
}

func toString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	default:
		return "", fmt.Errorf("expected text, got %T", raw)
	}
}

func toBool(raw any) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, fmt.Errorf("%q is not true or false", v)
		}
		return b, nil
	default:
		return false, fmt.Errorf("expected true or false, got %T", raw)
	}
}

func toFloat(raw any) (float64, error) {
	switch v := raw.(type) {
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", raw)
	}
}

// Config is a configuration that has passed its manifest's declaration.
type Config struct {
	values   map[string]any
	manifest *Manifest
}

// BuildConfig validates a raw configuration against a manifest.
//
// All problems are reported together rather than one per attempt, because an
// operator filling in a form wants the whole list, not a guessing game.
func BuildConfig(m *Manifest, raw map[string]any) (Config, error) {
	values := make(map[string]any, len(m.Config))
	var problems []string

	for _, field := range m.Config {
		supplied, present := raw[field.ID]
		if !present || supplied == nil || supplied == "" {
			switch {
			case field.Default != nil:
				value, err := field.coerce(field.Default)
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: %v", field.ID, err))
					continue
				}
				values[field.ID] = value
			case field.Required:
				problems = append(problems, fmt.Sprintf("%s is required", field.ID))
			}
			continue
		}

		value, err := field.coerce(supplied)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", field.ID, err))
			continue
		}
		values[field.ID] = value
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("%s: %s", m.Ref(), strings.Join(problems, "; "))
	}
	return Config{values: values, manifest: m}, nil
}

// Has reports whether a value is present.
func (c Config) Has(id string) bool {
	_, ok := c.values[id]
	return ok
}

// Value reports a raw value.
func (c Config) Value(id string) (any, bool) {
	v, ok := c.values[id]
	return v, ok
}

// String reports a text value, or "" when absent.
func (c Config) String(id string) string {
	if v, ok := c.values[id]; ok {
		if s, err := toString(v); err == nil {
			return s
		}
	}
	return ""
}

// Int reports a numeric value, or 0 when absent.
func (c Config) Int(id string) int {
	if v, ok := c.values[id]; ok {
		if n, err := toFloat(v); err == nil {
			return int(n)
		}
	}
	return 0
}

// Bool reports a flag, or false when absent.
func (c Config) Bool(id string) bool {
	if v, ok := c.values[id]; ok {
		if b, err := toBool(v); err == nil {
			return b
		}
	}
	return false
}

// Redacted reports the configuration with secrets removed, for logs and for
// any response that echoes a configuration back.
func (c Config) Redacted() map[string]any {
	out := make(map[string]any, len(c.values))
	for id, v := range c.values {
		if field, ok := c.manifest.Field(id); ok && field.Secret {
			continue
		}
		out[id] = v
	}
	return out
}
