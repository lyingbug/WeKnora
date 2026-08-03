package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeLintIssue(kbID, fingerprint string, seenAt time.Time) *types.WikiPageIssue {
	return &types.WikiPageIssue{
		ID: "issue-" + fingerprint, TenantID: 1, KnowledgeBaseID: kbID,
		PageID: "page-" + fingerprint, Slug: "concept/" + fingerprint,
		IssueType: "broken_link", Fingerprint: fingerprint, Description: "first sighting",
		Source: types.WikiIssueSourceLint, Status: types.WikiIssueStatusOpen,
		RepairMode: types.WikiIssueRepairDeterministic, LastSeenRunID: "run-1",
		LastSeenAt: seenAt, OccurrenceCount: 1, CreatedAt: seenAt, UpdatedAt: seenAt,
	}
}

// TestUpsertLintIssuesBatchMatchesSingleRowSemantics pins the invariant that
// makes batching safe to adopt: a multi-row upsert must resolve conflicts
// exactly like the single-row path, reading each row's payload from its own
// values rather than from whichever row happened to be last.
func TestUpsertLintIssuesBatchMatchesSingleRowSemantics(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-batch"
	seenAt := time.Now()

	first := []*types.WikiPageIssue{
		makeLintIssue(kbID, "fp-a", seenAt),
		makeLintIssue(kbID, "fp-b", seenAt),
		makeLintIssue(kbID, "fp-c", seenAt),
	}
	require.NoError(t, repo.UpsertLintIssues(ctx, first))

	_, total, err := repo.ListIssuesPage(ctx, kbID, "", "", "", "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)

	// Second run sees the same three findings with distinct new descriptions.
	second := make([]*types.WikiPageIssue, 0, 3)
	for i, fp := range []string{"fp-a", "fp-b", "fp-c"} {
		issue := makeLintIssue(kbID, fp, seenAt.Add(time.Minute))
		issue.ID = fmt.Sprintf("issue-second-%d", i)
		issue.Description = "second sighting of " + fp
		issue.LastSeenRunID = "run-2"
		second = append(second, issue)
	}
	require.NoError(t, repo.UpsertLintIssues(ctx, second))

	items, total, err := repo.ListIssuesPage(ctx, kbID, "", "", "", "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(3), total, "re-detection must update, not duplicate")
	for _, item := range items {
		assert.Equal(t, "issue-"+item.Fingerprint, item.ID, "the original row must survive")
		assert.Equal(t, "second sighting of "+item.Fingerprint, item.Description,
			"each row must take its own payload, not a neighbour's")
		assert.Equal(t, 2, item.OccurrenceCount)
		assert.Equal(t, "run-2", item.LastSeenRunID)
	}

	require.NoError(t, repo.UpsertLintIssues(ctx, nil), "an empty batch is a no-op")
}

// TestUpsertLintIssueRevivesSoftDeletedFinding covers the interaction between
// the soft-delete column and the (knowledge_base_id, fingerprint) unique index.
// The index is not partial, so a soft-deleted row still owns its fingerprint —
// without clearing deleted_at the upsert would land on the dead row and the
// finding would stay invisible forever.
func TestUpsertLintIssueRevivesSoftDeletedFinding(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-revive"
	seenAt := time.Now()

	issue := makeLintIssue(kbID, "fp-revive", seenAt)
	require.NoError(t, repo.UpsertLintIssue(ctx, issue))
	require.NoError(t, db.Delete(&types.WikiPageIssue{}, "id = ?", issue.ID).Error)

	_, total, err := repo.ListIssuesPage(ctx, kbID, "", "", "", "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)

	again := makeLintIssue(kbID, "fp-revive", seenAt.Add(time.Minute))
	again.ID = "issue-revived"
	require.NoError(t, repo.UpsertLintIssue(ctx, again))

	items, total, err := repo.ListIssuesPage(ctx, kbID, "", "", "", "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "a re-detected finding must become visible again")
	assert.Equal(t, issue.ID, items[0].ID)
}

// TestExpireStaleRepairAttemptsReleasesHeldIssues covers the recovery path that
// used to live inside the "list active attempts" read. An attempt whose worker
// vanished must free its issue so the user can retry, while a fresh attempt is
// left strictly alone.
func TestExpireStaleRepairAttemptsReleasesHeldIssues(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-reap"
	now := time.Now()

	claim := func(id string, startedAt time.Time) *types.WikiPageIssue {
		issue := makeLintIssue(kbID, id, now)
		require.NoError(t, repo.UpsertLintIssue(ctx, issue))
		attempt := &types.WikiRepairAttempt{
			ID: "attempt-" + id, TenantID: 1, KnowledgeBaseID: kbID, IssueID: issue.ID,
			PageID: issue.PageID, Status: types.WikiIssueStatusRepairing,
			StartedAt: &startedAt, CreatedAt: startedAt, UpdatedAt: startedAt,
		}
		require.NoError(t, repo.ClaimIssueAndCreateAttempt(ctx, issue, attempt))
		return issue
	}

	abandoned := claim("stale", now.Add(-2*time.Hour))
	live := claim("fresh", now)

	retired, err := repo.ExpireStaleRepairAttempts(
		ctx, now.Add(-time.Hour), "expired", now,
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), retired)

	reaped, err := repo.GetIssue(ctx, kbID, abandoned.ID)
	require.NoError(t, err)
	assert.Equal(t, types.WikiIssueStatusFailed, reaped.Status)
	assert.Empty(t, reaped.ActiveAttemptID, "the issue must be retryable again")

	untouched, err := repo.GetIssue(ctx, kbID, live.ID)
	require.NoError(t, err)
	assert.Equal(t, types.WikiIssueStatusRepairing, untouched.Status)
	assert.Equal(t, "attempt-fresh", untouched.ActiveAttemptID)

	remaining, err := repo.ListActiveRepairAttempts(ctx, kbID)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "attempt-fresh", remaining[0].ID)
}

// TestExpireStaleLintRunsFreesTheActiveSlot proves the reaper — not
// CreateLintRun — is what unblocks a KB whose lint worker died. CreateLintRun
// itself must stay a pure write that simply reports the conflict.
func TestExpireStaleLintRunsFreesTheActiveSlot(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-runs"
	now := time.Now()

	abandoned := &types.WikiLintRun{
		ID: "run-abandoned", TenantID: 1, KnowledgeBaseID: kbID,
		Status: "running", CreatedAt: now.Add(-8 * time.Hour), UpdatedAt: now.Add(-8 * time.Hour),
	}
	require.NoError(t, repo.CreateLintRun(ctx, abandoned))

	next := &types.WikiLintRun{
		ID: "run-next", TenantID: 1, KnowledgeBaseID: kbID, Status: "queued",
	}
	assert.ErrorIs(t, repo.CreateLintRun(ctx, next), ErrWikiIssueConflict,
		"a live run holds the slot; starting a run must not sweep it")

	retired, err := repo.ExpireStaleLintRuns(ctx, now.Add(-6*time.Hour), "expired", now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), retired)

	require.NoError(t, repo.CreateLintRun(ctx, next))
	latest, err := repo.GetLatestLintRun(ctx, kbID, "")
	require.NoError(t, err)
	assert.Equal(t, "run-next", latest.ID)

	reaped, err := repo.GetLintRun(ctx, kbID, abandoned.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", reaped.Status)
	assert.Equal(t, "expired", reaped.ErrorMessage)
}

// TestLintRunActiveSlotIsPerScope covers the reason the slot moved from the
// knowledge base to the scope key: a user checking one page must not be told
// the whole wiki is busy, and the latest full-wiki scan must stay reportable
// even after several page checks ran on top of it.
func TestLintRunActiveSlotIsPerScope(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-scoped-runs"

	fullScan := &types.WikiLintRun{
		ID: "run-kb", TenantID: 1, KnowledgeBaseID: kbID, Status: "running",
		Mode: types.WikiLintModeStatic, Scope: types.WikiLintScopeKB, ScopeKey: types.WikiLintScopeKB,
	}
	require.NoError(t, repo.CreateLintRun(ctx, fullScan))

	pageCheck := &types.WikiLintRun{
		ID: "run-page", TenantID: 1, KnowledgeBaseID: kbID, Status: "queued",
		Mode: types.WikiLintModeFull, Scope: types.WikiLintScopePage,
		ScopeKey: "page:concept/rag", TargetSlugs: types.StringArray{"concept/rag"},
	}
	require.NoError(t, repo.CreateLintRun(ctx, pageCheck),
		"a page check must not contend with a full-wiki scan")

	assert.ErrorIs(t, repo.CreateLintRun(ctx, &types.WikiLintRun{
		ID: "run-page-dup", TenantID: 1, KnowledgeBaseID: kbID, Status: "queued",
		Scope: types.WikiLintScopePage, ScopeKey: "page:concept/rag",
	}), ErrWikiIssueConflict, "two checks of the same page still collapse into one")

	latestKB, err := repo.GetLatestLintRun(ctx, kbID, types.WikiLintScopeKB)
	require.NoError(t, err)
	assert.Equal(t, "run-kb", latestKB.ID,
		"a page check must not become the reported state of the last full scan")

	latestPage, err := repo.GetLatestLintRun(ctx, kbID, "page:concept/rag")
	require.NoError(t, err)
	assert.Equal(t, "run-page", latestPage.ID)
}

// TestListPagesPendingReviewSpendsTheBudgetWhereItCanFindSomething exercises the
// candidate query every detector's cost profile depends on: never-reviewed pages
// first, pages a detector has already judged since their last write excluded, and
// the page-type / source-document filters that stop a detector paying for pages
// its defect class cannot apply to.
func TestListPagesPendingReviewSpendsTheBudgetWhereItCanFindSomething(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-pending-review"
	now := time.Now()

	seed := func(slug, pageType string, updatedAt time.Time, sourceRefs types.StringArray) *types.WikiPage {
		page := &types.WikiPage{
			ID: "page-" + slug, TenantID: 1, KnowledgeBaseID: kbID, Slug: slug,
			Title: slug, PageType: pageType, Status: types.WikiPageStatusPublished,
			Version: 1, Content: "body", SourceRefs: sourceRefs,
			CreatedAt: now, UpdatedAt: updatedAt,
		}
		require.NoError(t, repo.Create(ctx, page))
		// Create() stamps its own timestamps, so the ordering column is set
		// explicitly afterwards.
		require.NoError(t, db.Model(&types.WikiPage{}).Where("id = ?", page.ID).
			UpdateColumn("updated_at", updatedAt).Error)
		return page
	}

	const detectorID = "page-content"
	const version = "test-v1"

	oldest := seed("entity/oldest", types.WikiPageTypeEntity, now.Add(-3*time.Hour), types.StringArray{"doc-1"})
	newest := seed("entity/newest", types.WikiPageTypeEntity, now.Add(-time.Minute), nil)
	seed("index", types.WikiPageTypeIndex, now, types.StringArray{"doc-1"})
	summary := seed("summary/doc", types.WikiPageTypeSummary, now.Add(-2*time.Hour), types.StringArray{"doc-2"})

	// The newest page was already judged after its last write, so it must drop
	// out even though it would otherwise sort first.
	require.NoError(t, repo.UpsertReviewLedger(ctx, &types.WikiReviewLedger{
		ID: "ledger-1", TenantID: 1, KnowledgeBaseID: kbID, DetectorID: detectorID,
		UnitKey: newest.ID, UnitHash: "hash-1", ReviewerVersion: version,
		PrimarySlug: newest.Slug, ReviewedAt: now,
	}))

	pages, err := repo.ListPagesPendingReview(ctx, types.WikiPendingReviewQuery{
		KnowledgeBaseID: kbID, DetectorID: detectorID, ReviewerVersion: version, Limit: 10,
	})
	require.NoError(t, err)
	slugs := make([]string, 0, len(pages))
	for _, page := range pages {
		slugs = append(slugs, page.Slug)
	}
	assert.Equal(t, []string{summary.Slug, oldest.Slug}, slugs,
		"the index page is excluded, the already-judged page drops out, and the rest come newest first")

	// A different detector has judged nothing, so the same pages are pending for
	// it — the ledger is per detector, not per page.
	pages, err = repo.ListPagesPendingReview(ctx, types.WikiPendingReviewQuery{
		KnowledgeBaseID: kbID, DetectorID: "duplicate-pages", ReviewerVersion: version,
		PageTypes: []string{types.WikiPageTypeEntity, types.WikiPageTypeConcept}, Limit: 10,
	})
	require.NoError(t, err)
	assert.Len(t, pages, 2, "the page-type filter keeps the summary page out of pair detection")

	// Grounding has nothing to compare against without a source document.
	pages, err = repo.ListPagesPendingReview(ctx, types.WikiPendingReviewQuery{
		KnowledgeBaseID: kbID, DetectorID: "source-grounding", ReviewerVersion: version,
		RequireSourceRefs: true, Limit: 10,
	})
	require.NoError(t, err)
	for _, page := range pages {
		assert.NotEmpty(t, page.SourceRefs, "page %s has no source document to check against", page.Slug)
	}
	assert.Len(t, pages, 2)

	assert.Empty(t, mustPendingReview(t, repo, types.WikiPendingReviewQuery{
		KnowledgeBaseID: kbID, DetectorID: detectorID, ReviewerVersion: version, Limit: 0,
	}), "a zero budget asks for nothing")
}

func mustPendingReview(
	t *testing.T, repo interfaces.WikiPageRepository, query types.WikiPendingReviewQuery,
) []*types.WikiPage {
	t.Helper()
	pages, err := repo.ListPagesPendingReview(context.Background(), query)
	require.NoError(t, err)
	return pages
}

// TestReviewLedgerIsKeyedByDetectorAndUnit covers why the ledger is not keyed by
// page: the review units are not all pages, and two detectors judging the same
// page are two independent questions.
func TestReviewLedgerIsKeyedByDetectorAndUnit(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-ledger"
	now := time.Now()

	write := func(id, detectorID, unitKey, hash string) {
		require.NoError(t, repo.UpsertReviewLedger(ctx, &types.WikiReviewLedger{
			ID: id, TenantID: 1, KnowledgeBaseID: kbID, DetectorID: detectorID,
			UnitKey: unitKey, UnitHash: hash, ReviewerVersion: "v1",
			PrimarySlug: "entity/a", ReviewedAt: now,
		}))
	}
	write("l1", "page-content", "page-a", "hash-a")
	write("l2", "source-grounding", "page-a", "hash-b")
	write("l3", "duplicate-pages", "pair:abcdef", "hash-c")

	entries, err := repo.ListReviewLedger(ctx, kbID, "page-content", []string{"page-a", "pair:abcdef"})
	require.NoError(t, err)
	require.Len(t, entries, 1, "another detector's judgement of the same page is not this one's")
	assert.Equal(t, "hash-a", entries["page-a"].UnitHash)

	// Re-judging the same unit updates in place rather than accumulating rows.
	write("l4", "page-content", "page-a", "hash-a2")
	entries, err = repo.ListReviewLedger(ctx, kbID, "page-content", []string{"page-a"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "hash-a2", entries["page-a"].UnitHash)
	assert.Equal(t, "l1", entries["page-a"].ID, "the original row survives the upsert")

	empty, err := repo.ListReviewLedger(ctx, kbID, "page-content", nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// TestResolveMissingLintIssuesRespectsSourceAndPageScope is the invariant that
// keeps two detector families from erasing each other's findings, and keeps a
// single-page check from closing issues on pages it never read.
func TestResolveMissingLintIssuesRespectsSourceAndPageScope(t *testing.T) {
	db := setupWikiPagesTestDB(t)
	repo := NewWikiPageRepository(db)
	ctx := context.Background()
	kbID := "kb-reconcile-scope"
	seenAt := time.Now()

	seed := func(fingerprint, slug, source string) string {
		issue := makeLintIssue(kbID, fingerprint, seenAt)
		issue.Slug = slug
		issue.Source = source
		require.NoError(t, repo.UpsertLintIssue(ctx, issue))
		return issue.ID
	}
	staticOnPage := seed("fp-static-a", "concept/a", types.WikiIssueSourceLint)
	aiOnPage := seed("fp-ai-a", "concept/a", types.WikiIssueSourceAI)
	staticElsewhere := seed("fp-static-b", "concept/b", types.WikiIssueSourceLint)

	// A page-scoped static run of concept/a reported nothing.
	require.NoError(t, repo.ResolveMissingLintIssues(ctx, types.WikiLintReconcileScope{
		KnowledgeBaseID: kbID, RunID: "run-page-a",
		Sources: []string{types.WikiIssueSourceLint}, Slugs: []string{"concept/a"},
	}, seenAt))

	status := func(id string) string {
		issue, err := repo.GetIssue(ctx, kbID, id)
		require.NoError(t, err)
		return issue.Status
	}
	assert.Equal(t, types.WikiIssueStatusResolved, status(staticOnPage))
	assert.Equal(t, types.WikiIssueStatusOpen, status(aiOnPage),
		"a static run may not close a finding only the AI review can detect")
	assert.Equal(t, types.WikiIssueStatusOpen, status(staticElsewhere),
		"a page-scoped run may only speak for its own pages")

	// An empty (but non-nil) page set means the run covered no pages at all.
	require.NoError(t, repo.ResolveMissingLintIssues(ctx, types.WikiLintReconcileScope{
		KnowledgeBaseID: kbID, RunID: "run-empty",
		Sources: []string{types.WikiIssueSourceAI}, Slugs: []string{},
	}, seenAt))
	assert.Equal(t, types.WikiIssueStatusOpen, status(aiOnPage))
}
