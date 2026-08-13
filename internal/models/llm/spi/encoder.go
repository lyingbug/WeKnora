package spi

// Encoder writes one resolved parameter value onto the outbound draft. It is
// the whole of "how this vendor spells this knob".
//
// Encoders are small and composable so a new vendor is usually a descriptor
// that reuses existing encoders, not new code. When a vendor genuinely spells
// something no existing encoder covers, the new encoder lives beside the
// others in internal/models/llm/encoding and becomes reusable in turn.
type Encoder interface {
	// ID names the encoding. It is reported in diagnostics and surfaced to the
	// model editor so a user can see which wire field a knob maps to, which is
	// the question the previous free-text thinking_control setting was really
	// trying to answer.
	ID() string
	// Encode writes v onto the draft. It is invoked only with a value the
	// parameter's declared domain already accepted.
	Encode(d *Draft, v Value) error
}

// EncoderFunc adapts a function to Encoder, for the many encoders that need no
// state beyond their id.
type EncoderFunc struct {
	Name string
	Fn   func(d *Draft, v Value) error
}

// ID reports the encoder name.
func (e EncoderFunc) ID() string { return e.Name }

// Encode applies the wrapped function.
func (e EncoderFunc) Encode(d *Draft, v Value) error { return e.Fn(d, v) }

// Stripper is an encoder that can also remove its field from a draft.
//
// It exists for forbidden parameters, and the distinction matters: the
// protocol driver builds a canonical body that may already carry the field
// from the caller's generic options, so a vendor that rejects the field needs
// it actively removed, not merely "not set". OpenAI's reasoning models reject
// temperature outright, so leaving the protocol's value in place would fail
// the request rather than degrade politely.
//
// It is an optional interface so ordinary encoders stay a single method.
type Stripper interface {
	// Strip removes the encoder's field from the draft.
	Strip(d *Draft) error
}

// Constraint is a cross-parameter rule the plugin enforces before anything is
// encoded: a relationship the per-parameter domains cannot express.
//
// Constraints run against the resolved Plan, so they can drop a value, adjust
// it, or reject the request outright — and every adjustment they make is
// recorded as a note rather than applied silently. A user who asked for
// thinking and did not get it should be able to find out why.
type Constraint interface {
	// ID names the rule for diagnostics.
	ID() string
	// Apply inspects and may adjust the plan. Returning an error rejects the
	// request, which is correct when the vendor would reject it anyway and a
	// local error is clearer than a remote 400.
	Apply(p *Plan) error
}

// ConstraintFunc adapts a function to Constraint.
type ConstraintFunc struct {
	Name string
	Fn   func(p *Plan) error
}

// ID reports the constraint name.
func (c ConstraintFunc) ID() string { return c.Name }

// Apply runs the wrapped function.
func (c ConstraintFunc) Apply(p *Plan) error { return c.Fn(p) }
