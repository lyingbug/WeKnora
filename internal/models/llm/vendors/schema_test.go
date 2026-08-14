package vendors

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// The form schema is the fourth surface the descriptors drive. These tests
// assert that it reports the same facts the request path acts on, because the
// bug this seam replaces was precisely a form that disagreed with the wire.

func schemaFor(t *testing.T, vendor, model string) spi.FormSchema {
	t.Helper()
	desc, ok := spi.Resolve(spi.Query{Vendor: vendor, Kind: spi.KindChat, Model: model})
	if !ok {
		t.Fatalf("no descriptor for %s/%s", vendor, model)
	}
	return desc.Schema()
}

func findField(schema spi.FormSchema, id spi.ParamID) (spi.FieldSchema, string, bool) {
	for _, group := range schema.Groups {
		for _, field := range group.Fields {
			if field.ID == id {
				return field, group.Key, true
			}
		}
	}
	return spi.FieldSchema{}, "", false
}

// The form must name the wire field a control writes, and it must be the field
// the encoder actually uses rather than a category chosen separately.
func TestSchemaReportsTheWireField(t *testing.T) {
	cases := []struct {
		vendor, model string
		wantWire      string
	}{
		{"aliyun", "qwen3-max", "enable_thinking"},
		{"deepseek", "deepseek-chat", "thinking.type"},
		{"volcengine", "doubao-seed-1-6", "thinking.type"},
		{"generic", "qwen3-32b", "chat_template_kwargs.enable_thinking"},
		{"anthropic", "claude-sonnet-4-6", "thinking.type"},
	}

	for _, tc := range cases {
		t.Run(tc.vendor+"/"+tc.model, func(t *testing.T) {
			schema := schemaFor(t, tc.vendor, tc.model)
			if !schema.SupportsThinking {
				t.Fatalf("%s should report a thinking toggle", tc.vendor)
			}
			field, group, ok := findField(schema, spi.ParamThinkingMode)
			if !ok {
				t.Fatalf("no thinking control in the form")
			}
			if field.WireField != tc.wantWire {
				t.Errorf("wire field = %q, want %q", field.WireField, tc.wantWire)
			}
			if group != "thinking" {
				t.Errorf("thinking control landed in group %q", group)
			}
			if field.Widget != spi.WidgetSelect {
				t.Errorf("widget = %q, want select", field.Widget)
			}
		})
	}
}

// Only Volcengine documents a third thinking state, so only its form may offer
// one. A shared three-value control would send `auto` to vendors that reject it.
func TestSchemaOffersAutoOnlyWhereDocumented(t *testing.T) {
	ark, _, ok := findField(schemaFor(t, "volcengine", "doubao-seed-1-6"), spi.ParamThinkingMode)
	if !ok {
		t.Fatal("volcengine should expose a thinking control")
	}
	if len(ark.Options) != 3 {
		t.Errorf("volcengine should offer three modes, got %d", len(ark.Options))
	}

	deepseek, _, ok := findField(schemaFor(t, "deepseek", "deepseek-chat"), spi.ParamThinkingMode)
	if !ok {
		t.Fatal("deepseek should expose a thinking control")
	}
	if len(deepseek.Options) != 2 {
		t.Errorf("deepseek should offer two modes, got %d", len(deepseek.Options))
	}
}

// A parameter the user cannot influence must not appear as a control: a
// forbidden one would do nothing, and a pinned one would lie about its value.
func TestSchemaHidesParametersTheUserCannotSet(t *testing.T) {
	reasoning := schemaFor(t, "openai", "o3-mini")
	for _, hidden := range []spi.ParamID{spi.ParamTemperature, spi.ParamTopP} {
		if _, _, ok := findField(reasoning, hidden); ok {
			t.Errorf("%s is forbidden on reasoning models and must not be offered", hidden)
		}
	}
	if _, _, ok := findField(reasoning, spi.ParamMaxTokens); !ok {
		t.Error("the output ceiling is still settable and should be offered")
	}

	moonshot := schemaFor(t, "moonshot", "moonshot-v1-8k")
	if _, _, ok := findField(moonshot, spi.ParamTemperature); ok {
		t.Error("a pinned temperature must not be offered as a control")
	}
}

// Effort ladders differ per vendor, and the form must show the vendor's own.
func TestSchemaCarriesEachVendorsEffortLadder(t *testing.T) {
	deepseek, _, ok := findField(schemaFor(t, "deepseek", "deepseek-chat"), spi.ParamThinkingEffort)
	if !ok {
		t.Fatal("deepseek publishes an effort ladder")
	}
	if len(deepseek.Options) != 3 {
		t.Errorf("deepseek ladder has %d rungs, want 3", len(deepseek.Options))
	}

	if _, _, ok := findField(schemaFor(t, "zhipu", "glm-4.6"), spi.ParamThinkingEffort); ok {
		t.Error("GLM-4.6 has no effort ladder and must not be offered one")
	}
	if _, _, ok := findField(schemaFor(t, "zhipu", "glm-5.2"), spi.ParamThinkingEffort); !ok {
		t.Error("GLM-5.2 publishes an effort ladder")
	}
}

// A vendor offering two protocols must say so, and one offering a single
// protocol must not make the user choose.
func TestProtocolsReportedPerVendor(t *testing.T) {
	if got := spi.Protocols("openai", spi.KindChat); len(got) != 2 {
		t.Errorf("openai protocols = %v, want Chat Completions and Responses", got)
	}
	if got := spi.Protocols("deepseek", spi.KindChat); len(got) != 1 {
		t.Errorf("deepseek protocols = %v, want one", got)
	}
}
