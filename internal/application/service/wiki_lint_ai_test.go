package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const aiTestPage = "The Acme Widget shipped in 2019. Pricing is 49 USD per seat. " +
	"The Acme Widget was discontinued in 2021 and is no longer sold."

func pageContentSpec() wikiFindingSpec {
	detector := wikiPageContentDetector{}
	return wikiFindingSpec{
		AllowedTypes:  detector.IssueTypes(),
		QuoteSource:   aiTestPage,
		QuoteRequired: detector.IssueTypes(),
	}
}

// TestParseReviewFindingsRejectsWhatCannotBeTrusted covers every filter between
// a model answer and the problem centre. Each one exists because an unfiltered
// reviewer degrades the problem centre faster than it improves the wiki, and a
// finding an editor cannot act on is worse than no finding.
func TestParseReviewFindingsRejectsWhatCannotBeTrusted(t *testing.T) {
	raw := `Sure, here you go:
` + "```json" + `
{"findings":[
  {"issue_type":"contradictory_facts","severity":"error","confidence":0.9,
   "evidence":"The Acme Widget shipped in 2019.","problem":"Ships and discontinued conflict.",
   "suggestion":"State the lifecycle once."},
  {"issue_type":"tone_problem","severity":"warning","confidence":0.95,
   "evidence":"Pricing is 49 USD per seat.","problem":"Too terse."},
  {"issue_type":"out_of_date","severity":"warning","confidence":0.2,
   "evidence":"Pricing is 49 USD per seat.","problem":"Pricing may be stale."},
  {"issue_type":"unsupported_claim","severity":"warning","confidence":0.9,
   "evidence":"The Acme Widget won three industry awards.","problem":"No basis on the page."},
  {"issue_type":"contradictory_facts","severity":"error","confidence":0.8,
   "evidence":"the acme widget shipped   in 2019.","problem":"Same span, reworded."}
]}
` + "```"

	findings := parseWikiReviewFindings(raw, pageContentSpec())
	require.Len(t, findings, 1,
		"unknown types, low confidence, invented quotes and duplicate spans are all dropped")
	assert.Equal(t, types.WikiIssueTypeContradictory, findings[0].IssueType)
	assert.Equal(t, "high", findings[0].Severity)
}

// TestParseReviewFindingsAcceptsSilence pins the answer we want most units to
// give. A reviewer that always finds something is not a reviewer.
func TestParseReviewFindingsAcceptsSilence(t *testing.T) {
	assert.Empty(t, parseWikiReviewFindings(`{"findings":[]}`, pageContentSpec()))
	assert.Empty(t, parseWikiReviewFindings("I could not review this page.", pageContentSpec()))
}

// TestParseReviewFindingsKeepsQuotelessTypesButDropsInventedQuotes covers the
// asymmetry that lets one parser serve every detector: a claim about specific
// text must point at that text, while a judgement about a whole page or a pair
// has no span to quote — yet neither may show an editor a quote the page does
// not contain.
func TestParseReviewFindingsKeepsQuotelessTypesButDropsInventedQuotes(t *testing.T) {
	spec := wikiFindingSpec{
		AllowedTypes:  []string{types.WikiIssueTypeFactualError, types.WikiIssueTypeIncompleteSummary},
		QuoteSource:   aiTestPage,
		QuoteRequired: []string{types.WikiIssueTypeFactualError},
	}
	raw := `{"findings":[
  {"issue_type":"incomplete_summary","severity":"warning","confidence":0.8,
   "evidence":"a span that is not on the page","problem":"The page omits the roadmap."},
  {"issue_type":"factual_error","severity":"error","confidence":0.9,
   "evidence":"Pricing is 49 USD per seat.","problem":"The source says 59 USD."},
  {"issue_type":"factual_error","severity":"error","confidence":0.9,
   "evidence":"a span that is not on the page","problem":"Wrong ship date."}
]}`

	findings := parseWikiReviewFindings(raw, spec)
	require.Len(t, findings, 2, "the quote-required finding with an invented span is dropped")

	assert.Equal(t, types.WikiIssueTypeIncompleteSummary, findings[0].IssueType)
	assert.Empty(t, findings[0].Evidence,
		"an optional quote the page does not contain is cleared, not shown")
	assert.Equal(t, types.WikiIssueTypeFactualError, findings[1].IssueType)
	assert.Equal(t, "Pricing is 49 USD per seat.", findings[1].Evidence)
}

// TestParseReviewFindingsCollapsesRepeatedQuotelessFindings pins a consequence of
// how quoteless findings are identified. They are one judgement about the unit, so
// they share a fingerprint and would land on the same issue row anyway; keeping
// only the first is what stops the same issue's description from depending on
// which of two near-identical answers happened to be written last.
func TestParseReviewFindingsCollapsesRepeatedQuotelessFindings(t *testing.T) {
	findings := parseWikiReviewFindings(`{"findings":[
  {"issue_type":"incomplete_summary","severity":"warning","confidence":0.8,
   "evidence":"","problem":"The page never mentions the support policy."},
  {"issue_type":"incomplete_summary","severity":"warning","confidence":0.8,
   "evidence":"","problem":"The page omits the roadmap."}
]}`, wikiFindingSpec{
		AllowedTypes: []string{types.WikiIssueTypeIncompleteSummary},
		QuoteSource:  aiTestPage,
	})
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Problem, "support policy")
}

// TestReviewFindingFingerprintTracksItsIdentity is what makes a repeat review
// idempotent. The identity differs by defect class — a quoted span for a claim
// about text, the counterpart page for a duplicate — and in both cases
// re-detecting the same thing must update the existing issue rather than pile up
// near-duplicates when the model rewords its own prose.
func TestReviewFindingFingerprintTracksItsIdentity(t *testing.T) {
	page := &types.WikiPage{ID: "page-1", Slug: "entity/acme-widget", Version: 3}
	record := func(finding wikiReviewFinding) *types.WikiPageIssue {
		return wikiReviewIssueRecord(
			1, "kb-1", "run-1", "model-1", "page-content", nowForTest(), page, finding,
		)
	}

	first := record(wikiReviewFinding{
		IssueType: types.WikiIssueTypeContradictory, Severity: "high",
		Evidence: "The Acme Widget shipped in 2019.", Problem: "Conflicts with the discontinuation.",
		Suggestion: "State the lifecycle once.", Confidence: 0.9,
	})
	reworded := record(wikiReviewFinding{
		IssueType: types.WikiIssueTypeContradictory, Severity: "high",
		Evidence: "the acme widget shipped in 2019.", Problem: "Two ship states cannot both hold.",
		Confidence: 0.85,
	})
	assert.Equal(t, first.Fingerprint, reworded.Fingerprint)

	elsewhere := record(wikiReviewFinding{
		IssueType: types.WikiIssueTypeContradictory, Severity: "high",
		Evidence: "Pricing is 49 USD per seat.", Problem: "Different span.", Confidence: 0.9,
	})
	assert.NotEqual(t, first.Fingerprint, elsewhere.Fingerprint,
		"two distinct defects on one page must stay two issues")

	// A pair finding declares its own identity, because the defect is not
	// located in any single span of either page.
	pairA := record(wikiReviewFinding{
		IssueType: types.WikiIssueTypeDuplicatePages, Problem: "Same product.",
		Confidence: 0.9, fingerprintKey: "pair:entity/acme-widget-pro",
		Extra: map[string]interface{}{"other_slug": "entity/acme-widget-pro"},
	})
	pairB := record(wikiReviewFinding{
		IssueType: types.WikiIssueTypeDuplicatePages, Problem: "Reworded verdict.",
		Confidence: 0.7, fingerprintKey: "pair:entity/acme-widget-pro",
	})
	assert.Equal(t, pairA.Fingerprint, pairB.Fingerprint)

	var evidence map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Evidence, &evidence))
	assert.Equal(t, "The Acme Widget shipped in 2019.", evidence["quote"])
	assert.Equal(t, "State the lifecycle once.", evidence["suggestion"])
	assert.Equal(t, "model-1", evidence["model_id"])
	assert.Equal(t, "page-content", evidence["detector_id"])
	assert.Equal(t, types.WikiIssueSourceAI, first.Source)
	assert.Equal(t, types.WikiIssueRepairAgent, first.RepairMode)

	require.NoError(t, json.Unmarshal(pairA.Evidence, &evidence))
	assert.Equal(t, "entity/acme-widget-pro", evidence["other_slug"],
		"the counterpart must survive on the issue, since verification needs it")
}

// TestVerifyAIFindingClosesOnEvidenceWithoutAModelCall is the cheap half of the
// repair loop, and the half that runs almost every time: the reviewer had to
// quote the page verbatim, so a rewritten quote is proof on its own.
func TestVerifyAIFindingClosesOnEvidenceWithoutAModelCall(t *testing.T) {
	issue := &types.WikiPageIssue{
		IssueType: types.WikiIssueTypeContradictory, Source: types.WikiIssueSourceAI,
		DetectedPageVersion: 3,
	}
	rechecked := false
	recheck := func(context.Context, *types.WikiPageIssue, *types.WikiPage) (bool, error) {
		rechecked = true
		return true, nil
	}

	repaired := wikiVerifyInput{
		Issue: issue, Attempt: &types.WikiRepairAttempt{BeforeVersion: 3}, Recheck: recheck,
		EvidenceQuote: "The Acme Widget shipped in 2019.",
		Page:          &types.WikiPage{Version: 4, Content: "The Acme Widget was discontinued in 2021."},
	}
	require.NoError(t, verifyWikiIssuePostcondition(context.Background(), repaired))
	assert.False(t, rechecked, "a rewritten quote must not cost a model call")
}

// TestVerifyAIFindingRequiresRealProgressThenCanRecheck covers the two cases the
// cheap check cannot settle: an untouched page, and a page that changed somewhere
// other than the quoted span (a contradiction can be resolved from either side).
// Only the second is worth one bounded call.
func TestVerifyAIFindingRequiresRealProgressThenCanRecheck(t *testing.T) {
	issue := &types.WikiPageIssue{
		IssueType: types.WikiIssueTypeContradictory, Source: types.WikiIssueSourceAI,
		DetectedPageVersion: 3,
	}
	quote := "The Acme Widget shipped in 2019."
	page := func(version int) *types.WikiPage {
		return &types.WikiPage{Version: version, Content: aiTestPage}
	}

	stalled := wikiVerifyInput{
		Issue: issue, Attempt: &types.WikiRepairAttempt{BeforeVersion: 3},
		EvidenceQuote: quote, Page: page(3),
	}
	assert.Error(t, verifyWikiIssuePostcondition(context.Background(), stalled),
		"an untouched page cannot resolve anything")

	edited := wikiVerifyInput{
		Issue: issue, Attempt: &types.WikiRepairAttempt{BeforeVersion: 3},
		EvidenceQuote: quote, Page: page(4),
		Recheck: func(context.Context, *types.WikiPageIssue, *types.WikiPage) (bool, error) {
			return true, nil
		},
	}
	assert.Error(t, verifyWikiIssuePostcondition(context.Background(), edited),
		"the reviewer still sees the defect, so the issue stays open")

	edited.Recheck = func(context.Context, *types.WikiPageIssue, *types.WikiPage) (bool, error) {
		return false, nil
	}
	assert.NoError(t, verifyWikiIssuePostcondition(context.Background(), edited))
}

// TestVerifyIncompleteSummaryRequiresRealCoverageGrowth is the postcondition that
// stops a thin page from being closed by a copy-edit. The finding recorded what it
// measured, so the check is arithmetic rather than judgement.
func TestVerifyIncompleteSummaryRequiresRealCoverageGrowth(t *testing.T) {
	evidence, err := json.Marshal(map[string]interface{}{
		"cited_chunks": 2, "source_chunks": 40, "content_runes": 400,
	})
	require.NoError(t, err)
	issue := &types.WikiPageIssue{
		IssueType: types.WikiIssueTypeIncompleteSummary, Source: types.WikiIssueSourceAI,
		DetectedPageVersion: 2, Evidence: types.JSON(evidence),
	}
	attempt := &types.WikiRepairAttempt{BeforeVersion: 2}

	reworded := wikiVerifyInput{
		Issue: issue, Attempt: attempt,
		Page: &types.WikiPage{Version: 3, Content: string(make([]byte, 410))},
	}
	err = verifyWikiIssuePostcondition(context.Background(), reworded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no more of its source")

	expanded := wikiVerifyInput{
		Issue: issue, Attempt: attempt,
		Page: &types.WikiPage{Version: 3, Content: string(make([]byte, 700))},
	}
	assert.NoError(t, verifyWikiIssuePostcondition(context.Background(), expanded))

	// New citations are the other legitimate signal: the page took on source
	// material it previously ignored.
	cited := wikiVerifyInput{
		Issue: issue, Attempt: attempt,
		Page: &types.WikiPage{
			Version: 3, Content: string(make([]byte, 410)),
			ChunkRefs: types.StringArray{"c1", "c2", "c3"},
		},
	}
	assert.NoError(t, verifyWikiIssuePostcondition(context.Background(), cited))
}

// TestWikiReviewBudgetIsClampedToTheHardCap keeps a mistyped knowledge-base
// setting from turning one scan into thousands of model calls.
func TestWikiReviewBudgetIsClampedToTheHardCap(t *testing.T) {
	assert.Equal(t, wikiReviewDefaultBudget, wikiReviewBudget(&types.KnowledgeBase{}))
	assert.Equal(t, wikiReviewHardBudget, wikiReviewBudget(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{LintAIMaxPages: 100000},
	}))
	assert.Equal(t, 5, wikiReviewBudget(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{LintAIMaxPages: 5},
	}))
}

// TestWikiReviewSharesGuaranteeEveryDetectorACall is the property that matters
// more than the proportions: a detector that never gets a call is a defect class
// the product silently does not cover, and that is not something weight rounding
// should decide.
func TestWikiReviewSharesGuaranteeEveryDetectorACall(t *testing.T) {
	detectors := wikiReviewDetectors()
	require.Len(t, detectors, 3)

	shares := wikiReviewShares(wikiReviewDefaultBudget, detectors)
	assert.Equal(t, wikiReviewDefaultBudget, sum(shares))
	for i, share := range shares {
		assert.Positive(t, share, "detector %s must get at least one call", detectors[i].ID())
	}
	assert.Greater(t, shares[0], shares[2],
		"the broadest detector takes the largest share")

	// A budget smaller than the detector count still spends every call, and
	// never hands out a negative or duplicated one.
	tiny := wikiReviewShares(2, detectors)
	assert.Equal(t, 2, sum(tiny))
	for _, share := range tiny {
		assert.GreaterOrEqual(t, share, 0)
	}

	assert.Equal(t, []int{0, 0, 0}, wikiReviewShares(0, detectors))
	assert.Empty(t, wikiReviewShares(10, nil))
}

// TestReviewDetectorsCoverEveryAIIssueTypeExactlyOnce keeps the registry and the
// issue-type vocabulary from drifting apart. A type no detector reports is dead
// UI, and a type two detectors report cannot be reconciled by either.
func TestReviewDetectorsCoverEveryAIIssueTypeExactlyOnce(t *testing.T) {
	owner := map[string]string{}
	for _, detector := range wikiReviewDetectors() {
		identity := detector.Identity()
		partitioned := append(
			append([]string{}, identity.QuoteAnchored...), identity.UnitIdentified...,
		)
		assert.ElementsMatch(t, detector.IssueTypes(), partitioned,
			"detector %s must classify each of its issue types as quote-anchored or unit-identified",
			detector.ID())

		for _, issueType := range detector.IssueTypes() {
			if existing, dup := owner[issueType]; dup {
				t.Fatalf("issue type %s is reported by both %s and %s", issueType, existing, detector.ID())
			}
			owner[issueType] = detector.ID()
			// Every AI-reported type must also be resolvable back to its
			// detector, which is what makes a repair recheck possible.
			assert.Equal(t, detector.ID(), wikiIssueDetectorID(&types.WikiPageIssue{IssueType: issueType}))
		}
	}
	assert.Contains(t, owner, types.WikiIssueTypeMixedEntities)
	assert.Contains(t, owner, types.WikiIssueTypeIncompleteSummary)
	assert.Contains(t, owner, types.WikiIssueTypeDuplicatePages)
}

// TestWikiLintModelFallsBackToTheRepairModel keeps enabling the AI review from
// requiring a second configuration step, while still letting a knowledge base
// review with a cheaper model than it repairs with.
func TestWikiLintModelFallsBackToTheRepairModel(t *testing.T) {
	assert.Equal(t, "repair-model", WikiLintModelID(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{RepairModelID: "repair-model"},
	}))
	assert.Equal(t, "cheap-model", WikiLintModelID(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{RepairModelID: "repair-model", LintModelID: "cheap-model"},
	}))
	assert.Empty(t, WikiLintModelID(&types.KnowledgeBase{}))
}

// TestEnabledDetectorsHonourTheAllowList lets an operator stop paying for a
// defect class their wiki does not have, while an empty or unrecognized list
// still yields a working review rather than a silent no-op.
func TestEnabledDetectorsHonourTheAllowList(t *testing.T) {
	ids := func(detectors []wikiReviewDetector) []string {
		out := make([]string, 0, len(detectors))
		for _, detector := range detectors {
			out = append(out, detector.ID())
		}
		return out
	}
	assert.Len(t, enabledWikiReviewDetectors(&types.KnowledgeBase{}), 3)
	assert.Equal(t, []string{"page-content"}, ids(enabledWikiReviewDetectors(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{LintAIDetectors: types.StringArray{"page-content"}},
	})))
	assert.Len(t, enabledWikiReviewDetectors(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{LintAIDetectors: types.StringArray{"no-such-detector"}},
	}), 3, "an unrecognized allow-list must not disable the whole review")
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// nowForTest gives the ledger a fixed timestamp so fingerprints in this file
// depend only on the page and the finding.
func nowForTest() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
