package chatpipeline

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// Anchors on an ordinary knowledge base store the knowledge id, and anchors on a
// wiki store the page slug. Both have to be able to match a retrieval result, or
// "prefer what this person has engaged with" prefers nothing — which is what
// happened: every candidate compared was an id, so wiki content never matched,
// and the hints themselves were loaded only for anchors tied to a memory page,
// which excluded every anchor an ordinary knowledge base can produce.

func TestAnchorMatchesAnOrdinaryKnowledgeBaseResult(t *testing.T) {
	weights := map[string]float64{"kn-42": 1}
	result := &types.SearchResult{ID: "chunk-1", KnowledgeID: "kn-42"}

	weight, ok := anchorWeightForResult(weights, result)
	if !ok {
		t.Fatal("a document the user engaged with did not match its own anchor")
	}
	if weight != 1 {
		t.Fatalf("weight = %v, want 1", weight)
	}
}

func TestAnchorMatchesAWikiResultByItsSlug(t *testing.T) {
	weights := map[string]float64{"concept/rerank": 1}
	result := &types.SearchResult{
		ID:          "chunk-2",
		KnowledgeID: "kn-99",
		Metadata:    map[string]string{"wiki_slug": "concept/rerank"},
	}

	if _, ok := anchorWeightForResult(weights, result); !ok {
		t.Fatal("a wiki page the user engaged with did not match its own anchor")
	}
}

func TestAnchorWeightPrefersTheStrongestRelation(t *testing.T) {
	// The same result reachable by two identifiers, anchored with different
	// relations: the stronger one decides how hard it is promoted.
	weights := map[string]float64{"kn-42": 1, "concept/rerank": 2.5}
	result := &types.SearchResult{
		KnowledgeID: "kn-42",
		Metadata:    map[string]string{"wiki_slug": "concept/rerank"},
	}

	weight, ok := anchorWeightForResult(weights, result)
	if !ok {
		t.Fatal("no match")
	}
	if weight != 2.5 {
		t.Fatalf("weight = %v, want the stronger relation's 2.5", weight)
	}
}

func TestAnchorDoesNotMatchUnrelatedContent(t *testing.T) {
	weights := map[string]float64{"kn-42": 1}
	result := &types.SearchResult{ID: "chunk-3", KnowledgeID: "kn-7"}

	if _, ok := anchorWeightForResult(weights, result); ok {
		t.Fatal("content the user never engaged with was boosted")
	}
}
