package memory

import (
	"sort"
	"strings"
	"unicode"

	"github.com/Tencent/WeKnora/internal/types"
)

// Situational recall is lexical rather than vector-based. One subject holds a
// few hundred one-line items, so scanning them costs less than an embedding
// round trip would, and it keeps the read path free of both a model call and a
// vector store dependency. If real usage shows lexical matching missing
// paraphrases, adding a vector index here is an isolated change: the ranking
// function is the only thing that would move.

// tokenize splits text the same way NormalizeMemoryKey does, so a query and a
// stored item are compared on the same alphabet. CJK is split per ideograph
// because it has no word separators; everything else splits on non-alphanumeric.
func tokenize(text string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			tokens = append(tokens, string(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// bigrams pairs adjacent CJK ideographs. A single Chinese character matches far
// too much on its own ("数" appears in 数据, 数量, 参数), so scoring counts
// two-character sequences as well and weights them higher.
func bigrams(tokens []string) []string {
	var pairs []string
	for i := 0; i+1 < len(tokens); i++ {
		a, b := tokens[i], tokens[i+1]
		if isSingleHan(a) && isSingleHan(b) {
			pairs = append(pairs, a+b)
		}
	}
	return pairs
}

func isSingleHan(token string) bool {
	runes := []rune(token)
	return len(runes) == 1 && unicode.Is(unicode.Han, runes[0])
}

type scoredItem struct {
	item  *types.MemoryItem
	score float64
}

// scoreItems ranks situational items against the current query. Scoring is
// deliberately simple: overlap of query tokens with the item, with bigram hits
// weighted higher and importance used only to break ties.
func scoreItems(query string, items []*types.MemoryItem) []scoredItem {
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 || len(items) == 0 {
		return nil
	}

	queryUnigrams := make(map[string]struct{}, len(queryTokens))
	for _, token := range queryTokens {
		// One-character latin tokens carry no signal and match everywhere.
		if len([]rune(token)) < 2 && !isSingleHan(token) {
			continue
		}
		queryUnigrams[token] = struct{}{}
	}
	queryBigrams := make(map[string]struct{})
	for _, pair := range bigrams(queryTokens) {
		queryBigrams[pair] = struct{}{}
	}
	if len(queryUnigrams) == 0 && len(queryBigrams) == 0 {
		return nil
	}

	scored := make([]scoredItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		// Score against the topic as well as the statement, indexed separately
		// so no bigram spans the boundary between them. The normalized key is
		// deliberately not used: it is a sorted character soup built for
		// collision detection, so its adjacency carries no meaning.
		itemUnigrams := make(map[string]struct{})
		itemBigrams := make(map[string]struct{})
		for _, text := range []string{item.Content, item.Topic} {
			tokens := tokenize(text)
			for _, token := range tokens {
				itemUnigrams[token] = struct{}{}
			}
			for _, pair := range bigrams(tokens) {
				itemBigrams[pair] = struct{}{}
			}
		}
		if len(itemUnigrams) == 0 {
			continue
		}

		var hits float64
		for token := range queryUnigrams {
			if _, ok := itemUnigrams[token]; ok {
				hits++
			}
		}
		for pair := range queryBigrams {
			if _, ok := itemBigrams[pair]; ok {
				hits += 2
			}
		}
		if hits == 0 {
			continue
		}
		// Normalizing by query length keeps a long query from favouring long
		// items purely because there was more to match against.
		denominator := float64(len(queryUnigrams) + 2*len(queryBigrams))
		if denominator == 0 {
			continue
		}
		scored = append(scored, scoredItem{
			item:  item,
			score: hits/denominator + 0.01*float64(item.Importance),
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].item.ValidFrom.After(scored[j].item.ValidFrom)
	})

	return scored
}

// minRecallScore is the relevance floor. Injecting a weakly related memory
// costs context and, worse, invites the model to use it: a stale note that
// merely shares a word with the question is more harmful than no note at all.
const minRecallScore = 0.15

// selectRecallItems takes the best matches within both the count and rune
// budgets.
func selectRecallItems(query string, items []*types.MemoryItem, maxItems, runeBudget int) []*types.MemoryItem {
	selected := make([]*types.MemoryItem, 0, maxItems)
	used := 0
	for _, entry := range scoreItems(query, items) {
		if len(selected) >= maxItems {
			break
		}
		if entry.score < minRecallScore {
			break
		}
		cost := len([]rune(entry.item.Content)) + 3
		if used+cost > runeBudget {
			continue
		}
		selected = append(selected, entry.item)
		used += cost
	}
	return selected
}
