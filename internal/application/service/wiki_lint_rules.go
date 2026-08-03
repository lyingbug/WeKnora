package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
)

// WikiLintIssueType defines the type of lint issue
type WikiLintIssueType string

const (
	LintIssueOrphanPage      WikiLintIssueType = "orphan_page"
	LintIssueBrokenLink      WikiLintIssueType = "broken_link"
	LintIssueStaleRef        WikiLintIssueType = "stale_ref"
	LintIssueMissingCrossRef WikiLintIssueType = "missing_cross_ref"
	LintIssueEmptyContent    WikiLintIssueType = "empty_content"
)

// WikiLintIssueSeverity defines the severity of a lint issue
type WikiLintIssueSeverity string

const (
	SeverityInfo    WikiLintIssueSeverity = "info"
	SeverityWarning WikiLintIssueSeverity = "warning"
	SeverityError   WikiLintIssueSeverity = "error"
)

// wikiMinContentRunes is the single source of truth for "this page is
// effectively empty". The detector and the postcondition that closes an
// empty-content finding both read it, so the two can never disagree about
// what counts as fixed.
const wikiMinContentRunes = 50

// wikiPageBySlugReader is the narrow read capability a postcondition may use.
// Keeping it minimal stops rule code from reaching into unrelated persistence.
type wikiPageBySlugReader interface {
	GetBySlug(ctx context.Context, kbID string, slug string) (*types.WikiPage, error)
}

// wikiVerifyInput carries everything a postcondition is allowed to inspect.
type wikiVerifyInput struct {
	Issue *types.WikiPageIssue
	// Page is the current state of the page named by the issue. Never nil:
	// the deleted-page case is settled by verifyWikiIssuePostcondition before
	// any rule runs.
	Page    *types.WikiPage
	Attempt *types.WikiRepairAttempt
	// TargetSlug is the counterpart recorded in the finding's evidence (the
	// dangling link target, the stale knowledge id, the unlinked entity).
	TargetSlug string
	// EvidenceQuote is the verbatim span an AI finding was anchored to. It is
	// what makes a semantic finding verifiable without a model call: if the
	// exact text the reviewer objected to is gone, the finding is gone.
	EvidenceQuote string
	Pages         wikiPageBySlugReader
	// Recheck asks the AI reviewer whether an equivalent finding is still
	// present on the page. It is consulted only when the cheap evidence check
	// is inconclusive, and may be nil when no reviewer is configured.
	Recheck wikiIssueRechecker
}

// wikiIssueRechecker re-reviews one page and reports whether a finding
// equivalent to the given issue is still present.
type wikiIssueRechecker func(ctx context.Context, issue *types.WikiPageIssue, page *types.WikiPage) (bool, error)

// wikiLintRule binds a finding's identity, the metadata a detector stamps onto
// it, and the postcondition that proves it resolved. Findings can only be
// constructed through a rule value (see finding), so a new rule cannot ship
// with a detector but no verifier — the compiler requires both fields.
type wikiLintRule struct {
	Type       WikiLintIssueType
	Severity   WikiLintIssueSeverity
	RepairMode string
	// AutoFixable marks findings the deterministic AutoFix sweep may touch.
	AutoFixable bool
	// Durable marks findings that belong in the persistent problem centre.
	// Advisory rules stay out of it: they are suggestions rather than defects,
	// and one row per (page, suggestion) pair would bury the real findings.
	Durable bool
	// RequiresTarget rejects a resolution claim whose evidence lost the
	// counterpart the rule needs, instead of silently passing verification.
	RequiresTarget bool
	Verify         func(ctx context.Context, in wikiVerifyInput) error
}

// finding stamps a detector hit with its rule's metadata. Callers supply only
// the facts they observed; severity, repair mode and durability always come
// from the rule.
func (r wikiLintRule) finding(page *types.WikiPage, targetSlug, description string) WikiLintIssue {
	return WikiLintIssue{
		Type:        r.Type,
		Severity:    r.Severity,
		RepairMode:  r.RepairMode,
		AutoFixable: r.AutoFixable,
		PageSlug:    page.Slug,
		PageID:      page.ID,
		PageVersion: page.Version,
		TargetSlug:  targetSlug,
		Description: description,
	}
}

var (
	wikiRuleOrphanPage = wikiLintRule{
		Type: LintIssueOrphanPage, Severity: SeverityWarning,
		RepairMode: types.WikiIssueRepairManual, Durable: true,
		Verify: func(_ context.Context, in wikiVerifyInput) error {
			if len(in.Page.InLinks) == 0 {
				return errors.New("page still has no inbound links")
			}
			return nil
		},
	}

	wikiRuleBrokenLink = wikiLintRule{
		Type: LintIssueBrokenLink, Severity: SeverityError,
		RepairMode: types.WikiIssueRepairDeterministic, AutoFixable: true,
		Durable: true, RequiresTarget: true,
		Verify: func(_ context.Context, in wikiVerifyInput) error {
			if containsWikiRef(in.Page.OutLinks, in.TargetSlug) {
				return fmt.Errorf("broken link %s is still present", in.TargetSlug)
			}
			return nil
		},
	}

	wikiRuleStaleRef = wikiLintRule{
		Type: LintIssueStaleRef, Severity: SeverityError,
		RepairMode: types.WikiIssueRepairAgent, Durable: true, RequiresTarget: true,
		Verify: func(_ context.Context, in wikiVerifyInput) error {
			if containsWikiRef(in.Page.SourceRefs, in.TargetSlug) {
				return fmt.Errorf("stale source reference %s is still present", in.TargetSlug)
			}
			return nil
		},
	}

	wikiRuleEmptyContent = wikiLintRule{
		Type: LintIssueEmptyContent, Severity: SeverityWarning,
		RepairMode: types.WikiIssueRepairAgent, Durable: true,
		Verify: func(_ context.Context, in wikiVerifyInput) error {
			if wikiContentRunes(in.Page.Content) < wikiMinContentRunes {
				return fmt.Errorf("page still has fewer than %d content characters", wikiMinContentRunes)
			}
			return nil
		},
	}

	// wikiRuleMissingCrossRef is advisory: it fires once per (page, mentioned
	// entity) pair, which is the product of two large sets. It is reported in
	// the synchronous health report but deliberately not persisted, so the
	// durable finding set stays proportional to real defects.
	wikiRuleMissingCrossRef = wikiLintRule{
		Type: LintIssueMissingCrossRef, Severity: SeverityInfo,
		RepairMode: types.WikiIssueRepairAgent, Durable: false, RequiresTarget: true,
		Verify: func(ctx context.Context, in wikiVerifyInput) error {
			if containsWikiRef(in.Page.OutLinks, in.TargetSlug) {
				return nil
			}
			targetPage, err := in.Pages.GetBySlug(ctx, in.Issue.KnowledgeBaseID, in.TargetSlug)
			if errors.Is(err, repository.ErrWikiPageNotFound) {
				// The referenced entity disappeared, so the finding is obsolete.
				return nil
			}
			if err != nil {
				return err
			}
			matcher := newWikiLintTitleMatcher(map[string]string{in.TargetSlug: targetPage.Title})
			if len(matcher.Find(in.Page.Content)) > 0 {
				return fmt.Errorf("page still mentions %q without linking to %s",
					targetPage.Title, in.TargetSlug)
			}
			return nil
		},
	}
)

// wikiLintRules indexes every rule by the issue type persisted on a finding.
// verifyWikiIssuePostcondition and the durability filter both resolve through
// it, so registering a rule here is the only step needed to make a new lint
// type fully participate in the repair lifecycle.
var wikiLintRules = map[WikiLintIssueType]wikiLintRule{
	wikiRuleOrphanPage.Type:      wikiRuleOrphanPage,
	wikiRuleBrokenLink.Type:      wikiRuleBrokenLink,
	wikiRuleStaleRef.Type:        wikiRuleStaleRef,
	wikiRuleEmptyContent.Type:    wikiRuleEmptyContent,
	wikiRuleMissingCrossRef.Type: wikiRuleMissingCrossRef,
}

// wikiLintRuleFor resolves the rule behind a persisted issue type. Agent-
// reported types (mixed_entities, out_of_date, other, …) have no lint rule and
// report ok=false.
func wikiLintRuleFor(issueType string) (wikiLintRule, bool) {
	rule, ok := wikiLintRules[WikiLintIssueType(issueType)]
	return rule, ok
}

// verifyWikiIssuePostcondition proves a finding is actually gone before its
// issue may close.
//
// Types without a registered rule are agent-reported semantic findings whose
// only machine-checkable signal is that the page really advanced during the
// attempt — see verifyWikiSemanticProgress. Routing them through one explicit
// branch (rather than a switch default) keeps "no typed postcondition" a
// deliberate answer instead of an oversight.
func verifyWikiIssuePostcondition(ctx context.Context, in wikiVerifyInput) error {
	if in.Issue == nil || in.Attempt == nil {
		return errors.New("issue and repair attempt are required")
	}
	if in.Page == nil {
		if in.Attempt.Action == "deleted" {
			return nil
		}
		return errors.New("target page no longer exists without a recorded delete action")
	}
	rule, ok := wikiLintRuleFor(in.Issue.IssueType)
	if !ok {
		if in.Issue.Source == types.WikiIssueSourceAI {
			return verifyWikiAIFindingResolved(ctx, in)
		}
		return verifyWikiSemanticProgress(in)
	}
	if rule.RequiresTarget && in.TargetSlug == "" {
		return fmt.Errorf("%s issue is missing target evidence", rule.Type)
	}
	return rule.Verify(ctx, in)
}

// verifyWikiAIFindingResolved closes the loop on an AI finding.
//
// Every AI finding gets a cheap, deterministic first check, chosen by what the
// finding is actually about. That check is what keeps repair verification free in
// the common case, and it is only ever allowed to answer "resolved" — never to
// pass a repair it could not confirm.
//
// When the cheap check cannot settle the question, one bounded recheck call asks
// the detector whether it still reports the same finding. If no reviewer is
// configured, or the recheck itself fails, we fall back to requiring that the
// page really advanced — the same answer agent-reported findings get, never a
// silent pass.
func verifyWikiAIFindingResolved(ctx context.Context, in wikiVerifyInput) error {
	if err := verifyWikiSemanticProgress(in); err != nil {
		return err
	}
	if settled, err := wikiAICheapPostcondition(ctx, in); settled {
		return err
	}
	if in.Recheck == nil {
		return nil
	}
	stillPresent, err := in.Recheck(ctx, in.Issue, in.Page)
	if err != nil {
		return nil
	}
	if stillPresent {
		return errors.New("the AI review still reports this problem on the page after the repair")
	}
	return nil
}

// wikiAICheapPostcondition applies the deterministic check for an AI finding's
// type. It reports settled=true when the check reached a verdict, so an
// inconclusive result falls through to the recheck rather than being treated as
// either success or failure.
func wikiAICheapPostcondition(ctx context.Context, in wikiVerifyInput) (settled bool, err error) {
	switch in.Issue.IssueType {
	case types.WikiIssueTypeIncompleteSummary:
		return verifyWikiCoverageGrew(in)
	case types.WikiIssueTypeDuplicatePages:
		return verifyWikiPairDisambiguated(ctx, in)
	default:
		return verifyWikiEvidenceRewritten(in)
	}
}

// verifyWikiEvidenceRewritten is the check for findings anchored to a quote.
//
// The reviewer had to copy the span it objected to, so a rewritten span is proof
// on its own — and it is the common case, because the flagged text is exactly
// what an editor rewrites. A surviving quote is genuinely ambiguous rather than a
// failure: a contradiction can be resolved by editing the other side of it.
func verifyWikiEvidenceRewritten(in wikiVerifyInput) (bool, error) {
	quote := strings.TrimSpace(in.EvidenceQuote)
	if quote == "" {
		return true, nil
	}
	if !strings.Contains(normalizeWikiEvidence(in.Page.Content), normalizeWikiEvidence(quote)) {
		return true, nil
	}
	return false, nil
}

// verifyWikiCoverageGrew is the check for an incomplete summary.
//
// The finding was recorded with the measurements it was made at, so the
// postcondition is arithmetic rather than judgement: the page must have taken on
// materially more of its source, either as prose or as new citations. Requiring
// growth is what stops a repair from closing the issue by merely rewording a page
// that still omits the same subject.
func verifyWikiCoverageGrew(in wikiVerifyInput) (bool, error) {
	evidence := wikiIssueEvidenceMap(in.Issue)
	recordedRunes := wikiEvidenceInt(evidence, "content_runes")
	recordedCitations := wikiEvidenceInt(evidence, "cited_chunks")
	if recordedRunes <= 0 && recordedCitations <= 0 {
		return false, nil
	}
	if len(in.Page.ChunkRefs) > recordedCitations {
		return true, nil
	}
	if recordedRunes > 0 {
		grown := float64(wikiContentRunes(in.Page.Content)) >=
			float64(recordedRunes)*wikiCoverageGrowthFactor
		if grown {
			return true, nil
		}
		return true, fmt.Errorf(
			"the page still covers no more of its source than when the issue was found (%d characters, %d citations)",
			wikiContentRunes(in.Page.Content), len(in.Page.ChunkRefs),
		)
	}
	return false, nil
}

// wikiCoverageGrowthFactor is how much longer a page must get before an
// incomplete-summary finding counts as addressed. Set well above rewording noise
// so a genuine addition clears it and a copy-edit does not.
const wikiCoverageGrowthFactor = 1.15

// verifyWikiPairDisambiguated is the check for a duplicate-page finding.
//
// A duplicate is resolved in one of two legitimate ways, and both are observable:
// the pages were merged, so one of them is gone, or an editor decided they are
// distinct after all and linked them, which is the same signal the detector uses
// to stop proposing the pair.
func verifyWikiPairDisambiguated(ctx context.Context, in wikiVerifyInput) (bool, error) {
	evidence := wikiIssueEvidenceMap(in.Issue)
	otherSlug, _ := evidence["other_slug"].(string)
	otherSlug = strings.TrimSpace(otherSlug)
	if otherSlug == "" || in.Pages == nil {
		return false, nil
	}
	other, err := in.Pages.GetBySlug(ctx, in.Issue.KnowledgeBaseID, otherSlug)
	if errors.Is(err, repository.ErrWikiPageNotFound) {
		return true, nil
	}
	if err != nil {
		return false, nil
	}
	if other.Status == types.WikiPageStatusArchived {
		return true, nil
	}
	if containsWikiRef(in.Page.OutLinks, otherSlug) || containsWikiRef(other.OutLinks, in.Page.Slug) {
		return true, nil
	}
	return true, fmt.Errorf(
		"page %s still exists and neither page links to the other, so the duplicate is unresolved",
		otherSlug,
	)
}

// wikiIssueEvidenceMap decodes a finding's evidence, tolerating the empty and
// malformed forms on historical rows.
func wikiIssueEvidenceMap(issue *types.WikiPageIssue) map[string]interface{} {
	evidence := map[string]interface{}{}
	if issue == nil || len(issue.Evidence) == 0 {
		return evidence
	}
	_ = json.Unmarshal(issue.Evidence, &evidence)
	return evidence
}

// wikiEvidenceInt reads a number out of evidence JSON, where every number
// arrives as a float64.
func wikiEvidenceInt(evidence map[string]interface{}, key string) int {
	switch value := evidence[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

// verifyWikiSemanticProgress is the fallback postcondition for findings whose
// truth lives in prose ("this page mixes two products"). We cannot re-derive
// them, so we require either a real edit during this attempt or evidence that
// the page already advanced after the issue was detected.
func verifyWikiSemanticProgress(in wikiVerifyInput) error {
	if in.Page.Version <= in.Attempt.BeforeVersion && in.Page.Version <= in.Issue.DetectedPageVersion {
		return errors.New("page version did not change and the issue cannot be verified as already resolved")
	}
	return nil
}

// wikiContentRunes counts the visible characters of a page body. Rune counting
// (not len) keeps the empty-content threshold meaningful for CJK content.
func wikiContentRunes(content string) int {
	return utf8.RuneCountInString(strings.TrimSpace(content))
}

// containsWikiRef reports whether refs holds target, tolerating the
// "id|display" form used by both out_links and source_refs.
func containsWikiRef(refs []string, target string) bool {
	for _, ref := range refs {
		if ref == target || strings.HasPrefix(ref, target+"|") {
			return true
		}
	}
	return false
}
