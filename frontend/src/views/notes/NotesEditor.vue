<template>
  <div
    class="notes-editor-page"
    :class="{ 'is-focus': viewMode === 'preview' }"
    @dragover.prevent="onDragOver"
    @drop.prevent="onDrop"
  >
    <header class="top-bar" style="--wails-draggable: drag">
      <div class="bar-left" style="--wails-draggable: no-drag">
        <t-button
          variant="text"
          shape="square"
          size="small"
          class="bar-icon bar-back"
          :title="$t('common.back')"
          @click="goBack"
        >
          <template #icon><t-icon name="chevron-left" /></template>
        </t-button>
        <span
          class="bar-title"
          :class="{ 'is-placeholder': !title.trim() }"
          style="--wails-draggable: drag"
          :title="title.trim() || $t('notes.editor.titlePlaceholder')"
        >
          {{ title.trim() || $t('notes.editor.titlePlaceholder') }}
        </span>
        <span
          class="autosave"
          :class="{ saving, saved: !saving && !!lastSavedAt }"
          :title="autosaveTitle"
        >
          <t-icon v-if="saving" name="loading" class="spin" />
          <t-icon v-else-if="lastSavedAt" name="check-circle" />
          <t-icon v-else name="edit-1" />
          <span class="autosave-text">
            <template v-if="saving">{{ $t('notes.editor.saving') }}</template>
            <template v-else-if="!lastSavedAt">{{ status === 'publish' ? $t('notes.list.tagPublished') : $t('notes.list.tagDraft') }}</template>
            <template v-else>{{ savedRelative }}</template>
          </span>
        </span>
      </div>

      <div class="bar-right" style="--wails-draggable: no-drag">
        <div class="view-pill" role="tablist">
          <button
            v-for="opt in viewOptions"
            :key="opt.value"
            type="button"
            class="view-btn"
            :class="{ active: viewMode === opt.value }"
            :title="opt.label"
            :aria-pressed="viewMode === opt.value"
            @click="viewMode = opt.value"
          >
            <t-icon :name="opt.icon" />
          </button>
        </div>

        <t-popconfirm
          :content="$t('notes.editor.publishHint')"
          :confirm-btn="{ content: $t('common.confirm'), theme: 'primary' }"
          :cancel-btn="$t('common.cancel')"
          @confirm="confirmPublish"
          placement="bottom"
        >
          <t-button
            theme="primary"
            variant="base"
            size="small"
            class="bar-publish"
            :disabled="publishDisabled"
            :loading="saving"
          >
            <template #icon><t-icon name="refresh" v-if="status === 'publish'" /><t-icon name="cloud-upload" v-else /></template>
            <span class="bar-publish-label">
              {{ status === 'publish' ? $t('notes.editor.republish') : $t('manualEditor.actions.publish') }}
            </span>
          </t-button>
        </t-popconfirm>

        <div class="kb-selector" v-if="!authStore.isLiteMode">
          <t-select
            v-model="kbId"
            :options="kbOptions"
            size="small"
            :borderless="true"
            :placeholder="$t('manualEditor.form.knowledgeBaseLabel')"
            @change="handleKbChange"
          >
            <template #prefixIcon>
              <t-icon name="folder-open" />
            </template>
          </t-select>
        </div>

        <t-tooltip :content="$t('notes.list.exportMd')" placement="bottom">
          <t-button
            variant="text"
            shape="square"
            size="small"
            class="bar-icon"
            @click="exportMd"
          >
            <template #icon><t-icon name="download" /></template>
          </t-button>
        </t-tooltip>

        <t-popconfirm
          :content="$t('notes.list.deleteConfirmBody', { title: title.trim() || $t('notes.list.untitled') })"
          :confirm-btn="{ content: $t('common.delete'), theme: 'danger' }"
          :cancel-btn="$t('common.cancel')"
          @confirm="deleteCurrent"
          placement="bottom-right"
        >
          <t-button
            variant="text"
            shape="square"
            size="small"
            class="bar-icon delete-icon"
            :disabled="!knowledgeId"
          >
            <template #icon><t-icon name="delete" /></template>
          </t-button>
        </t-popconfirm>
        
        <div class="bar-divider"></div>

        <t-tooltip :content="$t('notes.wiki.title')" placement="bottom">
          <t-button
            variant="text"
            shape="square"
            size="small"
            class="bar-icon wiki-icon-btn"
            :class="{ 'is-active': uiStore.notesWikiPanelOpen }"
            @click="uiStore.setNotesWikiPanel(!uiStore.notesWikiPanelOpen)"
          >
            <template #icon><t-icon name="map" /></template>
          </t-button>
        </t-tooltip>

        <t-tooltip :content="$t('notes.chat.title')" placement="bottom">
          <t-button
            variant="text"
            shape="square"
            size="small"
            class="bar-icon chat-icon-btn"
            :class="{ 'is-active': uiStore.notesChatPanelOpen }"
            @click="uiStore.setNotesChatPanel(!uiStore.notesChatPanelOpen)"
          >
            <template #icon><t-icon name="chat" /></template>
          </t-button>
        </t-tooltip>
      </div>
    </header>

    <t-alert v-if="bannerText" theme="warning" class="banner" :message="bannerText" />

    <main class="editor-main">
      <div class="editor-body">
        <div class="editor-shell" :class="{ split: viewMode === 'split' }">
          <!-- 编辑列 -->
          <div
            v-if="viewMode !== 'preview'"
            ref="paneEditRef"
            class="pane pane-edit"
            @scroll="onEditPaneScroll"
          >
            <div class="pane-inner">
              <input
                v-model="title"
                class="title-input"
                :placeholder="$t('notes.editor.titlePlaceholder')"
                :maxlength="100"
              />
              <MarkdownEditorCore
                ref="coreRef"
                v-model="content"
                v-model:view-mode="coreViewMode"
                :disabled="saving"
                :content-loading="loading"
                :show-toolbar="false"
                :autosize="{ minRows: 24, maxRows: 9999 }"
                class="editor-core-host"
              />
            </div>
          </div>

          <!-- 预览列 -->
          <div
            v-if="viewMode !== 'edit'"
            ref="panePreviewRef"
            class="pane pane-preview"
            @scroll="onPreviewPaneScroll"
          >
            <div class="pane-inner">
              <h1 class="preview-title">{{ title || $t('notes.editor.titlePlaceholder') }}</h1>
              <article class="preview-md" v-html="previewHTML" />
            </div>
          </div>
        </div>

        <!-- Wiki 面板 -->
        <transition name="wiki-panel">
          <NotesWikiPanel
            v-if="uiStore.notesWikiPanelOpen && kbId"
            :kb-id="kbId"
            :knowledge-id="knowledgeId"
            :note-title="title"
            :note-content="content"
            :is-published="status === 'publish'"
            @insert-link="insertWikiLinkFromPanel"
          />
        </transition>
      </div>
    </main>

    <footer class="bottom-bar">
      <span class="stat">{{ $t('notes.editor.chars', { n: content.length }) }}</span>
      <span class="dot-sep">·</span>
      <span class="stat">{{ $t('notes.editor.words', { n: wordCount }) }}</span>
      <span v-if="currentKbName" class="dot-sep">·</span>
      <span v-if="currentKbName" class="kb-name">{{ currentKbName }}</span>
      <span class="spacer"></span>
      <span class="kbd-hint"><kbd>⌘S</kbd>&nbsp;{{ $t('manualEditor.actions.saveDraft') }}</span>
      <span class="kbd-hint"><kbd>⇧P</kbd>&nbsp;{{ $t('notes.editor.viewPreview') }}</span>
    </footer>

    <t-dialog v-model:visible="publishDialogVisible" :header="$t('notes.editor.publishTitle')" :width="480">
      <p class="publish-hint">{{ $t('notes.editor.publishHint') }}</p>
      <template #footer>
        <t-button variant="outline" @click="publishDialogVisible = false">{{ $t('common.cancel') }}</t-button>
        <t-button theme="primary" :loading="saving" @click="confirmPublish">{{ $t('common.confirm') }}</t-button>
      </template>
    </t-dialog>

    <!-- [[笔记标题]] 浮动自动补全 -->
    <Teleport to="body">
      <div
        v-if="noteSuggestions.length > 0"
        class="note-suggest-list"
        :style="suggestStyle"
      >
        <div class="note-suggest-header">{{ $t('notes.link.suggestHint') }}</div>
        <div
          v-for="(note, idx) in noteSuggestions"
          :key="note.id"
          class="note-suggest-item"
          :class="{ active: suggestIndex === idx }"
          @mousedown.prevent="insertNoteLink(note)"
        >
          <t-icon name="file-1" class="suggest-icon" />
          <div class="suggest-body">
            <span class="suggest-title">{{ note.title }}</span>
            <span v-if="note.excerpt" class="suggest-excerpt">{{ note.excerpt }}</span>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { marked } from 'marked'
import { sanitizeHTML, safeMarkdownToHTML } from '@/utils/security'
import MarkdownEditorCore from '@/components/MarkdownEditorCore.vue'
import NotesWikiPanel from '@/views/notes/NotesWikiPanel.vue'
import {
  ensureDefaultNotebook,
  listDocumentKbIds,
  parseNoteMetadata,
  NoModelConfiguredError,
} from '@/api/notes'
import {
  getKnowledgeDetails,
  createManualKnowledge,
  updateManualKnowledge,
  getKnowledgeBaseById,
  delKnowledgeDetails,
  moveKnowledge,
  waitForKnowledgeMoveTask,
} from '@/api/knowledge-base'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const authStore = useAuthStore()
const uiStore = useUIStore()

const idParam = computed(() => String(route.params.id || ''))
const isNew = computed(() => idParam.value === 'new')

const title = ref('')
const content = ref('')
const initialTitle = ref('')
const initialContent = ref('')
const status = ref<'draft' | 'publish'>('draft')
const knowledgeId = ref<string | null>(null)
const kbId = ref('')
/** 打开「目标知识库」弹窗时的草稿选择，确认后才写回 kbId */
const kbIdDraft = ref('')
/** 当前笔记已落库所在知识库（迁移成功后会更新） */
const initialKbId = ref('')
const kbOptions = ref<{ label: string; value: string }[]>([])
const loading = ref(false)
const saving = ref(false)
const viewMode = ref<'edit' | 'split' | 'preview'>('split')
const lastSavedAt = ref<number | null>(null)
const bannerText = ref('')
const kbInitialized = ref(true)
const publishDialogVisible = ref(false)
const kbDialogVisible = ref(false)
const coreRef = ref<InstanceType<typeof MarkdownEditorCore> | null>(null)
const tickNow = ref(Date.now())
const paneEditRef = ref<HTMLElement | null>(null)
const panePreviewRef = ref<HTMLElement | null>(null)
/** 分屏比例滚同步时避免两侧 scroll 互触发 */
let splitScrollSuppress = false

// ===== [[笔记标题]] 自动补全（Obsidian 风格）=====

interface NoteSuggestItem {
  id: string
  title: string
  excerpt: string
}

const noteSuggestions = ref<NoteSuggestItem[]>([])
const suggestIndex = ref(0)
const suggestPos = ref({ top: 0, left: 0 })
let suggestTimer: ReturnType<typeof setTimeout> | null = null
let suggestQueryText = ''

const suggestStyle = computed(() => ({
  position: 'fixed' as const,
  top: `${suggestPos.value.top}px`,
  left: `${suggestPos.value.left}px`,
  zIndex: 9999,
}))

const clearSuggest = () => {
  noteSuggestions.value = []
  suggestIndex.value = 0
  suggestQueryText = ''
}

const insertNoteLink = (item: NoteSuggestItem) => {
  const textarea = coreRef.value?.getTextareaElement()
  if (!textarea) return
  const pos = textarea.selectionStart ?? 0
  const val = content.value
  const before = val.slice(0, pos)
  const triggerIdx = before.lastIndexOf('[[')
  if (triggerIdx < 0) { clearSuggest(); return }

  const link = `[[${item.title}]]`
  content.value = val.slice(0, triggerIdx) + link + val.slice(pos)
  clearSuggest()
  nextTick(() => {
    const newPos = triggerIdx + link.length
    const el = coreRef.value?.getTextareaElement()
    el?.setSelectionRange(newPos, newPos)
    el?.focus()
  })
}

/** 从 Wiki 面板点击"插入链接"调用 */
const insertWikiLinkFromPanel = (slug: string, pageTitle: string) => {
  const link = `[[${pageTitle}]]`
  const textarea = coreRef.value?.getTextareaElement()
  if (!textarea) {
    content.value += `\n${link}`
    return
  }
  const pos = textarea.selectionStart ?? content.value.length
  content.value = content.value.slice(0, pos) + link + content.value.slice(pos)
  nextTick(() => {
    const newPos = pos + link.length
    const el = coreRef.value?.getTextareaElement()
    el?.setSelectionRange(newPos, newPos)
    el?.focus()
  })
}

/** 估算光标屏幕坐标 */
const getCaretScreenPos = (textarea: HTMLTextAreaElement, caretOffset: number) => {
  const rect = textarea.getBoundingClientRect()
  const style = getComputedStyle(textarea)
  const mirror = document.createElement('div')
  const mirrorStyles = [
    'fontFamily', 'fontSize', 'fontWeight', 'lineHeight', 'letterSpacing',
    'wordSpacing', 'textIndent', 'whiteSpace', 'wordWrap', 'overflowWrap',
    'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
    'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
    'boxSizing',
  ] as const
  mirror.style.position = 'absolute'
  mirror.style.visibility = 'hidden'
  mirror.style.overflow = 'hidden'
  mirror.style.width = `${textarea.clientWidth}px`
  for (const prop of mirrorStyles) {
    ;(mirror.style as any)[prop] = style.getPropertyValue(
      prop.replace(/[A-Z]/g, (m) => `-${m.toLowerCase()}`),
    )
  }
  mirror.textContent = textarea.value.slice(0, caretOffset)
  const caret = document.createElement('span')
  caret.textContent = '|'
  mirror.appendChild(caret)
  document.body.appendChild(mirror)
  const caretRect = caret.getBoundingClientRect()
  const mirrorRect = mirror.getBoundingClientRect()
  document.body.removeChild(mirror)
  const relY = caretRect.top - mirrorRect.top - textarea.scrollTop
  const relX = caretRect.left - mirrorRect.left
  return {
    top: Math.min(rect.top + relY + 22, rect.bottom),
    left: Math.min(rect.left + relX, rect.right - 260),
  }
}

/** 检测 [[ 触发（纯本地），停顿 500ms 后才按笔记标题搜索 */
const onTextareaInput = () => {
  const textarea = coreRef.value?.getTextareaElement()
  if (!textarea) return
  const pos = textarea.selectionStart ?? 0
  const before = textarea.value.slice(0, pos)
  const match = /\[\[([^\]\n]*)$/.exec(before)
  if (!match) { clearSuggest(); return }

  const query = match[1]
  suggestPos.value = getCaretScreenPos(textarea, pos)
  if (query === suggestQueryText && noteSuggestions.value.length > 0) return
  suggestQueryText = query

  if (suggestTimer) clearTimeout(suggestTimer)
  suggestTimer = setTimeout(() => fetchNoteSuggestions(query), 500)
}

const fetchNoteSuggestions = async (query: string) => {
  if (!kbId.value) return
  const searchKey = query.trim()
  try {
    const { listNotesInKb } = await import('@/api/notes')
    const notes = await listNotesInKb(kbId.value, {
      keyword: searchKey,
      maxPages: 1,
      pageSize: 10,
    })
    noteSuggestions.value = notes
      .filter((n) => n.id !== knowledgeId.value)
      .map((n) => ({ id: n.id, title: n.title, excerpt: n.excerpt }))
      .slice(0, 8)
    suggestIndex.value = 0
  } catch {
    clearSuggest()
  }
}

const onTextareaKeydown = (e: KeyboardEvent) => {
  if (noteSuggestions.value.length === 0) return
  if (e.key === 'ArrowDown') {
    e.preventDefault(); e.stopPropagation()
    suggestIndex.value = (suggestIndex.value + 1) % noteSuggestions.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault(); e.stopPropagation()
    suggestIndex.value = (suggestIndex.value - 1 + noteSuggestions.value.length) % noteSuggestions.value.length
  } else if (e.key === 'Enter' || e.key === 'Tab') {
    const selected = noteSuggestions.value[suggestIndex.value]
    if (selected) { e.preventDefault(); e.stopPropagation(); insertNoteLink(selected) }
  } else if (e.key === 'Escape' || e.key === ']') {
    clearSuggest()
  }
}

let linkTextareaRef: HTMLTextAreaElement | null = null

const attachLinkListeners = () => {
  const tryAttach = (attempts = 0) => {
    const el = coreRef.value?.getTextareaElement()
    if (!el) {
      if (attempts < 10) setTimeout(() => tryAttach(attempts + 1), 200)
      return
    }
    if (el === linkTextareaRef) return
    detachLinkListeners()
    linkTextareaRef = el
    el.addEventListener('input', onTextareaInput)
    el.addEventListener('keydown', onTextareaKeydown, true)
  }
  nextTick(() => tryAttach())
}

const detachLinkListeners = () => {
  if (linkTextareaRef) {
    linkTextareaRef.removeEventListener('input', onTextareaInput)
    linkTextareaRef.removeEventListener('keydown', onTextareaKeydown, true)
    linkTextareaRef = null
  }
}

// 给 MarkdownEditorCore 一个稳定的「edit-only」mode；预览由本页面接管
const coreViewMode = ref<'edit' | 'split' | 'preview'>('edit')

let autosaveTimer: ReturnType<typeof setTimeout> | null = null
let tickTimer: ReturnType<typeof setInterval> | null = null

/** 分屏：按可滚动高度比例同步两侧（原文与 HTML 高度不同，只能近似对齐） */
const syncPaneScroll = (source: 'edit' | 'preview') => {
  if (viewMode.value !== 'split') return
  const from = source === 'edit' ? paneEditRef.value : panePreviewRef.value
  const to = source === 'edit' ? panePreviewRef.value : paneEditRef.value
  if (!from || !to) return
  const fromMax = from.scrollHeight - from.clientHeight
  const toMax = to.scrollHeight - to.clientHeight
  if (fromMax <= 0) {
    to.scrollTop = 0
    return
  }
  const ratio = from.scrollTop / fromMax
  splitScrollSuppress = true
  to.scrollTop = toMax > 0 ? Math.round(ratio * toMax) : 0
  queueMicrotask(() => {
    splitScrollSuppress = false
  })
}

const onEditPaneScroll = () => {
  if (splitScrollSuppress) return
  syncPaneScroll('edit')
}

const onPreviewPaneScroll = () => {
  if (splitScrollSuppress) return
  syncPaneScroll('preview')
}

watch(viewMode, (mode, prev) => {
  if (mode !== 'split') return
  nextTick(() => {
    if (prev === 'edit') syncPaneScroll('edit')
    else if (prev === 'preview') syncPaneScroll('preview')
    else syncPaneScroll('edit')
  })
})

const viewOptions = computed(() => [
  { value: 'edit' as const, label: t('notes.editor.viewEdit'), icon: 'edit-1' },
  { value: 'split' as const, label: t('notes.editor.viewSplit'), icon: 'view-column' },
  { value: 'preview' as const, label: t('notes.editor.viewPreview'), icon: 'view-module' },
])

const currentKbName = computed(() => kbOptions.value.find((o) => o.value === kbId.value)?.label || '')

const wordCount = computed(() => {
  const text = content.value || ''
  // 中文按字计 + 英文按单词
  const cn = (text.match(/[\u4e00-\u9fa5]/g) || []).length
  const en = (text.match(/[A-Za-z]+/g) || []).length
  return cn + en
})

const savedRelative = computed(() => {
  if (!lastSavedAt.value) return ''
  // tickNow 触发刷新
  void tickNow.value
  const diff = Date.now() - lastSavedAt.value
  if (diff < 5_000) return t('notes.editor.savedJustNow')
  if (diff < 60_000) return t('notes.editor.savedSecondsAgo', { n: Math.floor(diff / 1000) })
  if (diff < 3_600_000) return t('notes.editor.savedMinutesAgo', { n: Math.floor(diff / 60_000) })
  return t('notes.editor.lastSaved', { time: new Date(lastSavedAt.value).toLocaleTimeString() })
})

const autosaveTitle = computed(() => {
  if (saving.value) return t('notes.editor.saving')
  if (!lastSavedAt.value) {
    return status.value === 'publish' ? t('notes.list.tagPublished') : t('notes.list.tagDraft')
  }
  return savedRelative.value
})

/**
 * 将 [[slug|显示文字]] 和 [[标题]] 转为 HTML 链接。
 * 预览阶段统一渲染为 wiki-link 样式，不做存在性校验（避免每次渲染拉接口）。
 * 存在性校验留给 Wiki 面板的 "本文链接" tab 按需做。
 */
const renderWikiLinks = (md: string): string =>
  md.replace(
    /\[\[([^\]|]+?)(?:\|([^\]]+?))?\]\]/g,
    (_match, slugOrTitle: string, displayText?: string) => {
      const label = (displayText || slugOrTitle).trim()
      const slug = slugOrTitle.trim()
      return `<a class="wiki-link" data-wiki-slug="${slug}" title="${slug}">${label}</a>`
    },
  )

const previewHTML = computed(() => {
  if (!content.value) {
    return `<p class="empty-md">${t('manualEditor.preview.empty')}</p>`
  }
  const withLinks = renderWikiLinks(content.value)
  const safe = safeMarkdownToHTML(withLinks)
  const parsed = marked.parse(safe)
  const html = typeof parsed === 'string' ? parsed : ''
  return sanitizeHTML(html)
})

const saveDisabled = computed(() => {
  if (!title.value.trim() || !content.value.trim()) return true
  if (!kbInitialized.value) return true
  return false
})

const publishDisabled = computed(() => {
  if (saveDisabled.value) return true
  if (!kbInitialized.value) return true
  return false
})

const goBack = () => {
  router.push('/platform/notes')
}

const handleKbChange = (value: string) => {
  if (!knowledgeId.value) {
    initialKbId.value = value
    void checkKbInit(value)
    return
  }
  
  if (value === initialKbId.value) {
    return
  }
  
  const sourceId = initialKbId.value
  const kId = knowledgeId.value
  
  // 直接执行迁移，不再弹窗确认
  const doMove = async () => {
    try {
      const res: any = await moveKnowledge({
        knowledge_ids: [kId],
        source_kb_id: sourceId,
        target_kb_id: value,
        mode: 'reparse',
      })
      if (!res?.success) {
        MessagePlugin.error(res?.message || t('notes.list.moveFailed'))
        kbId.value = sourceId // 恢复原值
        return
      }
      initialKbId.value = value
      kbId.value = value
      const taskId = res?.data?.task_id as string | undefined
      if (taskId) {
        const outcome = await waitForKnowledgeMoveTask(taskId)
        if (outcome === 'failed') {
          MessagePlugin.error(t('notes.list.moveFailed'))
          kbId.value = sourceId
          initialKbId.value = sourceId
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
      window.dispatchEvent(new CustomEvent('weknora:notes-changed'))
      await checkKbInit(value)
    } catch {
      MessagePlugin.error(t('notes.list.moveFailed'))
      kbId.value = sourceId // 恢复原值
    }
  }
  
  void doMove()
}

const loadKbOptions = async () => {
  try {
    const kbs = await listDocumentKbIds()
    kbOptions.value = kbs.map((k) => ({ label: k.name, value: k.id }))
  } catch {
    /* ignore */
  }
}

const resolveKbId = async (): Promise<string> => {
  if (authStore.isLiteMode) {
    return await ensureDefaultNotebook()
  }
  if (kbId.value) return kbId.value
  return await ensureDefaultNotebook()
}

const loadNote = async () => {
  if (isNew.value) {
    knowledgeId.value = null
    title.value = ''
    content.value = ''
    initialTitle.value = ''
    initialContent.value = ''
    status.value = 'draft'
    try {
      kbId.value = await ensureDefaultNotebook()
      initialKbId.value = kbId.value
      await checkKbInit(kbId.value)
    } catch (e) {
      if (e instanceof NoModelConfiguredError) {
        bannerText.value = t('notes.errors.noModel')
        kbInitialized.value = false
      }
    }
    await nextTick()
    coreRef.value?.focusEnd()
    return
  }
  loading.value = true
  bannerText.value = ''
  try {
    const res: any = await getKnowledgeDetails(idParam.value)
    const data = res?.data
    if (!data) {
      MessagePlugin.error(t('manualEditor.error.fetchDetailFailed'))
      router.replace('/platform/notes')
      return
    }
    knowledgeId.value = String(data.id)
    kbId.value = String(data.knowledge_base_id || '')
    initialKbId.value = kbId.value
    const meta = parseNoteMetadata(data.metadata)
    const loadedTitle = data.title || data.file_name?.replace(/\.md$/i, '') || ''
    const loadedContent = meta?.content || ''
    title.value = loadedTitle
    content.value = loadedContent
    initialTitle.value = loadedTitle
    initialContent.value = loadedContent
    status.value = meta?.status || (data.parse_status === 'completed' ? 'publish' : 'draft')
    await checkKbInit(kbId.value)
  } catch {
    MessagePlugin.error(t('manualEditor.error.fetchDetailFailed'))
    router.replace('/platform/notes')
  } finally {
    loading.value = false
    await nextTick()
    coreRef.value?.attachTextareaListeners()
  }
}

const checkKbInit = async (kid: string) => {
  kbInitialized.value = true
  bannerText.value = ''
  if (!kid) {
    kbInitialized.value = false
    return
  }
  try {
    const res: any = await getKnowledgeBaseById(kid)
    const kb = res?.data
    if (!kb?.summary_model_id) {
      kbInitialized.value = false
      bannerText.value = t('notes.editor.kbNotReady')
      return
    }
    const strategy = kb.indexing_strategy
    const needsEmbedding = !strategy || strategy.vector_enabled || strategy.keyword_enabled
    if (needsEmbedding && !kb.embedding_model_id) {
      kbInitialized.value = false
      bannerText.value = t('notes.editor.kbNotReady')
      return
    }
    const storageProvider =
      kb.storage_provider_config?.provider || kb.storage_config?.provider
    if (!storageProvider) {
      kbInitialized.value = false
      bannerText.value = t('notes.editor.kbNoStorage')
    }
  } catch {
    kbInitialized.value = false
  }
}

watch(kbId, (v) => {
  if (v) void checkKbInit(v)
  uiStore.setNotesCurrentKbId(v || '')
})

const persistDraft = async (silent: boolean) => {
  if (!title.value.trim() || !content.value.trim()) return
  if (!kbInitialized.value && silent) return
  saving.value = true
  try {
    const kid = await resolveKbId()
    if (!authStore.isLiteMode) {
      kbId.value = kid
    }
    const payload = { title: title.value.trim(), content: content.value, status: 'draft' as const }

    if (knowledgeId.value) {
      const res: any = await updateManualKnowledge(knowledgeId.value, payload)
      if (!res?.success) throw new Error(res?.message)
    } else {
      const res: any = await createManualKnowledge(kid, payload)
      if (!res?.success || !res?.data?.id) throw new Error(res?.message)
      knowledgeId.value = String(res.data.id)
      if (isNew.value) {
        await router.replace(`/platform/notes/${knowledgeId.value}`)
      }
    }
    if (status.value !== 'publish') status.value = 'draft'
    initialTitle.value = payload.title
    initialContent.value = payload.content
    lastSavedAt.value = Date.now()
    window.dispatchEvent(new CustomEvent('weknora:notes-changed'))
    if (!silent) MessagePlugin.success(t('manualEditor.success.draftSaved'))
  } catch (e: unknown) {
    if (!silent) {
      const msg = e instanceof Error ? e.message : t('manualEditor.error.saveFailed')
      MessagePlugin.error(msg)
    }
    if (e instanceof NoModelConfiguredError) {
      bannerText.value = t('notes.errors.noModel')
      kbInitialized.value = false
    }
  } finally {
    saving.value = false
  }
}

const scheduleAutosave = () => {
  if (autosaveTimer) clearTimeout(autosaveTimer)
  autosaveTimer = setTimeout(() => {
    void persistDraft(true)
  }, 2000)
}

watch([title, content], ([newTitle, newContent]) => {
  if (newTitle === initialTitle.value && newContent === initialContent.value) return
  scheduleAutosave()
})

const confirmPublish = async () => {
  if (!title.value.trim() || !content.value.trim()) return
  if (content.value.trim().length < 10) {
    MessagePlugin.warning(t('manualEditor.warning.contentTooShort'))
    return
  }
  saving.value = true
  try {
    const kid = await resolveKbId()
    kbId.value = kid
    const payload = { title: title.value.trim(), content: content.value, status: 'publish' as const }
    if (knowledgeId.value) {
      const res: any = await updateManualKnowledge(knowledgeId.value, payload)
      if (!res?.success) throw new Error(res?.message)
    } else {
      const res: any = await createManualKnowledge(kid, payload)
      if (!res?.success || !res?.data?.id) throw new Error(res?.message)
      knowledgeId.value = String(res.data.id)
      if (isNew.value) await router.replace(`/platform/notes/${knowledgeId.value}`)
    }
    status.value = 'publish'
    initialTitle.value = payload.title
    initialContent.value = payload.content
    lastSavedAt.value = Date.now()
    window.dispatchEvent(new CustomEvent('weknora:notes-changed'))
    MessagePlugin.success(t('manualEditor.success.published'))
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : t('manualEditor.error.saveFailed')
    MessagePlugin.error(msg)
  } finally {
    saving.value = false
  }
}

const exportMd = () => {
  const blob = new Blob([content.value], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${title.value.trim() || 'note'}.md`
  a.click()
  URL.revokeObjectURL(url)
}

const deleteCurrent = async () => {
  if (!knowledgeId.value) return
  try {
    const res: any = await delKnowledgeDetails(knowledgeId.value!)
    if (res?.success) {
      MessagePlugin.success(t('common.success'))
      window.dispatchEvent(new CustomEvent('weknora:notes-changed'))
      router.push('/platform/notes')
    } else {
      MessagePlugin.error(res?.message || t('notes.list.deleteFailed'))
    }
  } catch {
    MessagePlugin.error(t('notes.list.deleteFailed'))
  }
}

const onDragOver = (e: DragEvent) => {
  e.stopPropagation()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
}

const onDrop = async (e: DragEvent) => {
  e.stopPropagation()
  const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : []
  const mdFiles = files.filter((f) => f.name.toLowerCase().endsWith('.md'))
  if (mdFiles.length === 0) return
  const first = mdFiles[0]
  const text = await first.text()
  if (isNew.value && !content.value.trim()) {
    content.value = text
    title.value = first.name.replace(/\.md$/i, '')
  } else {
    router.push('/platform/notes/new')
    await nextTick()
    content.value = text
    title.value = first.name.replace(/\.md$/i, '')
  }
}

const onKeyDown = (e: KeyboardEvent) => {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    void persistDraft(false)
    return
  }
  if (e.shiftKey && e.key.toLowerCase() === 'p') {
    // 不在输入文本时才切换：避免在写「P」字符时触发
    const target = e.target as HTMLElement | null
    const isFormField = target && (target.tagName === 'TEXTAREA' || target.tagName === 'INPUT' || target.isContentEditable)
    if (isFormField) return
    e.preventDefault()
    viewMode.value = viewMode.value === 'preview' ? 'edit' : 'preview'
    return
  }
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'b') {
    e.preventDefault()
    coreRef.value?.applyBold()
    return
  }
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'i') {
    e.preventDefault()
    coreRef.value?.applyItalic()
    return
  }
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    coreRef.value?.insertLink()
    return
  }
  if ((e.metaKey || e.ctrlKey) && ['1', '2', '3'].includes(e.key)) {
    e.preventDefault()
    const level = Number(e.key) as 1 | 2 | 3
    coreRef.value?.applyHeading(level)
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKeyDown, true)
  tickTimer = setInterval(() => {
    tickNow.value = Date.now()
  }, 15_000)
  await loadKbOptions()
  await loadNote()
  attachLinkListeners()
})

watch(kbDialogVisible, (open) => {
  if (open) kbIdDraft.value = kbId.value
})

watch(idParam, async () => {
  clearSuggest()
  await loadNote()
  attachLinkListeners()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeyDown, true)
  if (autosaveTimer) clearTimeout(autosaveTimer)
  if (tickTimer) clearInterval(tickTimer)
  if (suggestTimer) clearTimeout(suggestTimer)
  detachLinkListeners()
  uiStore.setNotesCurrentKbId('')
})
</script>

<style scoped lang="less">
@accent: var(--td-brand-color);
@max-w: 760px;
@page-pad-x: 24px;
/* 分屏下列宽约为画布一半，再打开对话抽屉时更窄，需单独收紧避免「左右空太多 / 和编辑列不一致」 */
@split-pad-x: 16px;
@split-pad-y-top: 48px;

.notes-editor-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  width: 100%;
  min-width: 0;
  height: 100%;
  min-height: 0;
  background: var(--td-bg-color-container);
  color: var(--td-text-color-primary);
  position: relative;
}

/* 顶栏（父级 .notes-canvas 已设 container-type，可响应窄画布） */
.top-bar {
  min-height: 48px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
  padding: 0 12px 0 10px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.04);
  z-index: 2;
  user-select: none;
}

.bar-left,
.bar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.bar-left {
  flex: 1 1 0;
  min-width: 0;
  justify-content: flex-start;
  align-items: center;
  gap: 10px;
}

/* 返回后紧跟标题，状态在左侧末（靠近工具区），先「是谁」再「什么状态」 */
.bar-back {
  flex-shrink: 0;
}

.bar-title {
  display: block;
  flex: 1 1 0;
  min-width: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.2;
  letter-spacing: -0.02em;
  color: var(--td-text-color-primary);
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &.is-placeholder {
    color: var(--td-text-color-placeholder);
    font-weight: 500;
  }
}

.bar-right {
  flex: 0 1 auto;
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}

.bar-divider {
  width: 1px;
  height: 20px;
  background: var(--td-component-stroke);
  margin: 0 -4px;
}

.bar-icon {
  --td-button-text-color: var(--td-text-color-secondary);

  &.chat-icon-btn.is-active {
    --td-button-text-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }

  &.delete-icon {
    &:hover:not(:disabled) {
      --td-button-text-color: var(--td-error-color);
      background: var(--td-error-color-light);
    }
  }
}

.autosave {
  display: inline-flex;
  align-items: center;
  flex-shrink: 0;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  padding: 4px 10px;
  border-radius: 999px;
  background: var(--td-bg-color-secondarycontainer);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: min(180px, 32vw);

  .t-icon {
    font-size: 13px;
    flex-shrink: 0;
  }

  .autosave-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &.saving {
    color: var(--td-warning-color);
  }

  &.saved {
    color: var(--td-success-color);
  }
}

.bar-publish {
  flex-shrink: 0;

  :deep(.t-button__text) {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
}

@container (max-width: 520px) {
  .top-bar {
    padding: 0 6px 0 4px;
    gap: 4px;
  }

  .bar-left {
    gap: 6px;
  }

  .bar-title {
    font-size: 14px;
  }

  .bar-right {
    gap: 8px;
  }

  .bar-divider {
    margin: 0 -2px;
  }

  .bar-publish .bar-publish-label {
    display: none;
  }

  .autosave {
    max-width: 96px;
    padding: 4px 8px;
  }

  .view-pill {
    padding: 2px;
  }

  .view-btn {
    width: 28px;
    height: 24px;
  }
}

@container (max-width: 400px) {
  .autosave {
    max-width: 72px;
  }
}

.kb-selector {
  width: 140px;
  
  :deep(.t-input__wrap) {
    border: none;
    box-shadow: none;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 6px;
  }

  :deep(.t-input) {
    min-height: 28px;
    padding: 0 8px;
  }
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0); }
  to { transform: rotate(360deg); }
}

.view-pill {
  display: inline-flex;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 8px;
  padding: 3px;
  gap: 2px;

  .view-btn {
    width: 32px;
    height: 26px;
    border: none;
    background: transparent;
    border-radius: 5px;
    color: var(--td-text-color-secondary);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 14px;
    transition: all 0.15s ease;
    padding: 0;

    .t-icon {
      font-size: 14px;
    }

    &:hover {
      color: var(--td-text-color-primary);
    }

    &.active {
      background: var(--td-bg-color-container);
      color: @accent;
      box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
    }
  }
}

.banner {
  margin: 8px 16px 0;
}

/* 主体 */
.editor-main {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
}

.editor-body {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

.editor-shell {
  flex: 1;
  min-height: 0;
  display: flex;
  overflow: hidden;
}

/* Wiki 面板过渡 */
.wiki-panel-enter-active,
.wiki-panel-leave-active {
  transition: width 0.22s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.18s ease;
  overflow: hidden;
}

.wiki-panel-enter-from,
.wiki-panel-leave-to {
  width: 0 !important;
  opacity: 0;
}

/* note-suggest styles are in non-scoped block below (Teleport to body) */

/* wiki-icon-btn active 状态 */
.bar-icon.wiki-icon-btn.is-active {
  --td-button-text-color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

.editor-shell.split {
  .pane {
    width: 50%;
  }
  .pane-edit {
    border-right: 1px solid var(--td-component-stroke);
  }

  .pane-inner {
    max-width: none;
    min-width: 0;
    padding: @split-pad-y-top @split-pad-x 120px;
  }
}

.pane {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  /* 一列有滚动条、一列无滚动条时，避免预览与编辑可排版宽度错开 */
  scrollbar-gutter: stable;
  display: flex;
  justify-content: center;
}

.pane-inner {
  box-sizing: border-box;
  width: 100%;
  max-width: @max-w;
  min-width: 0;
  padding: 56px @page-pad-x 120px;
}

.notes-editor-page.is-focus .pane-inner {
  padding-top: 72px;
}

@container (max-width: 640px) {
  .editor-shell.split .pane-inner {
    padding-left: 12px;
    padding-right: 12px;
  }
}

/* 标题输入 */
.title-input {
  display: block;
  width: 100%;
  border: none;
  outline: none;
  background: transparent;
  font-size: 32px;
  font-weight: 700;
  letter-spacing: -0.2px;
  color: var(--td-text-color-primary);
  line-height: 1.3;
  padding: 0 0 18px;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Helvetica Neue', sans-serif;

  &::placeholder {
    color: var(--td-text-color-placeholder);
    font-weight: 700;
  }
}

/* 编辑器内核外观重置 */
.editor-core-host {
  :deep(.editor-area) {
    border: none;
    border-radius: 0;
    background: transparent;
    overflow: visible;
  }

  :deep(.editor-pane) {
    background: transparent;
    overflow: visible;
  }

  :deep(.t-textarea__inner) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Helvetica Neue', sans-serif;
    font-size: 16px;
    line-height: 1.85;
    border: none !important;
    box-shadow: none !important;
    background: transparent !important;
    padding: 0 !important;
    color: var(--td-text-color-primary);
    resize: none;
    overflow: hidden !important;
  }

  :deep(.preview-container) {
    /* 预览由本页面接管，不再使用 core 内部的预览样式 */
    display: none;
  }
}

/* 预览栏 */
.pane-preview {
  background: transparent;
}

.preview-title {
  margin: 0 0 18px;
  font-size: 32px;
  font-weight: 700;
  letter-spacing: -0.2px;
  line-height: 1.3;
  color: var(--td-text-color-primary);
}

.preview-md {
  font-size: 16px;
  line-height: 1.85;
  color: var(--td-text-color-primary);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Helvetica Neue', sans-serif;

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4) {
    font-weight: 700;
    margin: 1.6em 0 0.6em;
    letter-spacing: -0.1px;
  }

  :deep(h1) { font-size: 26px; }
  :deep(h2) { font-size: 22px; }
  :deep(h3) { font-size: 18px; }

  :deep(p) {
    margin: 0.6em 0;
  }

  :deep(blockquote) {
    margin: 1em 0;
    padding: 4px 16px;
    border-left: 3px solid var(--td-component-stroke);
    color: var(--td-text-color-secondary);
    background: transparent;
  }

  :deep(code) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 2px 6px;
    border-radius: 4px;
    font-family: 'JetBrains Mono', 'SF Mono', Consolas, monospace;
    font-size: 0.92em;
  }

  :deep(pre) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 14px 16px;
    border-radius: 8px;
    overflow: auto;
    line-height: 1.55;

    code {
      background: transparent;
      padding: 0;
      font-size: 13px;
    }
  }

  :deep(a) {
    color: @accent;
    text-decoration: none;
    border-bottom: 1px solid rgba(7, 192, 95, 0.3);

    &:hover {
      border-bottom-color: @accent;
    }
  }

  :deep(ul),
  :deep(ol) {
    padding-left: 1.4em;
  }

  :deep(hr) {
    border: none;
    border-top: 1px solid var(--td-component-stroke);
    margin: 2em 0;
  }

  :deep(table) {
    border-collapse: collapse;
    margin: 1em 0;

    th, td {
      border: 1px solid var(--td-component-stroke);
      padding: 6px 12px;
    }
    th {
      background: var(--td-bg-color-secondarycontainer);
    }
  }

  :deep(.empty-md) {
    color: var(--td-text-color-placeholder);
    font-style: italic;
  }
}

/* 底栏 */
.bottom-bar {
  height: 28px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 16px;
  border-top: 1px solid var(--td-component-stroke);
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  background: transparent;

  .stat {
    font-variant-numeric: tabular-nums;
  }

  .dot-sep {
    color: var(--td-text-color-placeholder);
    opacity: 0.5;
  }

  .spacer {
    flex: 1;
  }

  .kbd-hint {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  kbd {
    display: inline-block;
    padding: 1px 5px;
    font-family: 'JetBrains Mono', 'SF Mono', Consolas, monospace;
    font-size: 11px;
    border-radius: 3px;
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-secondary);
  }
}

.publish-hint {
  margin: 0;
  font-size: 14px;
  color: var(--td-text-color-secondary);
  line-height: 1.6;
}

/* 焦点模式：preview 时给一层极淡的背景，做阅读感分层 */
.notes-editor-page.is-focus .pane-preview {
  background: var(--td-bg-color-container);
}

/* [[wiki-link]] 在预览中的样式 */
.wiki-link {
  color: var(--td-brand-color);
  border-bottom: 1px solid var(--td-brand-color);
  text-decoration: none;
  cursor: pointer;
  transition: opacity 0.15s ease;
  padding: 0 1px;

  &:hover {
    opacity: 0.75;
  }
}

/* red link 判断留给 Wiki 面板的 "本文链接" tab，预览中统一样式 */
</style>

<!-- 笔记链接自动补全浮动层样式（Teleport 到 body，scoped 不生效） -->
<style lang="less">
.note-suggest-list {
  background: var(--td-bg-color-container, #fff);
  border: 1px solid var(--td-component-stroke, #e7e7e7);
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.14);
  overflow: hidden;
  min-width: 240px;
  max-width: 380px;
  max-height: 320px;
  overflow-y: auto;
}

.note-suggest-header {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  padding: 6px 10px 4px;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
  font-weight: 500;
}

.note-suggest-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 10px;
  cursor: pointer;
  transition: background 0.12s ease;

  &:hover,
  &.active {
    background: var(--td-bg-color-container-hover, #f5f5f5);
  }

  .suggest-icon {
    font-size: 14px;
    color: var(--td-text-color-placeholder, #999);
    margin-top: 1px;
    flex-shrink: 0;
  }
}

.suggest-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.suggest-title {
  font-size: 13px;
  color: var(--td-text-color-primary, #333);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.suggest-excerpt {
  font-size: 11px;
  color: var(--td-text-color-placeholder, #999);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
