package memory

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// Anchor resolution: turning the entity names an extraction guessed at into real
// knowledge-base wiki pages.
//
// This is the step that makes the memory-to-knowledge-base link automatic rather
// than something the user has to assert by hand. Without it the extractor's
// anchor candidates are collected and never used, and the illumination map only
// ever fills in from retrieval citations.
//
// Deliberately deterministic — no model involved. The extractor may propose
// "pgvector"; deciding whether that is the wiki page `entity/pgvector` is a
// lookup, not a judgement, and letting a model make it would introduce a way for
// a memory to be silently attached to the wrong page. Same reasoning, and the
// same first-exact-match-only discipline, as the wiki's own cross-link injection.
type anchorResolver struct {
	wiki    interfaces.WikiPageService
	kbs     interfaces.KnowledgeBaseService
	anchors interfaces.MemoryAnchorRepository
}

func newAnchorResolver(
	wiki interfaces.WikiPageService,
	kbs interfaces.KnowledgeBaseService,
	anchors interfaces.MemoryAnchorRepository,
) *anchorResolver {
	return &anchorResolver{wiki: wiki, kbs: kbs, anchors: anchors}
}

// maxCandidatesPerNote bounds how many names one observation may resolve, so a
// verbose extraction cannot turn into a burst of wiki searches.
const maxCandidatesPerNote = 5

// resolve links a freshly consolidated memory page to wiki pages named by the
// observation behind it.
//
// Failures are logged and skipped. A missing anchor costs a dimmer illumination
// map; a failed consolidation would cost the memory itself.
func (r *anchorResolver) resolve(
	ctx context.Context,
	tenantID uint64,
	spaceID string,
	page *types.MemoryPage,
	note *types.MemoryNote,
	knowledgeBaseIDs []string,
	settings types.MemorySettings,
) {
	if r.wiki == nil || !settings.AnchorResolveEnabled || page == nil || note == nil {
		return
	}
	candidates := note.AnchorCandidates
	if len(candidates) == 0 {
		return
	}
	if len(knowledgeBaseIDs) == 0 {
		return
	}
	if len(candidates) > maxCandidatesPerNote {
		candidates = candidates[:maxCandidatesPerNote]
	}

	for _, kbID := range knowledgeBaseIDs {
		if !r.isWikiEnabled(ctx, kbID) {
			continue
		}
		for _, candidate := range candidates {
			slug := r.matchWikiPage(ctx, kbID, candidate)
			if slug == "" {
				continue
			}
			if err := r.anchors.Upsert(ctx, &types.MemoryAnchorUpsert{
				SpaceID:         spaceID,
				TenantID:        tenantID,
				MemoryPageID:    page.ID,
				KnowledgeBaseID: kbID,
				TargetKind:      types.MemoryAnchorTargetWikiPage,
				TargetRef:       slug,
				// The memory mentions this page; it says nothing about whether
				// the user understood or owns it. Those relations are only ever
				// asserted by the person themselves.
				Relation:   types.MemoryRelationMentioned,
				Source:     types.MemorySourcePipeline,
				Confidence: note.Confidence,
				Evidence: types.MemoryAnchorEvidence{
					MessageIDs: note.SourceMessageIDs,
					SessionIDs: []string{note.SessionID},
				},
			}); err != nil {
				logger.Warnf(ctx, "memory: failed to anchor %q to %s: %v", candidate, slug, err)
			}
		}
	}
}

func (r *anchorResolver) isWikiEnabled(ctx context.Context, kbID string) bool {
	if r.kbs == nil {
		return true
	}
	kb, err := r.kbs.GetKnowledgeBaseByIDOnly(ctx, kbID)
	return err == nil && kb != nil && kb.IsWikiEnabled()
}

// matchWikiPage returns the slug of the wiki page a candidate name refers to, or
// "" when nothing matches confidently enough.
//
// Only exact title, alias or slug matches count. Search is used to narrow the
// candidate set cheaply, never to pick a winner: a fuzzy hit would attach a
// user's memory to a page they never read, which is worse than no anchor at all.
func (r *anchorResolver) matchWikiPage(ctx context.Context, kbID, candidate string) string {
	needle := strings.ToLower(strings.TrimSpace(candidate))
	if needle == "" || len([]rune(needle)) < 2 {
		return ""
	}

	pages, err := r.wiki.SearchPages(ctx, kbID, candidate, 10)
	if err != nil || len(pages) == 0 {
		return ""
	}
	for _, page := range pages {
		if strings.ToLower(strings.TrimSpace(page.Title)) == needle {
			return page.Slug
		}
		if strings.ToLower(strings.TrimSpace(page.Slug)) == needle {
			return page.Slug
		}
		for _, alias := range page.Aliases {
			if strings.ToLower(strings.TrimSpace(alias)) == needle {
				return page.Slug
			}
		}
	}
	return ""
}
