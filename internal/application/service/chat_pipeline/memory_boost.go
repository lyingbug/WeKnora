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

	// Anchors point at wiki slugs and knowledge ids; a result matches when
	// either identifier lines up. Matching on the knowledge id as well means a
	// page the user engaged with still gets its boost when retrieval returned
	// the underlying document chunk rather than the wiki page.
	refs := make(map[string]struct{}, len(chatManage.MemoryAnchorHints))
	for _, hint := range chatManage.MemoryAnchorHints {
		if hint.TargetRef != "" {
			refs[strings.ToLower(hint.TargetRef)] = struct{}{}
		}
	}

	boosted := 0
	for i := range chatManage.RerankResult {
		result := chatManage.RerankResult[i]
		if result == nil {
			continue
		}
		if !anchorMatchesResult(refs, result) {
			continue
		}
		chatManage.RerankResult[i].Score *= factor
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

func anchorMatchesResult(refs map[string]struct{}, result *types.SearchResult) bool {
	candidates := []string{result.KnowledgeID, result.ID}
	if result.KnowledgeTitle != "" {
		candidates = append(candidates, result.KnowledgeTitle)
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := refs[strings.ToLower(candidate)]; ok {
			return true
		}
	}
	return false
}
