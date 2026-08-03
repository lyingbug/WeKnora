package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMergeTestService(t *testing.T) (interfaces.WikiPageService, interfaces.WikiPageRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WikiFolder{}, &types.WikiPage{}, &types.WikiPageRevision{}))
	repo := repository.NewWikiPageRepository(db)
	return NewWikiPageService(repo, nil, nil, nil, nil), repo
}

// TestMergePagesTransfersProvenanceNotJustContent is the reason this operation
// exists rather than "write one page, delete the other". The absorbed page's
// aliases, source documents and citations are what let the ingest pipeline
// recognise the subject as already covered; losing them would recreate the
// duplicate on the next ingest of the same documents.
func TestMergePagesTransfersProvenanceNotJustContent(t *testing.T) {
	svc, repo := newMergeTestService(t)
	ctx := context.Background()
	const kbID = "kb-merge"
	now := time.Now()

	seed := func(slug, title string, page *types.WikiPage) *types.WikiPage {
		page.ID, page.TenantID, page.KnowledgeBaseID = uuid.New().String(), 1, kbID
		page.Slug, page.Title = slug, title
		page.PageType, page.Status, page.Version = types.WikiPageTypeEntity, types.WikiPageStatusPublished, 1
		page.CreatedAt, page.UpdatedAt = now, now
		require.NoError(t, repo.Create(ctx, page))
		return page
	}
	target := seed("entity/acme-widget", "Acme Widget", &types.WikiPage{
		Content:    "Acme Widget is a product.",
		Aliases:    types.StringArray{"Widget"},
		SourceRefs: types.StringArray{"doc-1"},
		ChunkRefs:  types.StringArray{"chunk-1"},
	})
	seed("entity/acme-widget-pro", "Acme Widget Pro", &types.WikiPage{
		Content:    "Acme Widget Pro is the same product under a newer name.",
		Aliases:    types.StringArray{"Widget Pro", "Widget"},
		SourceRefs: types.StringArray{"doc-2"},
		ChunkRefs:  types.StringArray{"chunk-2", "chunk-1"},
	})

	merged, err := svc.MergePages(ctx, types.WikiPageMergeRequest{
		KnowledgeBaseID: kbID, TargetSlug: target.Slug, SourceSlug: "entity/acme-widget-pro",
		Content: "Acme Widget, also sold as Acme Widget Pro, is a product.", Summary: "One product, two names.",
	})
	require.NoError(t, err)

	assert.Equal(t, "One product, two names.", merged.Summary)
	assert.Equal(t, 2, merged.Version, "a merge is a user-visible edit and must advance the version")
	assert.ElementsMatch(t, []string{"doc-1", "doc-2"}, merged.SourceRefs)
	assert.ElementsMatch(t, []string{"chunk-1", "chunk-2"}, merged.ChunkRefs,
		"citations are unioned, not duplicated")
	assert.ElementsMatch(t, []string{"Widget", "Widget Pro", "Acme Widget Pro"}, merged.Aliases,
		"the absorbed title becomes an alias so the name readers know still resolves")

	_, err = repo.GetBySlug(ctx, kbID, "entity/acme-widget-pro")
	assert.ErrorIs(t, err, repository.ErrWikiPageNotFound)
}

// TestMergePagesKeepsTheTargetTitleOutOfItsOwnAliases guards a small but visible
// detail: a page listing its own title as an alias reads as a bug in the UI.
func TestMergePagesKeepsTheTargetTitleOutOfItsOwnAliases(t *testing.T) {
	target := &types.WikiPage{Title: "Acme Widget", Aliases: types.StringArray{"Widget"}}
	source := &types.WikiPage{Title: "acme widget", Aliases: types.StringArray{"Acme Widget", "AW"}}
	assert.ElementsMatch(t, []string{"Widget", "AW"}, mergeWikiAliases(target, source))
}

// TestMergePagesRefusesWhatCannotBeUndone covers the guards on an irreversible
// operation. Deriving the merged content by concatenation would produce a page no
// one wrote, and merging the index page away would remove the wiki's entry point.
func TestMergePagesRefusesWhatCannotBeUndone(t *testing.T) {
	svc, repo := newMergeTestService(t)
	ctx := context.Background()
	const kbID = "kb-merge-guard"
	now := time.Now()

	require.NoError(t, repo.Create(ctx, &types.WikiPage{
		ID: uuid.New().String(), TenantID: 1, KnowledgeBaseID: kbID,
		Slug: "index", Title: "Index", PageType: types.WikiPageTypeIndex,
		Status: types.WikiPageStatusPublished, Version: 1, Content: "wiki index",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.Create(ctx, &types.WikiPage{
		ID: uuid.New().String(), TenantID: 1, KnowledgeBaseID: kbID,
		Slug: "entity/acme", Title: "Acme", PageType: types.WikiPageTypeEntity,
		Status: types.WikiPageStatusPublished, Version: 1, Content: "Acme is a company.",
		CreatedAt: now, UpdatedAt: now,
	}))

	_, err := svc.MergePages(ctx, types.WikiPageMergeRequest{
		KnowledgeBaseID: kbID, TargetSlug: "entity/acme", SourceSlug: "index", Content: "merged",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index page cannot take part")

	_, err = svc.MergePages(ctx, types.WikiPageMergeRequest{
		KnowledgeBaseID: kbID, TargetSlug: "entity/acme", SourceSlug: "entity/other", Content: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merged content is required")

	_, err = svc.MergePages(ctx, types.WikiPageMergeRequest{
		KnowledgeBaseID: kbID, TargetSlug: "entity/acme", SourceSlug: "entity/acme", Content: "merged",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merged into itself")

	// The index page must still be intact after every refusal.
	index, err := repo.GetBySlug(ctx, kbID, "index")
	require.NoError(t, err)
	assert.Equal(t, 1, index.Version)
}

// TestPairKeyIsOrderIndependent is what collapses the two directions of one
// comparison into a single unit, a single issue, and a single ledger entry.
func TestPairKeyIsOrderIndependent(t *testing.T) {
	forward := wikiPairKey("entity/a", "entity/b")
	assert.Equal(t, forward, wikiPairKey("entity/b", "entity/a"))
	assert.NotEqual(t, forward, wikiPairKey("entity/a", "entity/c"))
	assert.LessOrEqual(t, len(forward), 64, "the key must fit the ledger's column")
}

// TestPairIsReviewableSkipsPairsNotWorthACall pins the filters that keep the most
// speculative detector from spending its budget on pairs a human has already
// settled, or on pages there is nothing to compare.
func TestPairIsReviewableSkipsPairsNotWorthACall(t *testing.T) {
	detector := wikiDuplicatePagesDetector{}
	body := "This body is comfortably longer than the empty-content threshold of fifty runes."
	page := func(id, slug string) *types.WikiPage {
		return &types.WikiPage{
			ID: id, Slug: slug, PageType: types.WikiPageTypeEntity,
			Status: types.WikiPageStatusPublished, Content: body,
		}
	}

	a, b := page("a", "entity/a"), page("b", "entity/b")
	assert.True(t, detector.pairIsReviewable(a, b))

	assert.False(t, detector.pairIsReviewable(a, a), "a page is never its own duplicate")
	assert.False(t, detector.pairIsReviewable(a, nil))

	linked := page("b", "entity/b")
	linked.OutLinks = types.StringArray{"entity/a"}
	assert.False(t, detector.pairIsReviewable(a, linked),
		"an editor who linked the pages has decided they are distinct")

	thin := page("b", "entity/b")
	thin.Content = "too short"
	assert.False(t, detector.pairIsReviewable(a, thin),
		"the static empty-content rule owns an effectively empty page")

	summary := page("b", "summary/doc")
	summary.PageType = types.WikiPageTypeSummary
	assert.False(t, detector.pairIsReviewable(a, summary),
		"summary pages are per-document by construction and are never duplicates")

	archived := page("b", "entity/b")
	archived.Status = types.WikiPageStatusArchived
	assert.False(t, detector.pairIsReviewable(a, archived))
}

// TestPrimarySourceKnowledgeIDToleratesLegacyRefs keeps the grounding detector
// working on rows written before source_refs dropped the "id|title" form.
func TestPrimarySourceKnowledgeIDToleratesLegacyRefs(t *testing.T) {
	assert.Equal(t, "doc-1", wikiPrimarySourceKnowledgeID(&types.WikiPage{
		SourceRefs: types.StringArray{"doc-1"},
	}))
	assert.Equal(t, "doc-1", wikiPrimarySourceKnowledgeID(&types.WikiPage{
		SourceRefs: types.StringArray{"doc-1|Some Document.pdf"},
	}))
	assert.Equal(t, "doc-2", wikiPrimarySourceKnowledgeID(&types.WikiPage{
		SourceRefs: types.StringArray{"  ", "doc-2"},
	}))
	assert.Empty(t, wikiPrimarySourceKnowledgeID(&types.WikiPage{}))
}
