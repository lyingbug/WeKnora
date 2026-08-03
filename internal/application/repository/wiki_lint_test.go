package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
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
