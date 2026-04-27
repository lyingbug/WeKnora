<template>
  <div class="notes-shell">
    <!-- 统一侧栏：导航 + 笔记列表 + 底部菜单 -->
    <aside class="notes-sidebar">
      <div class="sidebar-head" style="--wails-draggable: drag">
        <div class="head-top" style="--wails-draggable: drag">
          <span class="head-title">{{ $t('notes.list.title') }}</span>
          <button class="new-icon-btn" :title="$t('notes.list.newNote')" style="--wails-draggable: no-drag" @click="goNew">
            <t-icon name="edit-1" />
          </button>
        </div>

        <!-- 搜索 / 筛选 / 知识库范围：不得依赖「全局是否已有笔记」。否则选中空库时 hasAnyNotes 为 false，会隐藏知识库切换，用户无法切走 -->
        <div class="search-bar" style="--wails-draggable: no-drag">
          <t-icon name="search" class="search-icon" />
          <input
            v-model="keyword"
            class="search-input"
            :placeholder="$t('notes.list.searchPlaceholder')"
          />
          <button v-if="keyword" class="clear-icon" :title="$t('common.clear')" @click="keyword = ''">
            <t-icon name="close" />
          </button>
        </div>
        <div class="filter-tabs" style="--wails-draggable: no-drag">
          <button
            v-for="opt in statusOptions"
            :key="opt.value"
            class="tab-btn"
            :class="{ active: statusFilter === opt.value }"
            @click="statusFilter = opt.value"
          >
            {{ opt.label }}
            <span class="tab-count">{{ statusTabCount(opt.value) }}</span>
          </button>
        </div>
        <div v-if="kbScopeOptions.length > 0" class="kb-select-block" style="--wails-draggable: no-drag">
          <div class="kb-scope-header">
            <span class="kb-scope-label">{{ $t('notes.list.kbScopeTitle') }}</span>
            <button class="new-kb-btn" :title="$t('notes.list.newNotebook')" @click="openAdvancedCreate">
              <t-icon name="add" />
            </button>
          </div>
          <div class="kb-select-wrap">
            <t-select
              v-model="kbScope"
              :options="kbSelectOptions"
              size="small"
              :borderless="true"
              :placeholder="$t('notes.list.kbScopeTitle')"
              @change="refresh"
            />
          </div>
        </div>
      </div>

      <t-alert v-if="modelBanner" theme="warning" class="sidebar-banner" :message="modelBanner" />

      <div class="sidebar-list">
        <div v-if="loading" class="list-loading">
          <t-loading size="small" />
        </div>

        <template v-else-if="filteredNotes.length === 0">
          <div class="list-empty" :class="{ slim: !hasAnyNotes }">
            <template v-if="hasAnyNotes">
              <t-icon name="search" class="empty-icon-mini" />
              <div class="empty-title-mini">{{ $t('notes.list.noMatch') }}</div>
              <button class="empty-link" @click="resetFilters">{{ $t('notes.list.clearFilters') }}</button>
            </template>
            <template v-else>
              <span class="empty-line">{{ $t('notes.list.noNotesYet') }}</span>
            </template>
          </div>
        </template>

        <template v-else>
          <div
            v-for="note in filteredNotes"
            :key="note.id"
            class="note-row"
            :class="{ active: currentId === note.id }"
          >
            <div class="row-main" @click="openNote(note.id)">
              <div class="row-r1">
                <span class="row-title">{{ note.title || $t('notes.list.untitled') }}</span>
                <span class="row-time">{{ formatRelative(note.updated_at) }}</span>
              </div>
              <div class="row-r2">
                <span v-if="note.status === 'draft'" class="row-status draft">{{ $t('notes.list.tagDraft') }}</span>
                <span class="row-excerpt">{{ note.excerpt || $t('notes.list.noExcerpt') }}</span>
              </div>
            </div>
            <t-dropdown
              :options="noteRowMenuOptions(note)"
              trigger="click"
              placement="bottom-right"
              :popup-props="{ overlayClassName: 'note-row-dropdown' }"
              @click="(data: { value: string }) => onNoteRowMenuSelect(data, note)"
            >
              <button
                type="button"
                class="row-more"
                :title="$t('common.more')"
                :aria-label="$t('common.more')"
                @click.stop
              >
                <t-icon name="ellipsis" />
              </button>
            </t-dropdown>
          </div>
        </template>
      </div>

      <!-- 底部用户区：与主导航相同 UserMenu 组件，保证样式一致 -->
      <div class="sidebar-footer">
        <UserMenu variant="notes" />
      </div>
    </aside>

    <!-- 右侧：路由出口（welcome 或 editor） -->
    <section class="notes-canvas">
      <router-view />
    </section>

    <!-- 右侧对话抽屉 -->
    <transition name="chat-drawer">
      <NotesChatDrawer v-if="uiStore.notesChatPanelOpen" />
    </transition>

    <t-dialog v-model:visible="renameVisible" :header="$t('notes.list.rename')" :confirm-btn="$t('common.confirm')" @confirm="confirmRename">
      <t-input v-model="renameTitle" :maxlength="100" show-limit-number />
    </t-dialog>

    <t-dialog v-model:visible="moveVisible" :header="$t('notes.list.move')" :confirm-btn="$t('common.confirm')" @confirm="confirmMove">
      <t-select v-model="moveTargetKbId" :options="moveKbOptions" />
    </t-dialog>

    <KnowledgeBaseEditorModal
      :visible="uiStore.showKBEditorModal"
      :mode="uiStore.kbEditorMode"
      :kb-id="uiStore.currentKBId || undefined"
      :initial-type="uiStore.kbEditorType"
      :initial-name="uiStore.kbEditorInitialName"
      @update:visible="(val) => val ? null : uiStore.closeKBEditor()"
      @success="handleKBEditorSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { useUIStore } from '@/stores/ui'
import UserMenu from '@/components/UserMenu.vue'
import NotesChatDrawer from '@/views/notes/NotesChatDrawer.vue'
import KnowledgeBaseEditorModal from '@/views/knowledge/KnowledgeBaseEditorModal.vue'
import {
  ensureDefaultNotebook,
  listNotesInKb,
  listNotesAcrossKbs,
  listDocumentKbIds,
  loadManualNoteCountsByKb,
  parseNoteMetadata,
  type NoteListItem,
  NoModelConfiguredError,
} from '@/api/notes'
import {
  updateManualKnowledge,
  delKnowledgeDetails,
  moveKnowledge,
  getKnowledgeDetails,
  waitForKnowledgeMoveTask,
} from '@/api/knowledge-base'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const uiStore = useUIStore()

const loading = ref(true)
const notes = ref<NoteListItem[]>([])
const keyword = ref('')
const kbScope = ref<string>('__all__')
const statusFilter = ref<'all' | 'draft' | 'publish'>('all')
const defaultKbId = ref<string | null>(null)
const modelBanner = ref('')

const kbScopeOptions = ref<{ label: string; value: string }[]>([])
const kbNoteCounts = ref<Record<string, number>>({})

const statusOptions = computed(() => [
  { value: 'all' as const, label: t('notes.list.filterAll') },
  { value: 'draft' as const, label: t('notes.list.filterDraft') },
  { value: 'publish' as const, label: t('notes.list.filterPublished') },
])

const kbSelectOptions = computed(() =>
  kbScopeOptions.value.map((o) => {
    const n = kbNoteCounts.value[o.value]
    const suffix = typeof n === 'number' ? ` (${n})` : ''
    return { label: o.label + suffix, value: o.value }
  }),
)

const filteredNotes = computed(() => {
  let list = notes.value
  if (statusFilter.value !== 'all') {
    list = list.filter((n) => n.status === statusFilter.value)
  }
  const kw = keyword.value.trim().toLowerCase()
  if (kw) {
    list = list.filter((n) => n.title.toLowerCase().includes(kw) || n.excerpt.toLowerCase().includes(kw))
  }
  return list
})

const hasAnyNotes = computed(() => notes.value.length > 0)
const countByStatus = (s: 'draft' | 'publish') => notes.value.filter((n) => n.status === s).length
const statusTabCount = (s: 'all' | 'draft' | 'publish') =>
  s === 'all' ? notes.value.length : countByStatus(s)
const resetFilters = () => {
  statusFilter.value = 'all'
  keyword.value = ''
}

const currentId = computed(() => {
  const id = String(route.params.id || '')
  return id && id !== 'new' ? id : null
})

const formatRelative = (iso?: string) => {
  if (!iso) return ''
  const ts = new Date(iso).getTime()
  if (Number.isNaN(ts)) return ''
  const diff = Date.now() - ts
  if (diff < 60_000) return t('notes.list.justNow')
  if (diff < 3_600_000) return t('notes.list.minutesAgo', { n: Math.floor(diff / 60_000) })
  if (diff < 86_400_000) return t('notes.list.hoursAgo', { n: Math.floor(diff / 3_600_000) })
  if (diff < 7 * 86_400_000) return t('notes.list.daysAgo', { n: Math.floor(diff / 86_400_000) })
  return new Date(iso).toLocaleDateString()
}

const goNew = () => router.push('/platform/notes/new')
const openNote = (id: string) => router.push(`/platform/notes/${id}`)

const openAdvancedCreate = () => {
  uiStore.openCreateKB('notebook')
}

const handleKBEditorSuccess = async (kbId: string) => {
  uiStore.closeKBEditor()
  MessagePlugin.success(t('common.success'))
  await buildKbOptions()
  kbScope.value = kbId
  await refresh()
}

const refresh = async () => {
  loading.value = true
  modelBanner.value = ''
  try {
    if (kbScope.value === '__default__') {
      defaultKbId.value = await ensureDefaultNotebook()
      notes.value = await listNotesInKb(defaultKbId.value)
    } else if (kbScope.value === '__all__') {
      const kbs = await listDocumentKbIds()
      const ids = kbs.map((k) => k.id).slice(0, 12)
      notes.value = await listNotesAcrossKbs(ids)
    } else {
      notes.value = await listNotesInKb(kbScope.value)
    }
  } catch (e: unknown) {
    if (e instanceof NoModelConfiguredError) {
      modelBanner.value = t('notes.errors.noModel')
      notes.value = []
    } else {
      console.error(e)
      MessagePlugin.error(t('notes.list.loadFailed'))
    }
  } finally {
    loading.value = false
    void refreshKbNoteCounts()
  }
}

const refreshKbNoteCounts = async () => {
  try {
    kbNoteCounts.value = await loadManualNoteCountsByKb()
  } catch {
    kbNoteCounts.value = {}
  }
}

const buildKbOptions = async () => {
  const opts: { label: string; value: string }[] = [
    { label: t('notes.list.kbAll'), value: '__all__' },
  ]
  try {
    const kbs = await listDocumentKbIds()
    kbs.forEach((k) => {
      opts.push({ label: k.name, value: k.id })
    })
  } catch {
    /* ignore */
  }
  kbScopeOptions.value = opts
}

onMounted(async () => {
  await buildKbOptions()
  await refresh()
})

// 当用户在编辑器里发布/删除/重命名后，子路由会通过事件通知父级刷新
const onChildRefresh = () => {
  void refresh()
}
window.addEventListener('weknora:notes-changed', onChildRefresh)
onUnmounted(() => window.removeEventListener('weknora:notes-changed', onChildRefresh))

watch(currentId, () => {
  /* 选中切换：列表自身高亮通过 currentId computed 体现 */
})

const noteRowMenuOptions = (note: NoteListItem) => [
  { content: t('notes.list.rename'), value: 'rename' },
  {
    content: note.status === 'publish' ? t('notes.list.unpublish') : t('notes.list.publish'),
    value: 'togglePublish',
  },
  { content: t('notes.list.exportMd'), value: 'export' },
  { content: t('notes.list.move'), value: 'move' },
  { content: t('common.delete'), value: 'delete' },
]

const onNoteRowMenuSelect = (data: { value: string }, note: NoteListItem) => {
  const v = data.value
  if (v === 'rename') {
    openRenameForNote(note)
  } else if (v === 'togglePublish') {
    void togglePublishForNote(note)
  } else if (v === 'export') {
    void exportNoteMd(note)
  } else if (v === 'move') {
    void openMoveForNote(note)
  } else if (v === 'delete') {
    confirmDeleteNote(note)
  }
}

const renameVisible = ref(false)
const renameTitle = ref('')
const renameTarget = ref<NoteListItem | null>(null)

const openRenameForNote = (note: NoteListItem) => {
  renameTarget.value = note
  renameTitle.value = note.title
  renameVisible.value = true
}

const confirmRename = async () => {
  const note = renameTarget.value
  if (!note || !renameTitle.value.trim()) return
  try {
    const detail: any = await getKnowledgeDetails(note.id)
    const meta = parseNoteMetadata(detail?.data?.metadata)
    const res: any = await updateManualKnowledge(note.id, {
      title: renameTitle.value.trim(),
      content: meta?.content ?? '',
      status: meta?.status === 'publish' ? 'publish' : 'draft',
    })
    if (res?.success) {
      MessagePlugin.success(t('common.success'))
      renameVisible.value = false
      await refresh()
    } else {
      MessagePlugin.error(res?.message || t('notes.list.renameFailed'))
    }
  } catch {
    MessagePlugin.error(t('notes.list.renameFailed'))
  }
}

const togglePublishForNote = async (note: NoteListItem) => {
  const next = note.status === 'publish' ? 'draft' : 'publish'
  try {
    const detail: any = await getKnowledgeDetails(note.id)
    const meta = parseNoteMetadata(detail?.data?.metadata)
    const res: any = await updateManualKnowledge(note.id, {
      title: note.title,
      content: meta?.content ?? '',
      status: next,
    })
    if (res?.success) {
      MessagePlugin.success(t('common.success'))
      await refresh()
    } else {
      MessagePlugin.error(res?.message || t('notes.list.saveFailed'))
    }
  } catch {
    MessagePlugin.error(t('notes.list.saveFailed'))
  }
}

const exportNoteMd = async (note: NoteListItem) => {
  try {
    const detail: any = await getKnowledgeDetails(note.id)
    const meta = parseNoteMetadata(detail?.data?.metadata)
    const body = meta?.content ?? ''
    const blob = new Blob([body], { type: 'text/markdown;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${note.title || 'note'}.md`
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    MessagePlugin.error(t('notes.list.exportFailed'))
  }
}

const moveVisible = ref(false)
const moveTargetKbId = ref('')
const moveSource = ref<NoteListItem | null>(null)
const moveKbOptions = ref<{ label: string; value: string }[]>([])

const openMoveForNote = async (note: NoteListItem) => {
  moveSource.value = note
  const kbs = await listDocumentKbIds()
  moveKbOptions.value = kbs.filter((k) => k.id !== note.knowledge_base_id).map((k) => ({ label: k.name, value: k.id }))
  moveTargetKbId.value = moveKbOptions.value[0]?.value ?? ''
  moveVisible.value = true
}

const confirmMove = async () => {
  const note = moveSource.value
  if (!note || !moveTargetKbId.value) return
  try {
    const res: any = await moveKnowledge({
      knowledge_ids: [note.id],
      source_kb_id: note.knowledge_base_id,
      target_kb_id: moveTargetKbId.value,
      mode: 'reparse',
    })
    if (res?.success) {
      const taskId = res?.data?.task_id as string | undefined
      if (taskId) {
        const outcome = await waitForKnowledgeMoveTask(taskId)
        if (outcome === 'failed') {
          MessagePlugin.error(t('notes.list.moveFailed'))
          return
        }
        if (outcome === 'timeout') {
          MessagePlugin.warning(t('notes.list.moveTimeout'))
        } else {
          MessagePlugin.success(t('common.success'))
        }
      } else {
        MessagePlugin.success(t('common.success'))
      }
      moveVisible.value = false
      if (currentId.value === note.id) {
        router.replace('/platform/notes')
      }
      await refresh()
    } else {
      MessagePlugin.error(res?.message || t('notes.list.moveFailed'))
    }
  } catch {
    MessagePlugin.error(t('notes.list.moveFailed'))
  }
}

const confirmDeleteNote = (note: NoteListItem) => {
  DialogPlugin.confirm({
    header: t('notes.list.deleteConfirmTitle'),
    body: t('notes.list.deleteConfirmBody', { title: note.title }),
    confirmBtn: { content: t('common.delete'), theme: 'danger' as const },
    cancelBtn: t('common.cancel'),
    onConfirm: async () => {
      try {
        const res: any = await delKnowledgeDetails(note.id)
        if (res?.success) {
          MessagePlugin.success(t('common.success'))
          if (currentId.value === note.id) {
            router.replace('/platform/notes')
          }
          await refresh()
        } else {
          MessagePlugin.error(res?.message || t('notes.list.deleteFailed'))
        }
      } catch {
        MessagePlugin.error(t('notes.list.deleteFailed'))
      }
    },
  })
}

</script>

<style scoped lang="less">
@accent: var(--td-brand-color);
@sidebar-w: 290px;

.notes-shell {
  flex: 1;
  display: flex;
  align-items: stretch;
  min-width: 0;
  height: 100%;
  background: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  overflow: hidden;
}

.notes-sidebar {
  width: @sidebar-w;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-right: 1px solid var(--td-component-stroke);

  html.wails-desktop & .sidebar-head {
    padding-top: 38px;
  }
}

.sidebar-head {
  padding: 16px 16px 12px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.head-top {
  display: flex;
  align-items: center;
  justify-content: space-between;

  .head-title {
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.2px;
    color: var(--td-text-color-primary);
  }

  .new-icon-btn {
    width: 28px;
    height: 28px;
    border-radius: 6px;
    border: none;
    background: transparent;
    color: var(--td-text-color-secondary);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s ease;

    &:hover {
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-primary);
    }

    :deep(.t-icon) {
      font-size: 16px;
    }
  }
}

.search-bar {
  position: relative;
  display: flex;
  align-items: center;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  padding: 0 10px;
  height: 32px;
  border: 1px solid transparent;
  transition: border-color 0.2s ease, background 0.2s ease;

  &:focus-within {
    background: var(--td-bg-color-container);
    border-color: @accent;
  }

  .search-icon {
    color: var(--td-text-color-placeholder);
    font-size: 14px;
    margin-right: 6px;
  }

  .search-input {
    flex: 1;
    height: 100%;
    border: none;
    outline: none;
    background: transparent;
    font-size: 13px;
    color: var(--td-text-color-primary);
    min-width: 0;

    &::placeholder {
      color: var(--td-text-color-placeholder);
    }
  }

  .clear-icon {
    border: none;
    background: transparent;
    color: var(--td-text-color-placeholder);
    cursor: pointer;
    padding: 2px;
    display: inline-flex;
    align-items: center;

    &:hover {
      color: var(--td-text-color-primary);
    }
  }
}

.filter-tabs {
  display: flex;
  background: var(--td-bg-color-secondarycontainer);
  padding: 2px;
  border-radius: 6px;
}

.tab-btn {
  flex: 1;
  height: 26px;
  border-radius: 5px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 0 4px;

  &:hover:not(.active) {
    color: var(--td-text-color-primary);
  }

  &.active {
    background: var(--td-bg-color-container);
    color: var(--td-text-color-primary);
    font-weight: 600;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  }

  .tab-count {
    font-size: 10px;
    color: var(--td-text-color-placeholder);
    font-weight: 500;
  }
}

.kb-select-block {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
}

.kb-scope-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 2px;
}

.kb-scope-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: var(--td-text-color-placeholder);
}

.new-kb-btn {
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: 4px;
  transition: all 0.15s ease;

  &:hover {
    background: var(--td-bg-color-component-hover);
    color: var(--td-text-color-primary);
  }

  :deep(.t-icon) {
    font-size: 14px;
  }
}

.kb-select-wrap {
  :deep(.t-input__wrap) {
    border: none;
    box-shadow: none;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 8px;
  }

  :deep(.t-input) {
    min-height: 32px;
  }
}

.sidebar-banner {
  margin: 8px 12px;
}

.sidebar-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px 12px;
}

.list-loading {
  display: flex;
  justify-content: center;
  padding: 32px 0;
}

.list-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 32px 16px;
  text-align: center;

  &.slim {
    padding: 18px 16px 4px;
  }

  .empty-icon-mini {
    font-size: 22px;
    color: var(--td-text-color-placeholder);
    margin-bottom: 4px;
  }

  .empty-title-mini {
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }

  .empty-line {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }

  .empty-link {
    margin-top: 4px;
    border: none;
    background: transparent;
    color: @accent;
    font-size: 12px;
    cursor: pointer;
    padding: 4px 8px;
    border-radius: 4px;

    &:hover {
      background: rgba(7, 192, 95, 0.1);
    }
  }
}

.note-row {
  position: relative;
  padding: 10px 8px 10px 14px;
  border-radius: 8px;
  margin-bottom: 4px;
  display: flex;
  align-items: flex-start;
  gap: 4px;
  transition: all 0.15s ease;
  border: 1px solid transparent;
  overflow: hidden;

  .row-main {
    flex: 1;
    min-width: 0;
    cursor: pointer;
    /* 为悬停时出现的更多按钮预留空间，防止文字被遮挡 */
    padding-right: 28px;
  }

  &:hover {
    background: var(--td-bg-color-container-hover);
  }

  &.active {
    background: var(--td-bg-color-secondarycontainer);
    border-color: var(--td-component-border);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.02);

    .row-title {
      color: var(--td-text-color-primary);
    }
  }
}

.row-more {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  margin: 0;
  padding: 0;
  border: none;
  background: transparent;
  border-radius: 6px;
  color: var(--td-text-color-placeholder);
  cursor: pointer;
  transition: color 0.15s ease, background 0.15s ease, opacity 0.15s ease;
  opacity: 0;
  position: absolute;
  right: 8px;
  top: 50%;
  transform: translateY(-50%);
  z-index: 2;

  /* 添加一个渐变遮罩，防止按钮遮挡文字时显得突兀 */
  &::before {
    content: '';
    position: absolute;
    right: 100%;
    top: 0;
    bottom: 0;
    width: 20px;
    background: linear-gradient(to right, transparent, var(--td-bg-color-container-hover));
    pointer-events: none;
  }

  .note-row.active &::before {
    background: linear-gradient(to right, transparent, var(--td-bg-color-secondarycontainer));
  }

  .note-row:hover &,
  .note-row.active & {
    opacity: 1;
  }

  &:hover,
  &:focus-visible {
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-component-hover);
  }
}

.row-r1 {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 6px;

  .row-title {
    flex: 1;
    font-size: 14px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    line-height: 1.4;
    word-break: break-all;
  }

  .row-time {
    flex-shrink: 0;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    font-variant-numeric: tabular-nums;
    margin-top: 1px;
  }
}

.row-r2 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  min-width: 0;
}

.row-status.draft {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 500;
  color: var(--td-warning-color);
}

.row-excerpt {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--td-text-color-secondary);
}

/* 底部用户区：与主导航 .menu_bottom 一致（无顶部分割线，仅靠与列表区的留白区分） */
.sidebar-footer {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  padding: 8px;
}

.notes-canvas {
  flex: 1;
  min-width: 360px; /* 保证哪怕开了抽屉，主画布也不会被挤成面条 */
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container);
  position: relative;
  container-type: inline-size;
}

.chat-drawer-enter-active,
.chat-drawer-leave-active {
  transition: width 0.2s ease, opacity 0.2s ease;
  overflow: hidden;
}

.chat-drawer-enter-from,
.chat-drawer-leave-to {
  width: 0 !important;
  opacity: 0;
}
</style>

<style lang="less">
/* 笔记侧栏 logo 暗色模式反色，与原菜单 logo 行为保持一致 */
html[theme-mode="dark"] .notes-shell .brand-row .brand-logo {
  filter: invert(1) hue-rotate(180deg);
}

.new-kb-popup {
  width: 240px;
  padding: 8px 4px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.new-kb-popup-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 4px;
}

.new-kb-popup-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 4px;

  .new-kb-popup-actions-left {
    display: flex;
    align-items: center;
  }

  .new-kb-popup-actions-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

/* 笔记列表行「更多」下拉面板的宽度，与原上下文菜单一致 */
.note-row-dropdown.t-dropdown__menu {
  min-width: 160px;
}
</style>
