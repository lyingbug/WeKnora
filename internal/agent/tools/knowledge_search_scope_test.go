package tools

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func searchResult(knowledgeID string, score float64) *searchResultWithMeta {
	return &searchResultWithMeta{
		SearchResult: &types.SearchResult{KnowledgeID: knowledgeID, Score: score},
	}
}

func TestRankedKnowledgeIDsCollapsesChunksOfOneDocument(t *testing.T) {
	got := rankedKnowledgeIDs([]*searchResultWithMeta{
		searchResult("doc-a", 0.9),
		searchResult("doc-a", 0.8),
		searchResult("doc-b", 0.7),
		searchResult("", 0.6),
		nil,
		searchResult("doc-a", 0.1),
	})

	if len(got) != 2 || got[0] != "doc-a" || got[1] != "doc-b" {
		t.Fatalf("expected [doc-a doc-b], got %v", got)
	}
}

// Deduplication upstream reads out of a map, so the candidate pool arrives in
// an arbitrary order and has to be re-sorted before it can act as a ranking.
func TestSortedByScoreRestoresRelevanceOrder(t *testing.T) {
	got := sortedByScore([]*searchResultWithMeta{
		searchResult("doc-c", 0.10),
		searchResult("doc-a", 0.90),
		searchResult("doc-b", 0.50),
	})

	want := []string{"doc-a", "doc-b", "doc-c"}
	for i, id := range want {
		if got[i].KnowledgeID != id {
			t.Fatalf("expected %v, got %s at position %d", want, got[i].KnowledgeID, i)
		}
	}
}

func TestSortedByScoreDoesNotMutateInput(t *testing.T) {
	input := []*searchResultWithMeta{
		searchResult("doc-c", 0.1),
		searchResult("doc-a", 0.9),
	}
	_ = sortedByScore(input)

	if input[0].KnowledgeID != "doc-c" {
		t.Fatalf("input slice was reordered: %s", input[0].KnowledgeID)
	}
}

func TestSortedByScoreIsDeterministicOnTies(t *testing.T) {
	first := sortedByScore([]*searchResultWithMeta{
		searchResult("doc-b", 0.5),
		searchResult("doc-a", 0.5),
	})
	if first[0].KnowledgeID != "doc-a" {
		t.Fatalf("expected ties broken by document ID, got %s", first[0].KnowledgeID)
	}
}

// The two recordings layer: reranked documents lead, and the wider candidate
// pool extends the ranking far enough to actually steer a scan over a large
// knowledge base.
func TestScopeLayersRerankedResultsAboveCandidatePool(t *testing.T) {
	final := []*searchResultWithMeta{
		searchResult("doc-quoted", 0.99),
	}
	pool := []*searchResultWithMeta{
		searchResult("doc-weak", 0.20),
		searchResult("doc-quoted", 0.95),
		searchResult("doc-mid", 0.60),
	}

	scope := NewRelevanceScope()
	scope.RecordRankedDocuments([]string{"the question"}, rankedKnowledgeIDs(final))
	scope.RecordRankedDocuments(nil, rankedKnowledgeIDs(sortedByScore(pool)))

	got := scope.RankedDocuments(10)
	want := []string{"doc-quoted", "doc-mid", "doc-weak"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
