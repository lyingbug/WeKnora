<template>
  <section class="wiki-scan-panel">
    <div class="wiki-scan-panel__modes" role="radiogroup" :aria-label="$t('knowledgeEditor.wikiBrowser.scanModeLegend')">
      <button
        v-for="option in modeOptions"
        :key="option.value"
        type="button"
        role="radio"
        :aria-checked="selectedMode === option.value"
        class="wiki-scan-mode"
        :class="{ 'is-active': selectedMode === option.value, 'is-blocked': !!option.blockedReason }"
        :title="option.blockedReason || option.hint"
        @click="selectMode(option.value)"
      >
        <span class="wiki-scan-mode__head">
          <t-icon :name="option.icon" />
          <span class="wiki-scan-mode__label">{{ option.label }}</span>
          <span class="wiki-scan-mode__cost" :class="`is-${option.costTheme}`">{{ option.cost }}</span>
        </span>
        <span class="wiki-scan-mode__hint">{{ option.blockedReason || option.hint }}</span>
      </button>
    </div>

    <div class="wiki-scan-panel__action">
      <t-button
        theme="primary"
        size="small"
        :loading="busy"
        :disabled="busy || !!activeBlockedReason"
        @click="emitRun"
      >
        {{ busy ? $t('knowledgeEditor.wikiBrowser.scanRunning') : $t('knowledgeEditor.wikiBrowser.scanStart') }}
      </t-button>
      <p v-if="activeBlockedReason" class="wiki-scan-panel__blocked">
        <t-icon name="info-circle" />
        <span>{{ activeBlockedReason }}</span>
      </p>
      <p v-else-if="!busy && lastRunSummary" class="wiki-scan-panel__summary">{{ lastRunSummary }}</p>
    </div>

    <p v-if="!busy && detectorSummary" class="wiki-scan-panel__detectors">{{ detectorSummary }}</p>

    <div v-if="busy" class="wiki-scan-panel__progress">
      <span class="wiki-scan-panel__phase">{{ phaseLabel }}</span>
      <t-progress :percentage="run?.progress || 0" theme="plump" size="small" />
    </div>

    <p v-if="failureMessage" class="wiki-scan-panel__error">
      <t-icon name="error-circle" />
      <span>{{ failureMessage }}</span>
    </p>
  </section>
</template>

<script setup lang="ts">
/**
 * The scan control surface for the wiki problem centre.
 *
 * Its job is to make the cost of a scan visible before it is started. The two
 * detector families are priced completely differently — the rules are pure
 * database work while the review spends one model call per page it re-reads —
 * so the mode is an explicit, labelled choice rather than a single "scan"
 * button that silently decides for the user. After a run finishes the panel
 * reports what was actually spent, including how many pages were skipped
 * because their content had not changed since the last review.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WikiLintMode, WikiLintRun } from '@/api/wiki'

const props = defineProps<{
  run: WikiLintRun | null
  busy: boolean
  /** Whether the knowledge base has a model configured for the AI review. */
  aiAvailable: boolean
  /** Scope label used in the mode hints: the whole wiki, or one page. */
  scope?: 'kb' | 'page'
}>()

const emit = defineEmits<{ (event: 'run', mode: WikiLintMode): void }>()

const { t } = useI18n()

const selectedMode = ref<WikiLintMode>('static')

// A run the user just started is the best statement of what they want next, so
// the panel adopts its mode instead of resetting to the default.
watch(
  () => props.run?.mode,
  (mode) => {
    if (mode === 'static' || mode === 'ai' || mode === 'full') selectedMode.value = mode
  },
  { immediate: true },
)

const scopeSuffix = computed(() => (props.scope === 'page' ? 'Page' : 'Kb'))

const modeOptions = computed(() => {
  const aiBlocked = props.aiAvailable ? '' : t('knowledgeEditor.wikiBrowser.scanModeAiNoModel')
  return [
    {
      value: 'static' as WikiLintMode,
      icon: 'check-rectangle',
      label: t('knowledgeEditor.wikiBrowser.scanModeStatic'),
      hint: t(`knowledgeEditor.wikiBrowser.scanModeStaticHint${scopeSuffix.value}`),
      cost: t('knowledgeEditor.wikiBrowser.scanCostFree'),
      costTheme: 'free',
      blockedReason: '',
    },
    {
      value: 'ai' as WikiLintMode,
      icon: 'system-sum',
      label: t('knowledgeEditor.wikiBrowser.scanModeAi'),
      hint: t(`knowledgeEditor.wikiBrowser.scanModeAiHint${scopeSuffix.value}`),
      cost: t('knowledgeEditor.wikiBrowser.scanCostModel'),
      costTheme: 'model',
      blockedReason: aiBlocked,
    },
    {
      value: 'full' as WikiLintMode,
      icon: 'layers',
      label: t('knowledgeEditor.wikiBrowser.scanModeFull'),
      hint: t(`knowledgeEditor.wikiBrowser.scanModeFullHint${scopeSuffix.value}`),
      cost: t('knowledgeEditor.wikiBrowser.scanCostModel'),
      costTheme: 'model',
      blockedReason: aiBlocked,
    },
  ]
})

const activeBlockedReason = computed(
  () => modeOptions.value.find((option) => option.value === selectedMode.value)?.blockedReason || '',
)

// The static rules are the fast phase and the review is the slow one, so the
// progress bar is labelled by phase — otherwise a long review reads as a stall.
const phaseLabel = computed(() => {
  const run = props.run
  if (!run) return t('knowledgeEditor.wikiBrowser.lintQueued')
  if (run.status === 'queued') return t('knowledgeEditor.wikiBrowser.lintQueued')
  if (run.mode !== 'static' && run.progress >= 40) {
    return t('knowledgeEditor.wikiBrowser.scanPhaseAi')
  }
  return t('knowledgeEditor.wikiBrowser.scanPhaseStatic')
})

// After a run, what it actually spent. Reporting the skipped units matters as
// much as the calls: it is the difference between "the review found nothing" and
// "nothing had changed since the review last looked".
const lastRunSummary = computed(() => {
  const run = props.run
  if (!run || run.status !== 'completed') return ''
  const parts = [t('knowledgeEditor.wikiBrowser.scanFindings', { count: run.finding_count ?? 0 })]
  if (run.ai_calls > 0) {
    parts.push(t('knowledgeEditor.wikiBrowser.scanAiSpend', {
      calls: run.ai_calls,
      units: run.ai_units_reviewed,
    }))
  }
  if (run.ai_units_skipped > 0) {
    parts.push(t('knowledgeEditor.wikiBrowser.scanAiSkipped', { count: run.ai_units_skipped }))
  }
  return parts.join(' · ')
})

// The detectors a completed AI run drew on. Naming them is what tells a user
// which classes of defect this scan was actually able to find.
const detectorSummary = computed(() => {
  const run = props.run
  if (!run || run.status !== 'completed' || run.mode === 'static') return ''
  const ids = run.ai_detectors || []
  if (ids.length === 0) return ''
  const labels = ids.map((id) => t(`knowledgeEditor.wikiBrowser.detector_${id.replace(/-/g, '_')}`))
  return t('knowledgeEditor.wikiBrowser.scanDetectors', { detectors: labels.join('、') })
})

const failureMessage = computed(() => {
  const run = props.run
  if (!run || run.status !== 'failed') return ''
  return run.error_message || t('knowledgeEditor.wikiBrowser.lintFailed')
})

function selectMode(mode: WikiLintMode) {
  selectedMode.value = mode
}

function emitRun() {
  if (props.busy || activeBlockedReason.value) return
  emit('run', selectedMode.value)
}
</script>

<style scoped lang="less">
.wiki-scan-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container-select);
}

.wiki-scan-panel__modes {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 8px;
}

.wiki-scan-mode {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  text-align: left;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color);
  }

  &.is-active {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }

  &.is-blocked {
    opacity: 0.6;
  }
}

.wiki-scan-mode__head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.wiki-scan-mode__label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wiki-scan-mode__cost {
  flex: none;
  padding: 0 6px;
  font-size: 11px;
  font-weight: 400;
  line-height: 18px;
  border-radius: 9px;

  &.is-free {
    color: var(--td-success-color);
    background: var(--td-success-color-1);
  }

  &.is-model {
    color: var(--td-warning-color);
    background: var(--td-warning-color-1);
  }
}

.wiki-scan-mode__hint {
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);
}

.wiki-scan-panel__action {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.wiki-scan-panel__detectors {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);
}

.wiki-scan-panel__summary,
.wiki-scan-panel__blocked,
.wiki-scan-panel__error {
  display: flex;
  align-items: center;
  gap: 4px;
  margin: 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.wiki-scan-panel__error {
  color: var(--td-error-color);
}

.wiki-scan-panel__progress {
  display: flex;
  align-items: center;
  gap: 8px;

  :deep(.t-progress) {
    flex: 1;
  }
}

.wiki-scan-panel__phase {
  flex: none;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
</style>
