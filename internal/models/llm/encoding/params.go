package encoding

import (
	"fmt"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// The constructors below build the parameter declarations vendors share, so a
// descriptor reads as a list of claims about one vendor rather than a wall of
// struct literals. A vendor that differs overrides the field it differs in;
// everything else stays the documented baseline.

// UI groups. The frontend renders one section per group, in this order.
const (
	// GroupSampling holds temperature and the other generation controls.
	GroupSampling = "sampling"
	// GroupThinking holds the reasoning controls.
	GroupThinking = "thinking"
	// GroupLimits holds output ceilings.
	GroupLimits = "limits"
)

// labelKey and helpKey build the i18n keys the frontend resolves. The backend
// never carries display text: it declares structure and identity, the frontend
// owns language.
func labelKey(id spi.ParamID) string { return fmt.Sprintf("model.param.%s.label", id) }
func helpKey(id spi.ParamID) string  { return fmt.Sprintf("model.param.%s.help", id) }

// optionKey builds the i18n key for one enum option.
func optionKey(id spi.ParamID, value string) string {
	return fmt.Sprintf("model.param.%s.option.%s", id, value)
}

// Options turns a vendor's documented vocabulary into enum options with their
// i18n keys attached.
func Options(id spi.ParamID, values ...string) []spi.EnumOption {
	out := make([]spi.EnumOption, 0, len(values))
	for _, v := range values {
		out = append(out, spi.EnumOption{Value: v, LabelKey: optionKey(id, v)})
	}
	return out
}

// Temperature declares the sampling temperature over a vendor's documented
// range. Vendors do differ here: the OpenAI-shaped range is 0–2 while several
// Chinese vendors document 0–1.
func Temperature(min, max float64) spi.Param {
	return numeric(spi.ParamTemperature, spi.KindFloat, min, max, GroupSampling, 10,
		Field{Key: "temperature"})
}

// TopP declares nucleus sampling mass.
func TopP() spi.Param {
	return numeric(spi.ParamTopP, spi.KindFloat, 0, 1, GroupSampling, 20,
		Field{Key: "top_p"})
}

// FrequencyPenalty declares the frequency penalty.
func FrequencyPenalty() spi.Param {
	return numeric(spi.ParamFrequencyPenalty, spi.KindFloat, -2, 2, GroupSampling, 30,
		Field{Key: "frequency_penalty"})
}

// PresencePenalty declares the presence penalty.
func PresencePenalty() spi.Param {
	return numeric(spi.ParamPresencePenalty, spi.KindFloat, -2, 2, GroupSampling, 40,
		Field{Key: "presence_penalty"})
}

// Seed declares a reproducible-sampling seed.
func Seed() spi.Param {
	p := numeric(spi.ParamSeed, spi.KindInt, 0, 0, GroupSampling, 50, Field{Key: "seed"})
	p.Min, p.Max = nil, nil
	return p
}

// MaxTokens declares the output ceiling under its standard name.
func MaxTokens() spi.Param {
	return MaxTokensAs("max_tokens")
}

// MaxTokensAs declares the output ceiling under a vendor-specific name,
// removing the protocol's canonical spelling. OpenAI's reasoning models
// require max_completion_tokens and the Responses protocol uses
// max_output_tokens, both in place of max_tokens rather than beside it.
func MaxTokensAs(wire string) spi.Param {
	var encoder spi.Encoder = Field{Key: wire}
	if wire != "max_tokens" {
		encoder = AliasField{Canonical: "max_tokens", Wire: wire}
	}
	p := numeric(spi.ParamMaxTokens, spi.KindInt, 1, 0, GroupLimits, 10, encoder)
	p.Max = nil
	p.UI.Widget = spi.WidgetNumber
	return p
}

// ThinkingMode declares the reasoning toggle with the modes a vendor supports.
// Passing only spi.ThinkingOn and spi.ThinkingOff yields a two-state control;
// adding spi.ThinkingAuto exposes the vendor's model-decides mode.
func ThinkingMode(encoder spi.Encoder, modes ...string) spi.Param {
	if len(modes) == 0 {
		modes = []string{spi.ThinkingOn, spi.ThinkingOff}
	}
	return spi.Param{
		ID:     spi.ParamThinkingMode,
		Kind:   spi.KindEnum,
		Enum:   Options(spi.ParamThinkingMode, modes...),
		Encode: encoder,
		UI: spi.ParamUI{
			Group:    GroupThinking,
			LabelKey: labelKey(spi.ParamThinkingMode),
			HelpKey:  helpKey(spi.ParamThinkingMode),
			Order:    10,
		},
	}
}

// ThinkingEffort declares a reasoning-depth ladder over the vendor's own
// vocabulary. The ladders are genuinely different — OpenAI, Zhipu, DeepSeek,
// and Alibaba each publish their own rungs — so this takes the values rather
// than normalizing them into a shared scale that would send rungs the vendor
// does not accept.
func ThinkingEffort(encoder spi.Encoder, values ...string) spi.Param {
	return spi.Param{
		ID:     spi.ParamThinkingEffort,
		Kind:   spi.KindEnum,
		Enum:   Options(spi.ParamThinkingEffort, values...),
		Encode: encoder,
		UI: spi.ParamUI{
			Group:    GroupThinking,
			LabelKey: labelKey(spi.ParamThinkingEffort),
			HelpKey:  helpKey(spi.ParamThinkingEffort),
			Order:    20,
		},
	}
}

// ThinkingBudget declares a reasoning-token cap over the vendor's documented
// range.
func ThinkingBudget(encoder spi.Encoder, min, max int) spi.Param {
	p := numeric(spi.ParamThinkingBudget, spi.KindInt, float64(min), float64(max),
		GroupThinking, 30, encoder)
	if max <= 0 {
		p.Max = nil
	}
	p.UI.Widget = spi.WidgetNumber
	return p
}

// Forbidden declares a parameter the vendor rejects, so the field is removed
// from the request and the form hides the control.
//
// Declaring it is not the same as omitting it: the protocol driver writes a
// canonical body from the caller's generic options, so a forbidden parameter
// must be actively removed, and saying so here is what tells the form not to
// offer a knob that cannot work.
func Forbidden(id spi.ParamID, kind spi.ValueKind, wireKey string) spi.Param {
	return spi.Param{
		ID:      id,
		Kind:    kind,
		Support: spi.SupportForbidden,
		Encode:  Field{Key: wireKey},
		UI:      spi.ParamUI{Hidden: true},
	}
}

// Pinned declares a parameter the vendor accepts at exactly one value.
func Pinned(id spi.ParamID, kind spi.ValueKind, value spi.Value, encoder spi.Encoder) spi.Param {
	return spi.Param{
		ID:      id,
		Kind:    kind,
		Support: spi.SupportPinned,
		Pin:     spi.Ptr(value),
		Encode:  encoder,
		UI:      spi.ParamUI{Hidden: true},
	}
}

// numeric builds a bounded numeric parameter with its UI metadata.
func numeric(id spi.ParamID, kind spi.ValueKind, min, max float64, group string, order int, encoder spi.Encoder) spi.Param {
	return spi.Param{
		ID:     id,
		Kind:   kind,
		Min:    spi.Float(min),
		Max:    spi.Float(max),
		Encode: encoder,
		UI: spi.ParamUI{
			Group:    group,
			LabelKey: labelKey(id),
			HelpKey:  helpKey(id),
			Order:    order,
		},
	}
}

// SamplingSet returns the sampling parameters an OpenAI-shaped vendor supports
// without deviation, which is the majority case.
func SamplingSet(maxTemperature float64) []spi.Param {
	return []spi.Param{
		Temperature(0, maxTemperature),
		TopP(),
		FrequencyPenalty(),
		PresencePenalty(),
		Seed(),
		MaxTokens(),
	}
}
