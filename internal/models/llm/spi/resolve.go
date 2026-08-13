package spi

import "fmt"

// Request is one call's neutral intent: which model, whether it streams, and
// the parameter values the caller wants. Callers never name wire fields, which
// is what lets the same request run against any vendor.
type Request struct {
	// Model is the model identifier to send.
	Model string
	// Stream reports whether the response is streamed.
	Stream bool
	// Values are the caller's parameter values, keyed by neutral id. A missing
	// key means "no opinion", which is distinct from an explicit zero.
	Values map[ParamID]Value
}

// Value reports a caller value and whether it was supplied.
func (r Request) Value(id ParamID) (Value, bool) {
	v, ok := r.Values[id]
	return v, ok
}

// WithValue returns a copy of the request carrying an additional value.
func (r Request) WithValue(id ParamID, v Value) Request {
	values := make(map[ParamID]Value, len(r.Values)+1)
	for k, existing := range r.Values {
		values[k] = existing
	}
	values[id] = v
	r.Values = values
	return r
}

// Plan resolves a request against the descriptor, producing the inspectable
// outcome without touching the wire.
//
// Resolution is deliberately a separate step from encoding: the same plan
// answers "what will actually be sent" for a test, for the debug endpoint, and
// for the model editor, so none of them has to reimplement the precedence
// rules and then drift from the request path.
//
// Precedence per parameter is pin, then caller, then plugin default, then
// omit. Every departure from the caller's request is recorded as a note.
func (d Descriptor) Plan(req Request) (*Plan, error) {
	model := req.Model
	if model == "" {
		model = d.Vendor
	}
	plan := newPlan(d.Vendor, d.Protocol, model, req.Stream)

	declared := make(map[ParamID]struct{}, len(d.Params))
	for _, p := range d.Params {
		declared[p.ID] = struct{}{}
		plan.params[p.ID] = p
		resolveParam(plan, p, req)
	}

	// A caller value with nowhere to go is worth reporting rather than
	// dropping in silence: it is usually a UI offering a control the resolved
	// vendor does not actually have.
	for id := range req.Values {
		if _, ok := declared[id]; !ok {
			plan.Note(id, NoteUnsupported, d.Vendor,
				fmt.Sprintf("%s does not declare %s for %s models", d.Vendor, id, d.Kind))
		}
	}

	for _, constraint := range d.Constraints {
		if err := constraint.Apply(plan); err != nil {
			return nil, fmt.Errorf("constraint %s: %w", constraint.ID(), err)
		}
	}
	return plan, nil
}

// resolveParam applies the precedence rules for one declared parameter.
func resolveParam(plan *Plan, p Param, req Request) {
	given, supplied := req.Value(p.ID)

	switch p.EffectiveSupport() {
	case SupportForbidden:
		plan.strip = append(plan.strip, p.ID)
		if supplied {
			plan.Note(p.ID, NoteForbidden, plan.Vendor,
				fmt.Sprintf("%s rejects %s for model %s; the field is removed from the request",
					plan.Vendor, p.ID, plan.Model))
		}
		return

	case SupportPinned:
		plan.Set(p.ID, *p.Pin)
		if supplied && given != *p.Pin {
			plan.Note(p.ID, NotePinned, plan.Vendor,
				fmt.Sprintf("%s accepts only %s=%s for model %s; the requested %s was replaced",
					plan.Vendor, p.ID, p.Pin, plan.Model, given))
		}
		return
	}

	if supplied {
		if p.AllowsValue(given) {
			plan.Set(p.ID, given)
			return
		}
		plan.Note(p.ID, NoteOutOfDomain, plan.Vendor,
			fmt.Sprintf("%s is outside the values %s accepts for %s", given, plan.Vendor, p.ID))
		// Fall through: a rejected value should not silently become the
		// plugin default, so only an explicitly declared default applies.
	}

	if p.Default != nil {
		plan.Set(p.ID, *p.Default)
		if !supplied {
			plan.Note(p.ID, NoteDefaulted, plan.Vendor,
				fmt.Sprintf("%s requires %s on every request; sending %s", plan.Vendor, p.ID, p.Default))
		}
	}
}
