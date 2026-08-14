package plugin

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Configuration is the kernel's second job, and the one the previous
// registries handled worst. Web search providers took a typed Go struct that
// no UI could introspect; document parser engines took a
// map[string]string of magic keys checked by hand; the model layer took both,
// plus an extra_config map whose keys were documented only in a Vue file.
//
// A schema replaces all three: one declaration that validates a configuration
// and renders its form, so a settings page cannot offer a field the plugin
// will reject, and a plugin cannot read a key the form never collected.

// ValueKind is the domain of a configuration value.
type ValueKind string

const (
	// KindString is free text.
	KindString ValueKind = "string"
	// KindBool is a flag.
	KindBool ValueKind = "bool"
	// KindInt is a whole number.
	KindInt ValueKind = "int"
	// KindFloat is a fractional number.
	KindFloat ValueKind = "float"
	// KindEnum is a closed vocabulary. The vocabulary belongs to the plugin,
	// not to a shared normalization: two vendors' effort ladders genuinely
	// differ, and pretending otherwise sends values one of them rejects.
	KindEnum ValueKind = "enum"
)

// Value is a configuration value tagged with its kind. It is a concrete struct
// rather than `any` because validators, encoders, and the form renderer all
// need the shape without a type switch at every access.
type Value struct {
	Kind ValueKind `json:"kind"`
	Str  string    `json:"str,omitempty"`
	Bool bool      `json:"bool,omitempty"`
	Num  float64   `json:"num,omitempty"`
}

// StringValue returns a text value.
func StringValue(v string) Value { return Value{Kind: KindString, Str: v} }

// BoolValue returns a flag value.
func BoolValue(v bool) Value { return Value{Kind: KindBool, Bool: v} }

// IntValue returns a whole-number value.
func IntValue(v int) Value { return Value{Kind: KindInt, Num: float64(v)} }

// FloatValue returns a fractional value.
func FloatValue(v float64) Value { return Value{Kind: KindFloat, Num: v} }

// EnumValue returns a value drawn from a plugin's vocabulary.
func EnumValue(v string) Value { return Value{Kind: KindEnum, Str: v} }

// Text reports the value as a string. The second result is false when the
// value is neither text nor an enum member.
func (v Value) Text() (string, bool) {
	if v.Kind != KindString && v.Kind != KindEnum {
		return "", false
	}
	return v.Str, true
}

// Int reports the value as a whole number, truncating a float toward zero.
func (v Value) Int() (int, bool) {
	if v.Kind != KindInt && v.Kind != KindFloat {
		return 0, false
	}
	return int(math.Trunc(v.Num)), true
}

// Float reports the value as a float.
func (v Value) Float() (float64, bool) {
	if v.Kind != KindInt && v.Kind != KindFloat {
		return 0, false
	}
	return v.Num, true
}

// Flag reports the value as a boolean.
func (v Value) Flag() (bool, bool) {
	if v.Kind != KindBool {
		return false, false
	}
	return v.Bool, true
}

// JSON reports the value as it should appear in a JSON document.
func (v Value) JSON() any {
	switch v.Kind {
	case KindString, KindEnum:
		return v.Str
	case KindBool:
		return v.Bool
	case KindInt:
		return int(math.Trunc(v.Num))
	case KindFloat:
		return v.Num
	default:
		return nil
	}
}

// String renders the value for logs and diagnostics.
func (v Value) String() string {
	switch v.Kind {
	case KindString, KindEnum:
		return v.Str
	case KindBool:
		return strconv.FormatBool(v.Bool)
	case KindInt:
		return strconv.Itoa(int(math.Trunc(v.Num)))
	case KindFloat:
		return strconv.FormatFloat(v.Num, 'g', -1, 64)
	default:
		return ""
	}
}

// Coerce converts a loosely typed value — as it arrives from JSON, a form, or
// a stored string map — into a value of the requested kind.
//
// It is lenient about representation and strict about domain: "8080" becomes
// an int because HTTP and YAML both lose the distinction, while "maybe" does
// not become a bool. Being lenient here is what lets stored configurations
// survive without a migration.
func Coerce(kind ValueKind, raw any) (Value, error) {
	switch typed := raw.(type) {
	case Value:
		if typed.Kind == kind {
			return typed, nil
		}
		return Coerce(kind, typed.JSON())
	case nil:
		return Value{}, fmt.Errorf("value is missing")
	case string:
		return coerceString(kind, typed)
	case bool:
		if kind != KindBool {
			return Value{}, fmt.Errorf("expected %s, got a boolean", kind)
		}
		return BoolValue(typed), nil
	case float64:
		return coerceNumber(kind, typed)
	case float32:
		return coerceNumber(kind, float64(typed))
	case int:
		return coerceNumber(kind, float64(typed))
	case int64:
		return coerceNumber(kind, float64(typed))
	default:
		return Value{}, fmt.Errorf("cannot read %T as %s", raw, kind)
	}
}

func coerceString(kind ValueKind, raw string) (Value, error) {
	trimmed := strings.TrimSpace(raw)
	switch kind {
	case KindString:
		return StringValue(raw), nil
	case KindEnum:
		if trimmed == "" {
			return Value{}, fmt.Errorf("value is empty")
		}
		return EnumValue(trimmed), nil
	case KindBool:
		b, err := strconv.ParseBool(trimmed)
		if err != nil {
			return Value{}, fmt.Errorf("%q is not a boolean", raw)
		}
		return BoolValue(b), nil
	case KindInt:
		n, err := strconv.Atoi(trimmed)
		if err != nil {
			return Value{}, fmt.Errorf("%q is not a whole number", raw)
		}
		return IntValue(n), nil
	case KindFloat:
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return Value{}, fmt.Errorf("%q is not a number", raw)
		}
		return FloatValue(f), nil
	default:
		return Value{}, fmt.Errorf("unknown value kind %q", kind)
	}
}

func coerceNumber(kind ValueKind, raw float64) (Value, error) {
	switch kind {
	case KindInt:
		return IntValue(int(math.Trunc(raw))), nil
	case KindFloat:
		return FloatValue(raw), nil
	case KindString:
		return StringValue(strconv.FormatFloat(raw, 'g', -1, 64)), nil
	default:
		return Value{}, fmt.Errorf("expected %s, got a number", kind)
	}
}

// Widget is the control a form should render. It is inferred from a field's
// shape, and overridable where the inference is wrong.
type Widget string

const (
	// WidgetText renders a single-line text input.
	WidgetText Widget = "text"
	// WidgetPassword renders a masked input for a secret.
	WidgetPassword Widget = "password"
	// WidgetSwitch renders a toggle.
	WidgetSwitch Widget = "switch"
	// WidgetSelect renders a closed vocabulary.
	WidgetSelect Widget = "select"
	// WidgetNumber renders a numeric input.
	WidgetNumber Widget = "number"
	// WidgetSlider renders a bounded numeric range.
	WidgetSlider Widget = "slider"
)

// EnumOption is one member of a plugin's vocabulary, carrying the stored value
// with the i18n key a form uses to label it.
type EnumOption struct {
	Value    string `json:"value"`
	LabelKey string `json:"label_key,omitempty"`
	HelpKey  string `json:"help_key,omitempty"`
}

// FieldUI is how a form presents a field. It travels with the field rather
// than in a parallel table so a plugin that adds configuration gets the form
// control for free, and a field that disappears cannot linger in the UI.
type FieldUI struct {
	// Hidden keeps a field out of the form while still accepting it in a
	// stored configuration.
	Hidden bool `json:"hidden,omitempty"`
	// Group buckets the field into a form section.
	Group string `json:"group,omitempty"`
	// LabelKey, HelpKey, and PlaceholderKey are i18n keys.
	LabelKey       string `json:"label_key,omitempty"`
	HelpKey        string `json:"help_key,omitempty"`
	PlaceholderKey string `json:"placeholder_key,omitempty"`
	// Widget overrides the control inferred from the field's shape.
	Widget Widget `json:"widget,omitempty"`
	// Order sorts fields within a group.
	Order int `json:"order,omitempty"`
}

// Support is how a plugin dispositions a field.
type Support string

const (
	// SupportUser means the operator sets it.
	SupportUser Support = "user"
	// SupportPinned means the plugin always uses Pin and the operator cannot
	// override it.
	SupportPinned Support = "pinned"
	// SupportForbidden means the field must never take effect. Declaring it —
	// rather than omitting it — is what lets a caller learn that a value it
	// supplied was dropped, and why.
	SupportForbidden Support = "forbidden"
)

// Field declares one configuration input.
type Field struct {
	// ID is the key under which the value is stored and submitted.
	ID string `json:"id"`
	// Kind is the value domain.
	Kind ValueKind `json:"kind"`
	// Support dispositions the field; the zero value is SupportUser.
	Support Support `json:"support,omitempty"`
	// Required rejects a configuration that omits the field and supplies no
	// default.
	Required bool `json:"required,omitempty"`
	// Secret marks a credential. The kernel keeps secrets out of rendered
	// forms and out of any configuration it echoes back, so a plugin cannot
	// accidentally leak one by being introspectable.
	Secret bool `json:"secret,omitempty"`
	// Enum is the vocabulary for an enum field.
	Enum []EnumOption `json:"enum,omitempty"`
	// Min and Max bound a numeric field.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Default is used when a configuration omits the field.
	Default *Value `json:"default,omitempty"`
	// Pin is the forced value for a pinned field.
	Pin *Value `json:"pin,omitempty"`
	// UI is the form presentation.
	UI FieldUI `json:"ui"`
	// DocURL links the documentation behind this field.
	DocURL string `json:"doc_url,omitempty"`
}

// EffectiveSupport reports the disposition, treating the zero value as user.
func (f Field) EffectiveSupport() Support {
	if f.Support == "" {
		return SupportUser
	}
	return f.Support
}

// EffectiveWidget reports the control to render.
func (f Field) EffectiveWidget() Widget {
	if f.UI.Widget != "" {
		return f.UI.Widget
	}
	switch {
	case f.Secret:
		return WidgetPassword
	case f.Kind == KindBool:
		return WidgetSwitch
	case f.Kind == KindEnum:
		return WidgetSelect
	case f.Kind == KindFloat && f.Min != nil && f.Max != nil:
		return WidgetSlider
	case f.Kind == KindInt || f.Kind == KindFloat:
		return WidgetNumber
	default:
		return WidgetText
	}
}

// Accepts reports whether a value falls inside the field's declared domain.
func (f Field) Accepts(v Value) bool {
	if v.Kind != f.Kind {
		return false
	}
	switch f.Kind {
	case KindEnum:
		for _, opt := range f.Enum {
			if opt.Value == v.Str {
				return true
			}
		}
		return false
	case KindInt, KindFloat:
		if f.Min != nil && v.Num < *f.Min {
			return false
		}
		if f.Max != nil && v.Num > *f.Max {
			return false
		}
		return true
	default:
		return true
	}
}

// validate checks a field declaration at registration time. These are the
// mistakes plugin authors actually make, and catching them at startup beats
// discovering them through a malformed form or a rejected request.
func (f Field) validate(where string) error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("%s: field id is required", where)
	}
	if f.Kind == "" {
		return fmt.Errorf("%s field %s: kind is required", where, f.ID)
	}
	if f.Kind == KindEnum && len(f.Enum) == 0 {
		return fmt.Errorf("%s field %s: an enum needs at least one option", where, f.ID)
	}
	if f.Kind != KindEnum && len(f.Enum) > 0 {
		return fmt.Errorf("%s field %s: only an enum may declare options", where, f.ID)
	}
	if f.EffectiveSupport() == SupportPinned && f.Pin == nil {
		return fmt.Errorf("%s field %s: a pinned field needs a pin value", where, f.ID)
	}
	if f.Pin != nil && !f.Accepts(*f.Pin) {
		return fmt.Errorf("%s field %s: pin %s is outside the declared domain", where, f.ID, f.Pin)
	}
	if f.Default != nil && !f.Accepts(*f.Default) {
		return fmt.Errorf("%s field %s: default %s is outside the declared domain", where, f.ID, f.Default)
	}
	if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
		return fmt.Errorf("%s field %s: min %v exceeds max %v", where, f.ID, *f.Min, *f.Max)
	}
	if f.Required && f.Default != nil {
		return fmt.Errorf("%s field %s: a required field must not also carry a default", where, f.ID)
	}
	return nil
}

// Schema is a plugin's configuration contract.
type Schema struct {
	// Fields are the inputs, in the order a form should present them.
	Fields []Field
}

// Field reports a declared field by id.
func (s Schema) Field(id string) (Field, bool) {
	for _, f := range s.Fields {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}

// Validate turns a loosely typed configuration into a validated one.
//
// Precedence is pin, then the supplied value, then the default. A value the
// domain rejects is an error rather than a silent fallback: a configuration
// that quietly means something other than what was written is worse than one
// that refuses to load.
func (s Schema) Validate(raw map[string]any) (Config, error) {
	values := make(map[string]Value, len(s.Fields))
	var problems []string

	declared := make(map[string]struct{}, len(s.Fields))
	for _, f := range s.Fields {
		declared[f.ID] = struct{}{}

		if f.EffectiveSupport() == SupportPinned {
			values[f.ID] = *f.Pin
			continue
		}
		if f.EffectiveSupport() == SupportForbidden {
			continue
		}

		supplied, ok := raw[f.ID]
		if !ok || supplied == nil || supplied == "" {
			if f.Default != nil {
				values[f.ID] = *f.Default
			} else if f.Required {
				problems = append(problems, fmt.Sprintf("%s is required", f.ID))
			}
			continue
		}

		value, err := Coerce(f.Kind, supplied)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", f.ID, err))
			continue
		}
		if !f.Accepts(value) {
			problems = append(problems, fmt.Sprintf("%s: %s is not an accepted value", f.ID, value))
			continue
		}
		values[f.ID] = value
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return Config{values: values, schema: s}, nil
}

// Group is one section of a rendered form.
type Group struct {
	Key    string  `json:"key"`
	Fields []Field `json:"fields"`
}

// Form renders the schema for a UI, omitting what an operator cannot set: a
// hidden field, and anything pinned or forbidden. Secrets are rendered as
// masked inputs but never carry a default, so a stored credential is not
// echoed back into a form.
func (s Schema) Form() []Group {
	byGroup := map[string][]Field{}
	var order []string

	for _, f := range s.Fields {
		if f.UI.Hidden || f.EffectiveSupport() != SupportUser {
			continue
		}
		rendered := f
		if rendered.Secret {
			rendered.Default = nil
		}
		key := f.UI.Group
		if key == "" {
			key = "general"
		}
		if _, seen := byGroup[key]; !seen {
			order = append(order, key)
		}
		byGroup[key] = append(byGroup[key], rendered)
	}

	groups := make([]Group, 0, len(order))
	for _, key := range order {
		fields := byGroup[key]
		sort.SliceStable(fields, func(i, j int) bool {
			return fields[i].UI.Order < fields[j].UI.Order
		})
		groups = append(groups, Group{Key: key, Fields: fields})
	}
	return groups
}

// validate checks the whole schema at registration time.
func (s Schema) validate(where string) error {
	seen := make(map[string]struct{}, len(s.Fields))
	for _, f := range s.Fields {
		if _, dup := seen[f.ID]; dup {
			return fmt.Errorf("%s: duplicate field %s", where, f.ID)
		}
		seen[f.ID] = struct{}{}
		if err := f.validate(where); err != nil {
			return err
		}
	}
	return nil
}

// Config is a configuration that has passed its schema.
//
// A plugin receives one instead of a raw map, so it never repeats validation
// and never reads a key the schema did not declare. The accessors report
// zero values for anything absent, because absence has already been decided
// to be acceptable by the time a Config exists.
type Config struct {
	values map[string]Value
	schema Schema
}

// NewConfig builds a validated configuration directly, for tests and for
// callers assembling one in code rather than from stored settings.
func NewConfig(schema Schema, raw map[string]any) (Config, error) {
	return schema.Validate(raw)
}

// Has reports whether a value is present.
func (c Config) Has(id string) bool {
	_, ok := c.values[id]
	return ok
}

// Value reports a raw value and whether it is present.
func (c Config) Value(id string) (Value, bool) {
	v, ok := c.values[id]
	return v, ok
}

// String reports a text or enum value, or "" when absent.
func (c Config) String(id string) string {
	v, ok := c.values[id]
	if !ok {
		return ""
	}
	text, _ := v.Text()
	return text
}

// Int reports a numeric value, or 0 when absent.
func (c Config) Int(id string) int {
	v, ok := c.values[id]
	if !ok {
		return 0
	}
	n, _ := v.Int()
	return n
}

// Float reports a numeric value, or 0 when absent.
func (c Config) Float(id string) float64 {
	v, ok := c.values[id]
	if !ok {
		return 0
	}
	f, _ := v.Float()
	return f
}

// Bool reports a flag, or false when absent.
func (c Config) Bool(id string) bool {
	v, ok := c.values[id]
	if !ok {
		return false
	}
	b, _ := v.Flag()
	return b
}

// Redacted reports the configuration with secrets removed, for logging and for
// any response that echoes a configuration back.
func (c Config) Redacted() map[string]any {
	out := make(map[string]any, len(c.values))
	for id, v := range c.values {
		if field, ok := c.schema.Field(id); ok && field.Secret {
			continue
		}
		out[id] = v.JSON()
	}
	return out
}

// Float64 returns a pointer to f, for populating bounds inline.
func Float64(f float64) *float64 { return &f }

// Ptr returns a pointer to v, for populating defaults and pins inline.
func Ptr(v Value) *Value { return &v }
