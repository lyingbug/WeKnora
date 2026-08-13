package spi

// ParamID is the neutral identity of a request parameter, shared across every
// vendor. Callers, stored configuration, and the UI all speak ParamIDs; each
// plugin declares how its vendor spells them on the wire.
//
// Thinking is not privileged here. It is three ordinary parameters, which is
// exactly why the same machinery that pins Moonshot's temperature can also
// express Anthropic's thinking budget.
type ParamID string

const (
	// ParamTemperature is the sampling temperature.
	ParamTemperature ParamID = "temperature"
	// ParamTopP is nucleus sampling mass.
	ParamTopP ParamID = "top_p"
	// ParamMaxTokens is the ceiling on generated tokens. Vendors disagree on
	// the wire name (max_tokens / max_completion_tokens / max_output_tokens),
	// which is an encoder concern, not a separate parameter.
	ParamMaxTokens ParamID = "max_tokens"
	// ParamFrequencyPenalty penalizes token frequency.
	ParamFrequencyPenalty ParamID = "frequency_penalty"
	// ParamPresencePenalty penalizes token presence.
	ParamPresencePenalty ParamID = "presence_penalty"
	// ParamSeed requests reproducible sampling.
	ParamSeed ParamID = "seed"
	// ParamToolChoice steers tool selection.
	ParamToolChoice ParamID = "tool_choice"
	// ParamParallelToolCalls allows several tool calls per assistant turn.
	ParamParallelToolCalls ParamID = "parallel_tool_calls"

	// ParamThinkingMode turns reasoning on, off, or hands the decision to the
	// model. Its wire spelling is the single largest source of vendor
	// divergence: enable_thinking, thinking.type, chat_template_kwargs,
	// reasoning.effort=none, or nothing at all for thinking-only models.
	ParamThinkingMode ParamID = "thinking.mode"
	// ParamThinkingEffort selects reasoning depth from a vendor's own ladder.
	// The vocabularies genuinely differ, so this is an enum over the vendor's
	// documented values rather than a normalized scale.
	ParamThinkingEffort ParamID = "thinking.effort"
	// ParamThinkingBudget caps reasoning tokens.
	ParamThinkingBudget ParamID = "thinking.budget"
	// ParamThinkingSummary requests a readable summary of the reasoning, for
	// the protocols that expose one. It is separate from the mode because a
	// model can reason without surfacing any of it: OpenAI's Responses API
	// omits the summary entirely unless it is asked for.
	ParamThinkingSummary ParamID = "thinking.summary"
)

// String reports the parameter id for logging and diagnostics.
func (p ParamID) String() string { return string(p) }

// Thinking mode is an enum rather than a bool because a third state is real:
// several vendors let the model decide per request (Ark `auto`, Anthropic
// adaptive, Zhipu dynamic thinking). Collapsing that into a bool would make
// those modes unreachable.
const (
	// ThinkingOff disables reasoning.
	ThinkingOff = "off"
	// ThinkingOn forces reasoning before the answer.
	ThinkingOn = "on"
	// ThinkingAuto lets the model decide whether to reason on each request.
	ThinkingAuto = "auto"
)

// Support is how a plugin dispositions a parameter.
type Support string

const (
	// SupportUser means the caller may set the parameter, within the declared
	// domain, and it reaches the wire.
	SupportUser Support = "user"
	// SupportPinned means the plugin always sends its Pin value and the caller
	// cannot override it — Moonshot's temperature=1, for instance.
	SupportPinned Support = "pinned"
	// SupportForbidden means the parameter must never reach the wire. The
	// OpenAI reasoning models reject temperature outright, so sending a
	// caller's value would fail the request rather than degrade politely.
	SupportForbidden Support = "forbidden"
)

// EnumOption is one member of a vendor's closed vocabulary, carrying the wire
// value together with the i18n key a form uses to label it.
type EnumOption struct {
	// Value is the exact string the vendor's API accepts.
	Value string `json:"value"`
	// LabelKey is an i18n key the frontend resolves; the backend never
	// hardcodes display text so language stays a frontend concern.
	LabelKey string `json:"label_key,omitempty"`
	// HelpKey optionally explains the option in the form.
	HelpKey string `json:"help_key,omitempty"`
}

// Widget is the control a form should render for a parameter. It is a hint
// derived from the parameter's shape, kept explicit so a plugin can ask for a
// slider where the generic mapping would pick a number box.
type Widget string

const (
	// WidgetSwitch renders a boolean toggle.
	WidgetSwitch Widget = "switch"
	// WidgetSelect renders a closed vocabulary.
	WidgetSelect Widget = "select"
	// WidgetNumber renders a numeric input.
	WidgetNumber Widget = "number"
	// WidgetSlider renders a bounded numeric range.
	WidgetSlider Widget = "slider"
)

// ParamUI is how a form should present a parameter. It is part of the same
// declaration as the wire encoding on purpose: a vendor that adds a knob gets
// the form field for free, and a knob that disappears cannot linger in the UI.
type ParamUI struct {
	// Hidden keeps a parameter off the form while still honoring it on the
	// wire (pinned and forbidden parameters are usually hidden).
	Hidden bool `json:"hidden,omitempty"`
	// Group buckets the field in the form, e.g. "thinking" or "sampling".
	Group string `json:"group,omitempty"`
	// LabelKey and HelpKey are i18n keys resolved by the frontend.
	LabelKey string `json:"label_key,omitempty"`
	HelpKey  string `json:"help_key,omitempty"`
	// Widget overrides the control inferred from the parameter's kind.
	Widget Widget `json:"widget,omitempty"`
	// Order sorts fields within a group; equal values keep declaration order.
	Order int `json:"order,omitempty"`
}

// Param declares one request parameter: its neutral identity, its accepted
// domain, how it reaches the wire, and how a form presents it.
//
// This single declaration is the source of truth for four surfaces — request
// building, server-side validation, the model editor form, and the debug
// endpoint's report — so they cannot drift apart the way a hand-maintained
// backend table and frontend copy always do.
type Param struct {
	// ID is the neutral parameter identity.
	ID ParamID `json:"id"`
	// Kind is the value domain.
	Kind ValueKind `json:"kind"`
	// Support dispositions the parameter; the zero value is SupportUser.
	Support Support `json:"support,omitempty"`
	// Enum is the vendor's closed vocabulary, required when Kind is KindEnum.
	Enum []EnumOption `json:"enum,omitempty"`
	// Min and Max bound a numeric parameter. Nil means unbounded on that side.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Default is what the plugin sends when the caller says nothing. Nil means
	// "send nothing and let the model's own default stand", which is different
	// from sending the vendor's documented default explicitly: it keeps the
	// request minimal and survives the vendor changing that default. A
	// non-nil default is therefore a deliberate claim that this vendor needs
	// the field on every request — Aliyun's thinking models are the case that
	// forces it, since omitting enable_thinking is not neutral there.
	Default *Value `json:"default,omitempty"`
	// Pin is the forced value for a SupportPinned parameter.
	Pin *Value `json:"pin,omitempty"`
	// Encode writes the resolved value onto the outbound draft. Nil means the
	// parameter is declared for the UI and validation but the protocol driver
	// already handles it (temperature on an OpenAI-shaped body, for instance).
	Encode Encoder `json:"-"`
	// UI is the form presentation.
	UI ParamUI `json:"ui"`
	// DocURL points at the vendor documentation this declaration follows, so a
	// reviewer can check the claim rather than trust it.
	DocURL string `json:"doc_url,omitempty"`
}

// EffectiveSupport reports the parameter's disposition, treating the zero
// value as SupportUser.
func (p Param) EffectiveSupport() Support {
	if p.Support == "" {
		return SupportUser
	}
	return p.Support
}

// EffectiveWidget reports the control to render, inferring one from the
// parameter's kind when the declaration does not override it.
func (p Param) EffectiveWidget() Widget {
	if p.UI.Widget != "" {
		return p.UI.Widget
	}
	switch p.Kind {
	case KindBool:
		return WidgetSwitch
	case KindEnum:
		return WidgetSelect
	case KindFloat:
		if p.Min != nil && p.Max != nil {
			return WidgetSlider
		}
		return WidgetNumber
	default:
		return WidgetNumber
	}
}

// AllowsValue reports whether v is inside the parameter's declared domain.
// Validation lives with the declaration so the API, the form, and the request
// path all reject the same values.
func (p Param) AllowsValue(v Value) bool {
	if v.Kind != p.Kind {
		return false
	}
	switch p.Kind {
	case KindEnum:
		for _, opt := range p.Enum {
			if opt.Value == v.Str {
				return true
			}
		}
		return false
	case KindInt, KindFloat:
		if p.Min != nil && v.Num < *p.Min {
			return false
		}
		if p.Max != nil && v.Num > *p.Max {
			return false
		}
		return true
	default:
		return true
	}
}

// Float returns a pointer to f, for populating Min, Max, and numeric bounds
// without a temporary at every call site.
func Float(f float64) *float64 { return &f }

// Ptr returns a pointer to v, for populating Default and Pin inline.
func Ptr(v Value) *Value { return &v }
