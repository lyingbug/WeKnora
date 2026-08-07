package types

import (
	"math"
	"sort"
	"time"
)

// The illumination layer is where personal memory meets the shared knowledge
// base. Every anchor a user accumulates lights up the wiki page it points at,
// turning the knowledge graph into a personal map of "where have I actually
// been".
//
// All of it is computed here, in pure functions over plain slices, for one
// concrete reason: the same numbers have to come out on PostgreSQL and on
// SQLite. Expressing heat as SQL (power(), ln(), GROUP BY ROLLUP) works on
// Postgres and fails outright on Lite, so the repository layer only ever runs
// portable SELECTs and hands the rows to the functions below.

// Memory illumination states, from cold to hot.
const (
	// MemoryStateUnlit means the user has never touched this page.
	MemoryStateUnlit = "unlit"
	// MemoryStateTouched means some interaction exists but it has faded.
	MemoryStateTouched = "touched"
	// MemoryStateFamiliar means recurring, recent engagement.
	MemoryStateFamiliar = "familiar"
	// MemoryStateMastered means engagement plus an explicit signal of
	// understanding or ownership.
	MemoryStateMastered = "mastered"
	// MemoryStateFlagged means the user disputed or corrected this page. It
	// takes precedence over heat: a disagreement does not fade just because
	// the user stopped asking about it.
	MemoryStateFlagged = "flagged"
)

// Heat scoring constants. These shape the curve rather than the policy, so they
// are not exposed as settings; the thresholds and weights that decide user-
// visible outcomes are.
const (
	// heatRelationWeight scales the anchor-relation contribution.
	heatRelationWeight = 0.8
	// heatMemoryWeight scales the "how many of my memories point here" term.
	heatMemoryWeight = 0.6
	// heatNormalizer maps the raw score into roughly 0..1 before clamping.
	heatNormalizer = 6.0
)

// MemoryOverlayAnchor is the minimal anchor projection the overlay needs.
// Keeping it separate from MemoryAnchor lets the repository select four columns
// instead of hydrating whole rows for a graph of thousands of pages.
type MemoryOverlayAnchor struct {
	TargetRef    string
	Relation     string
	HitCount     int
	LastSeenAt   time.Time
	MemoryPageID string
}

// MemoryOverlayNode is the per-page illumination result.
type MemoryOverlayNode struct {
	// Heat is the normalised 0..1 engagement score.
	Heat float64 `json:"heat"`
	// State is one of the MemoryState* constants.
	State string `json:"state"`
	// AnchorCount is the number of anchors pointing at this page.
	AnchorCount int `json:"anchor_count"`
	// MemoryCount is how many distinct memory pages reference it.
	MemoryCount int `json:"memory_count"`
	// Relations lists the distinct relations present, for the UI legend.
	Relations []string `json:"relations"`
	// LastSeenAt is the most recent interaction.
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// MemoryOverlayOptions carries the tunables from resolved settings.
type MemoryOverlayOptions struct {
	RelationWeights   map[string]float64
	DecayExempt       []string
	HalfLifeDays      int
	FamiliarThreshold float64
	MasteredThreshold float64
	Now               time.Time
}

// MemoryOverlayOptionsFrom builds overlay options from resolved settings.
func MemoryOverlayOptionsFrom(s MemorySettings, now time.Time) MemoryOverlayOptions {
	return MemoryOverlayOptions{
		RelationWeights:   s.RelationWeights,
		DecayExempt:       s.DecayExemptRelations,
		HalfLifeDays:      s.AnchorHalfLifeDays,
		FamiliarThreshold: s.FamiliarThreshold,
		MasteredThreshold: s.MasteredThreshold,
		Now:               now,
	}
}

func (o MemoryOverlayOptions) weight(relation string) float64 {
	if w, ok := o.RelationWeights[relation]; ok {
		return w
	}
	if w, ok := DefaultMemoryRelationWeights()[relation]; ok {
		return w
	}
	return 0
}

func (o MemoryOverlayOptions) decays(relation string) bool {
	return !containsString(o.DecayExempt, relation)
}

func (o MemoryOverlayOptions) normalized() MemoryOverlayOptions {
	if o.HalfLifeDays <= 0 {
		o.HalfLifeDays = 120
	}
	if o.FamiliarThreshold <= 0 {
		o.FamiliarThreshold = 0.25
	}
	if o.MasteredThreshold <= 0 {
		o.MasteredThreshold = 0.6
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	return o
}

// ComputeMemoryOverlay folds a flat anchor list into per-target illumination.
//
// The returned map is keyed by anchor target ref (a wiki slug for the wiki
// graph). Targets with no anchors are simply absent; callers render those as
// unlit rather than paying to materialise a zero entry per page.
func ComputeMemoryOverlay(
	anchors []MemoryOverlayAnchor, opts MemoryOverlayOptions,
) map[string]MemoryOverlayNode {
	opts = opts.normalized()

	type accumulator struct {
		decayedRaw  float64
		exemptRaw   float64
		memoryPages map[string]struct{}
		relations   map[string]struct{}
		anchorCount int
		lastSeen    time.Time
		lastDecayed time.Time
		hasLearned  bool
		hasConflict bool
	}

	acc := make(map[string]*accumulator, len(anchors))
	for _, a := range anchors {
		if a.TargetRef == "" {
			continue
		}
		entry, ok := acc[a.TargetRef]
		if !ok {
			entry = &accumulator{
				memoryPages: make(map[string]struct{}),
				relations:   make(map[string]struct{}),
			}
			acc[a.TargetRef] = entry
		}
		entry.anchorCount++
		entry.relations[a.Relation] = struct{}{}
		if a.MemoryPageID != "" {
			entry.memoryPages[a.MemoryPageID] = struct{}{}
		}
		if a.LastSeenAt.After(entry.lastSeen) {
			entry.lastSeen = a.LastSeenAt
		}

		contribution := opts.weight(a.Relation) * math.Log1p(float64(maxInt(a.HitCount, 0)))
		if opts.decays(a.Relation) {
			entry.decayedRaw += contribution
			if a.LastSeenAt.After(entry.lastDecayed) {
				entry.lastDecayed = a.LastSeenAt
			}
		} else {
			// Standing relationships contribute at full strength forever.
			entry.exemptRaw += contribution
		}

		switch a.Relation {
		case MemoryRelationLearned, MemoryRelationOwns:
			entry.hasLearned = true
		case MemoryRelationDisagreed, MemoryRelationCorrected:
			entry.hasConflict = true
		}
	}

	out := make(map[string]MemoryOverlayNode, len(acc))
	for ref, entry := range acc {
		memoryTerm := heatMemoryWeight * math.Log1p(float64(len(entry.memoryPages)))

		decayed := 0.0
		if entry.decayedRaw > 0 {
			base := clamp01((heatRelationWeight*entry.decayedRaw + memoryTerm) / heatNormalizer)
			decayed = base * decayFactor(opts.Now, entry.lastDecayed, opts.HalfLifeDays)
		}
		exempt := 0.0
		if entry.exemptRaw > 0 {
			exempt = clamp01((heatRelationWeight*entry.exemptRaw + memoryTerm) / heatNormalizer)
		}
		heat := clamp01(math.Max(decayed, exempt))

		node := MemoryOverlayNode{
			Heat:        heat,
			State:       memoryState(heat, entry.hasLearned, entry.hasConflict, opts),
			AnchorCount: entry.anchorCount,
			MemoryCount: len(entry.memoryPages),
			Relations:   sortedKeys(entry.relations),
		}
		if !entry.lastSeen.IsZero() {
			seen := entry.lastSeen
			node.LastSeenAt = &seen
		}
		out[ref] = node
	}
	return out
}

func memoryState(heat float64, hasLearned, hasConflict bool, opts MemoryOverlayOptions) string {
	switch {
	case hasConflict:
		return MemoryStateFlagged
	case heat >= opts.MasteredThreshold && hasLearned:
		return MemoryStateMastered
	case heat >= opts.FamiliarThreshold:
		return MemoryStateFamiliar
	case heat > 0:
		return MemoryStateTouched
	default:
		return MemoryStateUnlit
	}
}

// decayFactor is the exponential half-life multiplier for an interaction that
// last happened at lastSeen.
func decayFactor(now, lastSeen time.Time, halfLifeDays int) float64 {
	if lastSeen.IsZero() || halfLifeDays <= 0 {
		return 0
	}
	ageDays := now.Sub(lastSeen).Hours() / 24
	if ageDays <= 0 {
		return 1
	}
	return math.Pow(0.5, ageDays/float64(halfLifeDays))
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Coverage
// ---------------------------------------------------------------------------

// MemoryCoveragePage is the minimal wiki-page projection coverage needs.
type MemoryCoveragePage struct {
	Slug string
	// Folder is the first breadcrumb segment, or "" for the wiki root.
	Folder string
}

// MemoryCoverageBucket is coverage for one folder, or for the whole KB.
type MemoryCoverageBucket struct {
	Folder     string  `json:"folder"`
	TotalPages int     `json:"total_pages"`
	LitPages   int     `json:"lit_pages"`
	Percent    float64 `json:"percent"`
}

// MemoryCoverage is the full mastery report for one user in one knowledge base.
type MemoryCoverage struct {
	KnowledgeBaseID string                 `json:"knowledge_base_id"`
	TotalPages      int                    `json:"total_pages"`
	LitPages        int                    `json:"lit_pages"`
	Percent         float64                `json:"percent"`
	StateCounts     map[string]int         `json:"state_counts"`
	Folders         []MemoryCoverageBucket `json:"folders"`
}

// ComputeMemoryCoverage summarises how much of a knowledge base a user has lit
// up, overall and per folder.
//
// The page list must already exclude archived and deleted pages: counting
// retired pages in the denominator would make coverage drift downwards for
// reasons that have nothing to do with the user.
func ComputeMemoryCoverage(
	kbID string, pages []MemoryCoveragePage, overlay map[string]MemoryOverlayNode,
) MemoryCoverage {
	report := MemoryCoverage{
		KnowledgeBaseID: kbID,
		TotalPages:      len(pages),
		StateCounts:     map[string]int{},
	}
	buckets := map[string]*MemoryCoverageBucket{}
	order := make([]string, 0, 8)

	for _, page := range pages {
		bucket, ok := buckets[page.Folder]
		if !ok {
			bucket = &MemoryCoverageBucket{Folder: page.Folder}
			buckets[page.Folder] = bucket
			order = append(order, page.Folder)
		}
		bucket.TotalPages++

		node, lit := overlay[page.Slug]
		state := MemoryStateUnlit
		if lit {
			state = node.State
			report.LitPages++
			bucket.LitPages++
		}
		report.StateCounts[state]++
	}

	sort.Strings(order)
	report.Folders = make([]MemoryCoverageBucket, 0, len(order))
	for _, folder := range order {
		b := buckets[folder]
		b.Percent = percent(b.LitPages, b.TotalPages)
		report.Folders = append(report.Folders, *b)
	}
	report.Percent = percent(report.LitPages, report.TotalPages)
	return report
}

func percent(part, total int) float64 {
	if total == 0 {
		return 0
	}
	// One decimal place: coverage is a progress indicator, not a measurement,
	// and full float noise in the UI reads as false precision.
	return math.Round(float64(part)/float64(total)*1000) / 10
}
