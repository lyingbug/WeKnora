package vendors

import (
	"github.com/Tencent/WeKnora/internal/models/llm/encoding"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// The vendors below all serve an OpenAI-compatible Chat Completions endpoint
// and all support reasoning, yet no two spell it the same way. Collected here,
// the differences are easy to compare against the documentation each entry
// links; scattered across if-else branches, they were the reason this seam
// exists.

const (
	deepSeekBaseURL   = "https://api.deepseek.com"
	deepSeekThinkDoc  = "https://api-docs.deepseek.com/guides/thinking_mode"
	aliyunBaseURL     = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	aliyunThinkDoc    = "https://help.aliyun.com/zh/model-studio/deep-thinking"
	volcengineBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	volcengineDoc     = "https://www.volcengine.com/docs/82379/1494384"
	zhipuBaseURL      = "https://open.bigmodel.cn/api/paas/v4"
	zhipuThinkDoc     = "https://docs.bigmodel.cn/cn/guide/capabilities/thinking"
	lkeapDoc          = "https://cloud.tencent.com/document/product/1772/115963"
)

func init() {
	registerDeepSeek()
	registerAliyun()
	registerVolcengine()
	registerZhipu()
	registerMoonshot()
	registerLKEAP()
	registerSelfHosted()
}

// DeepSeek toggles thinking with the `thinking` object and sizes it with a
// three-rung reasoning_effort ladder. Its distinguishing requirement is on the
// response side: when a turn carries tool calls, that turn's reasoning_content
// must be replayed in every later request or the API answers 400.
//
// Docs: https://api-docs.deepseek.com/guides/thinking_mode
func registerDeepSeek() {
	spi.MustRegister(spi.Descriptor{
		Vendor:      "deepseek",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "DeepSeek",

		DefaultBaseURL: deepSeekBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params: append([]spi.Param{
			encoding.ThinkingMode(thinkingObject(""), thinkingModes(false)...),
			effortParam(encoding.Field{Key: "reasoning_effort"}, deepSeekThinkDoc, "low", "high", "max"),
			// Preserved from the previous implementation, which stripped
			// tool_choice for this vendor. Declaring it forbidden keeps that
			// behavior while making it visible: the request path removes the
			// field and the plan records that it did.
			encoding.Forbidden(spi.ParamToolChoice, spi.KindEnum, "tool_choice"),
		}, encoding.SamplingSet(2)...),
		Constraints: []spi.Constraint{
			encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingEffort}},
		},
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          deepSeekThinkDoc,
	})
}

// Alibaba Cloud Model Studio uses a plain boolean plus a token budget, both as
// top-level fields. Two properties make it the awkward one:
//
//   - The boolean must be sent on every request for hybrid-thinking models,
//     because omitting it is not neutral — several Qwen generations default to
//     thinking on, so silence means the opposite of what a caller expects.
//   - Some open-source Qwen3 models accept thinking only in streaming mode and
//     error otherwise, which the stream-only constraint downgrades locally.
//
// Docs: https://help.aliyun.com/zh/model-studio/deep-thinking
func registerAliyun() {
	thinkingModeParam := encoding.ThinkingMode(
		encoding.EnableThinkingBool{Key: "enable_thinking"}, thinkingModes(false)...)
	thinkingModeParam.Default = spi.Ptr(spi.EnumValue(spi.ThinkingOff))
	thinkingModeParam.DocURL = aliyunThinkDoc

	budget := encoding.ThinkingBudget(encoding.Field{Key: "thinking_budget"}, 1, 0)
	budget.DocURL = aliyunThinkDoc

	// Qwen thinking-capable families. Non-thinking Aliyun models fall through
	// to the catch-all descriptor, which sends no thinking fields at all.
	spi.MustRegister(spi.Descriptor{
		Vendor:      "aliyun",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Alibaba Cloud Model Studio (thinking)",
		Models: spi.ModelMatcher{
			Prefixes: []string{"qwen3", "qwen-plus", "qwen-max", "qwen-turbo", "qwq", "qvq"},
		},

		DefaultBaseURL: aliyunBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params:         append([]spi.Param{thinkingModeParam, budget}, encoding.SamplingSet(2)...),
		Constraints: []spi.Constraint{
			encoding.StreamOnlyThinking{},
			encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingBudget}},
		},
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          aliyunThinkDoc,
	})

	spi.MustRegister(spi.Descriptor{
		Vendor:         "aliyun",
		Kind:           spi.KindChat,
		Protocol:       spi.ProtocolOpenAIChat,
		DisplayName:    "Alibaba Cloud Model Studio",
		DefaultBaseURL: aliyunBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params:         encoding.SamplingSet(2),
		DocURL:         "https://help.aliyun.com/zh/model-studio/compatibility-of-openai-with-dashscope",
	})
}

// Volcengine Ark is the one vendor documenting a third thinking state: `auto`
// lets the model skip reasoning on simple questions. A boolean toggle cannot
// express that, which is why the neutral mode is an enum.
//
// Docs: https://www.volcengine.com/docs/82379/1494384
func registerVolcengine() {
	spi.MustRegister(spi.Descriptor{
		Vendor:      "volcengine",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Volcengine Ark",

		DefaultBaseURL: volcengineBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params: append([]spi.Param{
			encoding.ThinkingMode(thinkingObject("auto"), thinkingModes(true)...),
			budgetParam(encoding.Nested("thinking", "budget_tokens"), volcengineDoc),
		}, encoding.SamplingSet(2)...),
		Constraints: []spi.Constraint{
			encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingBudget}},
		},
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          volcengineDoc,
	})
}

// Zhipu GLM shares the `thinking` object with DeepSeek but publishes a
// seven-rung effort ladder, and only from GLM-5.2 onward. Declaring the ladder
// on a matcher rather than on every GLM keeps it off the models that would
// reject it.
//
// Docs: https://docs.bigmodel.cn/cn/guide/capabilities/thinking
func registerZhipu() {
	zhipuLadder := []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

	spi.MustRegister(spi.Descriptor{
		Vendor:      "zhipu",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Zhipu GLM (effort)",
		// GLM-5.2 and later support reasoning_effort; earlier thinking models
		// support only the on/off object.
		Models: spi.ModelMatcher{Pattern: `^glm-(5\.[2-9]|[6-9])`},

		DefaultBaseURL: zhipuBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params: append([]spi.Param{
			encoding.ThinkingMode(thinkingObject(""), thinkingModes(false)...),
			effortParam(encoding.Field{Key: "reasoning_effort"}, zhipuThinkDoc, zhipuLadder...),
		}, encoding.SamplingSet(1)...),
		Constraints: []spi.Constraint{
			encoding.DependsOnThinking{Params: []spi.ParamID{spi.ParamThinkingEffort}},
		},
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          zhipuThinkDoc,
	})

	spi.MustRegister(spi.Descriptor{
		Vendor:      "zhipu",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Zhipu GLM",
		Models:      spi.ModelMatcher{Prefixes: []string{"glm-4.5", "glm-4.6", "glm-4.7", "glm-5"}},

		DefaultBaseURL: zhipuBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params: append([]spi.Param{
			encoding.ThinkingMode(thinkingObject(""), thinkingModes(false)...),
		}, encoding.SamplingSet(1)...),
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          zhipuThinkDoc,
	})

	spi.MustRegister(spi.Descriptor{
		Vendor:         "zhipu",
		Kind:           spi.KindChat,
		Protocol:       spi.ProtocolOpenAIChat,
		DisplayName:    "Zhipu",
		DefaultBaseURL: zhipuBaseURL,
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params:         encoding.SamplingSet(1),
		DocURL:         "https://docs.bigmodel.cn/cn/guide/develop/openai/introduction",
	})
}

// Moonshot's v1 line accepts only temperature=1. Pinning it is a parameter
// disposition rather than request-shaping code, and the plan records the
// override so a user who set 0.2 can see what happened to it.
func registerMoonshot() {
	pinnedTemp := encoding.Pinned(spi.ParamTemperature, spi.KindFloat,
		spi.FloatValue(1), encoding.Field{Key: "temperature"})

	spi.MustRegister(spi.Descriptor{
		Vendor:      "moonshot",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Moonshot Kimi (fixed temperature)",
		Models:      spi.ModelMatcher{Prefixes: []string{"kimi-k2", "kimi-latest", "moonshot-v1"}},

		DefaultBaseURL: "https://api.moonshot.cn/v1",
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params: []spi.Param{
			pinnedTemp,
			encoding.MaxTokens(),
		},
		DocURL: "https://platform.moonshot.cn/docs/api/chat",
	})

	spi.MustRegister(spi.Descriptor{
		Vendor:         "moonshot",
		Kind:           spi.KindChat,
		Protocol:       spi.ProtocolOpenAIChat,
		DisplayName:    "Moonshot Kimi",
		DefaultBaseURL: "https://api.moonshot.cn/v1",
		Auth:           spi.Auth{Kind: spi.AuthBearer},
		Params:         encoding.SamplingSet(1),
		DocURL:         "https://platform.moonshot.cn/docs/api/chat",
	})
}

// Tencent LKEAP serves DeepSeek models. The V3 line takes the `thinking`
// object; the R1 line reasons unconditionally and rejects the toggle, so it
// falls through to a descriptor that sends none.
//
// Docs: https://cloud.tencent.com/document/product/1772/115963
func registerLKEAP() {
	spi.MustRegister(spi.Descriptor{
		Vendor:      "lkeap",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Tencent LKEAP (DeepSeek V3)",
		Models:      spi.ModelMatcher{Contains: []string{"deepseek-v3"}},

		Auth: spi.Auth{Kind: spi.AuthBearer},
		Params: append([]spi.Param{
			encoding.ThinkingMode(thinkingObject(""), thinkingModes(false)...),
		}, encoding.SamplingSet(2)...),
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          lkeapDoc,
	})

	spi.MustRegister(spi.Descriptor{
		Vendor:      "lkeap",
		Kind:        spi.KindChat,
		Protocol:    spi.ProtocolOpenAIChat,
		DisplayName: "Tencent LKEAP",

		Auth:            spi.Auth{Kind: spi.AuthBearer},
		Params:          encoding.SamplingSet(2),
		ReasoningReplay: spi.ReplayWithTools,
		DocURL:          lkeapDoc,
	})
}

// Self-hosted inference servers pass the flag through to the model's chat
// template rather than interpreting it themselves, so the field lives in
// chat_template_kwargs. This is the right default for a vLLM or SGLang
// deployment and for NVIDIA NIM, and the wrong one for any hosted vendor API —
// which is exactly the distinction a per-vendor declaration can make and a
// single global default could not.
func registerSelfHosted() {
	for _, v := range []struct{ vendor, display, doc string }{
		{"generic", "OpenAI-compatible deployment", "https://docs.vllm.ai/en/latest/features/reasoning_outputs.html"},
		{"nvidia", "NVIDIA NIM", "https://docs.nvidia.com/nim/large-language-models/latest/reasoning.html"},
		{"gpustack", "GPUStack", "https://docs.gpustack.ai"},
	} {
		spi.MustRegister(spi.Descriptor{
			Vendor:      v.vendor,
			Kind:        spi.KindChat,
			Protocol:    spi.ProtocolOpenAIChat,
			DisplayName: v.display,

			Auth: spi.Auth{Kind: spi.AuthBearer},
			Params: append([]spi.Param{
				encoding.ThinkingMode(
					encoding.ChatTemplateKwargs{Key: "chat_template_kwargs", Arg: "enable_thinking"},
					thinkingModes(false)...),
			}, encoding.SamplingSet(2)...),
			ReasoningReplay: spi.ReplayWithTools,
			DocURL:          v.doc,
		})
	}
}

// effortParam declares a reasoning ladder with its documentation link.
func effortParam(encoder spi.Encoder, doc string, values ...string) spi.Param {
	p := encoding.ThinkingEffort(encoder, values...)
	p.DocURL = doc
	return p
}

// budgetParam declares a reasoning-token cap with its documentation link.
func budgetParam(encoder spi.Encoder, doc string) spi.Param {
	p := encoding.ThinkingBudget(encoder, 1, 0)
	p.DocURL = doc
	return p
}
