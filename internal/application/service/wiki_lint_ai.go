package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// ErrWikiAIReviewUnavailable means the knowledge base has no usable model for
// the AI review. It is a configuration problem, not a failure of the scan, so
// callers surface it as a prompt to configure a model rather than as an error.
var ErrWikiAIReviewUnavailable = errors.New(
	"wiki AI review model is not configured for this knowledge base",
)

// WikiLintModelID resolves the model the AI review should use. A knowledge base
// may point the review at a cheaper model than the repair agent; when it does
// not, the repair model is reused so enabling the review needs no extra
// configuration step.
func WikiLintModelID(kb *types.KnowledgeBase) string {
	if kb == nil || kb.WikiConfig == nil {
		return ""
	}
	if id := strings.TrimSpace(kb.WikiConfig.LintModelID); id != "" {
		return id
	}
	return strings.TrimSpace(kb.WikiConfig.RepairModelID)
}

// wikiAIPhaseResult is what one AI review phase produced, in the shape
// reconciliation needs.
type wikiAIPhaseResult struct {
	Persisted int
	// QuoteScoped maps a detector id to the pages whose quote-anchored findings
	// this run re-examined in full. Only those pages may have such findings
	// closed by absence.
	QuoteScoped map[string][]string
	// SettledFingerprints are the unit-identified findings whose exact unit this
	// run re-examined. Absence among them is authoritative.
	SettledFingerprints []string
	// Detectors that contributed to the plan, for the run's audit trail.
	Detectors []string
}

// buildReviewEnv resolves everything the detectors need, or reports why the
// review cannot run.
func (s *WikiLintService) buildReviewEnv(
	ctx context.Context, kbID string, pages []*types.WikiPage,
) (*wikiReviewEnv, error) {
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("get KB: %w", err)
	}
	modelID := WikiLintModelID(kb)
	if modelID == "" || s.modelService == nil {
		return nil, ErrWikiAIReviewUnavailable
	}
	model, err := s.modelService.GetChatModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWikiAIReviewUnavailable, err)
	}
	return &wikiReviewEnv{
		KB: kb, Model: model, ModelID: modelID,
		Wiki: s.wikiService, Knowledge: s.knowledgeService,
		Chunks: s.chunkRepo, Repo: s.repo, Pages: pages,
	}, nil
}

// runAIPhase spends the run's model-call budget and persists what came back.
//
// A unit whose review call failed is deliberately left out of the ledger: it is
// retried by the next run rather than being recorded as reviewed and clean. The
// phase itself only fails when nothing could be reviewed at all — one bad page
// must not discard the findings of the units that succeeded.
func (s *WikiLintService) runAIPhase(
	ctx context.Context, payload WikiLintTaskPayload, run *types.WikiLintRun, seenAt time.Time,
) (*wikiAIPhaseResult, error) {
	result := &wikiAIPhaseResult{QuoteScoped: map[string][]string{}}

	pages, err := s.aiScopePages(ctx, payload.KnowledgeBaseID, run)
	if err != nil {
		return result, err
	}
	env, err := s.buildReviewEnv(ctx, payload.KnowledgeBaseID, pages)
	if err != nil {
		return result, err
	}

	detectors := enabledWikiReviewDetectors(env.KB)
	runner := &wikiReviewRunner{repo: s.repo}
	// A page-scoped run is an explicit request for a fresh opinion on named
	// pages, so it bypasses the unchanged-unit ledger; a wiki-wide run relies on
	// the ledger to stay affordable.
	force := env.scopedToPages()

	plan, err := runner.plan(ctx, env, detectors, wikiReviewBudget(env.KB), force)
	if err != nil {
		return result, err
	}
	result.Detectors = plan.detectorIDs
	run.AIDetectors = types.StringArray(plan.detectorIDs)
	run.AIUnitsSkipped += plan.skipped
	if len(plan.units) == 0 {
		run.Progress = 95
		_ = s.repo.UpdateLintRun(ctx, run)
		return result, nil
	}

	detectorByID := make(map[string]wikiReviewDetector, len(detectors))
	for _, detector := range detectors {
		detectorByID[detector.ID()] = detector
	}

	completed := 0
	var firstErr error
	runner.execute(ctx, env, plan, func(outcome wikiReviewOutcome) {
		completed++
		run.Progress = 40 + completed*55/len(plan.units)
		run.AICalls++

		if outcome.Err != nil {
			if firstErr == nil {
				firstErr = outcome.Err
			}
			logger.Warnf(ctx, "wiki review: detector %s unit %s failed: %v",
				outcome.DetectorID, outcome.Candidate.Key, outcome.Err)
			_ = s.repo.UpdateLintRun(ctx, run)
			return
		}
		run.AIUnitsReviewed++
		run.AIFindingCount += len(outcome.Findings)

		detector := detectorByID[outcome.DetectorID]
		if !s.commitReviewOutcome(ctx, payload, run, env, detector, outcome, seenAt, result) {
			if firstErr == nil {
				firstErr = errors.New("failed to persist review findings")
			}
			return
		}
		_ = s.repo.UpdateLintRun(ctx, run)
	})

	if run.AIUnitsReviewed == 0 && firstErr != nil {
		return result, fmt.Errorf("wiki AI review failed for every unit: %w", firstErr)
	}
	return result, nil
}

// commitReviewOutcome persists one unit's findings, records the ledger entry,
// and registers what the unit is now authoritative to close. Reports false when
// the findings could not be stored.
//
// The ledger is written only after the findings are durable, so a crash between
// the two re-reviews the unit instead of losing its findings.
func (s *WikiLintService) commitReviewOutcome(
	ctx context.Context, payload WikiLintTaskPayload, run *types.WikiLintRun,
	env *wikiReviewEnv, detector wikiReviewDetector, outcome wikiReviewOutcome,
	seenAt time.Time, result *wikiAIPhaseResult,
) bool {
	page := outcome.Candidate.primary()
	if page == nil || detector == nil {
		return true
	}
	if len(outcome.Findings) > 0 {
		records := make([]*types.WikiPageIssue, 0, len(outcome.Findings))
		for _, finding := range outcome.Findings {
			records = append(records, wikiReviewIssueRecord(
				payload.TenantID, payload.KnowledgeBaseID, run.ID, env.ModelID,
				outcome.DetectorID, seenAt, page, finding,
			))
		}
		if err := s.repo.UpsertLintIssues(ctx, records); err != nil {
			logger.Warnf(ctx, "wiki review: persist findings for %s failed: %v", page.Slug, err)
			return false
		}
		result.Persisted += len(records)
	}

	if err := s.repo.UpsertReviewLedger(ctx, &types.WikiReviewLedger{
		ID: uuid.New().String(), TenantID: payload.TenantID,
		KnowledgeBaseID: payload.KnowledgeBaseID, DetectorID: outcome.DetectorID,
		UnitKey: outcome.Candidate.Key, UnitHash: outcome.Candidate.Hash,
		ReviewerVersion: wikiReviewerVersion, PrimarySlug: page.Slug,
		FindingCount: len(outcome.Findings), RunID: run.ID, ModelID: env.ModelID,
		ReviewedAt: time.Now(),
	}); err != nil {
		logger.Warnf(ctx, "wiki review: ledger write for %s failed: %v", page.Slug, err)
	}

	identity := detector.Identity()
	if len(identity.QuoteAnchored) > 0 {
		result.QuoteScoped[outcome.DetectorID] = append(
			result.QuoteScoped[outcome.DetectorID], page.Slug,
		)
	}
	result.SettledFingerprints = append(
		result.SettledFingerprints,
		detector.UnitFingerprints(payload.KnowledgeBaseID, outcome.Candidate)...,
	)
	return true
}

// aiScopePages loads the pages a page-scoped run is confined to. A wiki-wide run
// returns nothing here and lets each detector choose its own candidates.
func (s *WikiLintService) aiScopePages(
	ctx context.Context, kbID string, run *types.WikiLintRun,
) ([]*types.WikiPage, error) {
	if len(run.TargetSlugs) == 0 {
		return nil, nil
	}
	pages := make([]*types.WikiPage, 0, len(run.TargetSlugs))
	for _, slug := range run.TargetSlugs {
		page, err := s.wikiService.GetPageBySlug(ctx, kbID, slug)
		if err != nil {
			return nil, fmt.Errorf("load page %s: %w", slug, err)
		}
		pages = append(pages, page)
	}
	return pages, nil
}

// reconcileAIFindings closes the AI findings this run is entitled to close.
//
// The scope is deliberately narrow on both axes. A run only spends a bounded
// budget, so it has looked at a small slice of the wiki; closing anything outside
// that slice would silently discard findings nobody re-examined. Quote-anchored
// types are closed over the pages their detector actually read, and
// unit-identified types only by the exact unit that owns them.
func (s *WikiLintService) reconcileAIFindings(
	ctx context.Context, kbID, runID string, phase *wikiAIPhaseResult, seenAt time.Time,
) error {
	for _, detectorID := range phase.Detectors {
		slugs := phase.QuoteScoped[detectorID]
		if len(slugs) == 0 {
			continue
		}
		detector := wikiReviewDetectorByID(detectorID)
		if detector == nil {
			continue
		}
		if err := s.repo.ResolveMissingLintIssues(ctx, types.WikiLintReconcileScope{
			KnowledgeBaseID: kbID,
			RunID:           runID,
			Sources:         []string{types.WikiIssueSourceAI},
			IssueTypes:      detector.Identity().QuoteAnchored,
			Slugs:           slugs,
		}, seenAt); err != nil {
			return fmt.Errorf("reconcile %s findings: %w", detectorID, err)
		}
	}
	if len(phase.SettledFingerprints) > 0 {
		if err := s.repo.ResolveReviewedUnitIssues(
			ctx, kbID, runID, phase.SettledFingerprints, seenAt,
		); err != nil {
			return fmt.Errorf("reconcile reviewed review units: %w", err)
		}
	}
	return nil
}

// wikiReviewDetectorByID looks a detector up in the registry.
func wikiReviewDetectorByID(id string) wikiReviewDetector {
	for _, detector := range wikiReviewDetectors() {
		if detector.ID() == id {
			return detector
		}
	}
	return nil
}

// AIReviewAvailable reports whether the knowledge base can run an AI review,
// returning ErrWikiAIReviewUnavailable with the reason when it cannot.
func (s *WikiLintService) AIReviewAvailable(ctx context.Context, kbID string) error {
	if s.modelService == nil {
		return ErrWikiAIReviewUnavailable
	}
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, kbID)
	if err != nil {
		return err
	}
	if WikiLintModelID(kb) == "" {
		return ErrWikiAIReviewUnavailable
	}
	return nil
}

// BindWikiAIRecheck connects the AI reviewer's single-page recheck to the wiki
// page service's repair verification.
//
// It is a post-construction step because the reviewer is built on top of the page
// service: wiring it through a constructor argument would make the two mutually
// dependent. When no review model is available the recheck simply reports that,
// and AI findings fall back to their deterministic postcondition.
func BindWikiAIRecheck(wikiService interfaces.WikiPageService, lint *WikiLintService) {
	if lint == nil || lint.modelService == nil {
		return
	}
	pageService, ok := wikiService.(*wikiPageService)
	if !ok {
		return
	}
	pageService.SetAIIssueRechecker(lint.RecheckAIIssue)
}

// RecheckAIIssue re-reviews the unit an AI finding came from and reports whether
// an equivalent finding is still present.
//
// This is the last link in the repair loop, and it is deliberately the only place
// a repair spends a model call: it runs once, on one unit, and only when the
// issue's cheap deterministic postcondition could not settle the question.
// Equivalence is decided by fingerprint, so "still present" means the reviewer
// produced the same finding about the same thing — not merely that it found
// something.
func (s *WikiLintService) RecheckAIIssue(
	ctx context.Context, issue *types.WikiPageIssue, page *types.WikiPage,
) (bool, error) {
	if issue == nil || page == nil {
		return false, ErrWikiAIReviewUnavailable
	}
	detector := wikiReviewDetectorByID(wikiIssueDetectorID(issue))
	if detector == nil {
		return false, fmt.Errorf("no wiki review detector can re-check issue type %s", issue.IssueType)
	}
	env, err := s.buildReviewEnv(ctx, issue.KnowledgeBaseID, []*types.WikiPage{page})
	if err != nil {
		return false, err
	}
	// Ask the detector for the unit that involves this page, so a pair finding is
	// re-checked as a pair rather than as a lone page.
	candidates, err := detector.Candidates(ctx, env, 1)
	if err != nil || len(candidates) == 0 {
		return false, fmt.Errorf("wiki review recheck found no unit for issue %s", issue.ID)
	}
	callCtx, cancel := context.WithTimeout(ctx, wikiReviewCallTimeout)
	defer cancel()
	findings, err := detector.Review(callCtx, env, candidates[0])
	if err != nil {
		return false, err
	}
	for _, finding := range findings {
		record := wikiReviewIssueRecord(
			issue.TenantID, issue.KnowledgeBaseID, "", env.ModelID,
			detector.ID(), time.Now(), page, finding,
		)
		if record.Fingerprint == issue.Fingerprint {
			return true, nil
		}
	}
	return false, nil
}

// wikiIssueDetectorID recovers which detector reported an issue. The id is
// recorded in evidence, and falls back to the detector that owns the issue type
// so findings written before the id existed can still be re-checked.
func wikiIssueDetectorID(issue *types.WikiPageIssue) string {
	evidence := wikiIssueEvidenceMap(issue)
	if id, ok := evidence["detector_id"].(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	for _, detector := range wikiReviewDetectors() {
		for _, issueType := range detector.IssueTypes() {
			if issueType == issue.IssueType {
				return detector.ID()
			}
		}
	}
	return ""
}
