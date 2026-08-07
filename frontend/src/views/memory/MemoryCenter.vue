<template>
  <div class="memory-center">
    <header class="memory-center__header">
      <div class="memory-center__title">
        <h1>{{ t('memory.title') }}</h1>
        <p>{{ t('memory.subtitle') }}</p>
      </div>
      <div class="memory-center__actions">
        <t-button variant="outline" @click="exportMemory">
          <template #icon><t-icon name="download" /></template>
          {{ t('memory.actions.export') }}
        </t-button>
        <t-button theme="danger" variant="outline" @click="confirmForgetAll">
          <template #icon><t-icon name="delete" /></template>
          {{ t('memory.actions.forgetAll') }}
        </t-button>
        <t-button theme="primary" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('memory.actions.create') }}
        </t-button>
      </div>
    </header>

    <t-alert
      v-if="disabled"
      theme="info"
      class="memory-center__notice"
      :message="t('memory.disabled.message')"
    />

    <div v-else class="memory-center__body">
      <div class="memory-center__stats">
        <div class="memory-stat">
          <span class="memory-stat__value">{{ stats.active_pages }}</span>
          <span class="memory-stat__label">{{ t('memory.stats.active') }}</span>
        </div>
        <div class="memory-stat">
          <span class="memory-stat__value">{{ stats.pending_notes }}</span>
          <span class="memory-stat__label">{{ t('memory.stats.pending') }}</span>
        </div>
        <div class="memory-stat">
          <span class="memory-stat__value">{{ stats.total_anchors }}</span>
          <span class="memory-stat__label">{{ t('memory.stats.anchors') }}</span>
        </div>
        <div class="memory-stat">
          <span class="memory-stat__value">{{ stats.archived_pages }}</span>
          <span class="memory-stat__label">{{ t('memory.stats.archived') }}</span>
        </div>
      </div>

      <t-tabs v-model="activeTab" class="memory-center__tabs">
        <t-tab-panel value="memories" :label="t('memory.tabs.memories')">
          <MemoryList
            :key="`list-${refreshKey}`"
            @edit="openEdit"
            @changed="refreshStats"
          />
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

        <t-tab-panel value="settings" :label="t('memory.tabs.settings')">
          <MemorySettingsPanel level="user" @saved="refreshAll" />
        </t-tab-panel>
      </t-tabs>
    </div>

    <MemoryEditorDialog
      v-model:visible="editorVisible"
      :slug="editorSlug"
      @saved="onEditorSaved"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * The memory centre.
 *
 * Everything a person can do with their own memory lives on one page, because
 * the feature only earns trust if "what does it know about me, and how do I
 * change it" has a single obvious answer. Four tabs, in the order people ask
 * the questions: what is remembered, what is waiting for my approval, how it
 * all connects, and what it is allowed to do.
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'

import { getMemorySpace, forgetMemories, exportMemoryUrl, type MemoryStats } from '@/api/memory'
import MemoryList from './components/MemoryList.vue'
import MemoryInbox from './components/MemoryInbox.vue'
import MemoryGraphPanel from './components/MemoryGraphPanel.vue'
import MemorySettingsPanel from './components/MemorySettingsPanel.vue'
import MemoryEditorDialog from './components/MemoryEditorDialog.vue'

const { t } = useI18n()

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

async function refreshStats() {
  try {
    const res: any = await getMemorySpace()
    stats.value = res?.data?.stats || { ...emptyStats }
    disabled.value = false
  } catch (error: any) {
    // A 404 here is the ordinary "memory is switched off for me" state, not a
    // fault, so it gets an explanation rather than an error toast.
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

onMounted(refreshStats)
</script>

<style scoped lang="less">
.memory-center {
  padding: 24px 32px 40px;
  max-width: 1240px;
  margin: 0 auto;

  &__header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 20px;
  }

  &__title {
    h1 {
      margin: 0;
      font-size: 22px;
      font-weight: 600;
      color: var(--td-text-color-primary, #000);
    }

    p {
      margin: 6px 0 0;
      font-size: 13px;
      color: var(--td-text-color-secondary, #666);
      max-width: 620px;
      line-height: 1.6;
    }
  }

  &__actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  &__notice {
    margin-top: 12px;
  }

  &__stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
    gap: 12px;
    margin-bottom: 16px;
  }

  &__tabs {
    background: var(--td-bg-color-container, #fff);
    border-radius: 10px;
  }
}

.memory-stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 14px 16px;
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 10px;

  &__value {
    font-size: 22px;
    font-weight: 600;
    line-height: 1.2;
    color: var(--td-text-color-primary, #000);
  }

  &__label {
    font-size: 12px;
    color: var(--td-text-color-secondary, #888);
  }
}
</style>
