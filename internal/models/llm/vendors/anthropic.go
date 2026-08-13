package vendors

import (
	"github.com/Tencent/WeKnora/internal/models/llm/encoding"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// Anthropic Claude on the Messages protocol.
//
// Claude has two thinking modes and they are not interchangeable, which is why
// this file registers two descriptors instead of one with a flag:
//
//   - Manual (extended) thinking, `{"thinking": {"type": "enabled",
//     "budget_tokens": N}}`, is the only mode on Claude 4.5 and earlier. It is
//     deprecated on the 4.6 generation and returns 400 on 4.7 and later.
//   - Adaptive thinking, `{"thinking": {"type": "adaptive"}}` with depth set by
//     `output_config.effort`, is available from 4.6 onward and rejects
//     budget_tokens.
//
// Sending the wrong one is a hard error rather than a degraded response, so the
// model matcher is doing real work here.
//
// Docs: https://platform.claude.com/docs/en/build-with-claude/thinking
//
//	https://platform.claude.com/docs/en/build-with-claude/extended-thinking
//	https://platform.claude.com/docs/en/build-with-claude/effort
const (
	anthropicBaseURL     = "https://api.anthropic.com"
	anthropicVersion     = "2023-06-01"
	anthropicThinkingDoc = "https://platform.claude.com/docs/en/build-with-claude/thinking"
	anthropicExtendedDoc = "https://platform.claude.com/docs/en/build-with-claude/extended-thinking"
	anthropicEffortDoc   = "https://platform.claude.com/docs/en/build-with-claude/effort"

	// anthropicMinBudget is the documented floor; the API rejects less.
	anthropicMinBudget = 1024
	// anthropicAnswerHeadroom is the room left for the answer when a budget
	// forces the output ceiling up. Thinking tokens count against max_tokens,
	// so a ceiling equal to the budget would leave nothing to answer with.
	anthropicAnswerHeadroom = 4096
	// anthropicDefaultMaxTokens is the fallback ceiling, since the Messages
	// API requires max_tokens on every request unlike the OpenAI-shaped
	// protocols where it is optional.
	anthropicDefaultMaxTokens = 1024
)

// anthropicAdaptiveModels matches the generations that support adaptive
// thinking: 4.6 and later, and the 5 series. Claude 4.5 and earlier, and the
// older claude-3-x names, fall through to the manual descriptor.
var anthropicAdaptiveModels = spi.ModelMatcher{
	Pattern: `^claude-[a-z]+-(4-[6-9]|[5-9])`,
}

// anthropicAuth is shared by both descriptors: the key travels in x-api-key,
// and the version header is mandatory on every request.
var anthropicAuth = spi.Auth{
	Kind:   spi.AuthHeader,
	Header: "x-api-key",
	Static: map[string]string{"anthropic-version": anthropicVersion},
}

func init() {
	// Adaptive thinking: Claude decides whether and how deeply to think, and
	// effort is the only depth control. Both "on" and "auto" map to adaptive
	// because these models have no always-think mode — asking for more
	// thinking means raising the effort, not pinning a budget.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "anthropic",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolAnthropicMessages,
		DisplayName: "Anthropic Claude (adaptive thinking)",
		Models:      anthropicAdaptiveModels,

		DefaultBaseURL: anthropicBaseURL,
		Auth:           anthropicAuth,
		Params: []spi.Param{
			encoding.ThinkingMode(
				encoding.ThinkingObject{Key: "thinking", On: "adaptive", Off: "disabled", Auto: "adaptive"},
				thinkingModes(true)...),
			// Effort lives in output_config, not in the thinking object. The
			// documentation calls this out explicitly, and "adaptive" is a
			// thinking mode rather than an effort rung.
			anthropicEffortParam(),
			encoding.Temperature(0, 1),
			encoding.TopP(),
			encoding.MaxTokens(),
		},
		Constraints: []spi.Constraint{
			encoding.RequireMaxTokens{Param: spi.ParamMaxTokens, Fallback: anthropicDefaultMaxTokens},
		},
		ReasoningReplay: spi.ReplayAlways,
		DocURL:          anthropicThinkingDoc,
	})

	// Manual extended thinking: a fixed token budget, with the constraints the
	// documentation states — a 1,024 floor and a budget strictly below the
	// output ceiling.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "anthropic",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolAnthropicMessages,
		DisplayName: "Anthropic Claude (extended thinking)",

		DefaultBaseURL: anthropicBaseURL,
		Auth:           anthropicAuth,
		Params: []spi.Param{
			encoding.ThinkingMode(
				encoding.ThinkingObject{Key: "thinking", On: "enabled", Off: "disabled"},
				thinkingModes(false)...),
			anthropicBudgetParam(),
			encoding.Temperature(0, 1),
			encoding.TopP(),
			encoding.MaxTokens(),
		},
		Constraints: []spi.Constraint{
			// Order matters: drop a budget that does not apply, then supply the
			// required ceiling, then raise it if the surviving budget needs
			// more room.
			encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingBudget}},
			encoding.RequireMaxTokens{Param: spi.ParamMaxTokens, Fallback: anthropicDefaultMaxTokens},
			encoding.BudgetBelowMaxTokens{
				Budget:    spi.ParamThinkingBudget,
				MaxTokens: spi.ParamMaxTokens,
				Headroom:  anthropicAnswerHeadroom,
			},
		},
		ReasoningReplay: spi.ReplayAlways,
		DocURL:          anthropicExtendedDoc,
	})
}

// anthropicEffortParam declares the effort ladder at output_config.effort.
// It is declared only on the adaptive descriptor: the effort documentation
// says the parameter is broadly available, while the extended-thinking page
// names Opus 4.5 as the only manual-mode model supporting it. Where the two
// disagree, sending nothing is the safe reading — an unsupported field is a
// 400, whereas omitting it costs only the vendor's own default.
func anthropicEffortParam() spi.Param {
	p := encoding.ThinkingEffort(encoding.Nested("output_config", "effort"),
		"low", "medium", "high", "xhigh", "max")
	p.DocURL = anthropicEffortDoc
	return p
}

// anthropicBudgetParam declares the manual thinking budget.
func anthropicBudgetParam() spi.Param {
	p := encoding.ThinkingBudget(encoding.Nested("thinking", "budget_tokens"), anthropicMinBudget, 0)
	p.DocURL = anthropicExtendedDoc
	return p
}
