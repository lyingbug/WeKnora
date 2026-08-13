package vendors

import (
	"github.com/Tencent/WeKnora/internal/models/llm/encoding"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// OpenAI, and the Azure deployment of it.
//
// Two facts drive these declarations. First, the reasoning models reject the
// sampling parameters the general models accept and require
// max_completion_tokens in place of max_tokens, so they are a separate
// descriptor rather than a branch inside one. Second, reasoning depth is a
// model-dependent ladder: the documentation lists none, minimal, low, medium,
// high, xhigh, and max, and says which rungs exist varies by model, so the
// declaration offers the full published ladder and lets the vendor reject a
// rung its model does not implement.
//
// Docs: https://developers.openai.com/api/docs/guides/reasoning

// openAIEffortLadder is the published reasoning-effort vocabulary. Depth and
// the on/off toggle are the same field here, which is why `none` doubles as
// the off switch through encoding.EffortNone.
var openAIEffortLadder = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

const (
	openAIBaseURL      = "https://api.openai.com/v1"
	openAIReasoningDoc = "https://developers.openai.com/api/docs/guides/reasoning"
)

// openAIReasoningModels matches the o-series and GPT-5 and later families,
// which is the boundary the API itself draws for the restricted parameter set.
var openAIReasoningModels = spi.ModelMatcher{
	Pattern: `^(o[1-9]|gpt-[5-9]|gpt-1[0-9])`,
}

// reasoningEffortParam declares the depth ladder on the Chat Completions
// spelling of the field.
func reasoningEffortParam(values ...string) spi.Param {
	p := encoding.ThinkingEffort(encoding.Field{Key: "reasoning_effort"}, values...)
	p.DocURL = openAIReasoningDoc
	return p
}

// openAIReasoningParams is the parameter set shared by OpenAI and Azure
// reasoning deployments: no sampling knobs, a renamed output ceiling, and the
// effort ladder doubling as the thinking toggle.
func openAIReasoningParams() []spi.Param {
	effort := encoding.Field{Key: "reasoning_effort"}
	return []spi.Param{
		encoding.ThinkingMode(encoding.EffortNone{Encoder: effort, Off: "none"},
			spi.ThinkingOn, spi.ThinkingOff),
		reasoningEffortParam(openAIEffortLadder...),
		// The reasoning models reject these outright rather than ignoring
		// them, so they must leave the body entirely.
		encoding.Forbidden(spi.ParamTemperature, spi.KindFloat, "temperature"),
		encoding.Forbidden(spi.ParamTopP, spi.KindFloat, "top_p"),
		encoding.Forbidden(spi.ParamFrequencyPenalty, spi.KindFloat, "frequency_penalty"),
		encoding.Forbidden(spi.ParamPresencePenalty, spi.KindFloat, "presence_penalty"),
		encoding.MaxTokensAs("max_completion_tokens"),
	}
}

// openAIReasoningConstraints keeps the toggle and the ladder from fighting
// over the single field they share.
func openAIReasoningConstraints() []spi.Constraint {
	return []spi.Constraint{
		encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingEffort}},
	}
}

func init() {
	// OpenAI reasoning models on Chat Completions.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "openai",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "OpenAI (reasoning)",
		Models:      openAIReasoningModels,

		DefaultBaseURL: openAIBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params:         openAIReasoningParams(),
		Constraints:    openAIReasoningConstraints(),
		DocURL:         openAIReasoningDoc,
	})

	// OpenAI general models on Chat Completions.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "openai",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "OpenAI",

		DefaultBaseURL: openAIBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params:         encoding.SamplingSet(2),
		DocURL:         "https://platform.openai.com/docs/api-reference/chat",
	})

	// OpenAI on the Responses protocol, where reasoning is a nested object and
	// the output ceiling is spelled max_output_tokens. Offering it as a second
	// protocol for the same vendor is what lets a model choose between them
	// without a second provider entry.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "openai",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIResponses,
		DisplayName: "OpenAI (Responses)",

		DefaultBaseURL: openAIBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params: []spi.Param{
			encoding.ThinkingMode(
				encoding.EffortNone{Encoder: encoding.Nested("reasoning", "effort"), Off: "none"},
				spi.ThinkingOn, spi.ThinkingOff),
			responsesEffortParam(openAIEffortLadder...),
			responsesSummaryParam(),
			encoding.Temperature(0, 2),
			encoding.TopP(),
			encoding.MaxTokensAs("max_output_tokens"),
		},
		Constraints: []spi.Constraint{
			encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingEffort}},
		},
		DocURL: openAIReasoningDoc,
	})

	// Azure OpenAI mirrors the model families but authenticates with the
	// api-key header instead of a bearer token.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "azure_openai",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Azure OpenAI (reasoning)",
		Models:      openAIReasoningModels,

		Auth:        spi.Auth{Kind: spi.AuthHeader, Header: "api-key"},
		Params:      openAIReasoningParams(),
		Constraints: openAIReasoningConstraints(),
		DocURL:      openAIReasoningDoc,
	})

	spi.MustRegister(spi.Descriptor{
		Vendor:      "azure_openai",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Azure OpenAI",

		Auth:   spi.Auth{Kind: spi.AuthHeader, Header: "api-key"},
		Params: encoding.SamplingSet(2),
		DocURL: "https://learn.microsoft.com/azure/ai-services/openai/reference",
	})
}
