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
          <span class="list-count">{{ t('memorySettings.listCount', { count: total }) }}</span>
        </div>
        <div class="list-actions">
          <t-radio-group v-model="status" variant="default-filled" size="small" @change="reload">
            <t-radio-button value="active">{{ t('memorySettings.statusActive') }}</t-radio-button>
            <t-radio-button value="pending">
              {{ t('memorySettings.statusPending') }}
              <span v-if="pendingCount > 0" class="pending-badge">{{ pendingCount }}</span>
            </t-radio-button>
            <t-radio-button value="superseded">{{ t('memorySettings.statusSuperseded') }}</t-radio-button>
            <t-radio-button value="archived">{{ t('memorySettings.statusArchived') }}</t-radio-button>
          </t-radio-group>
          <t-button size="small" theme="default" variant="outline" @click="handleExport">
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
            <t-button size="small" theme="danger" variant="outline" :disabled="total === 0">
              <template #icon><t-icon name="delete" /></template>
              {{ t('memorySettings.clear') }}
            </t-button>
          </t-popconfirm>
        </div>
      </div>

      <div class="add-row">
        <t-select v-model="draftKind" size="small" class="add-kind" :disabled="!canWrite">
          <t-option v-for="kind in kinds" :key="kind" :value="kind" :label="kindLabel(kind)" />
        </t-select>
        <t-input
          v-model="draftContent"
          size="small"
          class="add-input"
          :disabled="!canWrite"
          :placeholder="t('memorySettings.addPlaceholder')"
          @enter="handleCreate"
        />
        <t-button size="small" :disabled="!canWrite || !draftContent.trim()" @click="handleCreate">
          {{ t('memorySettings.add') }}
        </t-button>
      </div>

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
                <span v-if="item.topic" class="memory-topic">{{ item.topic }}</span>
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
import { computed, onMounted, ref } from 'vue'
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
const editingId = ref('')
const editingContent = ref('')
const editingImportance = ref(3)

const kinds: MemoryKind[] = ['profile', 'preference', 'fact', 'task']
const pendingCount = ref(0)

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

// The count is loaded separately so the tab can advertise waiting inferences
// while the user is looking at the active list.
const loadPendingCount = async () => {
  try {
    const response = await listMemoryItems({ status: 'pending', limit: 1 })
    pendingCount.value = response.total || 0
  } catch (error: any) {
    pendingCount.value = 0
  }
}

const reload = async () => {
  page.value = 1
  await Promise.all([loadItems(), loadPendingCount()])
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

const handleCreate = async () => {
  const content = draftContent.value.trim()
  if (!content) return
  try {
    await createMemoryItem({ kind: draftKind.value, content })
    draftContent.value = ''
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
    await loadItems()
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
  await Promise.all([loadItems(), loadPendingCount()])
})
</script>

<style lang="less" scoped>
.pending-badge {
  display: inline-block;
  min-width: 16px;
  margin-left: 4px;
  padding: 0 4px;
  border-radius: 8px;
  background: var(--td-warning-color-1, #fff1e9);
  color: var(--td-warning-color-6, #d4380d);
  font-size: 11px;
  line-height: 16px;
  text-align: center;
}

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

.add-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.add-kind {
  width: 120px;
  flex-shrink: 0;
}

.add-input {
  flex: 1;
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
