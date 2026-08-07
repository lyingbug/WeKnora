package types

import (
	"math"
	"testing"
	"time"
)

// The illumination maths is the user-visible heart of the feature: it decides
// whether a page on the knowledge graph glows. It is also the piece that has to
// produce identical numbers on PostgreSQL and SQLite, which is why it lives in
// a pure function and is tested here rather than through a database.

func overlayOptions(now time.Time) MemoryOverlayOptions {
	return MemoryOverlayOptionsFrom(DefaultMemorySettings(), now)
}

func TestComputeMemoryOverlay_States(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-24 * time.Hour)

	anchors := []MemoryOverlayAnchor{
		// Heavily used and explicitly learned.
		{TargetRef: "concept/rag", Relation: MemoryRelationAskedAbout, HitCount: 12, LastSeenAt: recent, MemoryPageID: "mp1"},
		{TargetRef: "concept/rag", Relation: MemoryRelationLearned, HitCount: 3, LastSeenAt: recent, MemoryPageID: "mp1"},
		// Corrected: the user disputes this page.
		{TargetRef: "concept/rerank", Relation: MemoryRelationAskedAbout, HitCount: 6, LastSeenAt: recent, MemoryPageID: "mp2"},
		{TargetRef: "concept/rerank", Relation: MemoryRelationCorrected, HitCount: 1, LastSeenAt: recent, MemoryPageID: "mp2"},
		// Asked about repeatedly but never confirmed as understood.
		{TargetRef: "concept/hybrid", Relation: MemoryRelationAskedAbout, HitCount: 9, LastSeenAt: recent, MemoryPageID: "mp2"},
		// Touched once, long ago.
		{TargetRef: "entity/milvus", Relation: MemoryRelationMentioned, HitCount: 1, LastSeenAt: now.Add(-300 * 24 * time.Hour), MemoryPageID: "mp3"},
	}

	overlay := ComputeMemoryOverlay(anchors, overlayOptions(now))

	cases := map[string]string{
		"concept/rag":    MemoryStateMastered,
		"concept/rerank": MemoryStateFlagged,
		"concept/hybrid": MemoryStateFamiliar,
		"entity/milvus":  MemoryStateTouched,
	}
	for slug, want := range cases {
		node, ok := overlay[slug]
		if !ok {
			t.Fatalf("%s missing from overlay", slug)
		}
		if node.State != want {
			t.Errorf("%s state = %q (heat %.3f), want %q", slug, node.State, node.Heat, want)
		}
	}

	if _, ok := overlay["concept/never-touched"]; ok {
		t.Error("pages with no anchors must be absent rather than materialised as zeros")
	}
}

// A standing relationship is not an interaction, and must not fade like one.
// This was a real defect: a four-hundred-day-old "I own this topic" anchor
// decayed all the way down to barely-touched.
func TestComputeMemoryOverlay_OwnershipIsExemptFromDecay(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	longAgo := now.Add(-400 * 24 * time.Hour)

	overlay := ComputeMemoryOverlay([]MemoryOverlayAnchor{
		{TargetRef: "entity/pgvector", Relation: MemoryRelationOwns, HitCount: 2, LastSeenAt: longAgo, MemoryPageID: "mp1"},
	}, overlayOptions(now))

	node := overlay["entity/pgvector"]
	if node.Heat < 0.4 {
		t.Errorf("heat = %.3f: an ownership anchor must not fade with time", node.Heat)
	}
	if node.State == MemoryStateTouched || node.State == MemoryStateUnlit {
		t.Errorf("state = %q: owning a subject should keep the page lit", node.State)
	}
}

// A dispute does not expire either. Someone who corrected a page months ago
// still disagrees with it until the page changes.
func TestComputeMemoryOverlay_ConflictOutranksHeat(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	overlay := ComputeMemoryOverlay([]MemoryOverlayAnchor{
		{TargetRef: "concept/stale", Relation: MemoryRelationDisagreed, HitCount: 1, LastSeenAt: now.Add(-500 * 24 * time.Hour)},
	}, overlayOptions(now))

	if got := overlay["concept/stale"].State; got != MemoryStateFlagged {
		t.Errorf("state = %q, want %q even though the heat has long since decayed", got, MemoryStateFlagged)
	}
}

func TestComputeMemoryOverlay_DecaysOrdinaryInteractions(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	opts := overlayOptions(now)

	fresh := ComputeMemoryOverlay([]MemoryOverlayAnchor{
		{TargetRef: "a", Relation: MemoryRelationAskedAbout, HitCount: 10, LastSeenAt: now},
	}, opts)["a"]

	// Exactly one half-life later the contribution should have halved.
	aged := ComputeMemoryOverlay([]MemoryOverlayAnchor{
		{TargetRef: "a", Relation: MemoryRelationAskedAbout, HitCount: 10,
			LastSeenAt: now.Add(-time.Duration(opts.HalfLifeDays) * 24 * time.Hour)},
	}, opts)["a"]

	if math.Abs(aged.Heat-fresh.Heat/2) > 0.001 {
		t.Errorf("aged heat = %.4f, want ~%.4f (half of %.4f)", aged.Heat, fresh.Heat/2, fresh.Heat)
	}
}

func TestComputeMemoryOverlay_IsDeterministic(t *testing.T) {
	now := time.Now()
	anchors := []MemoryOverlayAnchor{
		{TargetRef: "a", Relation: MemoryRelationAskedAbout, HitCount: 3, LastSeenAt: now, MemoryPageID: "m1"},
		{TargetRef: "a", Relation: MemoryRelationLearned, HitCount: 1, LastSeenAt: now, MemoryPageID: "m2"},
	}
	first := ComputeMemoryOverlay(anchors, overlayOptions(now))["a"]
	second := ComputeMemoryOverlay(anchors, overlayOptions(now))["a"]

	if first.Heat != second.Heat || first.State != second.State {
		t.Error("the same input must always produce the same output")
	}
	if first.MemoryCount != 2 {
		t.Errorf("memory count = %d, want 2 distinct memories", first.MemoryCount)
	}
	if len(first.Relations) != 2 {
		t.Errorf("relations = %v, want both listed for the UI legend", first.Relations)
	}
}

func TestComputeMemoryCoverage(t *testing.T) {
	pages := []MemoryCoveragePage{
		{Slug: "concept/rag", Folder: "检索"},
		{Slug: "concept/rerank", Folder: "检索"},
		{Slug: "concept/hybrid", Folder: "检索"},
		{Slug: "concept/chunking", Folder: "检索"},
		{Slug: "entity/pgvector", Folder: "存储"},
		{Slug: "entity/milvus", Folder: "存储"},
	}
	overlay := map[string]MemoryOverlayNode{
		"concept/rag":     {State: MemoryStateMastered, Heat: 0.8},
		"concept/rerank":  {State: MemoryStateFlagged, Heat: 0.5},
		"concept/hybrid":  {State: MemoryStateFamiliar, Heat: 0.4},
		"entity/pgvector": {State: MemoryStateTouched, Heat: 0.1},
	}

	coverage := ComputeMemoryCoverage("kb1", pages, overlay)

	if coverage.TotalPages != 6 || coverage.LitPages != 4 {
		t.Errorf("coverage = %d/%d, want 4/6", coverage.LitPages, coverage.TotalPages)
	}
	if coverage.Percent != 66.7 {
		t.Errorf("percent = %.1f, want 66.7", coverage.Percent)
	}
	if coverage.StateCounts[MemoryStateUnlit] != 2 {
		t.Errorf("unlit count = %d, want 2", coverage.StateCounts[MemoryStateUnlit])
	}

	byFolder := map[string]MemoryCoverageBucket{}
	for _, bucket := range coverage.Folders {
		byFolder[bucket.Folder] = bucket
	}
	if got := byFolder["检索"].Percent; got != 75 {
		t.Errorf("检索 coverage = %.1f, want 75", got)
	}
	if got := byFolder["存储"].Percent; got != 50 {
		t.Errorf("存储 coverage = %.1f, want 50", got)
	}
}

func TestComputeMemoryCoverage_EmptyKnowledgeBase(t *testing.T) {
	coverage := ComputeMemoryCoverage("kb1", nil, nil)
	if coverage.Percent != 0 || coverage.TotalPages != 0 {
		t.Errorf("empty knowledge base must report zero, got %+v", coverage)
	}
}
