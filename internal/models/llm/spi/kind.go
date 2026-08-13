// Package spi defines the model-plugin capability seam: the declarative
// descriptors a vendor plugin publishes, and the registry that resolves one
// for a configured model.
//
// The seam has three roles, and adding a capability means designing all three:
//
//   - Definition — the descriptors and interfaces in this package.
//   - Provider — a protocol driver (internal/models/llm/protocol) plus a vendor
//     descriptor (internal/models/llm/vendors) declaring how it differs.
//   - Consumer — the model factories in internal/models, plus the capability
//     API that renders the model editor form.
//
// One rule keeps the seam honest: a vendor plugin DECLARES facts, it never
// branches on a provider name. Anything that would otherwise need a central
// `switch providerName` belongs in a descriptor field instead. That is what
// lets a single declaration drive request building, validation, the frontend
// form, and diagnostics without those four surfaces drifting apart.
package spi

// ModelKind is the capability a model serves. A plugin declares the kinds it
// handles, so one descriptor can cover chat while another covers embeddings
// for the same vendor.
type ModelKind string

const (
	// KindChat is a conversational completion model.
	KindChat ModelKind = "chat"
	// KindEmbedding is a text or multimodal embedding model.
	KindEmbedding ModelKind = "embedding"
	// KindRerank is a query/document relevance scoring model.
	KindRerank ModelKind = "rerank"
	// KindVision is a vision-language model driven through a dedicated API.
	KindVision ModelKind = "vision"
	// KindASR is a speech-to-text model.
	KindASR ModelKind = "asr"
)

// ProtocolID names a wire protocol. A protocol owns request serialization,
// response parsing, and stream decoding; a vendor plugin picks one and
// declares only its deltas.
//
// The three standard protocols are the supported baseline. A vendor needing
// something none of them express implements its own driver and names it here.
type ProtocolID string

const (
	// ProtocolOpenAIChat is the OpenAI Chat Completions protocol
	// (POST <base>/chat/completions). The de-facto industry baseline: most
	// vendors are "OpenAI-compatible plus a few non-standard fields".
	ProtocolOpenAIChat ProtocolID = "openai-chat"
	// ProtocolOpenAIResponses is the OpenAI Responses protocol
	// (POST <base>/responses). Item-based input and output with first-class
	// reasoning items and a fully event-typed stream.
	ProtocolOpenAIResponses ProtocolID = "openai-responses"
	// ProtocolAnthropicMessages is the Anthropic Messages protocol
	// (POST <base>/v1/messages). Content-block based, with thinking blocks
	// that must be replayed verbatim across turns.
	ProtocolAnthropicMessages ProtocolID = "anthropic-messages"
	// ProtocolOllama is the local Ollama protocol, named here so local models
	// resolve through the same registry as remote ones.
	ProtocolOllama ProtocolID = "ollama"
)

// String reports the protocol id so it can be logged and surfaced in
// diagnostics without a conversion at every call site.
func (p ProtocolID) String() string { return string(p) }
