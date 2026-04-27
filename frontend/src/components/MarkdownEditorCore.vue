<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { marked } from 'marked'
import { useI18n } from 'vue-i18n'
import { sanitizeHTML, safeMarkdownToHTML } from '@/utils/security'

export type MarkdownEditorViewMode = 'edit' | 'split' | 'preview'

const props = withDefaults(
  defineProps<{
    modelValue: string
    /** 编辑 / 分屏 / 仅预览 */
    viewMode: MarkdownEditorViewMode
    disabled?: boolean
    contentLoading?: boolean
    autosize?: { minRows: number; maxRows: number }
    showToolbar?: boolean
  }>(),
  {
    disabled: false,
    contentLoading: false,
    autosize: () => ({ minRows: 16, maxRows: 24 }),
    showToolbar: true,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'update:viewMode': [mode: MarkdownEditorViewMode]
  save: []
}>()

const { t } = useI18n()

const textareaComponent = ref<any>(null)
const textareaElement = ref<HTMLTextAreaElement | null>(null)
const selectionRange = reactive({ start: 0, end: 0 })
const selectionEvents = ['select', 'keyup', 'click', 'mouseup', 'input']

const resolveTextareaElement = (): HTMLTextAreaElement | null => {
  const component = textareaComponent.value as any
  if (!component) return null
  if (component.textareaRef) {
    return component.textareaRef as HTMLTextAreaElement
  }
  if (component.$el) {
    const el = component.$el.querySelector('textarea')
    if (el) {
      return el as HTMLTextAreaElement
    }
  }
  return null
}

const handleTextareaSelectionEvent = () => {
  const textarea = textareaElement.value ?? resolveTextareaElement()
  if (!textarea) {
    return
  }
  selectionRange.start = textarea.selectionStart ?? 0
  selectionRange.end = textarea.selectionEnd ?? 0
}

const detachTextareaListeners = () => {
  if (!textareaElement.value) {
    return
  }
  selectionEvents.forEach((eventName) => {
    textareaElement.value?.removeEventListener(eventName, handleTextareaSelectionEvent)
  })
  textareaElement.value = null
}

const attachTextareaListeners = () => {
  nextTick(() => {
    const textarea = resolveTextareaElement()
    if (!textarea) {
      return
    }
    if (textareaElement.value === textarea) {
      return
    }
    detachTextareaListeners()
    textareaElement.value = textarea
    selectionEvents.forEach((eventName) => {
      textarea.addEventListener(eventName, handleTextareaSelectionEvent)
    })
    handleTextareaSelectionEvent()
  })
}

const editingVisible = computed(() => props.viewMode === 'edit' || props.viewMode === 'split')
const previewVisible = computed(() => props.viewMode === 'preview' || props.viewMode === 'split')

const setSelectionRange = (start: number, end: number) => {
  selectionRange.start = start
  selectionRange.end = end
  nextTick(() => {
    const textarea = resolveTextareaElement()
    if (!textarea || !editingVisible.value) {
      return
    }
    textarea.focus()
    textarea.setSelectionRange(start, end)
  })
}

const getSelectionRange = () => {
  return {
    start: selectionRange.start ?? 0,
    end: selectionRange.end ?? 0,
  }
}

const clampRange = (start: number, end: number, length: number) => {
  let safeStart = Math.max(0, Math.min(start, length))
  let safeEnd = Math.max(0, Math.min(end, length))
  if (safeEnd < safeStart) {
    ;[safeStart, safeEnd] = [safeEnd, safeStart]
  }
  return { safeStart, safeEnd }
}

const patchContent = (content: string, start: number, end: number) => {
  emit('update:modelValue', content)
  setSelectionRange(start, end)
}

const findLineStart = (value: string, index: number) => {
  if (index <= 0) return 0
  const lastNewline = value.lastIndexOf('\n', index - 1)
  return lastNewline === -1 ? 0 : lastNewline + 1
}

const findLineEnd = (value: string, index: number) => {
  if (index >= value.length) return value.length
  const newlineIndex = value.indexOf('\n', index)
  return newlineIndex === -1 ? value.length : newlineIndex
}

const transformSelectedLines = (transformer: (line: string, index: number) => string) => {
  const value = props.modelValue ?? ''
  const { start, end } = getSelectionRange()
  const { safeStart, safeEnd } = clampRange(start, end, value.length)
  const lineStart = findLineStart(value, safeStart)
  const lineEnd = findLineEnd(value, safeEnd)
  const selected = value.slice(lineStart, lineEnd)
  const lines = selected.split('\n')
  const transformed = lines.map((line, index) => transformer(line, index))
  const result = transformed.join('\n')
  const newContent = value.slice(0, lineStart) + result + value.slice(lineEnd)
  patchContent(newContent, lineStart, lineStart + result.length)
}

const wrapSelection = (prefix: string, suffix: string, placeholder: string) => {
  const value = props.modelValue ?? ''
  const { start, end } = getSelectionRange()
  const { safeStart, safeEnd } = clampRange(start, end, value.length)
  const hasSelection = safeEnd > safeStart
  const selectedText = hasSelection ? value.slice(safeStart, safeEnd) : placeholder
  const result =
    value.slice(0, safeStart) + prefix + selectedText + suffix + value.slice(safeEnd)
  const selectionStart = safeStart + prefix.length
  const selectionEnd = selectionStart + selectedText.length
  patchContent(result, selectionStart, selectionEnd)
}

const insertBlock = (
  text: string,
  selectionStartOffset?: number,
  selectionEndOffset?: number,
) => {
  const value = props.modelValue ?? ''
  const { start, end } = getSelectionRange()
  const { safeStart, safeEnd } = clampRange(start, end, value.length)
  const before = value.slice(0, safeStart)
  const after = value.slice(safeEnd)
  const result = before + text + after
  const base = safeStart
  const selectionStart =
    selectionStartOffset !== undefined ? base + selectionStartOffset : base + text.length
  const selectionEnd =
    selectionEndOffset !== undefined ? base + selectionEndOffset : selectionStart
  patchContent(result, selectionStart, selectionEnd)
}

const applyHeading = (level: number) => {
  const hashes = '#'.repeat(level)
  transformSelectedLines((line) => {
    const trimmed = line.replace(/^#+\s*/, '').trim()
    const content = trimmed || t('manualEditor.placeholders.heading', { level })
    return `${hashes} ${content}`
  })
}

const listPrefixPattern =
  /^(\s*(?:[-*+]|\d+\.)\s+|\s*-\s+\[[ xX]\]\s+)/

const applyBulletList = () => {
  transformSelectedLines((line) => {
    const trimmed = line.trim()
    const content = trimmed.replace(listPrefixPattern, '').trim()
    return `- ${content || t('manualEditor.placeholders.listItem')}`
  })
}

const applyOrderedList = () => {
  transformSelectedLines((line, index) => {
    const trimmed = line.trim()
    const content = trimmed.replace(listPrefixPattern, '').trim()
    return `${index + 1}. ${content || t('manualEditor.placeholders.listItem')}`
  })
}

const applyTaskList = () => {
  transformSelectedLines((line) => {
    const trimmed = line.trim()
    const content = trimmed.replace(listPrefixPattern, '').trim()
    return `- [ ] ${content || t('manualEditor.placeholders.taskItem')}`
  })
}

const applyBlockquote = () => {
  transformSelectedLines((line) => {
    const trimmed = line.trim().replace(/^>\s?/, '').trim()
    return `> ${trimmed || t('manualEditor.placeholders.quote')}`
  })
}

const insertCodeBlock = () => {
  const placeholder = t('manualEditor.placeholders.code')
  const block = `\n\`\`\`\n${placeholder}\n\`\`\`\n`
  const startOffset = block.indexOf(placeholder)
  insertBlock(block, startOffset, startOffset + placeholder.length)
}

const insertHorizontalRule = () => {
  insertBlock('\n---\n\n')
}

const insertTable = () => {
  const cell = t('manualEditor.table.cell')
  const template = `\n| ${t('manualEditor.table.column1')} | ${t('manualEditor.table.column2')} |\n| --- | --- |\n| ${cell} | ${cell} |\n`
  const placeholderIndex = template.indexOf(cell)
  insertBlock(template, placeholderIndex, placeholderIndex + cell.length)
}

const insertLink = () => {
  const value = props.modelValue ?? ''
  const { start, end } = getSelectionRange()
  const { safeStart, safeEnd } = clampRange(start, end, value.length)
  const selectedText =
    safeEnd > safeStart ? value.slice(safeStart, safeEnd) : t('manualEditor.placeholders.linkText')
  const urlPlaceholder = 'https://'
  const result =
    value.slice(0, safeStart) +
    `[${selectedText}](${urlPlaceholder})` +
    value.slice(safeEnd)
  const urlStart = safeStart + selectedText.length + 3
  const urlEnd = urlStart + urlPlaceholder.length
  patchContent(result, urlStart, urlEnd)
}

const insertImage = () => {
  const value = props.modelValue ?? ''
  const { start, end } = getSelectionRange()
  const { safeStart, safeEnd } = clampRange(start, end, value.length)
  const altText = safeEnd > safeStart ? value.slice(safeStart, safeEnd) : t('manualEditor.placeholders.imageAlt')
  const urlPlaceholder = 'https://'
  const result =
    value.slice(0, safeStart) +
    `![${altText}](${urlPlaceholder})` +
    value.slice(safeEnd)
  const urlStart = safeStart + altText.length + 4
  const urlEnd = urlStart + urlPlaceholder.length
  patchContent(result, urlStart, urlEnd)
}

type ToolbarAction = () => void
type ToolbarButton = {
  key: string
  tooltip: string
  action: ToolbarAction
  icon: string
}
type ToolbarGroup = {
  key: string
  buttons: ToolbarButton[]
}

const toolbarGroups = computed<ToolbarGroup[]>(() => [
  {
    key: 'format',
    buttons: [
      { key: 'bold', icon: 'textformat-bold', tooltip: t('manualEditor.toolbar.bold'), action: () => wrapSelection('**', '**', t('manualEditor.placeholders.bold')) },
      { key: 'italic', icon: 'textformat-italic', tooltip: t('manualEditor.toolbar.italic'), action: () => wrapSelection('*', '*', t('manualEditor.placeholders.italic')) },
      { key: 'strike', icon: 'textformat-strikethrough', tooltip: t('manualEditor.toolbar.strike'), action: () => wrapSelection('~~', '~~', t('manualEditor.placeholders.strike')) },
      { key: 'inline-code', icon: 'code', tooltip: t('manualEditor.toolbar.inlineCode'), action: () => wrapSelection('`', '`', t('manualEditor.placeholders.inlineCode')) },
    ],
  },
  {
    key: 'heading',
    buttons: [
      { key: 'h1', icon: 'numbers-1', tooltip: t('manualEditor.toolbar.heading1'), action: () => applyHeading(1) },
      { key: 'h2', icon: 'numbers-2', tooltip: t('manualEditor.toolbar.heading2'), action: () => applyHeading(2) },
      { key: 'h3', icon: 'numbers-3', tooltip: t('manualEditor.toolbar.heading3'), action: () => applyHeading(3) },
    ],
  },
  {
    key: 'list',
    buttons: [
      { key: 'ul', icon: 'view-list', tooltip: t('manualEditor.toolbar.bulletList'), action: applyBulletList },
      { key: 'ol', icon: 'list-numbered', tooltip: t('manualEditor.toolbar.orderedList'), action: applyOrderedList },
      { key: 'task', icon: 'check-rectangle', tooltip: t('manualEditor.toolbar.taskList'), action: applyTaskList },
      { key: 'quote', icon: 'quote', tooltip: t('manualEditor.toolbar.blockquote'), action: applyBlockquote },
    ],
  },
  {
    key: 'insert',
    buttons: [
      { key: 'codeblock', icon: 'code-1', tooltip: t('manualEditor.toolbar.codeBlock'), action: insertCodeBlock },
      { key: 'link', icon: 'link', tooltip: t('manualEditor.toolbar.link'), action: insertLink },
      { key: 'image', icon: 'image', tooltip: t('manualEditor.toolbar.image'), action: insertImage },
      { key: 'table', icon: 'table', tooltip: t('manualEditor.toolbar.table'), action: insertTable },
      { key: 'hr', icon: 'component-divider-horizontal', tooltip: t('manualEditor.toolbar.horizontalRule'), action: insertHorizontalRule },
    ],
  },
])

const ensureEditModeForToolbar = (then: ToolbarAction) => {
  if (props.disabled) return
  if (props.viewMode === 'preview') {
    emit('update:viewMode', 'edit')
    nextTick(() => {
      attachTextareaListeners()
      then()
    })
  } else {
    attachTextareaListeners()
    then()
  }
}

const handleToolbarAction = (action: ToolbarAction) => {
  ensureEditModeForToolbar(action)
}

marked.use({})

const previewHTML = computed(() => {
  if (!props.modelValue) {
    return `<p class="empty-preview">${t('manualEditor.preview.empty')}</p>`
  }
  const safeMarkdown = safeMarkdownToHTML(props.modelValue)
  const parsed = marked.parse(safeMarkdown)
  const html = typeof parsed === 'string' ? parsed : ''
  return sanitizeHTML(html)
})

watch(
  () => props.viewMode,
  (val) => {
    if (val === 'edit' || val === 'split') {
      nextTick(() => {
        attachTextareaListeners()
      })
    } else {
      detachTextareaListeners()
    }
  },
)

watch(
  () => props.modelValue,
  () => {
    if (editingVisible.value) {
      nextTick(() => attachTextareaListeners())
    }
  },
)

onBeforeUnmount(() => {
  detachTextareaListeners()
})

defineExpose({
  attachTextareaListeners,
  focusEnd() {
    const len = props.modelValue?.length ?? 0
    setSelectionRange(len, len)
  },
  applyBold: () => ensureEditModeForToolbar(() => wrapSelection('**', '**', t('manualEditor.placeholders.bold'))),
  applyItalic: () => ensureEditModeForToolbar(() => wrapSelection('*', '*', t('manualEditor.placeholders.italic'))),
  insertLink: () => ensureEditModeForToolbar(insertLink),
  applyHeading: (level: 1 | 2 | 3) => ensureEditModeForToolbar(() => applyHeading(level)),
  getTextareaElement: () => textareaElement.value ?? resolveTextareaElement(),
  insertAtCursor: (text: string) => insertBlock(text),
  getCaretPos: () => ({ start: selectionRange.start, end: selectionRange.end }),
})
</script>

<template>
  <div class="markdown-editor-core">
    <div v-if="showToolbar" class="editor-toolbar">
      <template v-for="(group, groupIndex) in toolbarGroups" :key="group.key">
        <div class="toolbar-group">
          <template v-for="btn in group.buttons" :key="btn.key">
            <t-tooltip :content="btn.tooltip" placement="top">
              <button
                type="button"
                class="toolbar-btn"
                :class="`btn-${btn.key}`"
                :disabled="disabled"
                @mousedown.prevent
                @click="handleToolbarAction(btn.action)"
              >
                <t-icon :name="btn.icon" size="18px" />
              </button>
            </t-tooltip>
          </template>
        </div>
        <div
          v-if="groupIndex < toolbarGroups.length - 1"
          class="toolbar-divider"
        />
      </template>
    </div>

    <div
      class="editor-area"
      :class="{
        'editor-area--split': viewMode === 'split',
        'editor-area--edit': viewMode === 'edit',
        'editor-area--preview': viewMode === 'preview',
      }"
    >
      <div v-show="editingVisible" class="editor-pane editor-pane--input">
        <t-textarea
          v-if="!contentLoading"
          ref="textareaComponent"
          :model-value="modelValue"
          :disabled="disabled"
          :placeholder="$t('manualEditor.form.contentPlaceholder')"
          :autosize="{ minRows: autosize.minRows, maxRows: autosize.maxRows }"
          @update:model-value="emit('update:modelValue', $event as string)"
        />
        <div v-else class="loading-placeholder">
          <t-loading size="small" :text="$t('manualEditor.loading.content')" />
        </div>
      </div>
      <div v-show="previewVisible" class="editor-pane editor-pane--preview">
        <div class="preview-container" v-html="previewHTML" />
      </div>
    </div>
  </div>
</template>

<style scoped lang="less">
.markdown-editor-core {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.editor-toolbar {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px 8px 0 0;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  overflow-x: auto;
}

.toolbar-group {
  display: flex;
  align-items: center;
  gap: 4px;
}

.toolbar-divider {
  width: 1px;
  height: 24px;
  background: var(--td-bg-color-secondarycontainer);
  margin: 0 2px;
}

.toolbar-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  border-radius: 6px;
  color: var(--td-text-color-secondary);
  border: none;
  background: transparent;
  cursor: pointer;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  justify-content: center;

  .t-icon {
    color: var(--td-text-color-secondary);
    font-size: 16px;
    width: 16px;
    height: 16px;
  }

  &:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
}

.toolbar-btn:hover:not(:disabled) {
  background: rgba(7, 192, 95, 0.08);
  color: var(--td-brand-color);

  .t-icon {
    color: var(--td-brand-color);
  }
}

.toolbar-btn:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px rgba(7, 192, 95, 0.25);
}

.editor-area {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-stroke);
  border-top: none;
  border-radius: 0 0 8px 8px;
  overflow: hidden;
  background: var(--td-bg-color-container);
}

.editor-area--split {
  flex-direction: row;
  min-height: 320px;

  .editor-pane--input {
    flex: 1;
    min-width: 0;
    border-right: 1px solid var(--td-component-stroke);
  }

  .editor-pane--preview {
    flex: 1;
    min-width: 0;
  }
}

.editor-pane {
  padding: 0;
  overflow: hidden;
  background: var(--td-bg-color-container);
}

.editor-pane--preview .preview-container {
  border-radius: 0;
}

:deep(.t-textarea__inner) {
  font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace;
  line-height: 1.6;
  border: none !important;
  box-shadow: none !important;
}

.preview-container {
  min-height: 200px;
  max-height: 70vh;
  overflow-y: auto;
  padding: 16px;
  background: var(--td-bg-color-secondarycontainer);
  font-size: 14px;
  line-height: 1.7;
  color: var(--td-text-color-primary);

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4) {
    margin-top: 16px;
    margin-bottom: 8px;
  }

  :deep(code) {
    background: var(--td-bg-color-container-hover);
    padding: 2px 4px;
    border-radius: 4px;
    font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace;
  }

  :deep(pre) {
    background: var(--td-bg-color-container-hover);
    padding: 12px;
    border-radius: 6px;
    overflow: auto;
  }

  :deep(blockquote) {
    border-left: 4px solid var(--td-brand-color);
    padding-left: 12px;
    color: var(--td-text-color-secondary);
    margin: 16px 0;
    background: rgba(7, 192, 95, 0.08);
  }

  :deep(a) {
    color: var(--td-brand-color);
  }
}

.loading-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 240px;
}

.empty-preview {
  color: var(--td-text-color-placeholder);
}
</style>
