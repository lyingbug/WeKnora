package memory

import (
	"sort"

	"github.com/Tencent/WeKnora/internal/types"
)

// thinPageContentLength is the body size below which a wiki page counts as
// "thin". It is a heuristic for "there is not much here yet", not a quality
// judgement, which is why the insight reports the length alongside the verdict
// and leaves the decision to a human.
const thinPageContentLength = 1000

// BuildMemoryInsights turns per-target anchor aggregates into the anonymised
// report a knowledge-base owner sees.
//
// This is the loop that makes memory pay for itself twice: the same anchors
// that light up one person's map of the knowledge base, summed across everyone
// and stripped of identity, show which pages are asked about but underwritten,
// which have been contested, and which nobody has ever needed.
//
// k-anonymity is enforced here rather than in the UI. A target that only one
// person interacted with says something about that person, not about the
// knowledge base, so it is counted as suppressed and dropped.
func BuildMemoryInsights(
	kbID string,
	aggregates []types.MemoryAnchorAggregate,
	pages []types.MemoryInsightPage,
	kAnonymity int,
) *types.MemoryInsightsResponse {
	if kAnonymity < 1 {
		kAnonymity = 5
	}
	resp := &types.MemoryInsightsResponse{
		KnowledgeBaseID: kbID,
		KAnonymity:      kAnonymity,
		Insights:        []types.MemoryInsight{},
	}

	pageBySlug := make(map[string]types.MemoryInsightPage, len(pages))
	for _, page := range pages {
		pageBySlug[page.Slug] = page
	}

	type rollup struct {
		asked          int
		contested      int
		distinctAsked  int
		distinctAll    int
		anyInteraction bool
	}
	byTarget := map[string]*rollup{}

	for _, agg := range aggregates {
		if agg.TargetKind != types.MemoryAnchorTargetWikiPage {
			continue
		}
		entry, ok := byTarget[agg.TargetRef]
		if !ok {
			entry = &rollup{}
			byTarget[agg.TargetRef] = entry
		}
		entry.anyInteraction = true
		if agg.DistinctSpaces > entry.distinctAll {
			entry.distinctAll = agg.DistinctSpaces
		}
		switch agg.Relation {
		case types.MemoryRelationAskedAbout:
			entry.asked += agg.Interactions
			if agg.DistinctSpaces > entry.distinctAsked {
				entry.distinctAsked = agg.DistinctSpaces
			}
		case types.MemoryRelationCorrected, types.MemoryRelationDisagreed:
			entry.contested += agg.Interactions
		}
	}

	for slug, entry := range byTarget {
		page, known := pageBySlug[slug]

		// A target people ask about that has no page at all is the clearest
		// possible signal of a missing article.
		if !known {
			if entry.distinctAll < kAnonymity {
				resp.Suppressed++
				continue
			}
			resp.Insights = append(resp.Insights, types.MemoryInsight{
				Kind:           types.MemoryInsightMissingPage,
				TargetRef:      slug,
				Interactions:   entry.asked,
				DistinctPeople: entry.distinctAll,
			})
			continue
		}

		if entry.contested > 0 {
			if entry.distinctAll < kAnonymity {
				resp.Suppressed++
			} else {
				resp.Insights = append(resp.Insights, types.MemoryInsight{
					Kind:           types.MemoryInsightContested,
					TargetRef:      slug,
					Title:          page.Title,
					ContentLength:  page.ContentLength,
					Interactions:   entry.contested,
					DistinctPeople: entry.distinctAll,
				})
			}
		}

		if entry.asked > 0 && page.ContentLength < thinPageContentLength {
			if entry.distinctAsked < kAnonymity {
				resp.Suppressed++
				continue
			}
			resp.Insights = append(resp.Insights, types.MemoryInsight{
				Kind:           types.MemoryInsightThinButHot,
				TargetRef:      slug,
				Title:          page.Title,
				ContentLength:  page.ContentLength,
				Interactions:   entry.asked,
				DistinctPeople: entry.distinctAsked,
			})
		}
	}

	// Pages nobody has ever touched need no k-anonymity gate: "zero people
	// interacted with this" reveals nothing about anyone.
	for _, page := range pages {
		if entry, ok := byTarget[page.Slug]; !ok || !entry.anyInteraction {
			resp.Insights = append(resp.Insights, types.MemoryInsight{
				Kind:          types.MemoryInsightNeverLit,
				TargetRef:     page.Slug,
				Title:         page.Title,
				ContentLength: page.ContentLength,
			})
		}
	}

	sort.SliceStable(resp.Insights, func(i, j int) bool {
		if insightPriority(resp.Insights[i].Kind) != insightPriority(resp.Insights[j].Kind) {
			return insightPriority(resp.Insights[i].Kind) < insightPriority(resp.Insights[j].Kind)
		}
		if resp.Insights[i].Interactions != resp.Insights[j].Interactions {
			return resp.Insights[i].Interactions > resp.Insights[j].Interactions
		}
		return resp.Insights[i].TargetRef < resp.Insights[j].TargetRef
	})
	return resp
}

// insightPriority orders the report by how much action each kind warrants.
func insightPriority(kind string) int {
	switch kind {
	case types.MemoryInsightContested:
		return 0
	case types.MemoryInsightThinButHot:
		return 1
	case types.MemoryInsightMissingPage:
		return 2
	default:
		return 3
	}
}
