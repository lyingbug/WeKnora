package protocol

import "github.com/Tencent/WeKnora/internal/types"

// Method names on the plugin ABI. New seams add methods; they do not add
// a new wire format.
const (
	MethodWebSearchSearch = "websearch.search"
	MethodShutdown        = "shutdown"
)

// SearchRequest is the params object for websearch.search.
type SearchRequest struct {
	Query       string                            `json:"query"`
	MaxResults  int                               `json:"max_results"`
	IncludeDate bool                              `json:"include_date"`
	Parameters  types.WebSearchProviderParameters `json:"parameters"`
}

// SearchResponse is the result object for websearch.search.
// Error is for the HTTP fallback body; stdio plugins should use JSON-RPC errors.
type SearchResponse struct {
	Results []*types.WebSearchResult `json:"results"`
	Error   string                   `json:"error,omitempty"`
}
