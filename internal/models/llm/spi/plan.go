package spi

import "fmt"

// NoteReason classifies why a plan differs from what the caller asked for.
type NoteReason string

const (
	// NoteDefaulted means the plugin supplied a value the caller omitted.
	NoteDefaulted NoteReason = "defaulted"
	// NotePinned means the plugin overrode the caller because the vendor
	// accepts only one value.
	NotePinned NoteReason = "pinned"
	// NoteForbidden means the parameter was dropped because the vendor rejects
	// it for this model.
	NoteForbidden NoteReason = "forbidden"
	// NoteUnsupported means the parameter is not declared by this plugin, so
	// the caller's value has nowhere to go.
	NoteUnsupported NoteReason = "unsupported"
	// NoteOutOfDomain means the caller's value fell outside the declared
	// domain and was dropped rather than sent for the vendor to reject.
	NoteOutOfDomain NoteReason = "out_of_domain"
	// NoteConstrained means a constraint adjusted or dropped the value.
	NoteConstrained NoteReason = "constrained"
)

// Note records one difference between the request as asked and the request as
// it will be sent. Notes are the plan's honesty mechanism: every silent
// adjustment a vendor quirk forces becomes an inspectable fact instead of
// surprising behavior, and the model debug endpoint reports them verbatim.
type Note struct {
	// Param is the affected parameter.
	Param ParamID `json:"param"`
	// Reason classifies the difference.
	Reason NoteReason `json:"reason"`
	// Detail explains it in one sentence, for a human reading diagnostics.
	Detail string `json:"detail,omitempty"`
	// Source names the constraint responsible, when one is.
	Source string `json:"source,omitempty"`
}

// Plan is the resolved outcome of applying a descriptor to one request: the
// parameters that will be sent, their values, and every adjustment made along
// the way.
//
// It exists as a separate step from encoding so the same resolution can be
// asserted in a test, reported by the debug endpoint, and rendered by the UI
// without issuing a request. A plan is what makes "which field will carry my
// thinking toggle" answerable rather than guessable.
type Plan struct {
	// Vendor is the descriptor that produced this plan.
	Vendor string `json:"vendor"`
	// Protocol is the wire protocol the request will use.
	Protocol ProtocolID `json:"protocol"`
	// Model is the resolved model identifier.
	Model string `json:"model"`
	// Stream reports whether this is a streaming request.
	Stream bool `json:"stream"`
	// Notes records every adjustment, in the order it was made.
	Notes []Note `json:"notes,omitempty"`

	values  map[ParamID]Value
	order   []ParamID
	params  map[ParamID]Param
	encoded map[ParamID]string
	strip   []ParamID
}

// newPlan returns an empty plan for a descriptor and request shape.
func newPlan(vendor string, protocol ProtocolID, model string, stream bool) *Plan {
	return &Plan{
		Vendor:   vendor,
		Protocol: protocol,
		Model:    model,
		Stream:   stream,
		values:   map[ParamID]Value{},
		params:   map[ParamID]Param{},
		encoded:  map[ParamID]string{},
	}
}

// Value reports the resolved value of a parameter and whether it is present.
func (p *Plan) Value(id ParamID) (Value, bool) {
	v, ok := p.values[id]
	return v, ok
}

// Has reports whether a parameter will be sent.
func (p *Plan) Has(id ParamID) bool {
	_, ok := p.values[id]
	return ok
}

// Param reports the declaration behind a resolved parameter, so a constraint
// can consult the domain it must respect.
func (p *Plan) Param(id ParamID) (Param, bool) {
	decl, ok := p.params[id]
	return decl, ok
}

// Params reports the resolved parameters in declaration order.
func (p *Plan) Params() []ParamID {
	out := make([]ParamID, 0, len(p.order))
	for _, id := range p.order {
		if _, ok := p.values[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// Set records a resolved value, appending it to the order the first time.
func (p *Plan) Set(id ParamID, v Value) {
	if _, exists := p.values[id]; !exists {
		p.order = append(p.order, id)
	}
	p.values[id] = v
}

// Drop removes a resolved value and records why.
func (p *Plan) Drop(id ParamID, reason NoteReason, source, detail string) {
	if _, ok := p.values[id]; !ok {
		return
	}
	delete(p.values, id)
	p.Note(id, reason, source, detail)
}

// Adjust replaces a resolved value and records why.
func (p *Plan) Adjust(id ParamID, v Value, source, detail string) {
	p.Set(id, v)
	p.Note(id, NoteConstrained, source, detail)
}

// Note records an adjustment without changing any value.
func (p *Plan) Note(id ParamID, reason NoteReason, source, detail string) {
	p.Notes = append(p.Notes, Note{Param: id, Reason: reason, Source: source, Detail: detail})
}

// EncodedBy reports the encoder id that carried a parameter onto the wire.
// This is the fact the model editor shows in place of the old free-text
// thinking_control setting, and it is derived from the request path rather
// than duplicated from it.
func (p *Plan) EncodedBy(id ParamID) string {
	return p.encoded[id]
}

// Encodings reports every parameter that reached the wire and the encoder that
// carried it, keyed by parameter id.
func (p *Plan) Encodings() map[ParamID]string {
	out := make(map[ParamID]string, len(p.encoded))
	for id, enc := range p.encoded {
		out[id] = enc
	}
	return out
}

// Stripped reports the parameters removed from the wire because the vendor
// forbids them.
func (p *Plan) Stripped() []ParamID {
	out := make([]ParamID, len(p.strip))
	copy(out, p.strip)
	return out
}

// Apply writes the plan onto a draft: forbidden fields are removed first, then
// each resolved parameter's encoder runs in declaration order so a later
// encoder can build on an earlier one's output.
//
// Stripping precedes encoding because the protocol driver has already written
// its canonical body, and a vendor that forbids a field needs it gone whether
// or not the caller mentioned it.
func (p *Plan) Apply(d *Draft) error {
	for _, id := range p.strip {
		decl, ok := p.params[id]
		if !ok || decl.Encode == nil {
			continue
		}
		stripper, ok := decl.Encode.(Stripper)
		if !ok {
			continue
		}
		if err := stripper.Strip(d); err != nil {
			return fmt.Errorf("strip %s via %s: %w", id, decl.Encode.ID(), err)
		}
	}
	for _, id := range p.order {
		v, ok := p.values[id]
		if !ok {
			continue
		}
		decl, ok := p.params[id]
		if !ok || decl.Encode == nil {
			continue
		}
		if err := decl.Encode.Encode(d, v); err != nil {
			return fmt.Errorf("encode %s via %s: %w", id, decl.Encode.ID(), err)
		}
		p.encoded[id] = decl.Encode.ID()
	}
	return nil
}
