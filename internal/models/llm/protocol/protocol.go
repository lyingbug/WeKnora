// Package protocol implements the wire protocols a model plugin can speak.
//
// A protocol owns four things and nothing else: the canonical request body,
// the endpoint path, how a complete response decodes, and how a stream
// decodes. Everything vendor-specific — which extra fields ride along, which
// standard ones are forbidden, how reasoning is spelled — belongs to the
// vendor descriptor that selects the protocol.
//
// That split is what makes a new OpenAI-compatible vendor a declaration
// instead of an implementation.
package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	"github.com/Tencent/WeKnora/internal/types"
)

// Call is one chat request in neutral terms, before any vendor encoding.
type Call struct {
	// Model is the model identifier to send.
	Model string
	// Stream reports whether the response is streamed.
	Stream bool
	// Messages is the conversation.
	Messages []spi.Message
	// Options are the caller's generation settings.
	Options *spi.Options
	// Replay is the descriptor's reasoning-replay rule, which decides whether
	// prior-turn reasoning is rendered back onto the wire.
	Replay spi.ReasoningReplay
}

// Driver implements one wire protocol.
type Driver interface {
	// ID reports the protocol this driver implements.
	ID() spi.ProtocolID
	// BuildDraft renders the call into a canonical request body. Vendor
	// encoders run over the result, so a driver writes what its protocol
	// defines and leaves the extensions alone.
	BuildDraft(call Call) (*spi.Draft, error)
	// EndpointPath is the path appended to a base URL, including the leading
	// slash. A descriptor may override it.
	EndpointPath() string
	// ParseResponse decodes a complete response body.
	ParseResponse(body []byte) (*types.ChatResponse, error)
	// DecodeStream decodes a streaming body, sending neutral chunks to out and
	// closing nothing: the caller owns the channel's lifetime because it also
	// owns the surrounding request.
	DecodeStream(ctx context.Context, body io.Reader, out chan<- types.StreamResponse)
}

// registry holds the available drivers.
var (
	registryMu sync.RWMutex
	registry   = map[spi.ProtocolID]Driver{}
)

// Register adds a driver, returning the function that removes it.
func Register(d Driver) func() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[d.ID()] = d
	id := d.ID()
	return func() {
		registryMu.Lock()
		defer registryMu.Unlock()
		delete(registry, id)
	}
}

// Get reports the driver for a protocol.
func Get(id spi.ProtocolID) (Driver, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := registry[id]
	return d, ok
}

// MustGet reports the driver for a protocol, erroring when none is registered.
func MustGet(id spi.ProtocolID) (Driver, error) {
	d, ok := Get(id)
	if !ok {
		return nil, fmt.Errorf("no driver registered for protocol %q", id)
	}
	return d, nil
}

// IDs reports the registered protocols.
func IDs() []spi.ProtocolID {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]spi.ProtocolID, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	return out
}

// ShouldReplayReasoning reports whether a turn's prior reasoning must be
// rendered back onto the wire under a replay rule.
//
// The rule is a vendor fact rather than a preference: DeepSeek answers 400
// when a tool-calling turn's reasoning is dropped, while replaying it to a
// vendor that does not want it is merely ignored. When in doubt the safe
// direction is to send it, so only an explicit "never" suppresses it.
func ShouldReplayReasoning(replay spi.ReasoningReplay, msg spi.Message) bool {
	if msg.ReasoningContent == "" {
		return false
	}
	switch replay {
	case spi.ReplayAlways:
		return true
	case spi.ReplayWithTools:
		return len(msg.ToolCalls) > 0
	default:
		return false
	}
}
