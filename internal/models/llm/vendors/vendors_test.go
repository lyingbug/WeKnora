package vendors

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// The table below is the executable form of every wire claim these plugins
// make. Each case names the vendor documentation it encodes, so a reviewer can
// check the expected body against the vendor's own examples rather than
// against the implementation that produced it.
//
// It is also the regression net for the behavior this seam replaced: the
// previous design expressed these differences as provider branches and a
// four-value thinking_control enum, and a change to one vendor could silently
// alter another.

// canonicalBody is what an OpenAI-shaped protocol driver writes before any
// vendor encoder runs. Cases assert on the body after the plan is applied, so
// a parameter a vendor forbids must actually disappear from this.
func canonicalBody() map[string]any {
	return map[string]any{
		"model":       "placeholder",
		"messages":    []any{},
		"temperature": 0.7,
		"max_tokens":  float64(2048),
		"tool_choice": "auto",
	}
}

type wireCase struct {
	name   string
	vendor string
	model  string
	// protocol pins the descriptor when a vendor offers more than one.
	protocol spi.ProtocolID
	stream   bool
	values   map[spi.ParamID]spi.Value

	// want lists body paths that must hold a value, as dotted paths.
	want map[string]any
	// absent lists body paths that must not be present.
	absent []string
	// notes lists parameters that must carry an adjustment note, so a silent
	// behavior change cannot pass as a deliberate one.
	notes []spi.ParamID
}

func TestVendorWireFormats(t *testing.T) {
	cases := []wireCase{
		// --- DeepSeek: thinking object plus a three-rung effort ladder.
		// https://api-docs.deepseek.com/guides/thinking_mode
		{
			name:   "deepseek thinking on",
			vendor: "deepseek", model: "deepseek-chat",
			values: modeOnly(spi.ThinkingOn),
			want:   map[string]any{"thinking.type": "enabled"},
			absent: []string{"tool_choice"},
		},
		{
			name:   "deepseek thinking on with effort",
			vendor: "deepseek", model: "deepseek-chat",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingEffort: spi.EnumValue("high"),
			},
			want: map[string]any{"thinking.type": "enabled", "reasoning_effort": "high"},
		},
		{
			// Effort must not survive a disabled toggle: on the Responses
			// spelling the two share one field, and a leftover rung would turn
			// thinking back on.
			name:   "deepseek effort dropped when thinking off",
			vendor: "deepseek", model: "deepseek-chat",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOff),
				spi.ParamThinkingEffort: spi.EnumValue("high"),
			},
			want:   map[string]any{"thinking.type": "disabled"},
			absent: []string{"reasoning_effort"},
			notes:  []spi.ParamID{spi.ParamThinkingEffort},
		},
		{
			name:   "deepseek rejects an effort rung it does not publish",
			vendor: "deepseek", model: "deepseek-chat",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingEffort: spi.EnumValue("medium"),
			},
			absent: []string{"reasoning_effort"},
			notes:  []spi.ParamID{spi.ParamThinkingEffort},
		},

		// --- Alibaba Cloud Model Studio: a top-level boolean plus a budget.
		// https://help.aliyun.com/zh/model-studio/deep-thinking
		{
			// The boolean is sent even when the caller says nothing, because
			// several Qwen generations default to thinking on: omitting it
			// would mean the opposite of the caller's silence.
			name:   "aliyun pins the boolean when the caller is silent",
			vendor: "aliyun", model: "qwen3-max", stream: true,
			want:  map[string]any{"enable_thinking": false},
			notes: []spi.ParamID{spi.ParamThinkingMode},
		},
		{
			name:   "aliyun thinking on with a budget",
			vendor: "aliyun", model: "qwen3-max", stream: true,
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingBudget: spi.IntValue(500),
			},
			want: map[string]any{"enable_thinking": true, "thinking_budget": 500},
		},
		{
			// Documented: several Qwen3 models accept thinking only when
			// streaming. Downgrading locally beats a vendor error.
			name:   "aliyun downgrades thinking off stream",
			vendor: "aliyun", model: "qwen3-max", stream: false,
			values: modeOnly(spi.ThinkingOn),
			want:   map[string]any{"enable_thinking": false},
			notes:  []spi.ParamID{spi.ParamThinkingMode},
		},
		{
			name:   "aliyun non-thinking model sends no thinking fields",
			vendor: "aliyun", model: "qwen2.5-14b-instruct", stream: true,
			values: modeOnly(spi.ThinkingOn),
			absent: []string{"enable_thinking", "thinking"},
			notes:  []spi.ParamID{spi.ParamThinkingMode},
		},

		// --- Volcengine Ark: the only vendor documenting a third state.
		{
			name:   "volcengine auto",
			vendor: "volcengine", model: "doubao-seed-1-6",
			values: modeOnly(spi.ThinkingAuto),
			want:   map[string]any{"thinking.type": "auto"},
		},
		{
			name:   "volcengine budget rides inside the thinking object",
			vendor: "volcengine", model: "doubao-seed-1-6",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingBudget: spi.IntValue(32000),
			},
			want: map[string]any{"thinking.type": "enabled", "thinking.budget_tokens": 32000},
		},

		// --- Zhipu GLM: the effort ladder exists only from GLM-5.2.
		// https://docs.bigmodel.cn/cn/guide/capabilities/thinking
		{
			name:   "zhipu 5.2 accepts effort",
			vendor: "zhipu", model: "glm-5.2",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingEffort: spi.EnumValue("xhigh"),
			},
			want: map[string]any{"thinking.type": "enabled", "reasoning_effort": "xhigh"},
		},
		{
			name:   "zhipu 4.6 has no effort ladder",
			vendor: "zhipu", model: "glm-4.6",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingEffort: spi.EnumValue("xhigh"),
			},
			want:   map[string]any{"thinking.type": "enabled"},
			absent: []string{"reasoning_effort"},
			notes:  []spi.ParamID{spi.ParamThinkingEffort},
		},

		// --- OpenAI: the reasoning families reject the sampling knobs and
		// rename the ceiling. https://developers.openai.com/api/docs/guides/reasoning
		{
			name:   "openai reasoning model strips sampling and renames the ceiling",
			vendor: "openai", model: "o3-mini",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingEffort: spi.EnumValue("high"),
				spi.ParamMaxTokens:      spi.IntValue(4096),
			},
			want:   map[string]any{"reasoning_effort": "high", "max_completion_tokens": 4096},
			absent: []string{"temperature", "top_p", "max_tokens"},
		},
		{
			name:   "openai general model keeps sampling",
			vendor: "openai", model: "gpt-4o",
			values: map[spi.ParamID]spi.Value{spi.ParamTemperature: spi.FloatValue(0.3)},
			want:   map[string]any{"temperature": 0.3, "max_tokens": 2048},
		},
		{
			name:   "openai thinking off writes the none rung",
			vendor: "openai", model: "gpt-5",
			values: modeOnly(spi.ThinkingOff),
			want:   map[string]any{"reasoning_effort": "none"},
		},
		{
			name:   "openai responses nests reasoning and renames the ceiling",
			vendor: "openai", model: "gpt-5", protocol: spi.ProtocolOpenAIResponses,
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:    spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingEffort:  spi.EnumValue("medium"),
				spi.ParamThinkingSummary: spi.EnumValue("auto"),
				spi.ParamMaxTokens:       spi.IntValue(300),
			},
			want: map[string]any{
				"reasoning.effort":  "medium",
				"reasoning.summary": "auto",
				"max_output_tokens": 300,
			},
			absent: []string{"max_tokens"},
		},

		// --- Anthropic: two incompatible thinking modes chosen by model.
		// https://platform.claude.com/docs/en/build-with-claude/thinking
		{
			name:   "anthropic 4.6 uses adaptive thinking and effort",
			vendor: "anthropic", model: "claude-sonnet-4-6",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingEffort: spi.EnumValue("high"),
			},
			want: map[string]any{
				"thinking.type":        "adaptive",
				"output_config.effort": "high",
				"max_tokens":           1024,
			},
			absent: []string{"thinking.budget_tokens"},
		},
		{
			// budget_tokens must stay below max_tokens, and thinking tokens
			// count against the same ceiling, so the ceiling is raised rather
			// than the budget cut.
			name:   "anthropic 4.5 uses a budget and raises the ceiling",
			vendor: "anthropic", model: "claude-sonnet-4-5",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingBudget: spi.IntValue(10000),
			},
			want: map[string]any{
				"thinking.type":          "enabled",
				"thinking.budget_tokens": 10000,
				"max_tokens":             14096,
			},
			notes: []spi.ParamID{spi.ParamMaxTokens},
		},
		{
			name:   "anthropic drops a budget when thinking is off",
			vendor: "anthropic", model: "claude-sonnet-4-5",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOff),
				spi.ParamThinkingBudget: spi.IntValue(10000),
			},
			want:   map[string]any{"thinking.type": "disabled"},
			absent: []string{"thinking.budget_tokens"},
			notes:  []spi.ParamID{spi.ParamThinkingBudget},
		},
		{
			name:   "anthropic rejects a budget below the documented floor",
			vendor: "anthropic", model: "claude-sonnet-4-5",
			values: map[spi.ParamID]spi.Value{
				spi.ParamThinkingMode:   spi.EnumValue(spi.ThinkingOn),
				spi.ParamThinkingBudget: spi.IntValue(512),
			},
			absent: []string{"thinking.budget_tokens"},
			notes:  []spi.ParamID{spi.ParamThinkingBudget},
		},

		// --- Self-hosted deployments pass the flag to the chat template.
		{
			name:   "generic deployment uses chat_template_kwargs",
			vendor: "generic", model: "qwen3-32b",
			values: modeOnly(spi.ThinkingOn),
			want:   map[string]any{"chat_template_kwargs.enable_thinking": true},
			absent: []string{"enable_thinking", "thinking"},
		},

		// --- Moonshot pins temperature for its fixed-temperature line.
		{
			name:   "moonshot pins temperature and reports the override",
			vendor: "moonshot", model: "moonshot-v1-8k",
			values: map[spi.ParamID]spi.Value{spi.ParamTemperature: spi.FloatValue(0.2)},
			want:   map[string]any{"temperature": float64(1)},
			notes:  []spi.ParamID{spi.ParamTemperature},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc, ok := spi.Resolve(spi.Query{
				Vendor:   tc.vendor,
				Kind:     spi.KindChat,
				Model:    tc.model,
				Protocol: tc.protocol,
			})
			if !ok {
				t.Fatalf("no descriptor registered for %s/%s", tc.vendor, tc.model)
			}

			plan, err := desc.Plan(spi.Request{Model: tc.model, Stream: tc.stream, Values: tc.values})
			if err != nil {
				t.Fatalf("plan: %v", err)
			}

			draft := spi.NewDraft(desc.Protocol, tc.model, tc.stream)
			draft.Body = canonicalBody()
			if err := plan.Apply(draft); err != nil {
				t.Fatalf("apply: %v", err)
			}

			for path, want := range tc.want {
				got, ok := lookup(draft, path)
				if !ok {
					t.Errorf("body is missing %s\nbody: %s", path, dump(draft))
					continue
				}
				if !numericEqual(got, want) {
					t.Errorf("body[%s] = %#v, want %#v\nbody: %s", path, got, want, dump(draft))
				}
			}
			for _, path := range tc.absent {
				if got, ok := lookup(draft, path); ok {
					t.Errorf("body[%s] should be absent, got %#v\nbody: %s", path, got, dump(draft))
				}
			}
			for _, id := range tc.notes {
				if !hasNote(plan, id) {
					t.Errorf("expected an adjustment note for %s, got %+v", id, plan.Notes)
				}
			}
		})
	}
}

func modeOnly(mode string) map[spi.ParamID]spi.Value {
	return map[spi.ParamID]spi.Value{spi.ParamThinkingMode: spi.EnumValue(mode)}
}

// lookup reads a dotted path out of the draft body.
func lookup(d *spi.Draft, path string) (any, bool) {
	return d.GetNested(splitPath(path)...)
}

func splitPath(path string) []string {
	var out []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			out = append(out, path[start:i])
			start = i + 1
		}
	}
	return append(out, path[start:])
}

// numericEqual compares values while tolerating the int/float distinction that
// JSON round-tripping erases; the assertion is about the wire value, not the
// Go type that produced it.
func numericEqual(got, want any) bool {
	gf, gok := asFloat(got)
	wf, wok := asFloat(want)
	if gok && wok {
		return gf == wf
	}
	return got == want
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func hasNote(plan *spi.Plan, id spi.ParamID) bool {
	for _, note := range plan.Notes {
		if note.Param == id {
			return true
		}
	}
	return false
}

func dump(d *spi.Draft) string {
	out, err := json.Marshal(d.Body)
	if err != nil {
		return err.Error()
	}
	return string(out)
}
