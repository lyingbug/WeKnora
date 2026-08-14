package chat

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/models/llm/encoding"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/models/provider"
)

// Model configuration keys read from parameters.extra_config.
const (
	// ExtraConfigThinkingControl was the manual override selecting how the
	// thinking toggle reached a provider. The plugin descriptors now derive
	// that from the vendor's documentation, so the key is honored only to keep
	// existing configurations working: a stored value still forces the wire
	// shape it names.
	//
	// Deprecated: leave it unset and let the resolved plugin decide.
	ExtraConfigThinkingControl = "thinking_control"

	// ExtraConfigProtocol pins the wire protocol for vendors that offer more
	// than one, such as OpenAI's Chat Completions and Responses APIs. Empty
	// selects the vendor's default.
	ExtraConfigProtocol = "protocol"
)

// ThinkingControlNone is the reported control for a model whose plugin sends
// no thinking field at all.
const ThinkingControlNone = "none"

// legacyThinkingEncoders maps the historical thinking_control values onto the
// encoders they selected, so a stored override keeps doing what it did.
var legacyThinkingEncoders = map[string]spi.Encoder{
	"enable_thinking": encoding.EnableThinkingBool{Key: "enable_thinking"},
	"thinking_type": encoding.ThinkingObject{
		Key: "thinking", On: "enabled", Off: "disabled",
	},
	"chat_template_kwargs": encoding.ChatTemplateKwargs{
		Key: "chat_template_kwargs", Arg: "enable_thinking",
	},
}

// applyLegacyThinkingOverride honors a stored extra_config.thinking_control by
// rewriting the resolved descriptor's thinking encoder.
//
// The override predates the plugin catalog and exists because the catalog used
// to be a guess; keeping it working means an operator who pinned a wire format
// against a misdetected provider does not silently start sending a different
// one after this refactor. Descriptors are values, so the override produces a
// copy and never mutates the registry.
func applyLegacyThinkingOverride(desc spi.Descriptor, extraConfig map[string]string) spi.Descriptor {
	control := strings.ToLower(strings.TrimSpace(extraConfig[ExtraConfigThinkingControl]))
	if control == "" {
		return desc
	}

	params := make([]spi.Param, 0, len(desc.Params)+1)
	var mode *spi.Param
	for _, p := range desc.Params {
		if p.ID == spi.ParamThinkingMode {
			clone := p
			mode = &clone
			continue
		}
		params = append(params, p)
	}

	if control == ThinkingControlNone {
		// "none" means send no thinking field, so the parameter goes away
		// entirely rather than becoming a knob that does nothing.
		desc.Params = params
		return desc
	}

	encoder, ok := legacyThinkingEncoders[control]
	if !ok {
		return desc
	}
	if mode == nil {
		// The override also enables the toggle on a model whose plugin does
		// not declare one, which is the case it was originally added for.
		mode = &spi.Param{}
		*mode = encoding.ThinkingMode(encoder, spi.ThinkingOn, spi.ThinkingOff)
	} else {
		mode.Encode = encoder
		// A vendor default belongs to the vendor's own field; forcing another
		// one should not also force a value the caller never asked for.
		mode.Default = nil
	}
	desc.Params = append([]spi.Param{*mode}, params...)
	return desc
}

// EffectiveThinkingControl reports the wire field that will carry the thinking
// toggle for a model, or "none" when the model has no toggle.
//
// It resolves through the same registry and descriptor the request path uses,
// so the answer is derived rather than predicted. That matters because this
// value is what the model editor and the debug drawer show, and the previous
// arrangement — a backend table plus a frontend copy of the same heuristics —
// could disagree with what was actually sent.
func EffectiveThinkingControl(config *ChatConfig) string {
	desc, ok := resolveDescriptor(config)
	if !ok {
		return ThinkingControlNone
	}
	param, ok := desc.Param(spi.ParamThinkingMode)
	if !ok || param.Encode == nil || param.EffectiveSupport() == spi.SupportForbidden {
		return ThinkingControlNone
	}
	return param.Encode.ID()
}

// SupportsThinking reports whether a model exposes a reasoning toggle. The UI
// asks this before offering the control, instead of re-deriving it from a
// provider name.
func SupportsThinking(config *ChatConfig) bool {
	return EffectiveThinkingControl(config) != ThinkingControlNone
}

// resolveDescriptor looks up the plugin backing a chat configuration, with any
// stored legacy override already applied. Every caller — the request path, the
// reporting helpers, the debug endpoint — goes through here, so none of them
// can disagree about what will be sent.
func resolveDescriptor(config *ChatConfig) (spi.Descriptor, bool) {
	if config == nil {
		return spi.Descriptor{}, false
	}
	name := provider.ProviderName(strings.TrimSpace(config.Provider))
	if name == "" {
		name = provider.DetectProvider(config.BaseURL)
	}
	desc, ok := spi.Resolve(spi.Query{
		Vendor:   string(name),
		Kind:     spi.KindChat,
		Model:    config.ModelName,
		Protocol: configuredProtocol(config),
	})
	if !ok {
		return spi.Descriptor{}, false
	}
	return applyLegacyThinkingOverride(desc, config.ExtraConfig), true
}
