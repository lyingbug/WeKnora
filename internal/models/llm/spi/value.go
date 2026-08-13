package spi

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ValueKind is the domain of a parameter value. Every request parameter a
// vendor exposes is one of these four shapes, which is what lets one
// declaration serve the wire, the validator, and the form widget.
type ValueKind string

const (
	// KindBool is an on/off parameter rendered as a switch.
	KindBool ValueKind = "bool"
	// KindInt is an integer parameter rendered as a number input.
	KindInt ValueKind = "int"
	// KindFloat is a fractional parameter rendered as a slider or number input.
	KindFloat ValueKind = "float"
	// KindEnum is a closed vocabulary rendered as a select. The vocabulary is
	// the VENDOR's, not a normalized one: OpenAI's reasoning effort ladder and
	// Zhipu's are different sets, and pretending otherwise would silently send
	// a value the vendor rejects.
	KindEnum ValueKind = "enum"
)

// Value is a parameter value tagged with its kind. It is deliberately a small
// concrete struct rather than `any`: encoders, validators, and the UI schema
// all need to know the shape without a type switch on every access.
type Value struct {
	Kind ValueKind `json:"kind"`
	Bool bool      `json:"bool,omitempty"`
	Num  float64   `json:"num,omitempty"`
	Str  string    `json:"str,omitempty"`
}

// Bool returns a boolean value.
func BoolValue(v bool) Value { return Value{Kind: KindBool, Bool: v} }

// IntValue returns an integer value.
func IntValue(v int) Value { return Value{Kind: KindInt, Num: float64(v)} }

// FloatValue returns a fractional value.
func FloatValue(v float64) Value { return Value{Kind: KindFloat, Num: v} }

// EnumValue returns a value drawn from a vendor's closed vocabulary.
func EnumValue(v string) Value { return Value{Kind: KindEnum, Str: v} }

// Int reports the value as an integer, rounding a float toward zero. The
// second result is false when the value is not numeric.
func (v Value) Int() (int, bool) {
	if v.Kind != KindInt && v.Kind != KindFloat {
		return 0, false
	}
	return int(math.Trunc(v.Num)), true
}

// Float reports the value as a float. The second result is false when the
// value is not numeric.
func (v Value) Float() (float64, bool) {
	if v.Kind != KindInt && v.Kind != KindFloat {
		return 0, false
	}
	return v.Num, true
}

// Enum reports the value as a vocabulary string. The second result is false
// when the value is not an enum.
func (v Value) Enum() (string, bool) {
	if v.Kind != KindEnum {
		return "", false
	}
	return v.Str, true
}

// JSON reports the value as it should appear in a JSON request body.
func (v Value) JSON() any {
	switch v.Kind {
	case KindBool:
		return v.Bool
	case KindInt:
		return int(math.Trunc(v.Num))
	case KindFloat:
		return v.Num
	case KindEnum:
		return v.Str
	default:
		return nil
	}
}

// String renders the value for logs and diagnostics.
func (v Value) String() string {
	switch v.Kind {
	case KindBool:
		return strconv.FormatBool(v.Bool)
	case KindInt:
		return strconv.Itoa(int(math.Trunc(v.Num)))
	case KindFloat:
		return strconv.FormatFloat(v.Num, 'g', -1, 64)
	case KindEnum:
		return v.Str
	default:
		return ""
	}
}

// ParseValue reads a value of the given kind from its string form, which is
// how values arrive from stored model configuration and from HTTP requests.
func ParseValue(kind ValueKind, raw string) (Value, error) {
	raw = strings.TrimSpace(raw)
	switch kind {
	case KindBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return Value{}, fmt.Errorf("parse bool %q: %w", raw, err)
		}
		return BoolValue(b), nil
	case KindInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Value{}, fmt.Errorf("parse int %q: %w", raw, err)
		}
		return IntValue(n), nil
	case KindFloat:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return Value{}, fmt.Errorf("parse float %q: %w", raw, err)
		}
		return FloatValue(f), nil
	case KindEnum:
		if raw == "" {
			return Value{}, fmt.Errorf("parse enum: empty value")
		}
		return EnumValue(raw), nil
	default:
		return Value{}, fmt.Errorf("unknown value kind %q", kind)
	}
}
