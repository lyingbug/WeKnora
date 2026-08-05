package tools

import (
	"context"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Tencent/WeKnora/internal/logger"
)

const (
	// grepRerankPoolSize bounds how many lexical matches are scored by the
	// rerank model. Matches arrive in relevance-guided traversal order, so the
	// pool is the leading slice of that order rather than an arbitrary sample.
	// It trades rerank latency against the chance of pulling a strong excerpt
	// out of a document whose overall ranking is weak.
	grepRerankPoolSize = 120

	// grepRerankPassageRunes caps the passage length handed to the reranker.
	// The judgement being made is "does this excerpt bear on the question",
	// which the opening of a chunk answers as well as the whole of it.
	grepRerankPassageRunes = 1000

	// grepRerankPoolEnv overrides grepRerankPoolSize; 0 disables match-level
	// reranking entirely and restores purely lexical ordering.
	grepRerankPoolEnv = "WEKNORA_GREP_RERANK_POOL"

	// semanticWeight splits the final ordering between the rerank model's
	// judgement and the lexical evidence. Lexical signal is deliberately kept
	// at a meaningful weight: grep is also how the agent looks up error codes
	// and identifiers, where hit density is the point and semantics say little.
	semanticWeight = 0.6
)

// regexMetaTokens are the regex constructs that carry no lexical meaning and
// must be stripped before a pattern can be read as a bag of search keywords.
var regexMetaTokens = regexp.MustCompile(`\\[bBdDsSwWAzZ]|\[[^\]]*\]|\\.|[.*+?^$(){}|\[\]]`)

// effectiveGrepRerankPool resolves the rerank pool size, honouring the env
// override. A non-positive value disables match-level reranking.
func effectiveGrepRerankPool() int {
	raw := strings.TrimSpace(os.Getenv(grepRerankPoolEnv))
	if raw == "" {
		return grepRerankPoolSize
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return grepRerankPoolSize
	}
	return parsed
}

// regexFocusKeywords reduces a POSIX regex to the literal terms it is looking
// for, so the pattern's intent can be expressed to an embedding/rerank model
// that does not understand regex syntax. Alternation branches become separate
// keywords; metacharacters, escapes and character classes are dropped.
func regexFocusKeywords(pattern string) []string {
	if strings.TrimSpace(pattern) == "" {
		return nil
	}

	var keywords []string
	seen := make(map[string]bool)
	for _, branch := range strings.Split(pattern, "|") {
		cleaned := regexMetaTokens.ReplaceAllString(branch, " ")
		for _, field := range strings.Fields(cleaned) {
			field = strings.Trim(field, "-_/,;:'\"")
			if !isUsefulKeyword(field) || seen[strings.ToLower(field)] {
				continue
			}
			seen[strings.ToLower(field)] = true
			keywords = append(keywords, field)
		}
	}
	return keywords
}

// isUsefulKeyword filters out fragments too short to disambiguate anything.
// CJK is judged by rune count because a single ideograph carries far more
// signal than a single Latin letter.
func isUsefulKeyword(token string) bool {
	runes := []rune(token)
	if len(runes) == 0 {
		return false
	}
	hasLetterOrDigit := false
	hasCJK := false
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasLetterOrDigit = true
		}
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			hasCJK = true
		}
	}
	if !hasLetterOrDigit {
		return false
	}
	if hasCJK {
		return true
	}
	return len(runes) >= 2
}

// buildMatchRerankQuery composes the query used to rerank grep matches. The
// scope query states the goal the whole turn is working towards, while the
// regex keywords state what this particular scan was checking — an excerpt is
// only worth the model's attention when it speaks to both.
//
// Returns "" when neither signal is available, in which case reranking is
// skipped rather than run against an empty query.
func buildMatchRerankQuery(scopeQuery string, keywords []string) string {
	scopeQuery = strings.TrimSpace(scopeQuery)
	if scopeQuery == "" && len(keywords) == 0 {
		return ""
	}

	var b strings.Builder
	if scopeQuery != "" {
		b.WriteString("Query: ")
		b.WriteString(scopeQuery)
	}
	if len(keywords) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Search focus: ")
		b.WriteString(strings.Join(keywords, " "))
	}
	return b.String()
}

// applyMatchRelevance reranks the leading slice of lexical matches with the
// rerank model and folds the result into MatchScore.
//
// Lexical hit counting cannot tell an excerpt that answers the question from
// one that merely repeats a word, so when a regex matches far more chunks than
// the model will ever see, the choice of which ones to surface is otherwise
// made on keyword density alone. Reranking makes that choice on relevance,
// which also lets a decisive excerpt inside an overall weak document compete
// for the budget.
//
// Reranking is skipped when it cannot change the outcome (every match already
// fits in the budget) or when no rerank model is configured.
func (t *GrepChunksTool) applyMatchRelevance(
	ctx context.Context,
	results []chunkWithTitle,
	pattern string,
	limit int,
) ([]chunkWithTitle, bool) {
	if t.reranker == nil || len(results) == 0 {
		return results, false
	}

	poolSize := effectiveGrepRerankPool()
	if poolSize <= 0 {
		logger.Debugf(ctx, "[Tool][GrepChunks] Match-level rerank disabled via %s", grepRerankPoolEnv)
		return results, false
	}
	if limit > 0 && len(results) <= limit {
		// Everything already fits in the observation budget; reordering it
		// costs a model call and changes nothing the agent gets to see.
		return results, false
	}

	keywords := regexFocusKeywords(pattern)
	rerankQuery := buildMatchRerankQuery(t.scope.Query(), keywords)
	if rerankQuery == "" {
		logger.Debugf(ctx, "[Tool][GrepChunks] No scope query or usable keywords; skipping match rerank")
		return results, false
	}

	pool := results
	if len(pool) > poolSize {
		pool = pool[:poolSize]
	}

	passages := make([]string, len(pool))
	for i, r := range pool {
		passages[i] = grepRerankPassage(r)
	}

	ranked, err := t.reranker.Rerank(ctx, rerankQuery, passages)
	if err != nil {
		logger.Warnf(ctx, "[Tool][GrepChunks] Match rerank failed, keeping lexical order: %v", err)
		return results, false
	}

	semantic := make(map[int]float64, len(ranked))
	for _, r := range ranked {
		if r.Index >= 0 && r.Index < len(pool) {
			semantic[r.Index] = clamp01(r.RelevanceScore)
		}
	}
	if len(semantic) == 0 {
		logger.Warnf(ctx, "[Tool][GrepChunks] Match rerank returned no usable scores, keeping lexical order")
		return results, false
	}

	out := make([]chunkWithTitle, len(pool))
	for i, r := range pool {
		r.MatchScore = semanticWeight*semantic[i] + (1-semanticWeight)*r.MatchScore
		out[i] = r
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].MatchScore > out[j].MatchScore
	})

	logger.Infof(ctx, "[Tool][GrepChunks] Match-level rerank: %d/%d candidates scored, query=%q",
		len(semantic), len(results), truncateRunes(rerankQuery, 120))
	return out, true
}

// grepRerankPassage renders a chunk for the reranker. The document title is
// prepended because a chunk in isolation frequently omits the subject its
// document is about.
func grepRerankPassage(r chunkWithTitle) string {
	body := r.Content
	if title := strings.TrimSpace(r.KnowledgeTitle); title != "" {
		body = title + "\n" + body
	}
	return truncateRunes(body, grepRerankPassageRunes)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
