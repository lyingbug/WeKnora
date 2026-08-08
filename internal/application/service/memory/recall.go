package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// recallService selects which memories to put in front of the model, and
// records what the conversation revealed about the user's engagement with the
// knowledge base.
//
// Two rules govern everything here:
//
//  1. Recall must never cost the user their answer. Every method swallows its
//     errors and degrades to "no memory this turn". A memory subsystem that can
//     break chat is worse than no memory subsystem.
//  2. Recall must never cost an extra model call. Resident memories come from
//     an indexed query; relevant ones are scored in Go. The whole stage is
//     database reads and arithmetic.
type recallService struct {
	spaces  interfaces.MemorySpaceRepository
	pages   interfaces.MemoryPageRepository
	anchors interfaces.MemoryAnchorRepository
}

// NewRecallService creates the recall service.
func NewRecallService(
	spaces interfaces.MemorySpaceRepository,
	pages interfaces.MemoryPageRepository,
	anchors interfaces.MemoryAnchorRepository,
) interfaces.MemoryRecallService {
	return &recallService{spaces: spaces, pages: pages, anchors: anchors}
}

// maxRelevanceCandidates bounds how many active pages the scorer will load.
// Well past the configured per-space cap, so in practice it never truncates;
// it exists so a misconfigured space cannot turn one chat turn into a table
// scan of thousands of rows.
const maxRelevanceCandidates = 3000

// Recall gathers the memories worth injecting for one turn.
func (s *recallService) Recall(
	ctx context.Context, req types.MemoryRecallRequest,
) *types.MemoryRecallResult {
	if req.SpaceID == "" || !req.Settings.Enabled || !req.Settings.RecallEnabled {
		return nil
	}

	// The whole stage lives under one deadline. Exceeding it means the user
	// gets an answer without memory, which is always the right trade.
	timeout := time.Duration(req.Settings.RecallTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 300 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := &types.MemoryRecallResult{SpaceID: req.SpaceID}

	// Saved memories are the user's explicit contract with the product. They
	// are considered on every turn regardless of type; chat-history memories
	// must earn a slot through relevance instead of becoming permanent prompt
	// furniture merely because an extractor labelled them profile/preference.
	resident, err := s.pages.ListBySaved(
		ctx, req.SpaceID, true,
		[]string{types.MemoryPageStatusActive}, req.Settings.RecallMaxItems*2,
	)
	if err != nil {
		logger.Warnf(ctx, "memory recall: resident lookup failed: %v", err)
	}

	var openQuestions []*types.MemoryPage
	if shouldRecallOpenQuestions(req.Query) {
		openQuestions, err = s.pages.ListByTypes(
			ctx, req.SpaceID, []string{types.MemoryTypeOpenQuestion},
			[]string{types.MemoryPageStatusActive}, 12,
		)
		if err != nil {
			logger.Warnf(ctx, "memory recall: open-question lookup failed: %v", err)
		}
	}

	var relevant []types.MemoryRecallItem
	if req.Settings.RelevanceRecall && req.Query != "" {
		relevant = s.scoreRelevant(ctx, req, resident)
	}

	result.Preference = mergePreferences(resident)
	result.Items = s.assemble(resident, relevant, req.Settings)
	if len(openQuestions) > 0 {
		result.OpenQuestions = scoreMemories(req.Query, openQuestions, func() float64 {
			return float64(time.Now().Unix())
		})
		if len(result.OpenQuestions) > 2 {
			result.OpenQuestions = result.OpenQuestions[:2]
		}
	}
	s.enforceRenderedBudget(result, req.Settings, req.Language)

	if result.IsEmpty() {
		return nil
	}
	result.AnchorHints = s.anchorHints(ctx, req, result)
	return result
}

// scoreRelevant ranks the space's other memories against the current question.
func (s *recallService) scoreRelevant(
	ctx context.Context, req types.MemoryRecallRequest, resident []*types.MemoryPage,
) []types.MemoryRecallItem {
	candidates, err := s.pages.ListByTypes(
		ctx, req.SpaceID, nil, []string{types.MemoryPageStatusActive}, maxRelevanceCandidates,
	)
	if err != nil {
		logger.Warnf(ctx, "memory recall: candidate lookup failed: %v", err)
		return nil
	}

	// Saved memories are injected unconditionally, so scoring them again
	// would only let them occupy the relevance slots twice.
	residentSlugs := make(map[string]struct{}, len(resident))
	for _, page := range resident {
		residentSlugs[page.Slug] = struct{}{}
	}
	filtered := candidates[:0]
	for _, page := range candidates {
		if _, ok := residentSlugs[page.Slug]; ok {
			continue
		}
		if page.PageType == types.MemoryTypeOpenQuestion {
			continue
		}
		filtered = append(filtered, page)
	}

	now := func() float64 { return float64(time.Now().Unix()) }
	return scoreMemories(req.Query, filtered, now)
}

func shouldRecallOpenQuestions(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	for _, marker := range []string{
		"继续", "接着", "上次", "之前", "进展", "未解决", "待办",
		"continue", "pick up", "last time", "previous", "progress", "open question", "todo",
	} {
		if strings.Contains(query, marker) {
			return true
		}
	}
	return false
}

// enforceRenderedBudget measures the complete block, including headers,
// structured preferences and open questions. The old implementation budgeted
// only ordinary bullets, so the actual prompt could exceed the advertised cap.
func (s *recallService) enforceRenderedBudget(
	result *types.MemoryRecallResult, settings types.MemorySettings, language string,
) {
	budget := settings.InjectionTokenBudget
	if budget <= 0 {
		budget = 600
	}
	for EstimateTokens(FormatMemoryBlock(result, language)) > budget {
		switch {
		case len(result.OpenQuestions) > 0:
			result.OpenQuestions = result.OpenQuestions[:len(result.OpenQuestions)-1]
		case len(result.Items) > 0:
			result.Items = result.Items[:len(result.Items)-1]
		default:
			// Structured preferences are bounded and sanitized; if an operator
			// configures a budget smaller than the block header, return no memory.
			result.Preference = types.MemoryPreference{}
			result.TokensUsed = 0
			return
		}
	}
	result.TokensUsed = EstimateTokens(FormatMemoryBlock(result, language))
}

// assemble applies the item cap and the token budget.
//
// Resident memories go in first and are only dropped when the budget is
// genuinely exhausted: knowing who the user is and how they want to be answered
// matters more on any given turn than any single recalled fact.
func (s *recallService) assemble(
	resident []*types.MemoryPage, relevant []types.MemoryRecallItem, settings types.MemorySettings,
) []types.MemoryRecallItem {
	budget := settings.InjectionTokenBudget
	if budget <= 0 {
		budget = 600
	}
	maxItems := settings.RecallMaxItems
	if maxItems <= 0 {
		maxItems = 8
	}

	out := make([]types.MemoryRecallItem, 0, maxItems)
	used := 0

	add := func(item types.MemoryRecallItem) bool {
		if len(out) >= maxItems {
			return false
		}
		cost := EstimateTokens(item.Text) + 8 // bullet, date prefix, newline
		if used+cost > budget {
			return false
		}
		used += cost
		out = append(out, item)
		return true
	}

	for _, page := range toRecallItems(resident, true) {
		if !add(page) {
			break
		}
	}
	for _, item := range relevant {
		if !add(item) {
			break
		}
	}
	return out
}

// anchorHints turns the recalled memories into ranking hints for retrieval.
//
// This is what makes retrieval personal: a knowledge-base page the user has
// already engaged with, on a subject one of their memories covers, is more
// likely to be the page they want now.
func (s *recallService) anchorHints(
	ctx context.Context, req types.MemoryRecallRequest, result *types.MemoryRecallResult,
) []types.MemoryAnchorHint {
	if !req.Settings.BoostEnabled {
		return nil
	}

	// Only anchors attached to memories actually selected for this turn may
	// influence retrieval. Loading every anchor in the space created a
	// rich-get-richer loop where unrelated past citations biased every query.
	slugs := make([]string, 0, len(result.Items)+len(result.OpenQuestions))
	for _, item := range append(append([]types.MemoryRecallItem{}, result.Items...), result.OpenQuestions...) {
		slugs = append(slugs, item.Slug)
	}
	pages, err := s.pages.GetBySlugs(ctx, req.SpaceID, slugs)
	if err != nil {
		return nil
	}
	allowedKBs := make(map[string]struct{}, len(req.KnowledgeBaseIDs))
	for _, id := range req.KnowledgeBaseIDs {
		allowedKBs[id] = struct{}{}
	}
	hints := map[string]types.MemoryAnchorHint{}
	for _, page := range pages {
		anchors, err := s.anchors.ListByPage(ctx, req.SpaceID, page.ID)
		if err != nil {
			continue
		}
		for _, anchor := range anchors {
			if len(allowedKBs) > 0 {
				if _, ok := allowedKBs[anchor.KnowledgeBaseID]; !ok {
					continue
				}
			}
			key := anchor.KnowledgeBaseID + "|" + anchor.TargetKind + "|" + anchor.TargetRef
			hint, exists := hints[key]
			if !exists {
				hint = types.MemoryAnchorHint{
					KnowledgeBaseID: anchor.KnowledgeBaseID,
					TargetKind:      anchor.TargetKind,
					TargetRef:       anchor.TargetRef,
					Weight:          1,
				}
			}
			if w := req.Settings.RelationWeight(anchor.Relation); w > hint.Weight {
				hint.Weight = w
			}
			hints[key] = hint
		}
	}

	out := make([]types.MemoryAnchorHint, 0, len(hints))
	for _, hint := range hints {
		out = append(out, hint)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TargetRef < out[j].TargetRef })
	return out
}

// RecordUsage marks injected memories as used.
//
// Usage is recorded on injection rather than on retrieval because a page that
// lost the token budget did not shape the answer, and rewarding it would keep
// weak memories alive forever.
func (s *recallService) RecordUsage(ctx context.Context, spaceID string, slugs []string) {
	if spaceID == "" || len(slugs) == 0 {
		return
	}
	pages, err := s.pages.GetBySlugs(ctx, spaceID, slugs)
	if err != nil || len(pages) == 0 {
		return
	}
	ids := make([]string, 0, len(pages))
	for _, page := range pages {
		ids = append(ids, page.ID)
	}
	if err := s.pages.BumpHits(ctx, spaceID, ids, time.Now()); err != nil {
		logger.Warnf(ctx, "memory recall: failed to record usage: %v", err)
	}
}

// RecordRetrievalAnchors records that the user asked about, and was answered
// from, specific knowledge-base pages.
//
// This is the cheapest and most valuable signal in the whole subsystem: it
// costs one upsert per cited page, involves no model, and is what fills in the
// illumination map over time.
func (s *recallService) RecordRetrievalAnchors(
	ctx context.Context, req types.MemoryAnchorRecordRequest,
) {
	if req.SpaceID == "" || len(req.Targets) == 0 {
		return
	}
	if !req.Settings.Enabled || !req.Settings.AnchorRuntimeEnabled {
		return
	}
	evidence := types.MemoryAnchorEvidence{}
	if req.MessageID != "" {
		evidence.MessageIDs = []string{req.MessageID}
	}
	if req.SessionID != "" {
		evidence.SessionIDs = []string{req.SessionID}
	}
	if req.Query != "" {
		evidence.Queries = []string{truncateRunes(req.Query, 120)}
	}

	for _, target := range req.Targets {
		if target.TargetRef == "" || target.KnowledgeBaseID == "" {
			continue
		}
		kind := target.TargetKind
		if kind == "" {
			kind = types.MemoryAnchorTargetWikiPage
		}
		if err := s.anchors.Upsert(ctx, &types.MemoryAnchorUpsert{
			SpaceID:         req.SpaceID,
			TenantID:        req.TenantID,
			KnowledgeBaseID: target.KnowledgeBaseID,
			TargetKind:      kind,
			TargetRef:       target.TargetRef,
			Relation:        types.MemoryRelationAskedAbout,
			Source:          types.MemorySourcePipeline,
			Confidence:      0.8,
			Evidence:        evidence,
		}); err != nil {
			logger.Warnf(ctx, "memory: failed to record anchor for %s: %v", target.TargetRef, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func toRecallItems(pages []*types.MemoryPage, resident bool) []types.MemoryRecallItem {
	out := make([]types.MemoryRecallItem, 0, len(pages))
	for _, page := range pages {
		text := page.InjectionText()
		if text == "" {
			continue
		}
		out = append(out, types.MemoryRecallItem{
			Slug:       page.Slug,
			Title:      page.Title,
			Type:       page.PageType,
			Text:       text,
			Confidence: page.Confidence,
			UpdatedAt:  page.UpdatedAt,
			Resident:   resident,
		})
	}
	return out
}

// mergePreferences folds every preference memory into one effective set.
//
// Later edits win field by field, so a user who once asked for detailed answers
// and later asked for concise ones gets concise, without losing the language
// they set at the very beginning.
func mergePreferences(pages []*types.MemoryPage) types.MemoryPreference {
	ordered := make([]*types.MemoryPage, 0, len(pages))
	for _, page := range pages {
		if page.PageType == types.MemoryTypePreference && !page.Structured.IsZero() {
			ordered = append(ordered, page)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].UpdatedAt.Before(ordered[j].UpdatedAt)
	})

	merged := types.MemoryPreference{}
	for _, page := range ordered {
		merged = merged.Merge(page.Structured)
	}
	return merged.Sanitize()
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}
