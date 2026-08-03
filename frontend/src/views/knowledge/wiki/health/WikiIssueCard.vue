<template>
  <article class="wiki-issue-card" :class="{ 'is-selected': selected, [`is-${severity.theme}`]: true }">
    <div class="wiki-issue-card__rail" aria-hidden="true" />

    <div class="wiki-issue-card__body">
      <header class="wiki-issue-card__head">
        <t-checkbox
          v-if="selectable"
          class="wiki-issue-card__check"
          :checked="selected"
          @change="(checked: boolean) => emit('select', checked)"
        />
        <span class="wiki-issue-card__type" :class="`is-${typeMeta.theme}`">
          <t-icon :name="typeIcon" />
          {{ typeMeta.label }}
        </span>
        <span class="wiki-issue-card__severity" :class="`is-${severity.theme}`">{{ severity.label }}</span>
        <span class="wiki-issue-card__source">{{ sourceLabel }}</span>
        <span v-if="issue.status === 'failed'" class="wiki-issue-card__failed">
          {{ $t('knowledgeEditor.wikiBrowser.repairFailed') }}
        </span>
      </header>

      <div class="wiki-issue-card__pages">
        <button type="button" class="wiki-issue-card__page" @click="emit('open-page')">
          <span class="wiki-issue-card__page-title">{{ pageTitle }}</span>
          <code class="wiki-issue-card__page-slug">{{ issue.slug }}</code>
        </button>
        <!-- A duplicate finding is about two pages, so both have to be reachable
             from the card: the question it asks cannot be judged from one. -->
        <template v-if="pairedPage">
          <span class="wiki-issue-card__pair-join">
            <t-icon name="swap-right" />
            {{ $t('knowledgeEditor.wikiBrowser.issuePairedWith') }}
          </span>
          <button
            type="button"
            class="wiki-issue-card__page"
            @click="emit('open-target', pairedPage.slug)"
          >
            <span class="wiki-issue-card__page-title">{{ pairedPage.title }}</span>
            <code class="wiki-issue-card__page-slug">{{ pairedPage.slug }}</code>
          </button>
        </template>
      </div>

      <p class="wiki-issue-card__desc">{{ issue.description }}</p>

      <blockquote v-if="aiEvidence?.quote" class="wiki-issue-card__quote">
        <t-icon name="quote" />
        <span>{{ aiEvidence.quote }}</span>
      </blockquote>
      <p v-if="aiEvidence?.suggestion" class="wiki-issue-card__suggestion">
        <t-icon name="lightbulb" />
        <span>{{ aiEvidence.suggestion }}</span>
      </p>

      <p v-if="sourceDocument" class="wiki-issue-card__source-doc">
        <t-icon name="file" />
        <span>{{ $t('knowledgeEditor.wikiBrowser.issueCheckedAgainst', { document: sourceDocument }) }}</span>
        <span v-if="coverage" class="wiki-issue-card__coverage">
          {{ $t('knowledgeEditor.wikiBrowser.issueCoverage', coverage) }}
        </span>
      </p>

      <div v-if="targetSlug" class="wiki-issue-card__target">
        <span class="wiki-issue-card__target-label">
          {{ $t('knowledgeEditor.wikiBrowser.issueEvidenceTarget') }}
        </span>
        <button type="button" class="wiki-issue-card__target-link" @click="emit('open-target', targetSlug)">
          <t-icon name="link" />
          {{ targetDisplayName }}
        </button>
      </div>

      <div v-if="issue.resolution_summary && showResolution" class="wiki-issue-card__resolution">
        <t-icon name="check-circle" />
        <span>{{ issue.resolution_summary }}</span>
      </div>

      <footer class="wiki-issue-card__foot">
        <span class="wiki-issue-card__timing">{{ timingLabel }}</span>
        <div class="wiki-issue-card__actions">
          <button type="button" class="wiki-issue-card__action" @click="emit('open-page')">
            <t-icon name="jump" />
            <span>{{ $t('knowledgeEditor.wikiBrowser.healthCardViewPage') }}</span>
          </button>
          <template v-if="canEdit">
            <t-tooltip
              v-if="showRepair"
              :content="repairBlockedReason || $t('knowledgeEditor.wikiBrowser.issueFixBtn')"
              placement="top"
            >
              <button
                type="button"
                class="wiki-issue-card__action is-primary"
                :disabled="!!repairBlockedReason"
                @click="emit('repair')"
              >
                <t-icon name="tools" />
                <span>{{ $t('knowledgeEditor.wikiBrowser.issueFixBtn') }}</span>
              </button>
            </t-tooltip>
            <button v-if="showRestore" type="button" class="wiki-issue-card__action" @click="emit('restore')">
              <t-icon name="rollback" />
              <span>{{ $t('knowledgeEditor.wikiBrowser.issueRestore') }}</span>
            </button>
            <button v-else-if="showIgnore" type="button" class="wiki-issue-card__action is-ghost" @click="emit('ignore')">
              {{ $t('knowledgeEditor.wikiBrowser.issueIgnore') }}
            </button>
          </template>
        </div>
      </footer>
    </div>
  </article>
</template>

<script setup lang="ts">
/**
 * One finding in the wiki problem centre.
 *
 * The layout is ordered by what a reader needs in order to decide: what kind of
 * defect it is, which page it is on, what is wrong, and — for AI findings — the
 * exact span the reviewer objected to. That quote is deliberately prominent:
 * it is both the reader's way to confirm the finding is about real text, and
 * the thing the backend checks to close the issue.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WikiPageIssue } from '@/api/wiki'
import {
  wikiIssueAiEvidence,
  wikiIssueCoverage,
  wikiIssueEvidenceTarget,
  wikiIssuePairedPage,
  wikiIssueRepairModeLabel,
  wikiIssueSeverityLabel,
  wikiIssueSourceDocument,
  wikiIssueSourceLabel,
  wikiIssueTypeIcon,
  wikiIssueTypeLabel,
} from './issueMeta'

const props = defineProps<{
  issue: WikiPageIssue
  canEdit: boolean
  selected: boolean
  selectable: boolean
  showRepair: boolean
  showIgnore: boolean
  showRestore: boolean
  showResolution: boolean
  /** Empty when the repair can start; otherwise why it cannot. */
  repairBlockedReason: string
  /** Renders a slug as a human-readable page name. */
  slugDisplayName: (slug: string) => string
}>()

const emit = defineEmits<{
  (event: 'select', checked: boolean): void
  (event: 'repair'): void
  (event: 'ignore'): void
  (event: 'restore'): void
  (event: 'open-page'): void
  (event: 'open-target', slug: string): void
}>()

const { t } = useI18n()

const typeMeta = computed(() => wikiIssueTypeLabel(t, props.issue.issue_type))
const typeIcon = computed(() => wikiIssueTypeIcon(props.issue.issue_type))
const severity = computed(() => wikiIssueSeverityLabel(t, props.issue.severity))
const sourceLabel = computed(() => wikiIssueSourceLabel(t, props.issue))
const aiEvidence = computed(() => wikiIssueAiEvidence(props.issue))
const pairedPage = computed(() => wikiIssuePairedPage(props.issue))
const sourceDocument = computed(() => wikiIssueSourceDocument(props.issue))
const coverage = computed(() => wikiIssueCoverage(props.issue))
const targetSlug = computed(() => wikiIssueEvidenceTarget(props.issue))
const targetDisplayName = computed(() => (targetSlug.value ? props.slugDisplayName(targetSlug.value) : ''))
const pageTitle = computed(() => props.slugDisplayName(props.issue.slug))

const timingLabel = computed(() => {
  const parts: string[] = [wikiIssueRepairModeLabel(t, props.issue.repair_mode)]
  if (props.issue.last_seen_at) {
    parts.push(t('knowledgeEditor.wikiBrowser.issueLastSeen', { time: formatTime(props.issue.last_seen_at) }))
  }
  if (props.issue.occurrence_count > 1) {
    parts.push(t('knowledgeEditor.wikiBrowser.issueOccurrenceCount', { count: props.issue.occurrence_count }))
  }
  if (props.issue.detected_page_version > 0) {
    parts.push(t('knowledgeEditor.wikiBrowser.issueDetectedVersion', { version: props.issue.detected_page_version }))
  }
  return parts.join(' · ')
})

function formatTime(dateStr: string): string {
  const d = new Date(dateStr)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
</script>

<style scoped lang="less">
.wiki-issue-card {
  display: flex;
  overflow: hidden;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color-light-active);
  }

  &.is-selected {
    border-color: var(--td-brand-color);
    box-shadow: 0 0 0 1px var(--td-brand-color-light) inset;
  }
}

// The severity rail carries the colour so the card body itself can stay
// neutral; a list of fully tinted cards is unreadable at a glance.
.wiki-issue-card__rail {
  flex: none;
  width: 3px;
  background: var(--td-component-stroke);

  .wiki-issue-card.is-danger & {
    background: var(--td-error-color);
  }

  .wiki-issue-card.is-warning & {
    background: var(--td-warning-color);
  }
}

.wiki-issue-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 12px;
}

.wiki-issue-card__head {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.wiki-issue-card__check {
  margin-right: 2px;
}

.wiki-issue-card__type,
.wiki-issue-card__severity,
.wiki-issue-card__source,
.wiki-issue-card__failed {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  font-size: 12px;
  line-height: 20px;
  border-radius: 10px;
  white-space: nowrap;
}

.wiki-issue-card__type {
  font-weight: 500;

  &.is-danger {
    color: var(--td-error-color);
    background: var(--td-error-color-1);
  }

  &.is-warning {
    color: var(--td-warning-color);
    background: var(--td-warning-color-1);
  }

  &.is-primary {
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }

  &.is-default {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-component);
  }
}

.wiki-issue-card__severity {
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-component);

  &.is-danger {
    color: var(--td-error-color);
  }

  &.is-warning {
    color: var(--td-warning-color);
  }
}

.wiki-issue-card__source {
  margin-left: auto;
  padding: 0;
  color: var(--td-text-color-placeholder);
  background: transparent;
}

.wiki-issue-card__failed {
  color: var(--td-error-color);
  background: var(--td-error-color-1);
}

.wiki-issue-card__pages {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.wiki-issue-card__page {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 0;
  border: none;
  background: none;
  cursor: pointer;
  text-align: left;

  &:hover .wiki-issue-card__page-title {
    text-decoration: underline;
  }
}

.wiki-issue-card__pair-join {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.wiki-issue-card__page-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.wiki-issue-card__page-slug {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  word-break: break-all;
}

.wiki-issue-card__desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
}

.wiki-issue-card__quote {
  display: flex;
  gap: 6px;
  margin: 0;
  padding: 8px 10px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-component);
  border-left: 2px solid var(--td-warning-color);
  border-radius: 0 4px 4px 0;
  word-break: break-word;

  .t-icon {
    flex: none;
    margin-top: 2px;
    color: var(--td-text-color-placeholder);
  }
}

.wiki-issue-card__suggestion,
.wiki-issue-card__source-doc,
.wiki-issue-card__resolution {
  display: flex;
  gap: 6px;
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);

  .t-icon {
    flex: none;
    margin-top: 2px;
  }
}

.wiki-issue-card__source-doc {
  color: var(--td-text-color-placeholder);
  flex-wrap: wrap;
}

.wiki-issue-card__coverage {
  padding: 0 6px;
  border-radius: 9px;
  background: var(--td-bg-color-component);
  font-variant-numeric: tabular-nums;
}

.wiki-issue-card__resolution .t-icon {
  color: var(--td-success-color);
}

.wiki-issue-card__target {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.wiki-issue-card__target-label {
  color: var(--td-text-color-placeholder);
}

.wiki-issue-card__target-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0;
  border: none;
  background: none;
  color: var(--td-brand-color);
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
}

.wiki-issue-card__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  padding-top: 6px;
  border-top: 1px solid var(--td-component-stroke);
}

.wiki-issue-card__timing {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
}

.wiki-issue-card__actions {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
}

.wiki-issue-card__action {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  border: 1px solid transparent;
  border-radius: 4px;
  background: none;
  cursor: pointer;

  &:hover:not(:disabled) {
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-container-hover);
  }

  &.is-primary {
    color: var(--td-brand-color);
    border-color: var(--td-brand-color-light-active);
  }

  &.is-ghost {
    color: var(--td-text-color-placeholder);
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}
</style>
