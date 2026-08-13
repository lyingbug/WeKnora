package encoding

import (
	"fmt"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// The constraints below express the cross-parameter rules vendors document but
// no per-parameter domain can capture. They run before encoding and record a
// note for every adjustment, so a request that came back without thinking can
// be explained rather than guessed at.

// ThinkingActive reports whether reasoning may happen, which is the predicate
// the depth constraints share.
//
// Only an explicit off makes it false. An unresolved mode means the caller had
// no opinion and the model's own default stands — and for a reasoning model
// that default is to think, so treating silence as "off" would discard a depth
// setting the request was going to honor. Auto is active too: the model may
// still choose to think, and a budget applies when it does.
func ThinkingActive(p *spi.Plan) bool {
	v, ok := p.Value(spi.ParamThinkingMode)
	if !ok {
		return true
	}
	mode, _ := v.Enum()
	return mode != spi.ThinkingOff
}

// DependsOnThinking drops depth controls when thinking is off.
//
// It is what keeps a depth parameter from overwriting the mode on vendors
// where both live in the same field — DeepSeek's Responses-format control
// spells "off" as `reasoning.effort: none`, so a leftover effort value would
// silently re-enable reasoning. It also prevents the contradictory request of
// a token budget with thinking disabled, which vendors reject.
type DependsOnThinking struct {
	// Params are the depth controls that require active thinking.
	Params []spi.ParamID
}

// ID reports the rule name.
func (d DependsOnThinking) ID() string { return "depends-on-thinking" }

// Apply drops the dependent parameters when thinking is off.
func (d DependsOnThinking) Apply(p *spi.Plan) error {
	if ThinkingActive(p) {
		return nil
	}
	for _, id := range d.Params {
		p.Drop(id, spi.NoteConstrained, d.ID(),
			fmt.Sprintf("thinking is off, so %s does not apply", id))
	}
	return nil
}

// StreamOnlyThinking turns thinking off for non-streaming requests.
//
// Alibaba Cloud documents that several open-source Qwen3 models support
// thinking only in streaming mode and return an error otherwise. Forcing the
// mode off locally turns a vendor error into a working non-streaming answer,
// and the note records that the downgrade happened.
type StreamOnlyThinking struct{}

// ID reports the rule name.
func (StreamOnlyThinking) ID() string { return "stream-only-thinking" }

// Apply forces the mode off when the request does not stream.
func (s StreamOnlyThinking) Apply(p *spi.Plan) error {
	if p.Stream || !ThinkingActive(p) {
		return nil
	}
	if _, declared := p.Param(spi.ParamThinkingMode); !declared {
		return nil
	}
	p.Adjust(spi.ParamThinkingMode, spi.EnumValue(spi.ThinkingOff), s.ID(),
		"this model supports thinking only in streaming mode; thinking was turned off for this non-streaming request")
	return nil
}

// BudgetBelowMaxTokens keeps a thinking budget under the output ceiling.
//
// Anthropic requires budget_tokens to be smaller than max_tokens because
// thinking tokens count against the same ceiling, and rejects a request where
// it is not. Raising the ceiling rather than lowering the budget preserves
// what the caller asked to spend on reasoning while leaving room for an actual
// answer.
type BudgetBelowMaxTokens struct {
	// Budget and MaxTokens are the parameters to relate.
	Budget    spi.ParamID
	MaxTokens spi.ParamID
	// Headroom is the number of tokens reserved for the answer itself.
	Headroom int
}

// ID reports the rule name.
func (b BudgetBelowMaxTokens) ID() string { return "budget-below-max-tokens" }

// Apply raises the output ceiling so the budget fits beneath it.
func (b BudgetBelowMaxTokens) Apply(p *spi.Plan) error {
	budgetValue, ok := p.Value(b.Budget)
	if !ok {
		return nil
	}
	budget, ok := budgetValue.Int()
	if !ok {
		return nil
	}
	headroom := b.Headroom
	if headroom <= 0 {
		headroom = 1
	}
	required := budget + headroom

	maxValue, hasMax := p.Value(b.MaxTokens)
	current, _ := maxValue.Int()
	if hasMax && current >= required {
		return nil
	}
	if _, declared := p.Param(b.MaxTokens); !declared {
		return fmt.Errorf("%s needs %s to be declared so the budget can fit beneath it", b.ID(), b.MaxTokens)
	}
	p.Adjust(b.MaxTokens, spi.IntValue(required), b.ID(),
		fmt.Sprintf("thinking budget %d must stay below %s; raised it to %d to leave room for the answer",
			budget, b.MaxTokens, required))
	return nil
}

// RequireMaxTokens supplies an output ceiling when the caller omits one.
//
// The Anthropic Messages API requires max_tokens on every request, unlike the
// OpenAI-shaped protocols where it is optional. Failing the request locally
// would be correct but unhelpful, so the plugin declares the fallback it wants
// and the note records that it was applied.
type RequireMaxTokens struct {
	// Param is the ceiling parameter.
	Param spi.ParamID
	// Fallback is the value to send when the caller omits one.
	Fallback int
}

// ID reports the rule name.
func (r RequireMaxTokens) ID() string { return "require-max-tokens" }

// Apply fills in the fallback ceiling.
func (r RequireMaxTokens) Apply(p *spi.Plan) error {
	if p.Has(r.Param) {
		return nil
	}
	if _, declared := p.Param(r.Param); !declared {
		return fmt.Errorf("%s needs %s to be declared", r.ID(), r.Param)
	}
	p.Set(r.Param, spi.IntValue(r.Fallback))
	p.Note(r.Param, spi.NoteDefaulted, r.ID(),
		fmt.Sprintf("this protocol requires %s; sending the plugin default of %d", r.Param, r.Fallback))
	return nil
}

// ExclusiveWith drops one parameter when another is present.
//
// It expresses the vendor rules where two controls contradict each other, such
// as an effort ladder and a token budget that both claim to size the same
// reasoning pass.
type ExclusiveWith struct {
	// Keep is the parameter that wins.
	Keep spi.ParamID
	// Drop is the parameter removed when Keep is present.
	Drop spi.ParamID
}

// ID reports the rule name.
func (e ExclusiveWith) ID() string { return "exclusive-with" }

// Apply removes the losing parameter.
func (e ExclusiveWith) Apply(p *spi.Plan) error {
	if !p.Has(e.Keep) || !p.Has(e.Drop) {
		return nil
	}
	p.Drop(e.Drop, spi.NoteConstrained, e.ID(),
		fmt.Sprintf("%s and %s cannot be sent together; %s takes precedence", e.Keep, e.Drop, e.Keep))
	return nil
}
