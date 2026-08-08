<template>
  <div class="memory-settings">
    <div class="section-header">
      <div class="section-header__top">
        <div>
          <h2>{{ t('memory.title') }}</h2>
          <p class="section-description">{{ t('memory.subtitle') }}</p>
        </div>
        <t-button v-if="!disabled" theme="primary" variant="text" size="medium" class="header-action"
          @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('memory.actions.create') }}
        </t-button>
      </div>

      <div v-if="hint" class="memory-hint" role="note">
        <p class="memory-hint__label">{{ t('memory.capture.label') }}</p>
        <p class="memory-hint__text">{{ hint }}</p>
      </div>
    </div>

    <MemorySettingsPanel ref="panel" level="user" :show-group-titles="false" @changed="onSettingsChanged" />

    <template v-if="!disabled">
      <div class="memory-block">
        <div class="memory-block__head">
          <h3 class="memory-block__title">{{ t('memory.list.title') }}</h3>
          <span class="memory-block__meta">{{ t('memory.stats.summary', {
            active: stats.active_pages, anchors: stats.total_anchors,
          }) }}</span>
        </div>

        <t-tabs v-model="activeTab" class="memory-tabs">
          <t-tab-panel value="memories" :label="`${t('memory.tabs.memories')}(${stats.active_pages})`" />
          <t-tab-panel value="inbox" :label="`${t('memory.tabs.inbox')}(${stats.pending_notes})`" />
          <t-tab-panel value="graph" :label="t('memory.tabs.graph')" />
        </t-tabs>

        <MemoryList v-if="activeTab === 'memories'" :key="`list-${refreshKey}`" @edit="openEdit"
          @changed="refreshStats" />
        <MemoryInbox v-else-if="activeTab === 'inbox'" :key="`inbox-${refreshKey}`" @changed="refreshAll" />
        <MemoryGraphPanel v-else :key="`graph-${refreshKey}`" @open="openBySlug" />
      </div>

      <div class="memory-block">
        <h3 class="memory-block__title">{{ t('memory.data.title') }}</h3>
        <div class="settings-group">
          <div class="setting-row">
            <div class="setting-info">
              <label>{{ t('memory.data.exportLabel') }}</label>
              <p class="desc">{{ t('memory.data.exportDesc') }}</p>
            </div>
            <div class="setting-control">
              <t-button size="small" variant="outline" @click="exportMemory">
                {{ t('memory.actions.export') }}
              </t-button>
            </div>
          </div>
          <div class="setting-row">
            <div class="setting-info">
              <label>{{ t('memory.data.forgetLabel') }}</label>
              <p class="desc">{{ t('memory.data.forgetDesc') }}</p>
            </div>
            <div class="setting-control">
              <t-button size="small" theme="danger" variant="outline" @click="confirmForgetAll">
                {{ t('memory.actions.forgetAll') }}
              </t-button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <MemoryEditorDialog v-model:visible="editorVisible" :slug="editorSlug" @saved="onEditorSaved" />
  </div>
</template>

<script setup lang="ts">
/**
 * Long-term memory, from the point of view of the person it is about: what is
 * remembered, what is waiting to be confirmed, how it all connects, and the
 * handful of choices that govern any of it.
 *
 * All of it lives in one settings section rather than a page of its own. Memory
 * is personal data one reviews occasionally, like message history — it does not
 * earn a slot in the main navigation, and splitting "browse" from "configure"
 * across two surfaces was what made the feature hard to find and harder to
 * reason about.
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'

import { getMemorySpace, getMemorySettings, forgetMemories, exportMemories, type MemoryStats } from '@/api/memory'
import MemorySettingsPanel from '../memory/components/MemorySettingsPanel.vue'
import MemoryList from '../memory/components/MemoryList.vue'
import MemoryInbox from '../memory/components/MemoryInbox.vue'
import MemoryGraphPanel from '../memory/components/MemoryGraphPanel.vue'
import MemoryEditorDialog from '../memory/components/MemoryEditorDialog.vue'

const { t } = useI18n()

const activeTab = ref('memories')
const disabled = ref(false)
const refreshKey = ref(0)
const editorVisible = ref(false)
const editorSlug = ref('')
const writeMode = ref('')
const panel = ref<InstanceType<typeof MemorySettingsPanel> | null>(null)

const emptyStats: MemoryStats = {
  total_pages: 0,
  active_pages: 0,
  archived_pages: 0,
  pending_notes: 0,
  total_anchors: 0,
  by_type: {},
}
const stats = ref<MemoryStats>({ ...emptyStats })

// What gets captured is the first question an empty list raises, and the answer
// is otherwise buried in a dropdown further down the same screen.
const hint = computed(() => {
  if (disabled.value) return t('memory.disabled.message')
  if (writeMode.value === 'off' || writeMode.value === 'explicit_only') {
    return t(`memory.capture.${writeMode.value}`)
  }
  return ''
})

async function refreshStats() {
  try {
    const res = await getMemorySpace()
    stats.value = res?.data?.stats || { ...emptyStats }
    disabled.value = false
  } catch (error: any) {
    if (error?.response?.status === 404) {
      disabled.value = true
      return
    }
    MessagePlugin.error(t('memory.errors.loadFailed'))
  }
}

async function refreshWriteMode() {
  try {
    const res = await getMemorySettings()
    writeMode.value = res?.data?.values?.['memory.write.mode']?.value || ''
  } catch {
    // The hint is an explanation, not a feature. Losing it changes nothing.
  }
}

function refreshAll() {
  refreshKey.value += 1
  refreshStats()
}

function onSettingsChanged() {
  refreshWriteMode()
  refreshStats()
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

async function exportMemory() {
  try {
    const blob = await exportMemories()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `weknora-memory-${new Date().toISOString().slice(0, 10)}.json`
    link.click()
    URL.revokeObjectURL(url)
  } catch {
    MessagePlugin.error(t('memory.errors.exportFailed'))
  }
}

function confirmForgetAll() {
  const dialog = DialogPlugin.confirm({
    header: t('memory.forgetAll.header'),
    body: t('memory.forgetAll.body'),
    confirmBtn: { content: t('memory.forgetAll.confirm'), theme: 'danger' },
    onConfirm: async () => {
      try {
        // "all" purges the observations too, server-side, so a later
        // extraction cannot rebuild what was just deleted.
        await forgetMemories({ scope: 'all' })
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
.memory-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 4px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px 0;
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.6;
  }
}

.section-header__top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
}

.header-action {
  --td-bg-color-container-hover: transparent;
  flex-shrink: 0;
  padding-left: 0;
  padding-right: 0;
  font-weight: 600;

  &:hover,
  &:focus,
  &:active {
    background-color: transparent !important;
    color: var(--td-brand-color-hover);
  }
}

/* The quiet note used elsewhere in this dialog for standing explanations
   (see the built-in models hint in ModelSettings.vue), rather than a coloured
   alert — this is context, not an event. */
.memory-hint {
  margin-top: 16px;
  padding: 10px 12px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;

  &__label {
    margin: 0 0 4px 0;
    font-size: 12px;
    font-weight: 500;
    color: var(--td-text-color-placeholder);
    letter-spacing: 0.02em;
  }

  &__text {
    margin: 0;
    font-size: 13px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }
}

.memory-block {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid var(--td-component-stroke);

  &__head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
  }

  &__title {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 16px 0;
  }

  &__meta {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }
}

.memory-tabs {
  margin-bottom: 16px;

  :deep(.t-tabs__nav-item) {
    font-size: 13px;
  }

  :deep(.t-tabs__nav-item-wrapper) {
    padding: 0 12px;
    margin: 0;
  }

  :deep(.t-tabs__operations) {
    display: none;
  }

  :deep(.t-tabs__content) {
    display: none;
  }
}

.settings-group {
  display: flex;
  flex-direction: column;
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 1;
  max-width: 65%;
  padding-right: 24px;

  label {
    font-size: 15px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 4px;
  }

  .desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex-shrink: 0;
  min-width: 280px;
  display: flex;
  justify-content: flex-end;
  align-items: center;
}
</style>
