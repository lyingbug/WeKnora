<template>
  <div class="memory-settings">
    <div class="memory-head">
      <div class="memory-head__text">
        <h2>{{ t('memory.title') }}</h2>
        <p class="memory-head__desc">{{ t('memory.subtitleShort') }}</p>
      </div>
      <div class="memory-head__actions">
        <t-tooltip :content="t('memory.actions.openSettings')" placement="top">
          <t-button variant="outline" shape="square" size="medium" @click="openSettings">
            <template #icon><t-icon name="setting" /></template>
          </t-button>
        </t-tooltip>
        <t-button v-if="activeTab !== 'graph'" theme="primary" size="medium" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          {{ t('memory.actions.create') }}
        </t-button>
      </div>
    </div>

    <div v-if="hint" class="memory-banner" role="note">
      <t-icon name="info-circle" class="memory-banner__icon" />
      <span class="memory-banner__text">
        {{ hint }}
        <t-link theme="primary" hover="color" @click="openSettings">
          {{ t('memory.capture.openSettings') }}
        </t-link>
      </span>
    </div>

    <template v-if="!disabled">
      <t-tabs v-model="activeTab" class="memory-tabs">
        <t-tab-panel value="memories" :label="memoryTabLabel" />
        <t-tab-panel value="inbox" :label="inboxTabLabel" />
        <t-tab-panel value="graph" :label="t('memory.tabs.graph')" />
      </t-tabs>

      <div class="memory-body" :class="{ 'memory-body--graph': activeTab === 'graph' }">
        <MemoryList v-if="activeTab === 'memories'" :key="`list-${refreshKey}`" @edit="openEdit"
          @changed="refreshStats" />
        <MemoryInbox v-else-if="activeTab === 'inbox'" :key="`inbox-${refreshKey}`" @changed="refreshAll" />
        <MemoryGraphPanel v-else :key="`graph-${refreshKey}`" @edit="openEdit" />
      </div>
    </template>

    <MemoryPersonalSettingsDrawer v-model:visible="settingsDrawerVisible" @changed="onSettingsChanged"
      @forgot-all="refreshAll" />
    <MemoryEditorDrawer v-model:visible="editorVisible" :slug="editorSlug" @saved="onEditorSaved" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

import { getMemorySpace, getMemorySettings, type MemoryStats } from '@/api/memory'
import MemoryList from '../memory/components/MemoryList.vue'
import MemoryInbox from '../memory/components/MemoryInbox.vue'
import MemoryGraphPanel from '../memory/components/MemoryGraphPanel.vue'
import MemoryEditorDrawer from '../memory/components/MemoryEditorDrawer.vue'
import MemoryPersonalSettingsDrawer from '../memory/components/MemoryPersonalSettingsDrawer.vue'

const { t } = useI18n()

const activeTab = ref('memories')
const disabled = ref(false)
const refreshKey = ref(0)
const editorVisible = ref(false)
const editorSlug = ref('')
const settingsDrawerVisible = ref(false)
const writeMode = ref('')

const emptyStats: MemoryStats = {
  total_pages: 0,
  active_pages: 0,
  archived_pages: 0,
  pending_notes: 0,
  total_anchors: 0,
  by_type: {},
}
const stats = ref<MemoryStats>({ ...emptyStats })

const memoryTabLabel = computed(() =>
  t('memory.tabs.memoriesCount', { count: stats.value.active_pages }),
)
const inboxTabLabel = computed(() =>
  t('memory.tabs.inboxCount', { count: stats.value.pending_notes }),
)

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
    // optional
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

function openSettings() {
  settingsDrawerVisible.value = true
}

function openCreate() {
  editorSlug.value = ''
  editorVisible.value = true
}

function openEdit(slug: string) {
  editorSlug.value = slug
  editorVisible.value = true
}

function onEditorSaved() {
  editorVisible.value = false
  refreshAll()
}

onMounted(() => {
  refreshStats()
  refreshWriteMode()
})
</script>

<style scoped lang="less">
.memory-settings {
  width: 100%;
  min-height: 0;
}

.memory-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 24px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 6px;
    line-height: 1.3;
  }

  &__desc {
    margin: 0;
    font-size: 14px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
    max-width: 52ch;
  }

  &__actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }
}

.memory-banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 20px;
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);

  &__icon {
    flex-shrink: 0;
    margin-top: 2px;
    font-size: 16px;
    color: var(--td-text-color-placeholder);
  }

  &__text {
    font-size: 13px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }
}

.memory-tabs {
  margin-bottom: 20px;

  :deep(.t-tabs__nav-item) {
    font-size: 14px;
  }

  :deep(.t-tabs__nav-item-wrapper) {
    padding: 0 14px;
    margin: 0;
  }

  :deep(.t-tabs__operations) {
    display: none;
  }

  :deep(.t-tabs__nav-scroll) {
    overflow-x: auto;
    scrollbar-width: none;

    &::-webkit-scrollbar {
      display: none;
    }
  }

  :deep(.t-tabs__content) {
    display: none;
  }
}

.memory-body {
  min-height: 0;

  &--graph {
    margin-top: 0;
  }
}
</style>
