package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Long-term memory hooks on the chat path.
//
// There are exactly three, and each is deliberately small:
//
//   - prepareMemory, before the pipeline runs, resolves whose memory this turn
//     belongs to and under which settings.
//   - recordMemoryAnchors, after the answer, notes which knowledge-base pages
//     were actually cited so the illumination map fills in over time.
//   - considerMemoryExtraction, also after the answer, decides whether anything
//     said this turn is worth remembering.
//
// All three are best-effort. Memory enriches a conversation; it must never be
// able to break one.

// prepareMemory resolves the caller's memory space and settings for this turn.
// Returns true when memory is active and the recall stage should run.
func (s *sessionService) prepareMemory(
	ctx context.Context, chatManage *types.ChatManage, req *types.QARequest,
) bool {
	if s.memoryService == nil || s.memorySettings == nil {
		return false
	}

	space, err := s.memoryService.EnsureSpace(ctx)
	if err != nil {
		logger.Warnf(ctx, "memory: could not resolve space for session %s: %v", req.Session.ID, err)
		return false
	}
	if space == nil {
		return false
	}

	opts := types.MemorySettingsResolveOptions{
		TenantID:   req.Session.TenantID,
		SpaceID:    space.ID,
		SpacePatch: space.Config,
	}
	if userID, ok := types.UserIDFromContext(ctx); ok {
		opts.UserID = userID
	}
	// The agent is already loaded here, so its overrides are passed straight
	// through rather than costing a second lookup on the chat path.
	if req.CustomAgent != nil {
		opts.AgentID = req.CustomAgent.ID
		opts.AgentPatch = req.CustomAgent.Config.Memory
	}

	resolution, err := s.memorySettings.Resolve(ctx, opts)
	if err != nil {
		logger.Warnf(ctx, "memory: could not resolve settings: %v", err)
		return false
	}
	settings := resolution.Settings
	if !settings.Enabled {
		return false
	}

	chatManage.MemorySpaceID = space.ID
	chatManage.MemorySettings = settings
	return settings.RecallEnabled
}

// recordMemoryAnchors notes the knowledge-base pages that were cited.
//
// This is the cheapest signal in the subsystem and the one that carries the
// most: no model, one upsert per cited page, and over time it draws the map of
// which parts of the knowledge base this person has actually walked through.
func (s *sessionService) recordMemoryAnchors(
	ctx context.Context, chatManage *types.ChatManage, messageID string,
) {
	if s.memoryRecall == nil || chatManage.MemorySpaceID == "" {
		return
	}
	if !chatManage.MemorySettings.AnchorRuntimeEnabled {
		return
	}

	targets := memoryAnchorTargets(chatManage.MergeResult)
	if len(targets) == 0 {
		return
	}
	s.memoryRecall.RecordRetrievalAnchors(ctx, types.MemoryAnchorRecordRequest{
		TenantID:  chatManage.TenantID,
		SpaceID:   chatManage.MemorySpaceID,
		SessionID: chatManage.SessionID,
		MessageID: messageID,
		Query:     chatManage.Query,
		Settings:  chatManage.MemorySettings,
		Targets:   targets,
	})
}

// memoryAnchorTargets projects the cited results onto anchor targets.
//
// Wiki pages anchor by slug because that is what the illumination overlay keys
// on; ordinary document chunks anchor by knowledge id, which is the closest
// stable handle on "the thing the user read".
func memoryAnchorTargets(results []*types.SearchResult) []types.MemoryAnchorTarget {
	seen := make(map[string]struct{}, len(results))
	targets := make([]types.MemoryAnchorTarget, 0, len(results))

	for _, result := range results {
		if result == nil || result.KnowledgeBaseID == "" {
			continue
		}
		target := types.MemoryAnchorTarget{KnowledgeBaseID: result.KnowledgeBaseID}
		if result.ChunkType == types.ChunkTypeWikiPage {
			slug := wikiSlugFromResult(result)
			if slug == "" {
				continue
			}
			target.TargetKind = types.MemoryAnchorTargetWikiPage
			target.TargetRef = slug
		} else {
			if result.KnowledgeID == "" {
				continue
			}
			target.TargetKind = types.MemoryAnchorTargetKnowledge
			target.TargetRef = result.KnowledgeID
		}

		key := target.KnowledgeBaseID + "|" + target.TargetKind + "|" + target.TargetRef
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

// wikiSlugFromResult recovers the wiki slug a retrieval result came from.
func wikiSlugFromResult(result *types.SearchResult) string {
	if slug := strings.TrimSpace(result.Metadata["wiki_slug"]); slug != "" {
		return slug
	}
	// Wiki chunks are stored under a synthetic id; the knowledge id is the
	// stable fallback identifier when no slug travelled with the result.
	if strings.TrimSpace(result.KnowledgeID) != "" {
		return result.KnowledgeID
	}
	return ""
}

// considerMemoryExtraction hands the finished turn to the write-path gate.
func (s *sessionService) considerMemoryExtraction(
	ctx context.Context, chatManage *types.ChatManage, turnIndex int,
) {
	if s.memoryWriter == nil || chatManage.MemorySpaceID == "" {
		return
	}
	s.memoryWriter.ConsiderSession(ctx, types.MemoryExtractTrigger{
		TenantID:  chatManage.TenantID,
		SpaceID:   chatManage.MemorySpaceID,
		SessionID: chatManage.SessionID,
		Settings:  chatManage.MemorySettings,
		UserText:  chatManage.Query,
		TurnIndex: turnIndex,
	})
}
