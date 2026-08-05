package tools

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestRelevanceScopePreservesRetrievalOrder(t *testing.T) {
	scope := NewRelevanceScope()
	scope.RecordRankedDocuments([]string{"how does billing work"}, []string{"doc-a", "doc-b", "doc-c"})

	got := scope.RankedDocuments(10)
	want := []string{"doc-a", "doc-b", "doc-c"}
	if len(got) != len(want) {
		t.Fatalf("expected %d documents, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ranking not preserved: got %v, want %v", got, want)
		}
	}
}

// A document that any one search ranked highly must keep that standing: the
// whole point of the scope is to remember where the evidence looked promising.
// Documents that tie on their best rank stay in the order they were first
// seen, so the ranking is reproducible across runs.
func TestRelevanceScopeKeepsBestRankAcrossSearches(t *testing.T) {
	scope := NewRelevanceScope()
	scope.RecordRankedDocuments([]string{"first"}, []string{"doc-a", "doc-b", "doc-c"})
	scope.RecordRankedDocuments([]string{"second"}, []string{"doc-c", "doc-d"})

	got := scope.RankedDocuments(10)
	want := []string{"doc-a", "doc-c", "doc-b", "doc-d"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestRelevanceScopeDeduplicatesWithinOneSearch(t *testing.T) {
	scope := NewRelevanceScope()
	scope.RecordRankedDocuments(nil, []string{"doc-a", "doc-a", "doc-b", "", "doc-a"})

	got := scope.RankedDocuments(10)
	if len(got) != 2 || got[0] != "doc-a" || got[1] != "doc-b" {
		t.Fatalf("expected [doc-a doc-b], got %v", got)
	}
}

func TestRelevanceScopeBoundsRetainedDocuments(t *testing.T) {
	scope := NewRelevanceScope()
	ids := make([]string, maxScopeDocuments+50)
	for i := range ids {
		ids[i] = fmt.Sprintf("doc-%d", i)
	}
	scope.RecordRankedDocuments(nil, ids)

	if got := len(scope.RankedDocuments(maxScopeDocuments * 2)); got != maxScopeDocuments {
		t.Fatalf("expected the scope to cap at %d documents, got %d", maxScopeDocuments, got)
	}
}

// Semantic queries describe the need in retrieval terms, so they outrank the
// raw question once a search has run — but the question has to serve until
// then, otherwise the first lexical scan of a turn has nothing to rank against.
func TestRelevanceScopeQueryPrefersSearchQueriesOverUserQuestion(t *testing.T) {
	scope := NewRelevanceScope()
	scope.SetUserQuery("what changed in the refund policy")

	if got := scope.Query(); got != "what changed in the refund policy" {
		t.Fatalf("expected the user question before any search, got %q", got)
	}

	scope.RecordRankedDocuments([]string{"refund policy", "chargeback rules"}, []string{"doc-a"})
	got := scope.Query()
	if !strings.Contains(got, "refund policy") || !strings.Contains(got, "chargeback rules") {
		t.Fatalf("expected both search queries in %q", got)
	}
}

func TestRelevanceScopeNilIsUsable(t *testing.T) {
	var scope *RelevanceScope
	scope.SetUserQuery("x")
	scope.RecordRankedDocuments(nil, []string{"doc-a"})
	if got := scope.RankedDocuments(5); got != nil {
		t.Fatalf("expected nil ranking from a nil scope, got %v", got)
	}
	if got := scope.Query(); got != "" {
		t.Fatalf("expected empty query from a nil scope, got %q", got)
	}
}

// Tool calls run concurrently, so the scope has to tolerate simultaneous
// writers and readers.
func TestRelevanceScopeConcurrentAccess(t *testing.T) {
	scope := NewRelevanceScope()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scope.RecordRankedDocuments(
				[]string{fmt.Sprintf("q-%d", i)},
				[]string{fmt.Sprintf("doc-%d", i), "doc-shared"},
			)
			_ = scope.RankedDocuments(32)
			_ = scope.Query()
		}(i)
	}
	wg.Wait()

	if len(scope.RankedDocuments(64)) != 17 {
		t.Fatalf("expected 17 distinct documents, got %d", len(scope.RankedDocuments(64)))
	}
}
