package spi

import "sort"

// This file renders a descriptor as a form schema.
//
// It is the fourth surface the declaration drives, after the request body,
// validation, and the debug report. Serving the form from the same source
// removes the failure this seam was built to end: a frontend that predicted a
// vendor's behavior with its own copy of the rules, and drifted.

// FieldSchema describes one control a form should render.
type FieldSchema struct {
	// ID is the neutral parameter identity the form submits back.
	ID ParamID `json:"id"`
	// Kind is the value domain.
	Kind ValueKind `json:"kind"`
	// Widget is the control to render.
	Widget Widget `json:"widget"`
	// LabelKey and HelpKey are i18n keys the frontend resolves. The backend
	// sends no display text, so language stays a frontend concern.
	LabelKey string `json:"label_key,omitempty"`
	HelpKey  string `json:"help_key,omitempty"`
	// Options is the vendor's vocabulary for an enum field.
	Options []EnumOption `json:"options,omitempty"`
	// Min and Max bound a numeric field.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Default is the value sent when the user leaves the field alone.
	Default *Value `json:"default,omitempty"`
	// WireField names the request field this control writes, which is what
	// the model editor shows in place of the old free-text wire-format
	// setting — and unlike that setting, it is read from the request path
	// rather than chosen independently of it.
	WireField string `json:"wire_field,omitempty"`
	// DocURL links the vendor documentation behind the field.
	DocURL string `json:"doc_url,omitempty"`
}

// GroupSchema is one section of the form.
type GroupSchema struct {
	// Key names the section, e.g. "thinking".
	Key string `json:"key"`
	// Fields are the controls in declaration order.
	Fields []FieldSchema `json:"fields"`
}

// FormSchema is everything a form needs to render a model's settings.
type FormSchema struct {
	// Vendor is the resolved plugin.
	Vendor string `json:"vendor"`
	// DisplayName is the plugin's human-readable name.
	DisplayName string `json:"display_name,omitempty"`
	// Protocol is the wire protocol this model will use.
	Protocol ProtocolID `json:"protocol"`
	// Protocols lists the alternatives, for vendors offering more than one. A
	// single entry means there is nothing to choose and the form should not
	// offer a selector.
	Protocols []ProtocolID `json:"protocols,omitempty"`
	// Groups are the form sections.
	Groups []GroupSchema `json:"groups"`
	// SupportsThinking reports whether the model has a reasoning toggle, which
	// is the one question the chat UI asks outside the form.
	SupportsThinking bool `json:"supports_thinking"`
	// ReasoningReplay reports whether prior reasoning must be replayed.
	ReasoningReplay ReasoningReplay `json:"reasoning_replay,omitempty"`
	// DocURL links the vendor documentation this plugin follows.
	DocURL string `json:"doc_url,omitempty"`
}

// groupOrder fixes the section order so the form does not reshuffle when a
// vendor happens to declare its parameters in a different sequence.
var groupOrder = map[string]int{
	"thinking": 10,
	"sampling": 20,
	"limits":   30,
}

// Schema renders a descriptor as a form schema, omitting the parameters a form
// must not offer: a hidden one, and anything pinned or forbidden, whose value
// the user cannot influence.
func (d Descriptor) Schema() FormSchema {
	schema := FormSchema{
		Vendor:          d.Vendor,
		DisplayName:     d.DisplayName,
		Protocol:        d.Protocol,
		ReasoningReplay: d.EffectiveReplay(),
		DocURL:          d.DocURL,
		Groups:          []GroupSchema{},
	}

	byGroup := map[string][]FieldSchema{}
	order := map[ParamID]int{}
	for _, p := range d.Params {
		if p.ID == ParamThinkingMode && p.EffectiveSupport() != SupportForbidden {
			schema.SupportsThinking = true
		}
		if p.UI.Hidden || p.EffectiveSupport() != SupportUser {
			continue
		}
		field := FieldSchema{
			ID:       p.ID,
			Kind:     p.Kind,
			Widget:   p.EffectiveWidget(),
			LabelKey: p.UI.LabelKey,
			HelpKey:  p.UI.HelpKey,
			Options:  p.Enum,
			Min:      p.Min,
			Max:      p.Max,
			Default:  p.Default,
			DocURL:   p.DocURL,
		}
		if p.Encode != nil {
			field.WireField = p.Encode.ID()
		}
		group := p.UI.Group
		if group == "" {
			group = "advanced"
		}
		order[p.ID] = p.UI.Order
		byGroup[group] = append(byGroup[group], field)
	}

	keys := make([]string, 0, len(byGroup))
	for key := range byGroup {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		oi, oki := groupOrder[keys[i]]
		oj, okj := groupOrder[keys[j]]
		if oki != okj {
			return oki
		}
		if oi != oj {
			return oi < oj
		}
		return keys[i] < keys[j]
	})

	for _, key := range keys {
		fields := byGroup[key]
		// Declaration order is the tiebreak, so a vendor that adds a field
		// does not reorder the ones already there.
		sort.SliceStable(fields, func(i, j int) bool {
			return order[fields[i].ID] < order[fields[j].ID]
		})
		schema.Groups = append(schema.Groups, GroupSchema{Key: key, Fields: fields})
	}
	return schema
}
