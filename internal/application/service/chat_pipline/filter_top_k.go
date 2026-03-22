package chatpipline

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// PluginFilterTopK is a plugin that filters search results to keep only the top K items
type PluginFilterTopK struct{}

// NewPluginFilterTopK creates a new instance of PluginFilterTopK and registers it with the event manager
func NewPluginFilterTopK(eventManager *EventManager) *PluginFilterTopK {
	res := &PluginFilterTopK{}
	eventManager.Register(res)
	return res
}

// ActivationEvents returns the event types that this plugin responds to
func (p *PluginFilterTopK) ActivationEvents() []types.EventType {
	return []types.EventType{types.FILTER_TOP_K}
}

// OnEvent handles the FILTER_TOP_K event by filtering results to keep only the top K items
// It can filter MergeResult, RerankResult, or SearchResult depending on which is available
func (p *PluginFilterTopK) OnEvent(ctx context.Context,
	eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	// Skip when KB search was bypassed (intent classification)
	if chatManage.SkipKBSearch {
		pipelineInfo(ctx, "FilterTopK", "skip", map[string]interface{}{
			"session_id": chatManage.SessionID,
			"reason":     "skip_kb_search",
		})
		return next()
	}

	topK := chatManage.RerankTopK

	// When extended thinking is enabled, reduce top_k to leave more token budget
	// for the model's internal reasoning. Thinking mode generates substantial
	// intermediate tokens that compete with context for the model's token window.
	if chatManage.SummaryConfig.Thinking != nil && *chatManage.SummaryConfig.Thinking && topK > 0 {
		thinkingTopK := (topK + 1) / 2 // ceil(topK/2): e.g. 10→5, 5→3, 3→2
		if thinkingTopK < 2 {
			thinkingTopK = 2 // keep at least 2 results for cross-referencing
		}
		pipelineInfo(ctx, "FilterTopK", "thinking_reduce", map[string]interface{}{
			"original_top_k": topK,
			"thinking_top_k": thinkingTopK,
		})
		topK = thinkingTopK
	}

	pipelineInfo(ctx, "FilterTopK", "input", map[string]interface{}{
		"session_id": chatManage.SessionID,
		"top_k":      topK,
		"merge_cnt":  len(chatManage.MergeResult),
		"rerank_cnt": len(chatManage.RerankResult),
		"search_cnt": len(chatManage.SearchResult),
	})

	filterTopK := func(searchResult []*types.SearchResult, k int) []*types.SearchResult {
		if k > 0 && len(searchResult) > k {
			pipelineInfo(ctx, "FilterTopK", "filter", map[string]interface{}{
				"before": len(searchResult),
				"after":  k,
			})
			searchResult = searchResult[:k]
		}
		return searchResult
	}

	if len(chatManage.MergeResult) > 0 {
		chatManage.MergeResult = filterTopK(chatManage.MergeResult, topK)
	} else if len(chatManage.RerankResult) > 0 {
		chatManage.RerankResult = filterTopK(chatManage.RerankResult, topK)
	} else if len(chatManage.SearchResult) > 0 {
		chatManage.SearchResult = filterTopK(chatManage.SearchResult, topK)
	} else {
		pipelineWarn(ctx, "FilterTopK", "skip", map[string]interface{}{
			"reason": "no_results",
		})
	}

	pipelineInfo(ctx, "FilterTopK", "output", map[string]interface{}{
		"merge_cnt":  len(chatManage.MergeResult),
		"rerank_cnt": len(chatManage.RerankResult),
		"search_cnt": len(chatManage.SearchResult),
	})
	return next()
}
