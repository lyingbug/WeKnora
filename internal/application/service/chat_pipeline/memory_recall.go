package chatpipeline

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PluginMemoryRecall selects the caller's long-term memories for this turn.
//
// It sits between query understanding and retrieval so the anchor hints it
// produces are available to reranking, and so the memory block is ready by the
// time the prompt is assembled.
//
// The stage adds no model call. Resident memories come from an indexed query
// and relevant ones are scored in Go, which is what lets memory be on by
// default without changing the cost of a conversation.
type PluginMemoryRecall struct {
	recall interfaces.MemoryRecallService
}

// NewPluginMemoryRecall creates and registers the recall plugin.
func NewPluginMemoryRecall(
	eventManager *EventManager, recall interfaces.MemoryRecallService,
) *PluginMemoryRecall {
	p := &PluginMemoryRecall{recall: recall}
	eventManager.Register(p)
	return p
}

// ActivationEvents returns the event types this plugin handles.
func (p *PluginMemoryRecall) ActivationEvents() []types.EventType {
	return []types.EventType{types.MEMORY_RECALL}
}

// OnEvent gathers memories for the current turn.
//
// Any failure here is swallowed. A memory subsystem that can turn a recoverable
// database hiccup into a failed answer is worse than no memory subsystem, so
// the stage either enriches the turn or gets out of the way.
func (p *PluginMemoryRecall) OnEvent(
	ctx context.Context, eventType types.EventType, chatManage *types.ChatManage, next func() *PluginError,
) *PluginError {
	if p.recall == nil || chatManage.MemorySpaceID == "" {
		return next()
	}

	// Since v0.7.2 an unparsable rewrite leaves RewriteQuery empty instead of
	// echoing the raw model output, so the original question is the fallback.
	query := chatManage.RewriteQuery
	if query == "" {
		query = chatManage.Query
	}

	result := p.recall.Recall(ctx, types.MemoryRecallRequest{
		TenantID:         chatManage.TenantID,
		SpaceID:          chatManage.MemorySpaceID,
		Query:            query,
		Settings:         chatManage.MemorySettings,
		KnowledgeBaseIDs: chatManage.KnowledgeBaseIDs,
		Language:         chatManage.Language,
	})
	if result == nil {
		return next()
	}

	chatManage.MemoryContext = result
	chatManage.MemoryAnchorHints = result.AnchorHints

	// Usage is recorded here, at the point the memories are committed to the
	// prompt, because that is what "this memory was useful" actually means.
	slugs := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		slugs = append(slugs, item.Slug)
	}
	p.recall.RecordUsage(ctx, chatManage.MemorySpaceID, slugs)

	// The pure-chat path assembles its prompt before the pipeline starts, so
	// there is no later stage to attach the block to. A populated UserContent
	// at this point means exactly that; the RAG path leaves it empty until
	// INTO_CHAT_MESSAGE, which applies the block itself. prependMemoryBlock is
	// idempotent, so neither path can double-inject.
	if chatManage.UserContent != "" {
		chatManage.UserContent = prependMemoryBlock(chatManage, chatManage.UserContent)
	}

	logger.Infof(ctx, "MemoryRecall: injected %d memories (%d tokens) for session %s",
		len(result.Items), result.TokensUsed, chatManage.SessionID)

	return next()
}
