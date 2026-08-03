package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// The AI health review is organised around one observation: a wiki defect is
// only findable by a reviewer that can see the right thing at once.
//
//	"this page mixes two products"        -> one page body
//	"this summary omits half its source"  -> a page AND the document behind it
//	"these two pages are the same thing"  -> two pages side by side
//
// A reviewer that only ever reads one page therefore cannot find the second or
// third class at all, no matter how good the model is. So the review is a set of
// detectors, each declaring the unit it judges, rather than a single per-page
// pass.
//
// Every detector has the same two-stage shape, because that is what keeps the
// cost bounded while still covering a large wiki:
//
//	1. Candidates — cheap, database-only work that proposes units worth looking
//	   at. Never a model call. This is where a 40k-page wiki is reduced to a
//	   handful of units, and where each detector's domain knowledge lives (which
//	   pages changed, which pages share a source document, which titles are
//	   near-identical).
//	2. Review — at most one bounded model call per unit, returning findings that
//	   must quote the page verbatim.
//
// A run's call budget is shared across the detectors, so adding a detector
// changes what a run looks at, not how much it costs.

// wikiReviewerVersion identifies the detector set and their prompts. It is part
// of the ledger key, so changing a prompt invalidates prior judgements rather
// than silently mixing two generations of findings.
const wikiReviewerVersion = "2026-08-v2"

const (
	// wikiReviewDefaultBudget is the per-run model-call budget when the
	// knowledge base does not override it. One call per unit, shared out
	// across the enabled detectors.
	wikiReviewDefaultBudget = 24
	// wikiReviewHardBudget bounds what an operator may configure, so a typo in
	// the knowledge base settings cannot turn one scan into thousands of calls.
	wikiReviewHardBudget = 240
	// wikiReviewConcurrency is how many units are reviewed in parallel. Kept
	// low because wiki ingest competes for the same provider quota.
	wikiReviewConcurrency = 2
	// wikiReviewCallTimeout bounds one unit so a hung provider cannot hold the
	// run's slot until the stale-run reaper picks it up.
	wikiReviewCallTimeout = 90 * time.Second
	// wikiReviewMaxCompletionTokens bounds an answer. The contract is a short
	// JSON array of short objects; anything longer is a malformed answer.
	wikiReviewMaxCompletionTokens = 900
	// wikiReviewMinConfidence drops the reviewer's own low-confidence guesses
	// before they reach a human. A speculative finding costs more attention
	// than it saves.
	wikiReviewMinConfidence = 0.6
	// wikiReviewMaxFindingsPerUnit keeps one bad page from flooding the problem
	// centre, and caps the completion length we have to pay for.
	wikiReviewMaxFindingsPerUnit = 3
	// wikiReviewEvidenceRunes caps the quoted span stored on a finding.
	wikiReviewEvidenceRunes = 300
	// wikiReviewOverFetch is how many candidates a detector is asked for
	// relative to its budget share. Candidate generation is cheap database
	// work, so over-fetching lets the runner drop units the ledger already
	// answers without the detector having to know about the ledger.
	wikiReviewOverFetch = 4
)

// wikiReviewEnv is everything a detector may reach. Detectors receive it rather
// than holding their own dependencies so a detector stays a pure description of
// one defect class.
type wikiReviewEnv struct {
	KB      *types.KnowledgeBase
	Model   chat.Chat
	ModelID string

	Wiki      interfaces.WikiPageService
	Knowledge interfaces.KnowledgeService
	Chunks    interfaces.ChunkRepository
	Repo      interfaces.WikiPageRepository

	// Pages, when non-empty, confines the run to units involving these pages.
	// A single-page check is the same detectors over a one-page world, not a
	// separate implementation.
	Pages []*types.WikiPage
}

// scopedToPages reports whether this run is a page-scoped check.
func (e *wikiReviewEnv) scopedToPages() bool { return len(e.Pages) > 0 }

// wikiReviewCandidate is one unit a detector proposes to spend a call on.
type wikiReviewCandidate struct {
	// Key identifies the unit within the detector, and Hash covers the inputs
	// the judgement depends on. Together they are the ledger entry that makes a
	// repeat run of an unchanged wiki nearly free.
	Key  string
	Hash string
	// Pages carries the pages the unit is about. Pages[0] is the page findings
	// are attributed to, so a pair candidate must order its pages canonically.
	Pages []*types.WikiPage
}

// primary returns the page findings are attributed to.
func (c wikiReviewCandidate) primary() *types.WikiPage {
	if len(c.Pages) == 0 {
		return nil
	}
	return c.Pages[0]
}

// wikiReviewFinding is one defect a detector reported.
type wikiReviewFinding struct {
	IssueType  string  `json:"issue_type"`
	Severity   string  `json:"severity"`
	Evidence   string  `json:"evidence"`
	Problem    string  `json:"problem"`
	Suggestion string  `json:"suggestion"`
	Confidence float64 `json:"confidence"`
	// Extra is merged into the issue's evidence JSON. Detectors use it to carry
	// the facts a postcondition needs later — the paired slug for a duplicate,
	// the source document for a grounding finding, the coverage numbers for an
	// incomplete summary.
	Extra map[string]interface{} `json:"-"`
	// fingerprintKey overrides the default evidence-derived identity. A finding
	// whose identity is not a quoted span (a page pair, for instance) sets it so
	// re-detection updates the same issue instead of creating a near-duplicate.
	fingerprintKey string
}

// wikiReviewDetector describes one defect class: the unit it judges, how to
// find units worth judging, and how to judge one.
type wikiReviewDetector interface {
	// ID is stable and appears in the ledger, so renaming one invalidates its
	// prior judgements.
	ID() string
	// IssueTypes are the types this detector may report. Reconciliation is
	// scoped by them, so a run may only retire findings of the detectors it ran.
	IssueTypes() []string
	// Weight is this detector's share of the run's call budget, relative to the
	// other enabled detectors.
	Weight() int
	// Candidates proposes at most limit units using only database work.
	Candidates(ctx context.Context, env *wikiReviewEnv, limit int) ([]wikiReviewCandidate, error)
	// Review spends at most one model call on one unit.
	Review(ctx context.Context, env *wikiReviewEnv, candidate wikiReviewCandidate) ([]wikiReviewFinding, error)
	// Identity partitions IssueTypes by how a finding is identified.
	Identity() wikiFindingIdentity
	// UnitFingerprints returns the fingerprints of the unit-identified findings
	// this unit is authoritative for — the ones a review of it either re-reports
	// or retires. Detectors with no unit-identified types return nothing.
	UnitFingerprints(kbID string, candidate wikiReviewCandidate) []string
}

// wikiFindingIdentity says how a detector's findings are identified, which is
// what decides whether absence may close them.
//
// This distinction is a soundness condition, not a detail. A quote-anchored
// finding is identified by a verbatim span on one page, so re-reviewing that page
// re-examines every such finding on it and absence over the reviewed pages is
// safe. A unit-identified finding belongs to a unit that is not a page — a pair
// of pages, or a page measured against its source — and only a review of that
// exact unit can retire it. Closing those by page would let a review of the pair
// (A, C) silently resolve a finding about (A, B).
type wikiFindingIdentity struct {
	QuoteAnchored  []string
	UnitIdentified []string
}

// wikiReviewDetectors is the registry, in the order the budget is handed out.
//
// Order matters: each detector takes its share and releases what it cannot use
// to the detectors after it, so the cheapest and most broadly applicable
// detector comes first and the most speculative one last. A wiki with no
// duplicate pages then spends its whole budget on the defects it does have.
func wikiReviewDetectors() []wikiReviewDetector {
	return []wikiReviewDetector{
		wikiPageContentDetector{},
		wikiSourceGroundingDetector{},
		wikiDuplicatePagesDetector{},
	}
}

// enabledWikiReviewDetectors applies the knowledge base's detector allow-list.
// An unknown id is ignored rather than failing the run, so removing a detector
// from the code does not break a knowledge base that still names it.
func enabledWikiReviewDetectors(kb *types.KnowledgeBase) []wikiReviewDetector {
	all := wikiReviewDetectors()
	if kb == nil || kb.WikiConfig == nil || len(kb.WikiConfig.LintAIDetectors) == 0 {
		return all
	}
	allowed := make(map[string]struct{}, len(kb.WikiConfig.LintAIDetectors))
	for _, id := range kb.WikiConfig.LintAIDetectors {
		allowed[strings.TrimSpace(id)] = struct{}{}
	}
	out := make([]wikiReviewDetector, 0, len(all))
	for _, detector := range all {
		if _, ok := allowed[detector.ID()]; ok {
			out = append(out, detector)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// wikiReviewBudget resolves a knowledge base's per-run call budget, clamped to
// the hard cap.
func wikiReviewBudget(kb *types.KnowledgeBase) int {
	budget := wikiReviewDefaultBudget
	if kb != nil && kb.WikiConfig != nil && kb.WikiConfig.LintAIMaxPages > 0 {
		budget = kb.WikiConfig.LintAIMaxPages
	}
	if budget > wikiReviewHardBudget {
		return wikiReviewHardBudget
	}
	if budget < 1 {
		return 1
	}
	return budget
}

// wikiReviewShares splits a budget across detectors by weight, guaranteeing
// every detector at least one call while the budget lasts.
//
// The guarantee matters more than the proportions: a detector that never gets a
// single call is a defect class the product silently does not cover, and that is
// not something a weight rounding decision should decide.
func wikiReviewShares(total int, detectors []wikiReviewDetector) []int {
	shares := make([]int, len(detectors))
	if len(detectors) == 0 || total <= 0 {
		return shares
	}
	totalWeight := 0
	for _, detector := range detectors {
		totalWeight += detector.Weight()
	}
	if totalWeight <= 0 {
		totalWeight = len(detectors)
	}
	assigned := 0
	for i, detector := range detectors {
		share := total * detector.Weight() / totalWeight
		if share < 1 && total > i {
			share = 1
		}
		shares[i] = share
		assigned += share
	}
	// Rounding leaves a remainder; give it to the first detector, which is the
	// broadest one. Over-assignment (many detectors, tiny budget) is trimmed
	// from the back so the front keeps its guaranteed call.
	for assigned > total {
		for i := len(shares) - 1; i >= 0 && assigned > total; i-- {
			if shares[i] > 0 {
				shares[i]--
				assigned--
			}
		}
	}
	if assigned < total {
		shares[0] += total - assigned
	}
	return shares
}

// wikiReviewOutcome is the result of one reviewed unit, handed to the caller as
// it lands so findings can be persisted incrementally.
type wikiReviewOutcome struct {
	DetectorID string
	Candidate  wikiReviewCandidate
	Findings   []wikiReviewFinding
	// Skipped means the unit was answered from the ledger and cost nothing.
	Skipped bool
	Err     error
}

// wikiReviewPlan is what a run decided to look at, before any model call.
type wikiReviewPlan struct {
	units   []plannedWikiReviewUnit
	skipped int
	// detectorIDs are the detectors that actually contributed to the plan, in
	// registry order. Reconciliation is scoped to their issue types.
	detectorIDs []string
}

type plannedWikiReviewUnit struct {
	detector  wikiReviewDetector
	candidate wikiReviewCandidate
}

// wikiReviewRunner executes a review plan.
type wikiReviewRunner struct {
	repo interfaces.WikiPageRepository
}

// plan decides which units this run will spend its budget on.
//
// Candidate generation is over-fetched and then filtered against the ledger
// here, in one place, rather than in each detector: a detector should describe
// a defect class, not re-implement caching. Units the ledger already answers
// are counted as skipped and cost nothing.
func (r *wikiReviewRunner) plan(
	ctx context.Context, env *wikiReviewEnv, detectors []wikiReviewDetector, budget int, force bool,
) (*wikiReviewPlan, error) {
	plan := &wikiReviewPlan{}
	shares := wikiReviewShares(budget, detectors)
	remaining := budget
	carry := 0

	for i, detector := range detectors {
		plan.detectorIDs = append(plan.detectorIDs, detector.ID())
		limit := shares[i] + carry
		if limit > remaining {
			limit = remaining
		}
		if limit <= 0 {
			continue
		}

		candidates, err := detector.Candidates(ctx, env, limit*wikiReviewOverFetch)
		if err != nil {
			// A detector that cannot generate candidates — a dialect without
			// trigram search, a transient query failure — must not fail the
			// run. It contributes nothing this time and releases its share.
			logger.Warnf(ctx, "wiki review: detector %s candidate generation failed: %v",
				detector.ID(), err)
			carry = limit
			continue
		}

		accepted, skipped, err := r.filterReviewed(ctx, env, detector, candidates, limit, force)
		if err != nil {
			return nil, err
		}
		plan.skipped += skipped
		for _, candidate := range accepted {
			plan.units = append(plan.units, plannedWikiReviewUnit{detector: detector, candidate: candidate})
		}
		remaining -= len(accepted)
		carry = limit - len(accepted)
		if carry < 0 {
			carry = 0
		}
	}
	return plan, nil
}

// filterReviewed keeps the first `limit` candidates the ledger cannot answer.
func (r *wikiReviewRunner) filterReviewed(
	ctx context.Context, env *wikiReviewEnv, detector wikiReviewDetector,
	candidates []wikiReviewCandidate, limit int, force bool,
) (accepted []wikiReviewCandidate, skipped int, err error) {
	if len(candidates) == 0 {
		return nil, 0, nil
	}
	ledger := map[string]*types.WikiReviewLedger{}
	if !force {
		keys := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			keys = append(keys, candidate.Key)
		}
		// A ledger read failure costs money, not correctness: every unit is
		// then treated as unreviewed.
		existing, lookupErr := r.repo.ListReviewLedger(ctx, env.KB.ID, detector.ID(), keys)
		if lookupErr != nil {
			logger.Warnf(ctx, "wiki review: ledger lookup for %s failed, reviewing all units: %v",
				detector.ID(), lookupErr)
		} else {
			ledger = existing
		}
	}
	for _, candidate := range candidates {
		if len(accepted) >= limit {
			break
		}
		entry := ledger[candidate.Key]
		if entry != nil && entry.ReviewerVersion == wikiReviewerVersion && entry.UnitHash == candidate.Hash {
			skipped++
			continue
		}
		accepted = append(accepted, candidate)
	}
	return accepted, skipped, nil
}

// execute reviews the planned units with bounded parallelism, handing each
// outcome to onDone as it lands.
//
// Each call is independently timed out and its failure is reported per unit, so
// one page that fails or one provider hiccup costs that unit's finding rather
// than the whole run. A partial review is useful; a discarded one is not.
func (r *wikiReviewRunner) execute(
	ctx context.Context, env *wikiReviewEnv, plan *wikiReviewPlan, onDone func(wikiReviewOutcome),
) {
	if plan == nil || len(plan.units) == 0 {
		return
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, wikiReviewConcurrency)

	for _, unit := range plan.units {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(unit plannedWikiReviewUnit) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			callCtx, cancel := context.WithTimeout(ctx, wikiReviewCallTimeout)
			findings, err := unit.detector.Review(callCtx, env, unit.candidate)
			cancel()

			mu.Lock()
			defer mu.Unlock()
			onDone(wikiReviewOutcome{
				DetectorID: unit.detector.ID(),
				Candidate:  unit.candidate,
				Findings:   findings,
				Err:        err,
			})
		}(unit)
	}
	wg.Wait()
}

// reviewWithModel is the single place a detector's prompt becomes a model call.
//
// Keeping it shared means every detector inherits the same bounds — zero
// temperature, a capped completion, no tools, no streaming — so a new detector
// cannot accidentally introduce an expensive call shape.
func reviewWithModel(
	ctx context.Context, env *wikiReviewEnv, systemPrompt, userPrompt string,
) (string, error) {
	thinking := false
	response, err := env.Model.Chat(ctx, []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{
		Temperature:         0,
		MaxCompletionTokens: wikiReviewMaxCompletionTokens,
		Thinking:            &thinking,
	})
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", fmt.Errorf("wiki review returned no response")
	}
	return response.Content, nil
}

// wikiFindingSpec is the contract a detector holds its own model answer to.
//
// QuoteRequired is a subset of AllowedTypes rather than a flag because the two
// kinds of finding differ: a claim about specific text must point at that text,
// while a judgement about a whole page or a pair of pages has no single span to
// quote and would be silently discarded by a blanket requirement.
type wikiFindingSpec struct {
	AllowedTypes  []string
	QuoteSource   string
	QuoteRequired []string
}

// parseWikiReviewFindings turns a model answer into findings the problem centre
// can trust.
//
// Every filter here exists because an unfiltered reviewer degrades the problem
// centre faster than it improves the wiki: an unknown type cannot be labelled,
// filtered, or verified; a low-confidence guess costs an editor's attention; and
// an evidence span the page does not contain means the finding was imagined.
func parseWikiReviewFindings(raw string, spec wikiFindingSpec) []wikiReviewFinding {
	payload := extractWikiJSONObject(raw)
	if payload == "" {
		return nil
	}
	var parsed struct {
		Findings []wikiReviewFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(spec.AllowedTypes))
	for _, issueType := range spec.AllowedTypes {
		allowed[issueType] = struct{}{}
	}
	quoteRequired := make(map[string]struct{}, len(spec.QuoteRequired))
	for _, issueType := range spec.QuoteRequired {
		quoteRequired[issueType] = struct{}{}
	}
	normalizedSource := normalizeWikiEvidence(spec.QuoteSource)
	out := make([]wikiReviewFinding, 0, len(parsed.Findings))
	seen := make(map[string]struct{}, len(parsed.Findings))
	for _, finding := range parsed.Findings {
		if len(out) >= wikiReviewMaxFindingsPerUnit {
			break
		}
		finding.IssueType = strings.ToLower(strings.TrimSpace(finding.IssueType))
		if _, ok := allowed[finding.IssueType]; !ok {
			continue
		}
		if finding.Confidence < wikiReviewMinConfidence {
			continue
		}
		finding.Evidence = truncateRunes(strings.TrimSpace(finding.Evidence), wikiReviewEvidenceRunes)
		finding.Problem = strings.TrimSpace(finding.Problem)
		finding.Suggestion = strings.TrimSpace(finding.Suggestion)
		if finding.Problem == "" {
			continue
		}
		if _, mustQuote := quoteRequired[finding.IssueType]; mustQuote {
			if finding.Evidence == "" {
				continue
			}
			// A quote the page does not contain is a hallucinated finding, and
			// it would also break the fingerprint's stability across runs.
			if !strings.Contains(normalizedSource, normalizeWikiEvidence(finding.Evidence)) {
				continue
			}
		} else if finding.Evidence != "" &&
			!strings.Contains(normalizedSource, normalizeWikiEvidence(finding.Evidence)) {
			// An optional quote that does not appear in the page is dropped
			// rather than shown: an editor who cannot find the quoted text has
			// no way to tell a real finding from an invented one.
			finding.Evidence = ""
		}
		finding.Severity = normalizeWikiReviewSeverity(finding.Severity)
		key := finding.IssueType + "\x00" + normalizeWikiEvidence(finding.Evidence)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, finding)
	}
	return out
}

func normalizeWikiReviewSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error", "high":
		return "high"
	case "info", "low":
		return "low"
	default:
		return "warning"
	}
}

// normalizeWikiEvidence collapses whitespace and case so an evidence span can
// be matched against the page body — and fingerprinted — without being
// sensitive to how the model reproduced the quote's spacing.
func normalizeWikiEvidence(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// extractWikiJSONObject pulls the JSON object out of an answer that may be
// wrapped in a code fence or prose despite the instruction not to.
func extractWikiJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if fence := strings.Index(trimmed, "```"); fence >= 0 {
		rest := trimmed[fence+3:]
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			rest = rest[nl+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			trimmed = strings.TrimSpace(rest[:end])
		}
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return ""
	}
	return trimmed[start : end+1]
}

// wikiContentHash identifies a page body for the review ledger.
func wikiContentHash(page *types.WikiPage) string {
	if page == nil {
		return ""
	}
	return wikiHashParts(page.Title, page.Content)
}

// wikiHashParts hashes an ordered set of inputs into a ledger unit hash.
func wikiHashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// wikiReviewIssueRecord projects one reviewed finding onto its problem-centre
// row.
//
// The fingerprint is keyed on the finding's identity — normally the quoted span,
// or whatever the detector declared instead — rather than on the model's prose.
// So re-reviewing an unchanged unit updates the existing issue instead of
// creating a near-duplicate, and changing the quoted text resolves it.
func wikiReviewIssueRecord(
	tenantID uint64, kbID, runID, modelID, detectorID string, seenAt time.Time,
	page *types.WikiPage, finding wikiReviewFinding,
) *types.WikiPageIssue {
	identity := finding.fingerprintKey
	if identity == "" {
		identity = normalizeWikiEvidence(finding.Evidence)
	}
	fingerprint := wikiIssueFingerprint(kbID, page.ID, page.Slug, finding.IssueType, identity)

	evidence := map[string]interface{}{
		"quote":            finding.Evidence,
		"suggestion":       finding.Suggestion,
		"confidence":       finding.Confidence,
		"model_id":         modelID,
		"detector_id":      detectorID,
		"reviewer_version": wikiReviewerVersion,
	}
	for key, value := range finding.Extra {
		evidence[key] = value
	}
	encoded, _ := json.Marshal(evidence)

	description := finding.Problem
	if finding.Suggestion != "" {
		description += " " + finding.Suggestion
	}
	return &types.WikiPageIssue{
		ID: uuid.New().String(), TenantID: tenantID, KnowledgeBaseID: kbID,
		PageID: page.ID, Slug: page.Slug, IssueType: finding.IssueType,
		Severity: finding.Severity, Source: types.WikiIssueSourceAI,
		Fingerprint: fingerprint, Description: description, Evidence: types.JSON(encoded),
		RepairMode: types.WikiIssueRepairAgent, DetectedPageVersion: page.Version,
		LastSeenRunID: runID, LastSeenAt: seenAt, OccurrenceCount: 1,
		Status: types.WikiIssueStatusOpen, ReportedBy: "wiki-ai-review",
	}
}

// wikiReviewIssueTypes collects the issue types a set of detectors may report,
// which is exactly what their run is allowed to close by absence.
func wikiReviewIssueTypes(detectors []wikiReviewDetector) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, detector := range detectors {
		for _, issueType := range detector.IssueTypes() {
			if _, dup := seen[issueType]; dup {
				continue
			}
			seen[issueType] = struct{}{}
			out = append(out, issueType)
		}
	}
	sort.Strings(out)
	return out
}
