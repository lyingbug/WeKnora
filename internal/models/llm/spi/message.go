package spi

import (
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/types"
)

// This file holds the neutral request vocabulary: the messages, tools, and
// options a caller expresses without naming a vendor. Protocol drivers render
// it onto their own wire, so it lives with the seam definition rather than
// inside any one protocol or the chat package that predates them.
//
// The chat package aliases these types, so existing callers are unaffected and
// there is exactly one definition of a message in the codebase.

// MessageContentPart is one span of a multi-part message, which is how text
// and images travel together.
type MessageContentPart struct {
	// Type is "text" or "image_url".
	Type string `json:"type"`
	// Text carries the span when Type is "text".
	Text string `json:"text,omitempty"`
	// ImageURL carries the span when Type is "image_url".
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL references an image by URL or inline data URI.
type ImageURL struct {
	// URL is an http(s) URL or a base64 data URI.
	URL string `json:"url"`
	// Detail requests a resolution tier: "auto", "low", or "high".
	Detail string `json:"detail,omitempty"`
}

// FunctionCall is a tool invocation's name and JSON-encoded arguments.
type FunctionCall struct {
	Name string `json:"name"`
	// Arguments is a JSON object encoded as a string, as every protocol
	// transports it.
	Arguments string `json:"arguments"`
}

// ToolCall is one tool invocation an assistant turn produced.
type ToolCall struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	Function FunctionCall `json:"function"`
	// ProviderMetadata carries vendor state that must survive a round trip,
	// such as Gemini's thought signatures or Anthropic's block signature.
	// Dropping it changes the meaning of the replayed turn, so it travels with
	// the call rather than being reconstructed.
	ProviderMetadata types.ToolCallMetadata `json:"provider_metadata,omitempty"`
}

// Message is one turn of the conversation.
type Message struct {
	// Role is "system", "user", "assistant", or "tool".
	Role string `json:"role"`
	// Content is the plain-text body.
	Content string `json:"content"`
	// MultiContent carries a mixed text and image body.
	MultiContent []MessageContentPart `json:"multi_content,omitempty"`
	// Name is the tool name on a tool-role message.
	Name string `json:"name,omitempty"`
	// ToolCallID links a tool-role message to the call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// ToolCalls are the calls an assistant turn made.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Images are image references attached to the current user turn.
	Images []string `json:"images,omitempty"`
	// ReasoningContent is the reasoning this assistant turn produced.
	//
	// Whether it must be replayed is a vendor fact, declared as a descriptor's
	// ReasoningReplay: DeepSeek answers 400 when a tool-calling turn's
	// reasoning is dropped, Anthropic requires thinking blocks back verbatim,
	// and vendors that need neither ignore the field.
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// ReasoningSignature is the vendor's integrity token over the reasoning,
	// replayed alongside it where the vendor issues one.
	ReasoningSignature string `json:"reasoning_signature,omitempty"`
}

// FunctionDef declares a callable tool.
type FunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Parameters is a JSON Schema object.
	Parameters json.RawMessage `json:"parameters"`
}

// Tool is a tool the model may call.
type Tool struct {
	// Type is "function".
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// Options are the generation settings a caller supplies.
//
// It keeps the flat, vendor-neutral shape callers already use. The plugin layer
// translates it into parameter values, which is where the vendor differences
// live — a caller sets Thinking, and whether that becomes enable_thinking,
// thinking.type, or a reasoning effort rung is the descriptor's business.
type Options struct {
	Temperature         float64 `json:"temperature"`
	TopP                float64 `json:"top_p"`
	Seed                int     `json:"seed"`
	MaxTokens           int     `json:"max_tokens"`
	MaxCompletionTokens int     `json:"max_completion_tokens"`
	FrequencyPenalty    float64 `json:"frequency_penalty"`
	PresencePenalty     float64 `json:"presence_penalty"`
	// Thinking is the neutral reasoning toggle; nil defers to the model.
	Thinking *bool `json:"thinking"`
	// ThinkingEffort and ThinkingBudget are the depth controls, empty or zero
	// when the caller has no opinion. A value a vendor does not accept is
	// reported in the plan rather than sent.
	ThinkingEffort string `json:"thinking_effort,omitempty"`
	ThinkingBudget int    `json:"thinking_budget,omitempty"`

	Tools             []Tool          `json:"tools,omitempty"`
	ToolChoice        string          `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Format            json.RawMessage `json:"format,omitempty"`
}

// ParamValues translates the caller's options into neutral parameter values.
//
// Only fields the caller actually set become values: a zero temperature is
// indistinguishable from an unset one in this flat struct, and sending it
// would override a model default the caller never meant to touch. That
// asymmetry is the price of the legacy shape, and confining it to this one
// function keeps it out of every plugin.
func (o *Options) ParamValues() map[ParamID]Value {
	values := map[ParamID]Value{}
	if o == nil {
		return values
	}
	if o.Temperature > 0 {
		values[ParamTemperature] = FloatValue(o.Temperature)
	}
	if o.TopP > 0 {
		values[ParamTopP] = FloatValue(o.TopP)
	}
	if o.FrequencyPenalty != 0 {
		values[ParamFrequencyPenalty] = FloatValue(o.FrequencyPenalty)
	}
	if o.PresencePenalty != 0 {
		values[ParamPresencePenalty] = FloatValue(o.PresencePenalty)
	}
	if o.Seed != 0 {
		values[ParamSeed] = IntValue(o.Seed)
	}
	if maxTokens := o.EffectiveMaxTokens(); maxTokens > 0 {
		values[ParamMaxTokens] = IntValue(maxTokens)
	}
	if o.Thinking != nil {
		mode := ThinkingOff
		if *o.Thinking {
			mode = ThinkingOn
		}
		values[ParamThinkingMode] = EnumValue(mode)
	}
	if o.ThinkingEffort != "" {
		values[ParamThinkingEffort] = EnumValue(o.ThinkingEffort)
	}
	if o.ThinkingBudget > 0 {
		values[ParamThinkingBudget] = IntValue(o.ThinkingBudget)
	}
	if o.ToolChoice != "" {
		values[ParamToolChoice] = EnumValue(o.ToolChoice)
	}
	if o.ParallelToolCalls != nil {
		values[ParamParallelToolCalls] = BoolValue(*o.ParallelToolCalls)
	}
	return values
}

// EffectiveMaxTokens reports the output ceiling, accepting either spelling the
// caller may have used.
func (o *Options) EffectiveMaxTokens() int {
	if o == nil {
		return 0
	}
	if o.MaxTokens > 0 {
		return o.MaxTokens
	}
	return o.MaxCompletionTokens
}
