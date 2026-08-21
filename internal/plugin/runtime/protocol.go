// Package runtime loads disk plugins onto WeKnora seams without a
// compile-time blank import. Language plugins speak protocol (JSON-RPC
// over stdio). JS runs in-process. HTTP is a fallback for an already
// running remote service — not the way to write a new plugin.
package runtime

import (
	"time"

	"github.com/Tencent/WeKnora/internal/plugin/protocol"
)

// SearchRequest is the JSON body posted to an HTTP search plugin.
type SearchRequest = protocol.SearchRequest

// SearchResponse is the JSON body returned by an HTTP search plugin.
type SearchResponse = protocol.SearchResponse

func clampTimeout(ms int) time.Duration {
	if ms <= 0 {
		ms = 10000
	}
	return time.Duration(ms) * time.Millisecond
}
