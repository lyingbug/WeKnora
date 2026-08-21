package tools

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	sqlitedrv "gorm.io/driver/sqlite"
)

// newFetchOrderDB builds a knowledge base where the chunks that matter were
// created long ago and thousands of newer, incidentally-matching chunks sit on
// top of them — the shape that makes a recency-ordered candidate pool hide the
// answer.
func newFetchOrderDB(t *testing.T, docCount, chunksPerDoc int) (*gorm.DB, []string) {
	t.Helper()

	db, err := gorm.Open(sqlitedrv.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	stmts := []string{
		`CREATE TABLE knowledges (id TEXT PRIMARY KEY, title TEXT, deleted_at DATETIME)`,
		`CREATE TABLE chunks (
			id TEXT PRIMARY KEY,
			content TEXT,
			chunk_index INTEGER,
			knowledge_id TEXT,
			knowledge_base_id TEXT,
			chunk_type TEXT,
			metadata TEXT,
			created_at DATETIME,
			is_enabled BOOLEAN,
			deleted_at DATETIME
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	docIDs := make([]string, docCount)
	for d := 0; d < docCount; d++ {
		docID := fmt.Sprintf("doc-%03d", d)
		docIDs[d] = docID
		if err := db.Exec(
			`INSERT INTO knowledges (id, title) VALUES (?, ?)`,
			docID, fmt.Sprintf("Document %d", d),
		).Error; err != nil {
			t.Fatalf("insert knowledge: %v", err)
		}
		for c := 0; c < chunksPerDoc; c++ {
			// Later documents are newer, so a recency ordering visits the
			// highest-numbered documents first.
			created := base.Add(time.Duration(d*chunksPerDoc+c) * time.Minute)
			if err := db.Exec(
				`INSERT INTO chunks
				 (id, content, chunk_index, knowledge_id, knowledge_base_id, chunk_type, metadata, created_at, is_enabled)
				 VALUES (?, ?, ?, ?, 'kb-1', 'text', '', ?, 1)`,
				fmt.Sprintf("%s-c%03d", docID, c),
				fmt.Sprintf("mentions the term in %s chunk %d", docID, c),
				c, docID, created,
			).Error; err != nil {
				t.Fatalf("insert chunk: %v", err)
			}
		}
	}
	return db, docIDs
}

func fetchOrderBaseQuery(db *gorm.DB) func() *gorm.DB {
	return func() *gorm.DB {
		return db.Table("chunks").
			Select("chunks.id, chunks.content, chunks.chunk_index, chunks.knowledge_id, "+
				"chunks.knowledge_base_id, chunks.chunk_type, chunks.metadata, chunks.created_at, "+
				"knowledges.title as knowledge_title").
			Joins("JOIN knowledges ON chunks.knowledge_id = knowledges.id").
			Where("chunks.is_enabled = ?", true).
			Where("chunks.deleted_at IS NULL").
			Where("knowledges.deleted_at IS NULL")
	}
}

func documentsIn(results []chunkWithTitle) map[string]bool {
	out := make(map[string]bool, len(results))
	for _, r := range results {
		out[r.KnowledgeID] = true
	}
	return out
}

// Without a ranking the pool is recency-ordered, which is the behaviour this
// test pins so the next one can show what changes.
func TestFetchRelevanceOrderedFallsBackToRecency(t *testing.T) {
	db, _ := newFetchOrderDB(t, 60, 20)
	tool := &GrepChunksTool{db: db, scope: NewRelevanceScope()}

	got, err := tool.fetchRelevanceOrdered(context.Background(), fetchOrderBaseQuery(db))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != maxFetchLimit {
		t.Fatalf("expected a full pool of %d, got %d", maxFetchLimit, len(got))
	}
	if got[0].KnowledgeID != "doc-059" {
		t.Fatalf("expected the newest document first, got %q", got[0].KnowledgeID)
	}
}

// The behaviour that matters: an old but highly relevant document is scanned
// first instead of being pushed past the candidate cap by newer noise.
func TestFetchRelevanceOrderedVisitsRankedDocumentsFirst(t *testing.T) {
	db, _ := newFetchOrderDB(t, 60, 20)

	scope := NewRelevanceScope()
	scope.RecordRankedDocuments([]string{"the question"}, []string{"doc-002", "doc-000"})
	tool := &GrepChunksTool{db: db, scope: scope}

	got, err := tool.fetchRelevanceOrdered(context.Background(), fetchOrderBaseQuery(db))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// The two ranked documents contribute 20 chunks each, in ranked order.
	for i := 0; i < 20; i++ {
		if got[i].KnowledgeID != "doc-002" {
			t.Fatalf("position %d: expected the top-ranked document, got %q", i, got[i].KnowledgeID)
		}
	}
	for i := 20; i < 40; i++ {
		if got[i].KnowledgeID != "doc-000" {
			t.Fatalf("position %d: expected the second-ranked document, got %q", i, got[i].KnowledgeID)
		}
	}
}

// Ranking steers the scan; it must not become a filter, because finding the
// literal string that semantic retrieval missed is the reason grep exists.
func TestFetchRelevanceOrderedStillReachesUnrankedDocuments(t *testing.T) {
	db, _ := newFetchOrderDB(t, 60, 20)

	scope := NewRelevanceScope()
	scope.RecordRankedDocuments(nil, []string{"doc-002"})
	tool := &GrepChunksTool{db: db, scope: scope}

	got, err := tool.fetchRelevanceOrdered(context.Background(), fetchOrderBaseQuery(db))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != maxFetchLimit {
		t.Fatalf("expected the pool still filled to %d, got %d", maxFetchLimit, len(got))
	}

	docs := documentsIn(got)
	if !docs["doc-002"] {
		t.Fatal("the ranked document must be present")
	}
	if len(docs) < 5 {
		t.Fatalf("expected unranked documents to fill the remaining budget, saw %d documents", len(docs))
	}

	// No chunk may appear twice across the two passes.
	seen := make(map[string]bool, len(got))
	for _, r := range got {
		if seen[r.ID] {
			t.Fatalf("chunk %s returned twice", r.ID)
		}
		seen[r.ID] = true
	}
}

// A ranking wide enough to fill the pool on its own must not spend a second
// query on documents that cannot fit.
func TestFetchRelevanceOrderedSkipsSecondPassWhenPoolIsFull(t *testing.T) {
	db, docIDs := newFetchOrderDB(t, 60, 20)

	scope := NewRelevanceScope()
	scope.RecordRankedDocuments(nil, docIDs[:40]) // 800 chunks > maxFetchLimit
	tool := &GrepChunksTool{db: db, scope: scope}

	got, err := tool.fetchRelevanceOrdered(context.Background(), fetchOrderBaseQuery(db))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != maxFetchLimit {
		t.Fatalf("expected exactly %d results, got %d", maxFetchLimit, len(got))
	}
	for _, r := range got {
		if r.KnowledgeID >= "doc-040" {
			t.Fatalf("unranked document %q leaked into a full pool", r.KnowledgeID)
		}
	}
}

// Repeated identical scans must return identical pools, otherwise the agent
// sees different evidence for the same command.
func TestFetchRelevanceOrderedIsDeterministic(t *testing.T) {
	db, _ := newFetchOrderDB(t, 60, 20)
	scope := NewRelevanceScope()
	scope.RecordRankedDocuments(nil, []string{"doc-002", "doc-000"})
	tool := &GrepChunksTool{db: db, scope: scope}

	first, err := tool.fetchRelevanceOrdered(context.Background(), fetchOrderBaseQuery(db))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	for i := 0; i < 3; i++ {
		next, err := tool.fetchRelevanceOrdered(context.Background(), fetchOrderBaseQuery(db))
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if len(next) != len(first) {
			t.Fatalf("pool size changed between identical scans: %d vs %d", len(first), len(next))
		}
		for j := range first {
			if first[j].ID != next[j].ID {
				t.Fatalf("scan %d diverged at position %d: %q vs %q", i, j, first[j].ID, next[j].ID)
			}
		}
	}
}

func TestScopeRankOrderExprBindsDocumentIDs(t *testing.T) {
	expr := scopeRankOrderExpr([]string{"doc-a", "doc-b"})

	wantSQL := "CASE chunks.knowledge_id WHEN ? THEN ? WHEN ? THEN ? ELSE ? END, " + grepOrderTieBreak
	if expr.SQL != wantSQL {
		t.Fatalf("unexpected SQL: %q", expr.SQL)
	}
	want := []interface{}{"doc-a", 0, "doc-b", 1, 2}
	if len(expr.Vars) != len(want) {
		t.Fatalf("expected %d bound values, got %v", len(want), expr.Vars)
	}
	for i := range want {
		if expr.Vars[i] != want[i] {
			t.Fatalf("bound values differ: got %v, want %v", expr.Vars, want)
		}
	}
}
