package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/rerank"
)

// stubReranker scores passages by a caller-supplied function so tests can
// express "this excerpt is the relevant one" without a model.
type stubReranker struct {
	score     func(passage string) float64
	err       error
	lastQuery string
	callCount int
}

func (s *stubReranker) Rerank(_ context.Context, query string, documents []string) ([]rerank.RankResult, error) {
	s.callCount++
	s.lastQuery = query
	if s.err != nil {
		return nil, s.err
	}
	out := make([]rerank.RankResult, len(documents))
	for i, d := range documents {
		out[i] = rerank.RankResult{Index: i, RelevanceScore: s.score(d)}
	}
	return out, nil
}

func (s *stubReranker) GetModelName() string { return "stub" }
func (s *stubReranker) GetModelID() string   { return "stub" }

func TestRegexFocusKeywordsStripsMetacharacters(t *testing.T) {
	cases := []struct {
		pattern string
		want    []string
	}{
		{`stardust|skyvault|psionic`, []string{"stardust", "skyvault", "psionic"}},
		{`psionic.*engine`, []string{"psionic", "engine"}},
		{`\bRAG\b`, []string{"RAG"}},
		{`^chapter\s+\d+`, []string{"chapter"}},
		{`ERR\-\d{4}`, []string{"ERR"}},
		{`退款|发票`, []string{"退款", "发票"}},
		{`[a-z]+`, nil},
	}

	for _, tc := range cases {
		got := regexFocusKeywords(tc.pattern)
		if len(got) != len(tc.want) {
			t.Fatalf("pattern %q: expected %v, got %v", tc.pattern, tc.want, got)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("pattern %q: expected %v, got %v", tc.pattern, tc.want, got)
			}
		}
	}
}

// Single Latin letters are noise, but a single ideograph is a real term.
func TestRegexFocusKeywordsKeepsSingleCJKCharacters(t *testing.T) {
	if got := regexFocusKeywords(`票`); len(got) != 1 || got[0] != "票" {
		t.Fatalf("expected a single CJK keyword, got %v", got)
	}
	if got := regexFocusKeywords(`a|b`); got != nil {
		t.Fatalf("expected single Latin letters to be dropped, got %v", got)
	}
}

func TestBuildMatchRerankQueryCombinesGoalAndScanIntent(t *testing.T) {
	got := buildMatchRerankQuery("why did the refund fail", []string{"refund", "declined"})
	if !strings.Contains(got, "why did the refund fail") {
		t.Fatalf("expected the overall goal in %q", got)
	}
	if !strings.Contains(got, "refund declined") {
		t.Fatalf("expected the scan keywords in %q", got)
	}
}

func TestBuildMatchRerankQueryEmptyWhenNoSignal(t *testing.T) {
	if got := buildMatchRerankQuery("  ", nil); got != "" {
		t.Fatalf("expected an empty query, got %q", got)
	}
}

func newGrepToolWithReranker(r rerank.Reranker, scopeQuery string) *GrepChunksTool {
	scope := NewRelevanceScope()
	scope.SetUserQuery(scopeQuery)
	return (&GrepChunksTool{
		BaseTool:   grepChunksTool,
		scope:      NewRelevanceScope(),
		seenChunks: map[string]bool{},
	}).WithRelevanceScope(scope).WithReranker(r)
}

func makeCandidates(n int) []chunkWithTitle {
	out := make([]chunkWithTitle, n)
	for i := range out {
		out[i].ID = fmt.Sprintf("chunk-%d", i)
		out[i].Content = fmt.Sprintf("filler mention %d", i)
		out[i].MatchScore = 0.5
		out[i].MatchedPatterns = 1
	}
	return out
}

// The failure this guards against: a chunk that repeats the keyword the most
// wins the limited budget, while the one chunk that actually answers the
// question is dropped because it mentions the term only once.
func TestApplyMatchRelevancePromotesTheAnsweringExcerpt(t *testing.T) {
	candidates := makeCandidates(40)
	candidates[37].Content = "refunds are declined when the card issuer rejects the chargeback"

	reranker := &stubReranker{score: func(passage string) float64 {
		if strings.Contains(passage, "card issuer rejects") {
			return 0.95
		}
		return 0.05
	}}
	tool := newGrepToolWithReranker(reranker, "why was my refund declined")

	got, reranked := tool.applyMatchRelevance(context.Background(), candidates, "refund|declined", 30)
	if !reranked {
		t.Fatal("expected match-level reranking to run")
	}
	if got[0].ID != "chunk-37" {
		t.Fatalf("expected the answering excerpt first, got %q", got[0].ID)
	}
	if !strings.Contains(reranker.lastQuery, "why was my refund declined") {
		t.Fatalf("rerank query lost the overall goal: %q", reranker.lastQuery)
	}
	if !strings.Contains(reranker.lastQuery, "refund declined") {
		t.Fatalf("rerank query lost the scan keywords: %q", reranker.lastQuery)
	}
}

// Lexical evidence keeps real weight, so identifier lookups — where hit
// density is the whole signal — are not overturned by a weak semantic score.
func TestApplyMatchRelevanceKeepsLexicalWeight(t *testing.T) {
	candidates := makeCandidates(40)
	candidates[5].MatchScore = 1.0
	candidates[6].MatchScore = 0.0

	reranker := &stubReranker{score: func(string) float64 { return 0.5 }}
	tool := newGrepToolWithReranker(reranker, "find error code")

	got, _ := tool.applyMatchRelevance(context.Background(), candidates, "E1042", 30)
	var strong, weak float64
	for _, c := range got {
		switch c.ID {
		case "chunk-5":
			strong = c.MatchScore
		case "chunk-6":
			weak = c.MatchScore
		}
	}
	if strong <= weak {
		t.Fatalf("lexical evidence must still separate results: strong=%.3f weak=%.3f", strong, weak)
	}
}

// Reranking cannot change what the agent sees when every match already fits,
// so paying for a model call there is pure latency.
func TestApplyMatchRelevanceSkippedWhenEverythingFits(t *testing.T) {
	reranker := &stubReranker{score: func(string) float64 { return 1 }}
	tool := newGrepToolWithReranker(reranker, "anything")

	_, reranked := tool.applyMatchRelevance(context.Background(), makeCandidates(12), "term", 30)
	if reranked || reranker.callCount != 0 {
		t.Fatalf("expected no rerank call, got reranked=%v calls=%d", reranked, reranker.callCount)
	}
}

func TestApplyMatchRelevanceNoRerankerIsNoOp(t *testing.T) {
	tool := newGrepToolWithReranker(nil, "anything")
	candidates := makeCandidates(40)

	got, reranked := tool.applyMatchRelevance(context.Background(), candidates, "term", 30)
	if reranked || len(got) != len(candidates) {
		t.Fatalf("expected the candidates untouched, got reranked=%v len=%d", reranked, len(got))
	}
}

// A rerank outage must degrade to lexical ordering, never drop the matches.
func TestApplyMatchRelevanceFallsBackWhenRerankFails(t *testing.T) {
	reranker := &stubReranker{
		score: func(string) float64 { return 1 },
		err:   errors.New("rerank endpoint unavailable"),
	}
	tool := newGrepToolWithReranker(reranker, "anything")
	candidates := makeCandidates(40)

	got, reranked := tool.applyMatchRelevance(context.Background(), candidates, "term", 30)
	if reranked {
		t.Fatal("a failed rerank must not be reported as applied")
	}
	if len(got) != len(candidates) {
		t.Fatalf("expected all %d candidates preserved, got %d", len(candidates), len(got))
	}
}

func TestApplyMatchRelevanceSkippedWithoutAnyQuerySignal(t *testing.T) {
	reranker := &stubReranker{score: func(string) float64 { return 1 }}
	tool := (&GrepChunksTool{
		BaseTool:   grepChunksTool,
		scope:      NewRelevanceScope(),
		seenChunks: map[string]bool{},
	}).WithReranker(reranker)

	// No user question, no prior search, and a pattern with no literal terms.
	_, reranked := tool.applyMatchRelevance(context.Background(), makeCandidates(40), `[a-z]+`, 30)
	if reranked || reranker.callCount != 0 {
		t.Fatalf("expected the rerank to be skipped, got reranked=%v calls=%d", reranked, reranker.callCount)
	}
}

func TestApplyMatchRelevanceBoundsTheRerankPool(t *testing.T) {
	var poolSize int
	reranker := &stubReranker{score: func(string) float64 { return 0.5 }}
	tool := newGrepToolWithReranker(reranker, "question")

	got, _ := tool.applyMatchRelevance(context.Background(), makeCandidates(maxFetchLimit), "term", 30)
	poolSize = len(got)
	if poolSize != grepRerankPoolSize {
		t.Fatalf("expected the pool bounded to %d, got %d", grepRerankPoolSize, poolSize)
	}
}
