// Package vendors holds the built-in model plugins: one declaration per vendor
// describing how it differs from the protocol baseline.
//
// Each declaration follows the vendor's published documentation and links to
// it, so a reviewer can check a claim instead of trusting it. A vendor that
// changes its API is a change to one declaration, and the request path, the
// validator, the model editor form, and the debug report all follow from it.
package vendors

import (
	"github.com/Tencent/WeKnora/internal/models/llm/encoding"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
)

// responsesEffortParam declares the reasoning ladder on the Responses
// protocol, where effort is nested inside the reasoning object.
func responsesEffortParam(values ...string) spi.Param {
	return encoding.ThinkingEffort(encoding.Nested("reasoning", "effort"), values...)
}

// responsesSummaryParam declares the reasoning-summary control the Responses
// protocol exposes. The documentation is explicit that no summary is returned
// unless the request opts in, so this is a real knob rather than a display
// preference the client could apply afterwards.
//
// Docs: https://developers.openai.com/api/docs/guides/reasoning
func responsesSummaryParam() spi.Param {
	p := encoding.ThinkingEffort(encoding.Nested("reasoning", "summary"),
		"auto", "concise", "detailed")
	p.ID = spi.ParamThinkingSummary
	p.Enum = encoding.Options(spi.ParamThinkingSummary, "auto", "concise", "detailed")
	p.UI.LabelKey = "model.param.thinking.summary.label"
	p.UI.HelpKey = "model.param.thinking.summary.help"
	p.UI.Order = 25
	return p
}

// thinkingObject builds the `{"thinking": {"type": ...}}` toggle shared by
// DeepSeek, Zhipu, Volcengine, and Tencent LKEAP, with the vendor's own
// spellings. Only Volcengine documents the third `auto` value, so the others
// pass an empty string and the encoder refuses to invent one.
func thinkingObject(auto string) encoding.ThinkingObject {
	return encoding.ThinkingObject{Key: "thinking", On: "enabled", Off: "disabled", Auto: auto}
}

// thinkingModes returns the mode vocabulary for a vendor, including auto only
// when the vendor documents it.
func thinkingModes(withAuto bool) []string {
	if withAuto {
		return []string{spi.ThinkingOn, spi.ThinkingOff, spi.ThinkingAuto}
	}
	return []string{spi.ThinkingOn, spi.ThinkingOff}
}
