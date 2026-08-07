package tools

import (
	"strings"
	"sync"
)

// maxScopeDocuments bounds how many ranked document IDs a scope retains.
// The ordering is only ever used as a traversal prior, so the tail of a very
// long ranking contributes nothing while making the ORDER BY expression that
// carries it into SQL unboundedly large.
const maxScopeDocuments = 256

// RelevanceScope carries a relevance ranking produced by semantic retrieval
// (knowledge_search) over to the lexical tools that run afterwards in the same
// agent execution, so they can traverse the most promising documents first
// instead of scanning in storage order.
//
// Without it, grep_chunks has no way to tell which of the thousands of chunks
// a broad regex matches is worth showing the model, and a hard candidate cap
// silently keeps whichever ones the database happened to return first.
//
// One instance is created per agent run and shared by the retrieval tools;
// all methods are safe for the concurrent tool calls the engine issues.
type RelevanceScope struct {
	mu sync.RWMutex

	// userQuery is the question that opened the turn. It anchors relevance
	// judgements made before any semantic search has run.
	userQuery string

	// scopeQueries are the semantic queries whose rankings built this scope.
	scopeQueries []string

	// docRank maps a knowledge (document) ID to its best observed rank,
	// 0 being the most relevant. A document ranked highly by any one search
	// keeps that rank even if a later, differently-phrased search buries it.
	docRank map[string]int

	// docOrder preserves insertion order so ties resolve deterministically.
	docOrder []string
}

// NewRelevanceScope creates an empty scope.
func NewRelevanceScope() *RelevanceScope {
	return &RelevanceScope{docRank: make(map[string]int)}
}

// SetUserQuery records the question that opened the turn.
func (s *RelevanceScope) SetUserQuery(query string) {
	if s == nil {
		return
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userQuery = query
}

// RecordRankedDocuments merges one relevance-ordered list of document IDs into
// the scope. knowledgeIDs must be ordered most-relevant-first; duplicates are
// collapsed onto their first occurrence.
func (s *RelevanceScope) RecordRankedDocuments(queries []string, knowledgeIDs []string) {
	if s == nil || len(knowledgeIDs) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" || containsString(s.scopeQueries, q) {
			continue
		}
		s.scopeQueries = append(s.scopeQueries, q)
	}

	rank := 0
	seenInCall := make(map[string]bool, len(knowledgeIDs))
	for _, id := range knowledgeIDs {
		if id == "" || seenInCall[id] {
			continue
		}
		seenInCall[id] = true

		if existing, ok := s.docRank[id]; ok {
			if rank < existing {
				s.docRank[id] = rank
			}
		} else if len(s.docOrder) < maxScopeDocuments {
			s.docRank[id] = rank
			s.docOrder = append(s.docOrder, id)
		}
		rank++
	}
}

// RankedDocuments returns up to limit document IDs ordered most-relevant-first.
func (s *RelevanceScope) RankedDocuments(limit int) []string {
	if s == nil || limit <= 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.docOrder) == 0 {
		return nil
	}

	ordered := make([]string, len(s.docOrder))
	copy(ordered, s.docOrder)
	// Insertion order already breaks ties, so a stable sort on rank alone
	// reproduces "best rank first, earliest observed first".
	stableSortByRank(ordered, s.docRank)

	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered
}

// Query returns the text describing what the agent is ultimately looking for.
// Semantic scope queries take precedence because they are the model's own,
// search-shaped restatement of the need; the raw user question is the fallback
// for tools that run before any semantic search.
func (s *RelevanceScope) Query() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.scopeQueries) > 0 {
		return strings.Join(s.scopeQueries, " ")
	}
	return s.userQuery
}

func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// stableSortByRank sorts ids ascending by their rank using insertion sort,
// which is stable and fast enough for the few hundred entries a scope holds.
func stableSortByRank(ids []string, rank map[string]int) {
	for i := 1; i < len(ids); i++ {
		cur := ids[i]
		curRank := rank[cur]
		j := i - 1
		for j >= 0 && rank[ids[j]] > curRank {
			ids[j+1] = ids[j]
			j--
		}
		ids[j+1] = cur
	}
}
