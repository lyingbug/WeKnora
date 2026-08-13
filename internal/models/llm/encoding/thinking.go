package encoding

import (
	"fmt"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// The thinking encoders below map the neutral thinking mode onto each
// documented wire shape. Reasoning depth needs no dedicated encoder: an effort
// ladder is a plain field (`reasoning_effort`) or a nested one
// (`reasoning.effort`, `output_config.effort`), and a token budget likewise
// (`thinking_budget`, `thinking.budget_tokens`), so Field and NestedField
// already cover them. Only the mode needs translating, because "on" is spelled
// as a boolean by one vendor, an enum by another, and a nested template
// argument by a third.

// ThinkingObject encodes the mode as an object with a type discriminator:
//
//	{"thinking": {"type": "enabled"}}
//
// It is the most widely adopted shape, used by DeepSeek, Zhipu GLM, Volcengine
// Ark, and Tencent LKEAP. The vendor spellings are parameters because they do
// differ: Ark documents a third `auto` value that lets the model decide, and
// Anthropic reuses the same object with `adaptive` in that role.
type ThinkingObject struct {
	// Key is the body field holding the object, `thinking` for every
	// documented user of this shape.
	Key string
	// On, Off, and Auto are the vendor's spellings. An empty spelling means
	// the vendor does not document that mode, and encoding it is an error
	// rather than a silent omission.
	On   string
	Off  string
	Auto string
}

// ID reports the wire field.
func (t ThinkingObject) ID() string { return t.Key + ".type" }

// Encode writes the vendor's spelling of the requested mode.
func (t ThinkingObject) Encode(d *spi.Draft, v spi.Value) error {
	spelling, err := t.spell(v)
	if err != nil {
		return err
	}
	d.SetNested(spelling, t.Key, "type")
	return nil
}

// Strip removes the whole object, including any budget a sibling parameter
// wrote: a disabled thinking configuration carrying a budget is contradictory,
// and vendors reject it.
func (t ThinkingObject) Strip(d *spi.Draft) error {
	d.Delete(t.Key)
	return nil
}

func (t ThinkingObject) spell(v spi.Value) (string, error) {
	mode, ok := v.Enum()
	if !ok {
		return "", fmt.Errorf("thinking mode must be an enum, got %s", v.Kind)
	}
	var spelling string
	switch mode {
	case spi.ThinkingOn:
		spelling = t.On
	case spi.ThinkingOff:
		spelling = t.Off
	case spi.ThinkingAuto:
		spelling = t.Auto
	default:
		return "", fmt.Errorf("unknown thinking mode %q", mode)
	}
	if spelling == "" {
		return "", fmt.Errorf("thinking mode %q has no wire spelling for %s", mode, t.Key)
	}
	return spelling, nil
}

// EnableThinkingBool encodes the mode as a top-level boolean:
//
//	{"enable_thinking": true}
//
// This is Alibaba Cloud Model Studio's shape for the Qwen hybrid-thinking
// models. The OpenAI Python SDK reaches it through `extra_body`, which is only
// that SDK's mechanism for passing a non-standard field: on the wire it is an
// ordinary top-level key, and Model Studio's own batch-inference documentation
// says so explicitly by requiring it beside `model`.
//
// The shape has no third state, so a descriptor using it must not offer auto.
type EnableThinkingBool struct {
	// Key is the body field, `enable_thinking` as documented.
	Key string
}

// ID reports the wire field.
func (e EnableThinkingBool) ID() string { return e.Key }

// Encode writes the boolean form of the requested mode.
func (e EnableThinkingBool) Encode(d *spi.Draft, v spi.Value) error {
	mode, ok := v.Enum()
	if !ok {
		return fmt.Errorf("thinking mode must be an enum, got %s", v.Kind)
	}
	switch mode {
	case spi.ThinkingOn:
		d.Set(e.Key, true)
	case spi.ThinkingOff:
		d.Set(e.Key, false)
	default:
		return fmt.Errorf("%s cannot express thinking mode %q", e.Key, mode)
	}
	return nil
}

// Strip removes the field.
func (e EnableThinkingBool) Strip(d *spi.Draft) error {
	d.Delete(e.Key)
	return nil
}

// ChatTemplateKwargs encodes the mode as a chat-template argument:
//
//	{"chat_template_kwargs": {"enable_thinking": true}}
//
// This is how a self-hosted inference server passes the flag through to the
// model's chat template, so it is the right default for vLLM-style generic
// deployments and NVIDIA NIM rather than for any hosted vendor API.
type ChatTemplateKwargs struct {
	// Key is the outer field, `chat_template_kwargs` by convention.
	Key string
	// Arg is the template argument name, `enable_thinking` by convention.
	Arg string
}

// ID reports the dotted path.
func (c ChatTemplateKwargs) ID() string { return c.Key + "." + c.Arg }

// Encode writes the boolean template argument.
func (c ChatTemplateKwargs) Encode(d *spi.Draft, v spi.Value) error {
	mode, ok := v.Enum()
	if !ok {
		return fmt.Errorf("thinking mode must be an enum, got %s", v.Kind)
	}
	switch mode {
	case spi.ThinkingOn:
		d.SetNested(true, c.Key, c.Arg)
	case spi.ThinkingOff:
		d.SetNested(false, c.Key, c.Arg)
	default:
		return fmt.Errorf("%s cannot express thinking mode %q", c.ID(), mode)
	}
	return nil
}

// Strip removes the template argument, leaving other arguments in place.
func (c ChatTemplateKwargs) Strip(d *spi.Draft) error {
	return Nested(c.Key, c.Arg).Strip(d)
}

// EffortNone encodes the mode by writing a sentinel onto an effort ladder whose
// lowest rung means "do not think":
//
//	{"reasoning": {"effort": "none"}}
//
// DeepSeek's Responses-format control and Alibaba's Responses API both
// document `none` this way, so on those surfaces the mode toggle and the depth
// control are the same field. Keeping that as its own encoder means the
// descriptor still declares two parameters — a toggle a user understands and a
// depth ladder — while the wire carries the one field the vendor defines.
type EffortNone struct {
	// Encoder writes the effort value; it is the same encoder the depth
	// parameter uses, so both agree on where the field lives.
	Encoder spi.Encoder
	// Off is the ladder rung meaning "no thinking", `none` as documented.
	Off string
	// On is the rung to write when thinking is requested but no explicit depth
	// was given. Empty leaves the vendor's own default in place, which is the
	// safer choice when the vendor documents one.
	On string
}

// ID reports the underlying field.
func (e EffortNone) ID() string { return e.Encoder.ID() }

// Encode writes the ladder rung standing for the requested mode.
func (e EffortNone) Encode(d *spi.Draft, v spi.Value) error {
	mode, ok := v.Enum()
	if !ok {
		return fmt.Errorf("thinking mode must be an enum, got %s", v.Kind)
	}
	switch mode {
	case spi.ThinkingOff:
		return e.Encoder.Encode(d, spi.EnumValue(e.Off))
	case spi.ThinkingOn:
		if e.On == "" {
			return nil
		}
		return e.Encoder.Encode(d, spi.EnumValue(e.On))
	default:
		return fmt.Errorf("%s cannot express thinking mode %q", e.ID(), mode)
	}
}

// Strip removes the effort field.
func (e EffortNone) Strip(d *spi.Draft) error {
	if stripper, ok := e.Encoder.(spi.Stripper); ok {
		return stripper.Strip(d)
	}
	return nil
}
