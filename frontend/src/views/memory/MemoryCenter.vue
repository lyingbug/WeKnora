<template>
  <div class="memory-center-container">
    <div class="memory-center-content">
      <div class="header" style="--wails-draggable: drag">
        <div class="header-title" style="--wails-draggable: drag">
          <div class="title-row" style="--wails-draggable: drag">
            <h2 style="--wails-draggable: drag">{{ t('memory.title') }}</h2>

            <t-tooltip v-if="!disabled" :content="t('memory.actions.create')" placement="bottom">
              <t-button variant="text" theme="default" size="small" class="header-action-btn"
                style="--wails-draggable: no-drag" @click="openCreate">
                <template #icon><t-icon name="add" /></template>
              </t-button>
            </t-tooltip>

            <t-tooltip v-if="!disabled" :content="t('memory.actions.export')" placement="bottom">
              <t-button variant="text" theme="default" size="small" class="header-action-btn"
                style="--wails-draggable: no-drag" @click="exportMemory">
                <template #icon><t-icon name="download" /></template>
              </t-button>
            </t-tooltip>

            <!-- Settings live where the layer they belong to lives: a person's
                 own preferences under account settings, the workspace policy
                 under workspace settings, and the per-agent overrides in the
                 agent editor. This page only links to them. -->
            <t-tooltip :content="t('memory.actions.openSettings')" placement="bottom">
              <t-button variant="text" theme="default" size="small" class="header-action-btn"
                style="--wails-draggable: no-drag" @click="openSettings">
                <template #icon><t-icon name="setting" /></template>
              </t-button>
            </t-tooltip>

            <t-tooltip v-if="!disabled" :content="t('memory.actions.forgetAll')" placement="bottom">
              <t-button variant="text" theme="danger" size="small" class="header-action-btn"
                style="--wails-draggable: no-drag" @click="confirmForgetAll">
                <template #icon><t-icon name="delete" /></template>
              </t-button>
            </t-tooltip>
          </div>
          <p class="header-subtitle" style="--wails-draggable: drag">{{ t('memory.subtitle') }}</p>
        </div>

        <div v-if="!disabled" class="header-meta">
          <span>{{ t('memory.stats.active') }} <b>{{ stats.active_pages }}</b></span>
          <span>{{ t('memory.stats.anchors') }} <b>{{ stats.total_anchors }}</b></span>
          <span v-if="stats.archived_pages > 0">
            {{ t('memory.stats.archived') }} <b>{{ stats.archived_pages }}</b>
          </span>
        </div>
      </div>

      <div class="memory-center-main">
        <t-alert v-if="disabled" theme="info" :message="t('memory.disabled.message')">
          <template #operation>
            <t-link theme="primary" hover="color" @click="openSettings">
              {{ t('memory.disabled.openSettings') }}
            </t-link>
          </template>
        </t-alert>

        <template v-else>
          <!-- What gets captured is the first question an empty memory list
               raises, and the answer lives three clicks away in settings. -->
          <t-alert v-if="captureHint" theme="info" class="memory-capture-hint">
            <template #message>
              {{ captureHint }}
              <t-link theme="primary" hover="color" @click="openSettings">
                {{ t('memory.capture.openSettings') }}
              </t-link>
            </template>
          </t-alert>

          <t-tabs v-model="activeTab" class="memory-tabs">
            <t-tab-panel value="memories" :label="t('memory.tabs.memories')">
              <MemoryList :key="`list-${refreshKey}`" @edit="openEdit" @changed="refreshStats" />
            </t-tab-panel>

            <t-tab-panel value="inbox">
              <template #label>
                <t-badge :count="stats.pending_notes" :max-count="99" :offset="[-6, 2]">
                  {{ t('memory.tabs.inbox') }}
                </t-badge>
              </template>
              <MemoryInbox :key="`inbox-${refreshKey}`" @changed="refreshAll" />
            </t-tab-panel>

            <t-tab-panel value="graph" :label="t('memory.tabs.graph')">
              <MemoryGraphPanel :key="`graph-${refreshKey}`" @open="openBySlug" />
            </t-tab-panel>
          </t-tabs>
        </template>
      </div>
    </div>

    <MemoryEditorDialog v-model:visible="editorVisible" :slug="editorSlug" @saved="onEditorSaved" />
  </div>
</template>

<script setup lang="ts">
/**
 * The memory centre: where a person reads, edits and prunes what is remembered
 * about them.
 *
 * Explicitly NOT where memory is configured. Settings belong to the layer that
 * owns them — personal preferences under account settings, workspace policy
 * under workspace settings, per-agent overrides in the agent editor — and
 * duplicating them here would give the same value two homes and leave the user
 * guessing which one wins.
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'

import {
  getMemorySpace,
  getMemorySettings,
  forgetMemories,
  exportMemoryUrl,
  type MemoryStats,
} from '@/api/memory'
import MemoryList from './components/MemoryList.vue'
import MemoryInbox from './components/MemoryInbox.vue'
import MemoryGraphPanel from './components/MemoryGraphPanel.vue'
import MemoryEditorDialog from './components/MemoryEditorDialog.vue'

const { t } = useI18n()
const router = useRouter()

const activeTab = ref('memories')
const disabled = ref(false)
const refreshKey = ref(0)
const editorVisible = ref(false)
const editorSlug = ref('')

const emptyStats: MemoryStats = {
  total_pages: 0,
  active_pages: 0,
  archived_pages: 0,
  pending_notes: 0,
  total_anchors: 0,
  by_type: {},
}
const stats = ref<MemoryStats>({ ...emptyStats })

// The effective write mode, resolved across every settings layer. Read only to
// explain how memories arrive here; changing it is settings' job.
const writeMode = ref('')

const captureHint = computed(() => {
  if (writeMode.value === 'off' || writeMode.value === 'explicit_only') {
    return t(`memory.capture.${writeMode.value}`)
  }
  return ''
})

async function refreshWriteMode() {
  try {
    const res: any = await getMemorySettings()
    writeMode.value = res?.data?.values?.['memory.write.mode']?.value || ''
  } catch {
    // The hint is an explanation, not a feature. Losing it changes nothing.
  }
}

async function refreshStats() {
  try {
    const res: any = await getMemorySpace()
    stats.value = res?.data?.stats || { ...emptyStats }
    disabled.value = false
  } catch (error: any) {
    // A 404 here is the ordinary "memory is switched off for me" state, not a
    // fault, so it gets an explanation and a way to change it.
    if (error?.response?.status === 404) {
      disabled.value = true
      return
    }
    MessagePlugin.error(t('memory.errors.loadFailed'))
  }
}

function refreshAll() {
  refreshKey.value += 1
  refreshStats()
  refreshWriteMode()
}

function openCreate() {
  editorSlug.value = ''
  editorVisible.value = true
}

function openEdit(slug: string) {
  editorSlug.value = slug
  editorVisible.value = true
}

function openBySlug(slug: string) {
  activeTab.value = 'memories'
  openEdit(slug)
}

function onEditorSaved() {
  editorVisible.value = false
  refreshAll()
}

function openSettings() {
  router.push({ path: '/platform/settings', query: { section: 'memory-personal' } })
}

function exportMemory() {
  window.open(exportMemoryUrl(), '_blank')
}

function confirmForgetAll() {
  const dialog = DialogPlugin.confirm({
    header: t('memory.forgetAll.header'),
    body: t('memory.forgetAll.body'),
    confirmBtn: { content: t('memory.forgetAll.confirm'), theme: 'danger' },
    onConfirm: async () => {
      try {
        // Forgetting everything clears the underlying observations too;
        // otherwise the next extraction would rebuild what was just deleted.
        await forgetMemories({ scope: 'all', purge_notes: true })
        MessagePlugin.success(t('memory.forgetAll.done'))
        refreshAll()
      } catch {
        MessagePlugin.error(t('memory.errors.forgetFailed'))
      } finally {
        dialog.destroy()
      }
    },
  })
}

onMounted(() => {
  refreshStats()
  refreshWriteMode()
})
</script>

<style scoped lang="less">
.memory-capture-hint {
  margin-bottom: 12px;
}

/* Mirrors the shell used by AgentList and KnowledgeBaseList: fill the content
   area, no max-width or centering, padding on the header and main rather than
   the container so a scrollbar sits flush with the right edge. */
.memory-center-container {
  margin: 0;
  height: 100%;
  box-sizing: border-box;
  flex: 1;
  display: flex;
  position: relative;
  min-height: 0;
}

.memory-center-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 20px 0 0 28px;
  overflow: hidden;
}

.memory-center-main {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 28px 8px 0;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
  padding-right: 28px;
}

.header-title {
  min-width: 0;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 2px;

  h2 {
    margin: 0 6px 0 0;
    font-size: 20px;
    font-weight: 600;
    line-height: 28px;
    color: var(--td-text-color-primary);
  }
}

.header-action-btn {
  color: var(--td-text-color-placeholder);

  &:hover {
    color: var(--td-brand-color);
  }
}

.header-subtitle {
  margin: 4px 0 0;
  font-size: 13px;
  line-height: 20px;
  color: var(--td-text-color-secondary);
  max-width: 720px;
}

/* Counts belong beside the title as a quiet line, not as a row of cards: no
   other list page in the app has stat cards, and four of them pushed the actual
   memories below the fold. */
.header-meta {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-shrink: 0;
  font-size: 12px;
  color: var(--td-text-color-secondary);

  b {
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.memory-tabs {
  background: transparent;
}
</style>
