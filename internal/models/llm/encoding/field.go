// Package encoding provides the reusable encoders a vendor descriptor composes
// to express its wire format.
//
// Every encoder here corresponds to a shape that at least one vendor documents.
// Adding a vendor should normally mean composing these, not writing new code;
// when a vendor genuinely spells something none of them cover, the new encoder
// belongs here beside the others so the next vendor can reuse it.
package encoding

import (
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// Field encodes a value as a top-level body field. It covers the standard
// parameters every OpenAI-shaped protocol already understands, and the
// non-standard top-level fields several vendors add beside them.
type Field struct {
	// Key is the JSON field name.
	Key string
}

// ID reports the encoding as the wire field it writes, which is the answer a
// user wants when asking where a knob goes.
func (f Field) ID() string { return f.Key }

// Encode writes the value at the top level.
func (f Field) Encode(d *spi.Draft, v spi.Value) error {
	d.Set(f.Key, v.JSON())
	return nil
}

// Strip removes the field, for vendors that reject it.
func (f Field) Strip(d *spi.Draft) error {
	d.Delete(f.Key)
	return nil
}

// NestedField encodes a value inside a nested object, creating the
// intermediate objects as needed. It covers `thinking.budget_tokens`,
// `reasoning.effort`, `output_config.effort`, and the other object-shaped
// vendor extensions.
type NestedField struct {
	// Path is the object path, outermost first.
	Path []string
}

// Nested returns a NestedField for a path.
func Nested(path ...string) NestedField { return NestedField{Path: path} }

// ID reports the dotted path.
func (n NestedField) ID() string { return joinPath(n.Path) }

// Encode writes the value at the nested path.
func (n NestedField) Encode(d *spi.Draft, v spi.Value) error {
	d.SetNested(v.JSON(), n.Path...)
	return nil
}

// Strip removes the leaf key, leaving any sibling fields in the same object
// intact — another parameter may legitimately own them.
func (n NestedField) Strip(d *spi.Draft) error {
	if len(n.Path) == 0 {
		return nil
	}
	if len(n.Path) == 1 {
		d.Delete(n.Path[0])
		return nil
	}
	parent, ok := d.GetNested(n.Path[:len(n.Path)-1]...)
	if !ok {
		return nil
	}
	if obj, ok := parent.(map[string]any); ok {
		delete(obj, n.Path[len(n.Path)-1])
	}
	return nil
}

// AliasField encodes a value under a vendor-specific name while removing the
// protocol's canonical spelling of the same parameter.
//
// It exists because renaming is not the same as writing a second field: the
// protocol driver has already written the canonical key from the caller's
// generic options, and OpenAI's reasoning models reject a request that carries
// both max_tokens and max_completion_tokens.
type AliasField struct {
	// Canonical is the protocol's own spelling, removed on encode.
	Canonical string
	// Wire is the vendor's spelling, written on encode.
	Wire string
}

// ID reports the vendor spelling this alias writes.
func (a AliasField) ID() string { return a.Wire }

// Encode moves the value from the canonical key to the vendor key.
func (a AliasField) Encode(d *spi.Draft, v spi.Value) error {
	d.Delete(a.Canonical)
	d.Set(a.Wire, v.JSON())
	return nil
}

// Strip removes both spellings.
func (a AliasField) Strip(d *spi.Draft) error {
	d.Delete(a.Canonical)
	d.Delete(a.Wire)
	return nil
}

func joinPath(path []string) string {
	out := ""
	for i, seg := range path {
		if i > 0 {
			out += "."
		}
		out += seg
	}
	return out
}
