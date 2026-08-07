package memory

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/Tencent/WeKnora/internal/types"
)

// Relevance scoring for recall.
//
// A memory space holds tens to a few hundred pages, not millions of documents,
// which changes the right answer for retrieval. Scoring that many short strings
// in Go takes microseconds, needs no embedding model, no vector store and no
// hidden knowledge base per user, and behaves identically on PostgreSQL and
// SQLite. An embedding-backed path would buy better paraphrase matching at the
// cost of an infrastructure dependency on the one feature that should work the
// moment a user switches it on.
//
// The scorer is deliberately conservative: recall injects into every prompt, so
// a wrong memory is more expensive than a missing one.

// scoreWeights shape the ranking. Relevance dominates, but a memory the user
// keeps returning to, or one they pinned, deserves to win a close call.
const (
	weightLexical  = 1.0
	weightStrength = 0.35
	weightRecency  = 0.25
	weightPinned   = 0.4
	// minLexicalScore is the floor below which a page is considered unrelated.
	// Set high enough that a single incidental token overlap does not drag an
	// unrelated memory into the prompt.
	minLexicalScore = 0.12
	// recencyHalfLifeDays shapes the recency bonus only; real decay lives in
	// the lifecycle sweep.
	recencyHalfLifeDays = 60.0
)

// tokenize splits text into comparable units.
//
// CJK has no spaces, so a whitespace split would reduce a Chinese memory to one
// giant token and never match anything. Character bigrams are the standard
// answer and are what the SQLite retriever already does for its FTS index, so
// the two behave consistently.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var (
		tokens []string
		latin  strings.Builder
		cjk    []rune
	)

	flushLatin := func() {
		if latin.Len() > 0 {
			tokens = append(tokens, latin.String())
			latin.Reset()
		}
	}
	flushCJK := func() {
		switch len(cjk) {
		case 0:
		case 1:
			tokens = append(tokens, string(cjk))
		default:
			for i := 0; i < len(cjk)-1; i++ {
				tokens = append(tokens, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}

	for _, r := range text {
		switch {
		case isCJK(r):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin.WriteRune(r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

// pageIndex is the precomputed token set for one memory.
type pageIndex struct {
	page   *types.MemoryPage
	tokens map[string]int
	length int
}

func indexPage(page *types.MemoryPage) pageIndex {
	// Only the head of the body is indexed. A memory's meaning lives in its
	// title and summary; indexing a long pasted note in full would let its bulk
	// outvote a short, precise memory.
	body := page.Content
	if runes := []rune(body); len(runes) > 600 {
		body = string(runes[:600])
	}
	text := strings.Join([]string{
		page.Title, page.Summary, strings.Join(page.Aliases, " "), body,
	}, " ")

	tokens := tokenize(text)
	counts := make(map[string]int, len(tokens))
	for _, t := range tokens {
		counts[t]++
	}
	return pageIndex{page: page, tokens: counts, length: len(tokens)}
}

// scoreMemories ranks pages against a query using tf-idf cosine-ish overlap.
func scoreMemories(query string, pages []*types.MemoryPage, now nowFunc) []types.MemoryRecallItem {
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 || len(pages) == 0 {
		return nil
	}

	indexes := make([]pageIndex, 0, len(pages))
	docFreq := map[string]int{}
	for _, page := range pages {
		idx := indexPage(page)
		indexes = append(indexes, idx)
		for token := range idx.tokens {
			docFreq[token]++
		}
	}

	total := float64(len(indexes))
	queryWeights := map[string]float64{}
	for _, token := range queryTokens {
		if _, done := queryWeights[token]; done {
			continue
		}
		df := float64(docFreq[token])
		if df == 0 {
			continue
		}
		// Smoothed inverse document frequency: a token every memory contains
		// (the user's own name, say) carries almost no signal.
		queryWeights[token] = math.Log(1 + total/df)
	}
	if len(queryWeights) == 0 {
		return nil
	}

	var norm float64
	for _, w := range queryWeights {
		norm += w * w
	}
	norm = math.Sqrt(norm)

	items := make([]types.MemoryRecallItem, 0, len(indexes))
	for _, idx := range indexes {
		var dot float64
		for token, weight := range queryWeights {
			if tf, ok := idx.tokens[token]; ok {
				// Sub-linear term frequency: the fifth occurrence of a word
				// says much less than the first.
				dot += weight * (1 + math.Log(float64(tf)))
			}
		}
		if dot == 0 {
			continue
		}
		lexical := dot / (norm * math.Sqrt(float64(idx.length)+1))
		if lexical < minLexicalScore {
			continue
		}

		page := idx.page
		score := weightLexical*lexical +
			weightStrength*page.Strength +
			weightRecency*recencyBonus(page, now)
		if page.Pinned {
			score += weightPinned
		}

		items = append(items, types.MemoryRecallItem{
			Slug:       page.Slug,
			Title:      page.Title,
			Type:       page.PageType,
			Text:       page.InjectionText(),
			Confidence: page.Confidence,
			UpdatedAt:  page.UpdatedAt,
			Score:      score,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return items[i].Slug < items[j].Slug
	})
	return items
}

type nowFunc func() (secondsSinceEpoch float64)

func recencyBonus(page *types.MemoryPage, now nowFunc) float64 {
	if page.LastSeenAt == nil {
		return 0
	}
	ageDays := (now() - float64(page.LastSeenAt.Unix())) / 86400
	if ageDays <= 0 {
		return 1
	}
	return math.Pow(0.5, ageDays/recencyHalfLifeDays)
}
