// Package runtime loads disk plugins (JS / HTTP) onto WeKnora seams
// without a compile-time blank import.
package runtime

import (
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// SearchRequest is the JSON body posted to an HTTP search plugin.
type SearchRequest struct {
	Query       string                            `json:"query"`
	MaxResults  int                               `json:"max_results"`
	IncludeDate bool                              `json:"include_date"`
	Parameters  types.WebSearchProviderParameters `json:"parameters"`
}

// SearchResponse is the JSON body returned by an HTTP search plugin.
type SearchResponse struct {
	Results []*types.WebSearchResult `json:"results"`
	Error   string                   `json:"error,omitempty"`
}

func clampTimeout(ms int) time.Duration {
	if ms <= 0 {
		ms = 10000
	}
	return time.Duration(ms) * time.Millisecond
}
