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

// TestParseWikiAIFindingsRejectsWhatCannotBeTrusted covers every filter between
// the model's answer and the problem centre. Each one exists because an
// unfiltered reviewer degrades the problem centre faster than it improves the
// wiki, and a finding an editor cannot act on is worse than no finding.
func TestParseWikiAIFindingsRejectsWhatCannotBeTrusted(t *testing.T) {
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

	findings := parseWikiAIFindings(raw, aiTestPage)
	require.Len(t, findings, 1,
		"unknown types, low confidence, invented quotes and duplicate spans are all dropped")
	assert.Equal(t, types.WikiIssueTypeContradictory, findings[0].IssueType)
	assert.Equal(t, "high", findings[0].Severity)
}

// TestParseWikiAIFindingsAcceptsSilence pins the answer we want most pages to
// give. A reviewer that always finds something is not a reviewer.
func TestParseWikiAIFindingsAcceptsSilence(t *testing.T) {
	assert.Empty(t, parseWikiAIFindings(`{"findings":[]}`, aiTestPage))
	assert.Empty(t, parseWikiAIFindings("I could not review this page.", aiTestPage))
}

// TestWikiAIFindingFingerprintTracksTheQuotedSpan is what makes a repeat review
// idempotent: the same defect on an unchanged page must update the existing
// issue rather than pile up near-duplicates, even when the model rewords its
// own prose between runs.
func TestWikiAIFindingFingerprintTracksTheQuotedSpan(t *testing.T) {
	page := &types.WikiPage{ID: "page-1", Slug: "entity/acme-widget", Version: 3}
	first := wikiAIIssueRecord(1, "kb-1", "run-1", "model-1", nowForTest(), page, wikiAIFinding{
		IssueType: types.WikiIssueTypeContradictory, Severity: "high",
		Evidence: "The Acme Widget shipped in 2019.", Problem: "Conflicts with the discontinuation.",
		Suggestion: "State the lifecycle once.", Confidence: 0.9,
	})
	reworded := wikiAIIssueRecord(1, "kb-1", "run-2", "model-1", nowForTest(), page, wikiAIFinding{
		IssueType: types.WikiIssueTypeContradictory, Severity: "high",
		Evidence: "the acme widget shipped in 2019.", Problem: "Two ship states cannot both hold.",
		Confidence: 0.85,
	})
	assert.Equal(t, first.Fingerprint, reworded.Fingerprint)

	elsewhere := wikiAIIssueRecord(1, "kb-1", "run-2", "model-1", nowForTest(), page, wikiAIFinding{
		IssueType: types.WikiIssueTypeContradictory, Severity: "high",
		Evidence: "Pricing is 49 USD per seat.", Problem: "Different span.", Confidence: 0.9,
	})
	assert.NotEqual(t, first.Fingerprint, elsewhere.Fingerprint,
		"two distinct defects on one page must stay two issues")

	var evidence map[string]interface{}
	require.NoError(t, json.Unmarshal(first.Evidence, &evidence))
	assert.Equal(t, "The Acme Widget shipped in 2019.", evidence["quote"])
	assert.Equal(t, "State the lifecycle once.", evidence["suggestion"])
	assert.Equal(t, "model-1", evidence["model_id"])
	assert.Equal(t, types.WikiIssueSourceAI, first.Source)
	assert.Equal(t, types.WikiIssueRepairAgent, first.RepairMode)
}

// TestVerifyAIFindingClosesOnEvidenceWithoutAModelCall is the cheap half of the
// repair loop, and the half that runs almost every time: the reviewer had to
// quote the page verbatim, so a rewritten quote is proof on its own.
func TestVerifyAIFindingClosesOnEvidenceWithoutAModelCall(t *testing.T) {
	issue := &types.WikiPageIssue{
		IssueType: types.WikiIssueTypeContradictory, Source: types.WikiIssueSourceAI,
		DetectedPageVersion: 3,
	}
	attempt := &types.WikiRepairAttempt{BeforeVersion: 3}
	rechecked := false
	recheck := func(context.Context, *types.WikiPageIssue, *types.WikiPage) (bool, error) {
		rechecked = true
		return true, nil
	}

	repaired := wikiVerifyInput{
		Issue: issue, Attempt: attempt, Recheck: recheck,
		EvidenceQuote: "The Acme Widget shipped in 2019.",
		Page:          &types.WikiPage{Version: 4, Content: "The Acme Widget was discontinued in 2021."},
	}
	require.NoError(t, verifyWikiIssuePostcondition(context.Background(), repaired))
	assert.False(t, rechecked, "a rewritten quote must not cost a model call")
}

// TestVerifyAIFindingRequiresRealProgressThenCanRecheck covers the two cases
// the cheap check cannot settle: an untouched page, and a page that changed
// somewhere other than the quoted span (a contradiction can be resolved from
// either side). Only the second is worth one bounded call.
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

// TestWikiAIBudgetIsClampedToTheHardCap keeps a mistyped knowledge-base setting
// from turning one scan into thousands of model calls.
func TestWikiAIBudgetIsClampedToTheHardCap(t *testing.T) {
	assert.Equal(t, wikiAIDefaultMaxPages, wikiAIMaxPagesFor(&types.KnowledgeBase{}))
	assert.Equal(t, wikiAIHardMaxPages, wikiAIMaxPagesFor(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{LintAIMaxPages: 100000},
	}))
	assert.Equal(t, 5, wikiAIMaxPagesFor(&types.KnowledgeBase{
		WikiConfig: &types.WikiConfig{LintAIMaxPages: 5},
	}))
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

// nowForTest gives the ledger a fixed timestamp so fingerprints in this file
// depend only on the page and the finding.
func nowForTest() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
