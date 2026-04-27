<template>
  <aside class="notes-wiki-panel" :style="{ width: panelWidth + 'px' }">
    <div class="panel-resizer" @mousedown="startResize" />

    <header class="panel-head">
      <div class="head-left">
        <t-icon name="map" class="head-icon" aria-hidden="true" />
        <span class="head-title">{{ $t('notes.wiki.title') }}</span>
        <span v-if="totalPages > 0" class="head-badge">{{ totalPages }}</span>
      </div>
      <div class="head-actions">
        <t-tooltip :content="$t('notes.wiki.refresh')" placement="bottom">
          <button class="head-btn" :disabled="loading" @click="reload">
            <t-icon name="refresh" :class="{ spin: loading }" />
          </button>
        </t-tooltip>
        <t-tooltip :content="$t('common.close')" placement="bottom">
          <button class="head-btn" @click="close">
            <t-icon name="close" />
          </button>
        </t-tooltip>
      </div>
    </header>

    <!-- 搜索栏 -->
    <div class="panel-search">
      <t-icon name="search" class="search-icon" />
      <input
        v-model="searchQuery"
        class="search-input"
        :placeholder="$t('notes.wiki.searchPlaceholder')"
        @input="debouncedSearch"
      />
      <button v-if="searchQuery" class="search-clear" @click="searchQuery = ''; loadContent()">
        <t-icon name="close" />
      </button>
    </div>

    <!-- Tab 栏 -->
    <div class="panel-tabs">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab-btn"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key; loadContent()"
      >
        {{ tab.label }}
        <span v-if="tab.count > 0" class="tab-count">{{ tab.count }}</span>
      </button>
    </div>

    <div class="panel-body">
      <!-- Wiki 未启用 -->
      <div v-if="!wikiEnabled && !loading" class="wiki-prompt">
        <t-icon name="map" class="prompt-icon" />
        <p class="prompt-title">{{ $t('notes.wiki.notEnabled') }}</p>
        <p class="prompt-desc">{{ $t('notes.wiki.notEnabledDesc') }}</p>
      </div>

      <!-- 加载中 -->
      <div v-else-if="loading" class="wiki-loading">
        <t-loading size="small" />
      </div>

      <template v-else>
        <!-- 当前笔记中的 [[链接]] 分析（content-scan tab）-->
        <template v-if="activeTab === 'links'">
          <div v-if="noteLinks.length === 0" class="wiki-empty">
            <t-icon name="link" class="empty-icon" />
            <p>{{ $t('notes.wiki.noLinks') }}</p>
            <p class="empty-hint">{{ $t('notes.wiki.noLinksHint') }}</p>
          </div>
          <template v-else>
            <div
              v-for="link in noteLinks"
              :key="link.raw"
              class="wiki-card"
              :class="{ 'is-missing': !link.resolved }"
            >
              <div class="card-top">
                <span class="link-status-dot" :class="link.resolved ? 'dot-ok' : 'dot-missing'" />
                <span class="card-title">{{ link.display }}</span>
                <span v-if="link.resolved" class="link-badge ok">{{ $t('notes.wiki.linkExists') }}</span>
                <span v-else class="link-badge missing">{{ $t('notes.wiki.linkMissing') }}</span>
              </div>
              <div class="card-actions">
                <button v-if="link.resolved && link.page" class="card-action-btn" @click="openPage(link.page!)">
                  <t-icon name="browse" /> {{ $t('notes.wiki.viewPage') }}
                </button>
                <button class="card-action-btn" @click="emit('insertLink', link.slug, link.display)">
                  <t-icon name="enter" /> {{ $t('notes.wiki.insertLink') }}
                </button>
              </div>
            </div>
          </template>
        </template>

        <!-- Wiki 浏览 (all / entity / concept / summary tabs) -->
        <template v-else>
          <div v-if="displayPages.length === 0" class="wiki-empty">
            <t-icon name="map" class="empty-icon" />
            <p>{{ $t('notes.wiki.noPages') }}</p>
          </div>
          <div
            v-for="page in displayPages"
            :key="page.slug"
            class="wiki-card"
          >
            <div class="card-top" @click="openPage(page)">
              <span class="page-type-tag" :class="`type-${page.page_type}`">
                {{ pageTypeLabel(page.page_type) }}
              </span>
              <span class="card-title">{{ page.title }}</span>
            </div>
            <p v-if="page.summary" class="card-summary" @click="openPage(page)">{{ page.summary }}</p>
            <button class="card-insert-btn" @click.stop="emit('insertLink', page.slug, page.title)" :title="$t('notes.wiki.insertLink')">
              <t-icon name="enter" /> {{ $t('notes.wiki.insertLink') }}
            </button>
          </div>
        </template>
      </template>
    </div>

    <!-- Wiki 页面预览 Dialog -->
    <t-dialog
      v-model:visible="previewVisible"
      :header="previewPage?.title || ''"
      :width="640"
      :footer="false"
    >
      <div class="wiki-preview-content" v-html="previewHTML" />
    </t-dialog>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import { sanitizeHTML } from '@/utils/security'
import { useUIStore } from '@/stores/ui'
import { getKnowledgeBaseById } from '@/api/knowledge-base'
import { listWikiPages, searchWikiPages, type WikiPage } from '@/api/wiki'

const props = defineProps<{
  kbId: string
  knowledgeId: string | null
  noteTitle: string
  noteContent: string
  isPublished: boolean
}>()

const emit = defineEmits<{
  insertLink: [slug: string, title: string]
}>()

const { t } = useI18n()
const uiStore = useUIStore()

const panelWidth = ref(320)
const minWidth = 240
const maxWidth = 600

const loading = ref(false)
const wikiEnabled = ref(false)
const searchQuery = ref('')
const activeTab = ref<string>('all')
const allPages = ref<WikiPage[]>([])
const totalPages = ref(0)

// Links analysis
interface NoteLink {
  raw: string
  slug: string
  display: string
  resolved: boolean
  page: WikiPage | null
}
const noteLinks = ref<NoteLink[]>([])

const previewVisible = ref(false)
const previewPage = ref<WikiPage | null>(null)

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
  if (!previewPage.value?.content) return ''
  const withLinks = renderWikiLinks(previewPage.value.content)
  const parsed = marked.parse(withLinks)
  const html = typeof parsed === 'string' ? parsed : ''
  return sanitizeHTML(html)
})

// Tabs
const tabs = computed(() => {
  const byType: Record<string, number> = {}
  for (const p of allPages.value) {
    byType[p.page_type] = (byType[p.page_type] || 0) + 1
  }
  const items = [
    { key: 'all', label: t('notes.wiki.tabAll'), count: totalPages.value },
    { key: 'links', label: t('notes.wiki.tabLinks'), count: noteLinks.value.length },
  ]
  if (byType['entity']) items.push({ key: 'entity', label: t('notes.wiki.typeEntity'), count: byType['entity'] })
  if (byType['concept']) items.push({ key: 'concept', label: t('notes.wiki.typeConcept'), count: byType['concept'] })
  if (byType['summary']) items.push({ key: 'summary', label: t('notes.wiki.typeSummary'), count: byType['summary'] })
  return items
})

const displayPages = computed(() => {
  if (activeTab.value === 'links') return []
  if (activeTab.value === 'all') {
    return allPages.value.filter((p) => p.page_type !== 'index' && p.page_type !== 'log')
  }
  return allPages.value.filter((p) => p.page_type === activeTab.value)
})

let startX = 0
let startWidth = 0

const startResize = (e: MouseEvent) => {
  e.preventDefault()
  startX = e.clientX
  startWidth = panelWidth.value
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
  document.body.style.cursor = 'col-resize'
}

const onMouseMove = (e: MouseEvent) => {
  const delta = startX - e.clientX
  let w = startWidth + delta
  if (w < minWidth) w = minWidth
  if (w > maxWidth) w = maxWidth
  panelWidth.value = w
}

const onMouseUp = () => {
  document.removeEventListener('mousemove', onMouseMove)
  document.removeEventListener('mouseup', onMouseUp)
  document.body.style.cursor = ''
}

const pageTypeLabel = (type: string): string => {
  const map: Record<string, string> = {
    entity: t('notes.wiki.typeEntity'),
    concept: t('notes.wiki.typeConcept'),
    summary: t('notes.wiki.typeSummary'),
    synthesis: t('notes.wiki.typeSynthesis'),
    comparison: t('notes.wiki.typeComparison'),
    index: t('notes.wiki.typeIndex'),
    log: t('notes.wiki.typeLog'),
  }
  return map[type] || type
}

const checkWikiEnabled = async () => {
  if (!props.kbId) return
  try {
    const res: any = await getKnowledgeBaseById(props.kbId)
    const kb = res?.data
    wikiEnabled.value = !!(kb?.indexing_strategy?.wiki_enabled)
  } catch {
    wikiEnabled.value = false
  }
}

/**
 * 从笔记正文中提取所有 [[xxx]] 和 [[slug|text]] 链接，
 * 然后与已有 Wiki 页做匹配，标记 resolved / missing。
 */
const scanNoteLinks = () => {
  const regex = /\[\[([^\]|]+?)(?:\|([^\]]+?))?\]\]/g
  const results: NoteLink[] = []
  const seen = new Set<string>()
  let match: RegExpExecArray | null
  while ((match = regex.exec(props.noteContent)) !== null) {
    const slug = match[1].trim()
    const display = (match[2] || match[1]).trim()
    const key = slug.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)

    // 在 allPages 中找 slug 或 title 完全匹配
    const found = allPages.value.find(
      (p) =>
        p.slug === slug ||
        p.title.toLowerCase() === slug.toLowerCase() ||
        p.aliases?.some((a) => a.toLowerCase() === slug.toLowerCase()),
    )
    results.push({
      raw: match[0],
      slug,
      display,
      resolved: !!found,
      page: found || null,
    })
  }
  noteLinks.value = results
}

const loadContent = async () => {
  if (!props.kbId || !wikiEnabled.value) {
    allPages.value = []
    totalPages.value = 0
    scanNoteLinks()
    return
  }
  loading.value = true
  try {
    if (searchQuery.value.trim()) {
      const res: any = await searchWikiPages(props.kbId, searchQuery.value.trim(), 30)
      allPages.value = res?.pages || []
      totalPages.value = allPages.value.length
    } else {
      const res: any = await listWikiPages(props.kbId, {
        page: 1,
        page_size: 100,
        sort_by: 'updated_at',
        sort_order: 'desc',
      })
      allPages.value = res?.pages || []
      totalPages.value = typeof res?.total === 'number' ? res.total : allPages.value.length
    }
  } catch {
    allPages.value = []
    totalPages.value = 0
  } finally {
    loading.value = false
    scanNoteLinks()
  }
}

let searchTimer: ReturnType<typeof setTimeout> | null = null
const debouncedSearch = () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => loadContent(), 300)
}

const reload = () => loadContent()
const close = () => uiStore.setNotesWikiPanel(false)
const openPage = (page: WikiPage) => {
  previewPage.value = page
  previewVisible.value = true
}

watch(
  () => [props.kbId] as const,
  async () => {
    await checkWikiEnabled()
    await loadContent()
  },
  { immediate: false },
)

// 笔记内容变化时重新扫描 [[links]]
watch(
  () => props.noteContent,
  () => scanNoteLinks(),
  { immediate: false },
)

onMounted(async () => {
  await checkWikiEnabled()
  await loadContent()
})
</script>

<style scoped lang="less">
@accent: var(--td-brand-color);

.notes-wiki-panel {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  min-height: 0;
  position: relative;
}

.panel-resizer {
  position: absolute;
  top: 0;
  left: -3px;
  width: 6px;
  height: 100%;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
  transition: background 0.2s ease;

  &:hover,
  &:active {
    background: rgba(var(--td-brand-color-rgb), 0.2);
  }
}

.panel-head {
  height: 44px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  gap: 6px;
}

.head-left {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.head-icon {
  font-size: 15px;
  color: @accent;
  flex-shrink: 0;
}

.head-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  white-space: nowrap;
}

.head-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(var(--td-brand-color-rgb), 0.12);
  color: @accent;
  font-size: 11px;
  font-weight: 600;
  border-radius: 10px;
  padding: 0 6px;
  min-width: 18px;
  height: 18px;
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex-shrink: 0;
}

.head-btn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 6px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease, color 0.15s ease;

  &:hover:not(:disabled) {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  :deep(.t-icon) {
    font-size: 14px;
  }
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 搜索 */
.panel-search {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px 4px;
  position: relative;
}

.search-icon {
  font-size: 14px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.search-input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 12px;
  color: var(--td-text-color-primary);
  min-width: 0;

  &::placeholder {
    color: var(--td-text-color-placeholder);
  }
}

.search-clear {
  width: 18px;
  height: 18px;
  border: none;
  background: transparent;
  color: var(--td-text-color-placeholder);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;

  &:hover { color: var(--td-text-color-primary); }

  :deep(.t-icon) { font-size: 12px; }
}

/* Tab 栏 */
.panel-tabs {
  display: flex;
  gap: 2px;
  padding: 4px 12px 6px;
  border-bottom: 1px solid var(--td-component-stroke);
  overflow-x: auto;
  flex-shrink: 0;
}

.tab-btn {
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  font-size: 11px;
  font-weight: 500;
  padding: 3px 8px;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  transition: all 0.15s ease;

  .tab-count {
    font-size: 10px;
    color: var(--td-text-color-placeholder);
  }

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.active {
    background: rgba(var(--td-brand-color-rgb), 0.1);
    color: @accent;

    .tab-count { color: @accent; }
  }
}

.panel-body {
  flex: 1;
  overflow-y: auto;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.wiki-loading {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.wiki-prompt {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 32px 16px;
  gap: 10px;

  .prompt-icon {
    font-size: 32px;
    color: var(--td-text-color-placeholder);
    opacity: 0.5;
  }

  .prompt-title {
    margin: 0;
    font-size: 13px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .prompt-desc {
    margin: 0;
    font-size: 12px;
    color: var(--td-text-color-secondary);
    line-height: 1.5;
  }
}

.wiki-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--td-text-color-placeholder);
  padding: 32px 16px;
  text-align: center;

  .empty-icon { font-size: 28px; opacity: 0.4; }
  p { margin: 0; font-size: 12px; }
  .empty-hint { font-size: 11px; opacity: 0.7; }
}

.wiki-card {
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  transition: border-color 0.15s ease, background 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;

  &:hover {
    border-color: @accent;
    background: rgba(var(--td-brand-color-rgb), 0.04);
  }

  &.is-missing {
    border-color: var(--td-warning-color);
    border-style: dashed;

    &:hover {
      background: rgba(var(--td-warning-color-rgb, 255, 152, 0), 0.06);
    }
  }
}

.card-top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  cursor: pointer;
}

.page-type-tag {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  flex-shrink: 0;
  text-transform: uppercase;
  letter-spacing: 0.04em;

  &.type-entity {
    background: rgba(var(--td-brand-color-rgb), 0.12);
    color: @accent;
  }

  &.type-concept {
    background: rgba(var(--td-warning-color-rgb, 255, 152, 0), 0.12);
    color: var(--td-warning-color);
  }

  &.type-summary {
    background: rgba(var(--td-success-color-rgb, 0, 186, 78), 0.12);
    color: var(--td-success-color);
  }

  &.type-synthesis,
  &.type-comparison {
    background: rgba(var(--td-error-color-rgb, 229, 0, 18), 0.1);
    color: var(--td-error-color);
  }
}

.link-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;

  &.dot-ok { background: var(--td-success-color); }
  &.dot-missing { background: var(--td-warning-color); }
}

.link-badge {
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 4px;
  flex-shrink: 0;
  margin-left: auto;

  &.ok {
    background: rgba(var(--td-success-color-rgb, 0, 186, 78), 0.12);
    color: var(--td-success-color);
  }

  &.missing {
    background: rgba(var(--td-warning-color-rgb, 255, 152, 0), 0.12);
    color: var(--td-warning-color);
  }
}

.card-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}

.card-summary {
  margin: 0;
  font-size: 11px;
  color: var(--td-text-color-secondary);
  line-height: 1.5;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  cursor: pointer;
}

.card-actions {
  display: flex;
  gap: 8px;
  padding-top: 2px;
}

.card-action-btn,
.card-insert-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: none;
  background: transparent;
  color: var(--td-text-color-placeholder);
  font-size: 11px;
  cursor: pointer;
  padding: 2px 0 0;
  transition: color 0.15s ease;

  :deep(.t-icon) { font-size: 12px; }

  &:hover {
    color: @accent;
  }
}

.wiki-preview-content {
  font-size: 14px;
  line-height: 1.7;
  color: var(--td-text-color-primary);
  max-height: 60vh;
  overflow-y: auto;

  :deep(h1), :deep(h2), :deep(h3) {
    margin: 16px 0 8px;
  }

  :deep(a) {
    color: @accent;
  }

  :deep(code) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 2px 4px;
    border-radius: 4px;
    font-family: monospace;
  }

  :deep(pre) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 12px;
    border-radius: 6px;
    overflow: auto;
  }

  :deep(blockquote) {
    border-left: 3px solid @accent;
    padding-left: 12px;
    color: var(--td-text-color-secondary);
    margin: 12px 0;
  }
}
</style>
