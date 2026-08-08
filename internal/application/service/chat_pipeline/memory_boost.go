package chatpipeline

import (
	"context"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// PluginMemoryBoost personalises ranking using the caller's anchors.
//
// A knowledge-base page the caller has already worked with, on a subject one of
// their memories covers, is more likely to be the page they want now. That is
// the entire claim, and it is a modest one, which is why the boost is a small
// multiplier applied after reranking rather than a change to retrieval itself
// — and why it is off by default until a workspace has measured it.
//
// Structurally identical to PluginWikiBoost, deliberately: two plugins that do
// the same kind of thing should look the same.
type PluginMemoryBoost struct{}

// NewPluginMemoryBoost creates and registers the memory boost plugin.
func NewPluginMemoryBoost(eventManager *EventManager) *PluginMemoryBoost {
	p := &PluginMemoryBoost{}
	eventManager.Register(p)
	return p
}

// ActivationEvents returns the event types this plugin handles.
func (p *PluginMemoryBoost) ActivationEvents() []types.EventType {
	return []types.EventType{types.CHUNK_RERANK}
}

// OnEvent applies the anchor boost after reranking.
func (p *PluginMemoryBoost) OnEvent(
	ctx context.Context, eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	if err := next(); err != nil {
		return err
	}
	if !chatManage.MemorySettings.BoostEnabled || len(chatManage.MemoryAnchorHints) == 0 {
		return nil
	}

	factor := chatManage.MemorySettings.BoostFactor
	if factor <= 1 {
		return nil
	}

	// Anchors point at wiki slugs and knowledge ids; a result matches when any of
	// its identifiers lines up. Matching on the knowledge id as well means a page
	// the user engaged with still gets its boost when retrieval returned the
	// underlying document chunk rather than the wiki page — and it is the only
	// way an ordinary, non-wiki knowledge base can match at all.
	//
	// Each hint carries the weight of its strongest relation, and that weight
	// scales the boost. It used to be computed and then dropped, so a page the
	// person merely mentioned was promoted exactly as hard as one they had
	// corrected, which makes memory.overlay.relation_weights meaningless in
	// ranking.
	weights := make(map[string]float64, len(chatManage.MemoryAnchorHints))
	for _, hint := range chatManage.MemoryAnchorHints {
		if hint.TargetRef == "" {
			continue
		}
		ref := strings.ToLower(hint.TargetRef)
		if hint.Weight > weights[ref] {
			weights[ref] = hint.Weight
		}
	}

	boosted := 0
	for i := range chatManage.RerankResult {
		result := chatManage.RerankResult[i]
		if result == nil {
			continue
		}
		weight, ok := anchorWeightForResult(weights, result)
		if !ok {
			continue
		}
		// weight 1 leaves the configured factor as-is; a heavier relation scales
		// it further, and the whole thing is still bounded by the factor a
		// workspace chose.
		chatManage.RerankResult[i].Score *= 1 + (factor-1)*weight
		boosted++
	}

	if boosted > 0 {
		logger.Infof(ctx, "MemoryBoost: boosted %d results by %.2fx for session %s",
			boosted, factor, chatManage.SessionID)
		sort.SliceStable(chatManage.RerankResult, func(i, j int) bool {
			return chatManage.RerankResult[i].Score > chatManage.RerankResult[j].Score
		})
	}
	return nil
}

// anchorWeightForResult returns the weight of the anchor matching this result.
func anchorWeightForResult(
	weights map[string]float64, result *types.SearchResult,
) (float64, bool) {
	// A wiki anchor stores the page slug, so the slug has to be one of the
	// candidates. Without it the boost could never match wiki content, which is
	// the content anchors were designed around: every wiki anchor is written as
	// a slug (see memoryAnchorTargets) and every candidate here was an id.
	candidates := []string{
		result.KnowledgeID,
		result.ID,
		strings.TrimSpace(result.Metadata["wiki_slug"]),
	}
	if result.KnowledgeTitle != "" {
		candidates = append(candidates, result.KnowledgeTitle)
	}
	best, found := 0.0, false
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if w, ok := weights[strings.ToLower(candidate)]; ok && (!found || w > best) {
			best, found = w, true
		}
	}
	return best, found
}
