<template>
  <div class="memory-settings">
    <div class="section-header">
      <h2>{{ t('memorySettings.title') }}</h2>
      <p class="section-description">{{ t('memorySettings.description') }}</p>
    </div>

    <!-- Workspace switch is off: say so plainly instead of showing a personal
         toggle that would appear to work and change nothing. -->
    <div v-if="settings && !settings.workspace_enabled" class="notice">
      <t-icon name="info-circle" />
      <span>{{ t('memorySettings.workspaceDisabled') }}</span>
    </div>

    <div class="settings-group">
      <div class="setting-row">
        <div class="setting-info">
          <label>{{ t('memorySettings.enableLabel') }}</label>
          <p class="desc">{{ t('memorySettings.enableDescription') }}</p>
        </div>
        <div class="setting-control">
          <t-switch
            v-model="userEnabled"
            :disabled="!settings || !settings.workspace_enabled"
            @change="handleEnabledChange"
          />
        </div>
      </div>
    </div>

    <div class="list-section">
      <div class="list-header">
        <div class="list-title">
          <h3>{{ t('memorySettings.listTitle') }}</h3>
          <span class="list-count">{{ t('memorySettings.listCount', { count: totalAll }) }}</span>
        </div>
        <div class="list-actions">
          <t-popup
            v-model="addVisible"
            trigger="click"
            placement="bottom-end"
            destroy-on-close
            overlay-class-name="memory-add-popup-overlay"
          >
            <t-button size="small" variant="text" :disabled="!canWrite">
              <template #icon><t-icon name="add" /></template>
              {{ t('memorySettings.add') }}
            </t-button>
            <template #content>
              <div class="add-popup" @click.stop>
                <div class="add-popup-title">{{ t('memorySettings.addTitle') }}</div>
                <label class="add-field">
                  <span class="add-label">{{ t('memorySettings.addKindLabel') }}</span>
                  <t-select
                    v-model="draftKind"
                    size="small"
                    :popup-props="{ overlayClassName: 'memory-add-kind-popup' }"
                  >
                    <t-option v-for="kind in kinds" :key="kind" :value="kind" :label="kindLabel(kind)" />
                  </t-select>
                </label>
                <label class="add-field">
                  <span class="add-label">{{ t('memorySettings.addContentLabel') }}</span>
                  <t-textarea
                    v-model="draftContent"
                    :placeholder="t('memorySettings.addPlaceholder')"
                    :maxlength="300"
                    :autosize="{ minRows: 3, maxRows: 6 }"
                  />
                </label>
                <div class="add-popup-footer">
                  <t-button size="small" variant="outline" @click="addVisible = false">
                    {{ t('common.cancel') }}
                  </t-button>
                  <t-button size="small" theme="primary" :disabled="!draftContent.trim()" @click="handleCreate">
                    {{ t('memorySettings.add') }}
                  </t-button>
                </div>
              </div>
            </template>
          </t-popup>
          <t-button size="small" variant="text" @click="handleExport">
            <template #icon><t-icon name="download" /></template>
            {{ t('memorySettings.export') }}
          </t-button>
          <t-popconfirm
            theme="danger"
            :content="t('memorySettings.clearConfirm')"
            :confirm-btn="{ content: t('memorySettings.clear'), theme: 'danger' }"
            :cancel-btn="t('common.cancel')"
            placement="left"
            @confirm="handleClear"
          >
            <t-button size="small" theme="danger" variant="text" :disabled="totalAll === 0">
              <template #icon><t-icon name="delete" /></template>
              {{ t('memorySettings.clear') }}
            </t-button>
          </t-popconfirm>
        </div>
      </div>

      <t-tabs :value="status" class="status-tabs" @change="handleStatusChange">
        <t-tab-panel v-for="value in statuses" :key="value" :value="value" :label="statusLabel(value)" />
      </t-tabs>

      <t-loading :loading="loading">
        <div v-if="items.length === 0" class="empty">
          <p class="empty-title">
            {{ status === 'pending' ? t('memorySettings.pendingEmptyTitle') : t('memorySettings.emptyTitle') }}
          </p>
          <p class="empty-desc">
            {{
              status === 'pending'
                ? t('memorySettings.pendingEmptyDescription')
                : t('memorySettings.emptyDescription')
            }}
          </p>
        </div>
        <p v-if="status === 'pending' && items.length > 0" class="pending-hint">
          {{ t('memorySettings.pendingHint') }}
        </p>
        <ul v-if="items.length > 0" class="memory-list">
          <li v-for="item in items" :key="item.id" class="memory-item">
            <div class="memory-main">
              <div class="memory-meta">
                <t-tag size="small" :theme="kindTheme(item.kind)" variant="light">
                  {{ kindLabel(item.kind) }}
                </t-tag>
                <!-- An interest is promoted from a subject label, so its topic
                     and content are the same string; showing both reads as a
                     rendering bug. -->
                <span v-if="item.topic && item.topic !== item.content" class="memory-topic">
                  {{ item.topic }}
                </span>
                <t-tag size="small" variant="outline" theme="default">
                  {{ originLabel(item.origin) }}
                </t-tag>
                <span class="memory-time">{{ formatTime(item.valid_from) }}</span>
              </div>
              <div v-if="editingId === item.id" class="memory-edit">
                <t-input v-model="editingContent" size="small" @enter="handleSaveEdit(item)" />
                <t-button size="small" @click="handleSaveEdit(item)">{{ t('common.save') }}</t-button>
                <t-button size="small" theme="default" variant="text" @click="editingId = ''">
                  {{ t('common.cancel') }}
                </t-button>
              </div>
              <p v-else class="memory-content" :class="{ inactive: item.status !== 'active' }">
                {{ item.content }}
              </p>
            </div>
            <div class="memory-actions">
              <template v-if="item.status === 'pending'">
                <t-button
                  size="small"
                  theme="primary"
                  variant="text"
                  :disabled="!canWrite"
                  @click="handleConfirm(item)"
                >
                  {{ t('memorySettings.confirmGuess') }}
                </t-button>
                <t-button size="small" theme="default" variant="text" @click="handleReject(item)">
                  {{ t('memorySettings.rejectGuess') }}
                </t-button>
              </template>
              <t-button
                v-if="item.status === 'active'"
                size="small"
                theme="default"
                variant="text"
                shape="square"
                :disabled="!canWrite"
                @click="startEdit(item)"
              >
                <template #icon><t-icon name="edit" /></template>
              </t-button>
              <t-popconfirm
                theme="danger"
                :content="t('memorySettings.deleteConfirm')"
                :confirm-btn="{ content: t('common.delete'), theme: 'danger' }"
                :cancel-btn="t('common.cancel')"
                placement="left"
                @confirm="handleDelete(item)"
              >
                <t-button size="small" theme="danger" variant="text" shape="square">
                  <template #icon><t-icon name="delete" /></template>
                </t-button>
              </t-popconfirm>
            </div>
          </li>
        </ul>
      </t-loading>

      <t-pagination
        v-if="total > pageSize"
        class="memory-pagination"
        :total="total"
        :page-size="pageSize"
        :current="page"
        :show-jumper="false"
        :show-page-size="false"
        @current-change="handlePageChange"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import {
  clearMemoryItems,
  createMemoryItem,
  confirmMemoryItem,
  deleteMemoryItem,
  exportMemoryItems,
  getMemorySettings,
  listMemoryItems,
  rejectMemoryItem,
  updateMemoryEnabled,
  updateMemoryItem,
  type MemoryItem,
  type MemoryKind,
  type MemorySettings,
  type MemoryStatus,
} from '@/api/memory'

const { t } = useI18n()

const settings = ref<MemorySettings | null>(null)
const userEnabled = ref(false)
const items = ref<MemoryItem[]>([])
const total = ref(0)
const status = ref<MemoryStatus>('active')
const loading = ref(false)
const page = ref(1)
const pageSize = 20

const draftKind = ref<MemoryKind>('fact')
const draftContent = ref('')
const addVisible = ref(false)
const editingId = ref('')
const editingContent = ref('')
const editingImportance = ref(3)

const kinds: MemoryKind[] = ['profile', 'preference', 'fact', 'task', 'interest']

const statuses: MemoryStatus[] = ['active', 'pending', 'superseded', 'archived']
const statusLabelKeys: Record<MemoryStatus, string> = {
  active: 'memorySettings.statusActive',
  pending: 'memorySettings.statusPending',
  superseded: 'memorySettings.statusSuperseded',
  archived: 'memorySettings.statusArchived',
}
const counts = ref<Record<MemoryStatus, number>>({
  active: 0,
  pending: 0,
  superseded: 0,
  archived: 0,
})

const statusLabel = (value: MemoryStatus) => `${t(statusLabelKeys[value])}(${counts.value[value]})`

const totalAll = computed(() => statuses.reduce((sum, value) => sum + counts.value[value], 0))

// Writing requires both switches; the list itself stays readable either way so
// a user who just turned memory off can still review and delete what is stored.
const canWrite = computed(() => settings.value?.effective === true)

const kindLabel = (kind: MemoryKind) => t(`memorySettings.kinds.${kind}`)

const kindTheme = (kind: MemoryKind) => {
  switch (kind) {
    case 'profile':
      return 'primary'
    case 'preference':
      return 'success'
    case 'task':
      return 'warning'
    case 'interest':
      return 'primary'
    default:
      return 'default'
  }
}

const originLabel = (origin: MemoryItem['origin']) => t(`memorySettings.origins.${origin}`)

const formatTime = (value: string) => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString()
}

const loadSettings = async () => {
  try {
    const response = await getMemorySettings()
    settings.value = response.data
    userEnabled.value = response.data.user_enabled
  } catch (error: any) {
    console.error('Failed to load memory settings:', error)
  }
}

const loadItems = async () => {
  loading.value = true
  try {
    const response = await listMemoryItems({
      status: status.value,
      limit: pageSize,
      offset: (page.value - 1) * pageSize,
    })
    items.value = response.data || []
    total.value = response.total || 0
  } catch (error: any) {
    console.error('Failed to load memories:', error)
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

// Counts are loaded per status so every tab carries its own size, which is how
// a user finds out an inference is waiting for them without switching tabs.
const loadCounts = async () => {
  const totals = await Promise.all(
    statuses.map(async (value) => {
      try {
        const response = await listMemoryItems({ status: value, limit: 1 })
        return response.total || 0
      } catch (error: any) {
        return 0
      }
    }),
  )
  statuses.forEach((value, index) => {
    counts.value[value] = totals[index]
  })
}

const reload = async () => {
  page.value = 1
  await Promise.all([loadItems(), loadCounts()])
}

const handleConfirm = async (item: MemoryItem) => {
  try {
    await confirmMemoryItem(item.id)
    MessagePlugin.success(t('memorySettings.confirmSuccess'))
    await reload()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('memorySettings.confirmFailed'))
  }
}

const handleReject = async (item: MemoryItem) => {
  try {
    await rejectMemoryItem(item.id)
    MessagePlugin.success(t('memorySettings.rejectSuccess'))
    await reload()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('memorySettings.rejectFailed'))
  }
}

const handleStatusChange = async (value: string | number) => {
  status.value = value as MemoryStatus
  await reload()
}

const handlePageChange = async (current: number) => {
  page.value = current
  await loadItems()
}

const handleEnabledChange = async (value: boolean) => {
  try {
    const response = await updateMemoryEnabled(value)
    settings.value = response.data
    userEnabled.value = response.data.user_enabled
    MessagePlugin.success(
      value ? t('memorySettings.toasts.enabled') : t('memorySettings.toasts.disabled'),
    )
  } catch (error: any) {
    userEnabled.value = !value
    MessagePlugin.error(t('memorySettings.toasts.saveFailed', { message: error?.message || '' }))
  }
}

watch(addVisible, (visible) => {
  if (visible) draftContent.value = ''
})

const handleCreate = async () => {
  const content = draftContent.value.trim()
  if (!content) return
  try {
    await createMemoryItem({ kind: draftKind.value, content })
    draftContent.value = ''
    addVisible.value = false
    status.value = 'active'
    await reload()
    await loadSettings()
    MessagePlugin.success(t('memorySettings.toasts.added'))
  } catch (error: any) {
    MessagePlugin.error(t('memorySettings.toasts.saveFailed', { message: error?.message || '' }))
  }
}

const startEdit = (item: MemoryItem) => {
  editingId.value = item.id
  editingContent.value = item.content
  editingImportance.value = item.importance
}

const handleSaveEdit = async (item: MemoryItem) => {
  const content = editingContent.value.trim()
  if (!content) return
  try {
    await updateMemoryItem(item.id, { content, importance: editingImportance.value })
    editingId.value = ''
    await loadItems()
    MessagePlugin.success(t('memorySettings.toasts.updated'))
  } catch (error: any) {
    MessagePlugin.error(t('memorySettings.toasts.saveFailed', { message: error?.message || '' }))
  }
}

const handleDelete = async (item: MemoryItem) => {
  try {
    await deleteMemoryItem(item.id)
    await Promise.all([loadItems(), loadCounts()])
    await loadSettings()
    MessagePlugin.success(t('memorySettings.toasts.deleted'))
  } catch (error: any) {
    MessagePlugin.error(t('memorySettings.toasts.saveFailed', { message: error?.message || '' }))
  }
}

const handleClear = async () => {
  try {
    const response = await clearMemoryItems()
    await reload()
    await loadSettings()
    MessagePlugin.success(t('memorySettings.toasts.cleared', { count: response.removed || 0 }))
  } catch (error: any) {
    MessagePlugin.error(t('memorySettings.toasts.saveFailed', { message: error?.message || '' }))
  }
}

const handleExport = async () => {
  try {
    const response = await exportMemoryItems()
    const blob = new Blob([JSON.stringify(response.data || [], null, 2)], {
      type: 'application/json',
    })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'weknora-memories.json'
    link.click()
    URL.revokeObjectURL(url)
  } catch (error: any) {
    MessagePlugin.error(t('memorySettings.toasts.saveFailed', { message: error?.message || '' }))
  }
}

onMounted(async () => {
  await loadSettings()
  await Promise.all([loadItems(), loadCounts()])
})
</script>

<style lang="less" scoped>
.pending-hint {
  margin: 0 0 12px;
  color: var(--td-text-color-secondary, #86909c);
  font-size: 12px;
  line-height: 18px;
}

.memory-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 24px;

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
    line-height: 1.5;
  }
}

.notice {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
  border-radius: 6px;
  background: var(--td-warning-color-1);
  color: var(--td-text-color-primary);
  font-size: 13px;
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
  display: flex;
  justify-content: flex-end;
  align-items: center;
}

.list-section {
  margin-top: 24px;
}

.list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.list-title {
  display: flex;
  align-items: baseline;
  gap: 8px;

  h3 {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0;
  }
}

.list-count {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}

.list-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.status-tabs {
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


.memory-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
}

.memory-item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.memory-main {
  flex: 1;
  min-width: 0;
}

.memory-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 6px;
}

.memory-topic {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.memory-time {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.memory-content {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
  color: var(--td-text-color-primary);
  word-break: break-word;

  &.inactive {
    color: var(--td-text-color-placeholder);
    text-decoration: line-through;
  }
}

.memory-edit {
  display: flex;
  align-items: center;
  gap: 8px;
}

.memory-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.memory-pagination {
  margin-top: 16px;
}

.empty {
  border: 1px dashed var(--td-component-stroke);
  border-radius: 8px;
  padding: 32px;
  text-align: center;
}

.empty-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  margin: 0 0 4px 0;
}

.empty-desc {
  font-size: 13px;
  color: var(--td-text-color-placeholder);
  margin: 0;
}
</style>

<!-- t-popup renders into body, so the add form has to be styled globally. -->
<style lang="less">
.memory-add-popup-overlay {
  z-index: 3050;

  .t-popup__content {
    padding: 14px 16px !important;
    width: 320px;
    max-width: calc(100vw - 24px);
    border-radius: 12px !important;
    background: var(--td-bg-color-container) !important;
    border: 0.5px solid var(--td-component-stroke) !important;
    box-shadow:
      0 0 0 0.5px rgba(0, 0, 0, 0.03),
      0 2px 4px rgba(0, 0, 0, 0.04),
      0 8px 24px rgba(0, 0, 0, 0.1) !important;
  }

  .add-popup {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .add-popup-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .add-field {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .add-label {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }

  .add-popup-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
}

/* The kind dropdown mounts to body too, above the popup that opened it. */
.memory-add-kind-popup {
  z-index: 6200;
}

:root[theme-mode='dark'] .memory-add-popup-overlay .t-popup__content {
  background: rgba(36, 36, 36, 0.92) !important;
  border-color: rgba(255, 255, 255, 0.08) !important;
  box-shadow:
    0 0 0 0.5px rgba(255, 255, 255, 0.05),
    0 2px 4px rgba(0, 0, 0, 0.12),
    0 8px 32px rgba(0, 0, 0, 0.28) !important;
}
</style>
