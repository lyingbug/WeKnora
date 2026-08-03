<template>
  <div class="wiki-page-check">
    <t-dropdown
      :options="menuOptions"
      trigger="click"
      placement="bottom-right"
      :disabled="busy"
      @click="onMenuSelect"
    >
      <button type="button" class="wiki-page-check__trigger" :disabled="busy">
        <t-icon :name="busy ? 'loading' : 'search'" />
        <span>{{ busy ? statusText : $t('knowledgeEditor.wikiBrowser.pageCheckStart') }}</span>
        <t-icon v-if="!busy" name="chevron-down" class="wiki-page-check__caret" />
      </button>
    </t-dropdown>

    <t-progress
      v-if="busy"
      class="wiki-page-check__progress"
      :percentage="run?.progress || 0"
      theme="line"
      size="small"
      :label="false"
    />
    <span v-else-if="resultText" class="wiki-page-check__result" :class="{ 'is-error': isFailed }">
      {{ resultText }}
    </span>
  </div>
</template>

<script setup lang="ts">
/**
 * "Check this page" for the page a user is currently reading.
 *
 * Before this existed the only way to re-examine one page was to scan the whole
 * wiki, which is both slow and — once the AI review is involved — a much larger
 * model spend than the question deserves. It runs on the same durable lint-run
 * machinery as a full scan, so results land in the problem centre and on this
 * page's own issue list through one code path.
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { getWikiLintRun, startWikiPageCheck, type WikiLintMode, type WikiLintRun } from '@/api/wiki'

const props = defineProps<{
  kbId: string
  slug: string
  /** Whether the knowledge base has a model configured for the AI review. */
  aiAvailable: boolean
}>()

const emit = defineEmits<{ (event: 'completed', run: WikiLintRun): void }>()

const { t } = useI18n()

const run = ref<WikiLintRun | null>(null)
let pollTimer: ReturnType<typeof setInterval> | null = null

const busy = computed(() => run.value?.status === 'queued' || run.value?.status === 'running')
const isFailed = computed(() => run.value?.status === 'failed')

const menuOptions = computed(() => [
  {
    content: t('knowledgeEditor.wikiBrowser.scanModeStatic'),
    value: 'static',
  },
  {
    content: t('knowledgeEditor.wikiBrowser.scanModeAi'),
    value: 'ai',
    disabled: !props.aiAvailable,
  },
  {
    content: t('knowledgeEditor.wikiBrowser.scanModeFull'),
    value: 'full',
    disabled: !props.aiAvailable,
  },
])

const statusText = computed(() => {
  if (run.value?.status === 'queued') return t('knowledgeEditor.wikiBrowser.lintQueued')
  return t('knowledgeEditor.wikiBrowser.pageCheckRunning')
})

const resultText = computed(() => {
  const current = run.value
  if (!current) return ''
  if (current.status === 'failed') {
    return current.error_message || t('knowledgeEditor.wikiBrowser.lintFailed')
  }
  if (current.status !== 'completed') return ''
  // An unchanged page that was already reviewed is reported as skipped rather
  // than as "clean", so a user is never told a review happened when the point
  // was that it did not have to.
  if (current.ai_pages_skipped > 0 && current.ai_pages_scanned === 0) {
    return t('knowledgeEditor.wikiBrowser.pageCheckUnchanged')
  }
  if ((current.finding_count ?? 0) === 0) {
    return t('knowledgeEditor.wikiBrowser.pageCheckClean')
  }
  return t('knowledgeEditor.wikiBrowser.pageCheckFindings', { count: current.finding_count })
})

// A different page is a different question; drop any result from the last one
// so the bar never attributes findings to the page now on screen.
watch(
  () => props.slug,
  () => {
    stopPolling()
    run.value = null
  },
)

onBeforeUnmount(stopPolling)

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function unwrap(response: unknown): WikiLintRun {
  return ((response as { data?: WikiLintRun }).data || response) as WikiLintRun
}

async function onMenuSelect(option: { value?: string | number }) {
  const mode = String(option?.value || 'static') as WikiLintMode
  if (busy.value) return
  try {
    run.value = unwrap(await startWikiPageCheck(props.kbId, props.slug, mode))
    startPolling()
  } catch (e: any) {
    MessagePlugin.error(e?.message || t('knowledgeEditor.wikiBrowser.pageCheckFailed'))
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(pollRun, 1500)
}

async function pollRun() {
  const runId = run.value?.id
  if (!runId) return
  try {
    run.value = unwrap(await getWikiLintRun(props.kbId, runId))
    if (!busy.value) {
      stopPolling()
      emit('completed', run.value)
    }
  } catch (e) {
    console.error('Failed to poll Wiki page check:', e)
  }
}
</script>

<style scoped lang="less">
.wiki-page-check {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.wiki-page-check__trigger {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  border: 1px solid var(--td-component-stroke);
  border-radius: 4px;
  background: var(--td-bg-color-container);
  cursor: pointer;

  &:hover:not(:disabled) {
    color: var(--td-brand-color);
    border-color: var(--td-brand-color);
  }

  &:disabled {
    cursor: default;
    opacity: 0.7;
  }
}

.wiki-page-check__caret {
  color: var(--td-text-color-placeholder);
}

.wiki-page-check__progress {
  width: 72px;
}

.wiki-page-check__result {
  font-size: 12px;
  color: var(--td-text-color-placeholder);

  &.is-error {
    color: var(--td-error-color);
  }
}
</style>
