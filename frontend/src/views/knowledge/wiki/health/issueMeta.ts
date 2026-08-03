import type { WikiPageIssue } from '@/api/wiki'

export type WikiIssueTagTheme = 'default' | 'primary' | 'warning' | 'danger' | 'success'

/** Translator signature shared with vue-i18n's `t`, narrowed to what this
 * module needs so the helpers stay usable from both components and tests. */
type Translate = (key: string, named?: Record<string, unknown>) => string

interface IssueTypePreset {
  key: string
  theme: WikiIssueTagTheme
  icon: string
}

/**
 * The issue types the problem centre can label, in the order the filter row
 * offers them: structural defects the rule scanner finds, then semantic ones
 * that only the AI review or an agent can report.
 *
 * A type absent from this table still renders, as a generic "needs attention"
 * finding — but the backend restricts the AI review to a closed set precisely
 * so that a finding always arrives with a label a user can filter on.
 */
const ISSUE_TYPE_PRESETS: Record<string, IssueTypePreset> = {
  broken_link: { key: 'issueBrokenLink', theme: 'danger', icon: 'link-unlink' },
  orphan_page: { key: 'issueOrphanPage', theme: 'warning', icon: 'root-list' },
  empty_content: { key: 'issueEmptyContent', theme: 'warning', icon: 'file-1' },
  stale_ref: { key: 'issueStaleRef', theme: 'danger', icon: 'history' },
  missing_cross_ref: { key: 'issueMissingCrossRef', theme: 'primary', icon: 'link' },
  mixed_entities: { key: 'issueMixed', theme: 'warning', icon: 'layers' },
  contradictory_facts: { key: 'issueConflict', theme: 'danger', icon: 'error-circle' },
  out_of_date: { key: 'issueOutdated', theme: 'default', icon: 'time' },
  unsupported_claim: { key: 'issueUnsupportedClaim', theme: 'warning', icon: 'help-circle' },
}

export const WIKI_ISSUE_TYPES = Object.keys(ISSUE_TYPE_PRESETS)

const SEVERITY_PRESETS: Record<string, { key: string; theme: WikiIssueTagTheme }> = {
  error: { key: 'issueSeverityError', theme: 'danger' },
  high: { key: 'issueSeverityError', theme: 'danger' },
  warning: { key: 'issueSeverityWarning', theme: 'warning' },
  info: { key: 'issueSeverityInfo', theme: 'default' },
  low: { key: 'issueSeverityInfo', theme: 'default' },
}

export function wikiIssueTypeLabel(t: Translate, issueType: string) {
  const preset = ISSUE_TYPE_PRESETS[issueType]
  if (!preset) {
    return { label: t('knowledgeEditor.wikiBrowser.issueAttention'), theme: 'primary' as WikiIssueTagTheme }
  }
  return { label: t(`knowledgeEditor.wikiBrowser.${preset.key}`), theme: preset.theme }
}

export function wikiIssueTypeIcon(issueType: string) {
  return ISSUE_TYPE_PRESETS[issueType]?.icon || 'error-circle'
}

export function wikiIssueSeverityLabel(t: Translate, severity: string) {
  const preset = SEVERITY_PRESETS[severity] || SEVERITY_PRESETS.warning
  return { label: t(`knowledgeEditor.wikiBrowser.${preset.key}`), theme: preset.theme }
}

export function wikiIssueRepairModeLabel(t: Translate, repairMode: string) {
  const keys: Record<string, string> = {
    deterministic: 'issueRepairModeDeterministic',
    agent: 'issueRepairModeAgent',
    manual: 'issueRepairModeManual',
  }
  return t(`knowledgeEditor.wikiBrowser.${keys[repairMode] || 'issueRepairModeAgent'}`)
}

/**
 * Where a finding came from. This is the label a user reads when deciding how
 * much to trust it, so the three detector families stay distinguishable:
 * deterministic rules, the bounded AI review, and an agent that noticed the
 * problem while answering a question.
 */
export function wikiIssueSourceLabel(t: Translate, issue: WikiPageIssue): string {
  if (issue.source === 'ai' || issue.reported_by === 'wiki-ai-review') {
    return t('knowledgeEditor.wikiBrowser.issueSourceAiReview')
  }
  if (issue.source === 'lint' || issue.reported_by === 'wiki-lint') {
    return t('knowledgeEditor.wikiBrowser.issueSourceLintReport')
  }
  if (issue.reported_by === 'wiki-researcher-agent') {
    return t('knowledgeEditor.wikiBrowser.issueAiLinter')
  }
  if (issue.reported_by) {
    return t('knowledgeEditor.wikiBrowser.issueReportedBy', { reporter: issue.reported_by })
  }
  return t('knowledgeEditor.wikiBrowser.issueSourceLintReport')
}

/** The counterpart page or knowledge id a structural finding points at. */
export function wikiIssueEvidenceTarget(issue: WikiPageIssue): string | null {
  const slug = issue.evidence?.target_slug
  if (typeof slug !== 'string' || !slug.trim()) return null
  return slug.trim()
}

export interface WikiIssueAiEvidence {
  quote: string
  suggestion: string
  confidence: number
}

/**
 * The verbatim span an AI finding was anchored to, plus the edit it proposes.
 *
 * Showing the quote is what makes an AI finding reviewable at a glance: the
 * reviewer was required to copy it from the page, so a reader can confirm the
 * finding is about real text before spending a repair on it. It is also what
 * the backend checks to close the issue, so the same span the user reads is the
 * one the repair has to change.
 */
export function wikiIssueAiEvidence(issue: WikiPageIssue): WikiIssueAiEvidence | null {
  const quote = typeof issue.evidence?.quote === 'string' ? issue.evidence.quote.trim() : ''
  if (!quote) return null
  const suggestion = typeof issue.evidence?.suggestion === 'string' ? issue.evidence.suggestion.trim() : ''
  const confidence = Number(issue.evidence?.confidence)
  return {
    quote,
    suggestion,
    confidence: Number.isFinite(confidence) ? confidence : 0,
  }
}
