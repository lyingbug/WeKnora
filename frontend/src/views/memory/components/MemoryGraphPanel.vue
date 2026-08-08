<template>
  <div class="memory-graph">
    <div class="memory-graph__frame">
      <div class="memory-graph__bar">
        <t-radio-group v-model="mode" variant="default-filled" size="small">
          <t-radio-button value="personal">{{ t('memory.graph.modePersonal') }}</t-radio-button>
          <t-radio-button value="bridged">{{ t('memory.graph.modeBridged') }}</t-radio-button>
        </t-radio-group>

        <t-popup trigger="click" placement="bottom-left" :show-arrow="true"
          overlay-class-name="memory-graph-help-popup">
          <button type="button" class="memory-graph__help" :title="t('memory.graph.helpTitle')">
            <t-icon name="help-circle" />
          </button>
          <template #content>
            <div class="memory-graph-help">
              <div class="memory-graph-help__title">{{ t('memory.graph.helpTitle') }}</div>
              <div class="memory-graph-help__rows">
                <div v-for="row in helpRows" :key="row.action" class="memory-graph-help__row">
                  <span class="memory-graph-help__key">{{ row.action }}</span>
                  <span class="memory-graph-help__desc">{{ row.desc }}</span>
                </div>
              </div>
            </div>
          </template>
        </t-popup>

        <span v-if="graph.meta?.truncated" class="memory-graph__truncated">
          {{ t('memory.graph.truncated', { returned: graph.meta.returned, total: graph.meta.total }) }}
        </span>

        <div class="memory-graph__bar-spacer" />

        <div class="memory-graph__legend-inline">
          <span class="legend-inline-item">
            <i class="legend-dot is-memory" />
            {{ t('memory.graph.legendMemory') }}
          </span>
          <span v-if="mode === 'bridged'" class="legend-inline-item">
            <i class="legend-dot is-wiki" />
            {{ t('memory.graph.legendWiki') }}
          </span>
        </div>
      </div>

      <div class="memory-graph__canvas-wrap">
        <GraphCanvas ref="canvasRef" :nodes="graph.nodes" :edges="graph.edges" :node-style="nodeStyle"
          :active-id="activeId" :empty-text="t('memory.graph.empty')" @select="onNodeSelect" @center="onNodeCenter" />

        <div v-if="mode === 'bridged' && graph.nodes.length && !hasSatellites" class="memory-graph__empty-note">
          {{ t('memory.graph.noAnchors') }}
        </div>
      </div>

      <t-drawer v-model:visible="drawerVisible" :header="drawerTitle" size="480px" :footer="false" placement="right"
        :show-overlay="false" :close-btn="true" destroy-on-close class="memory-graph-drawer">
        <t-loading :loading="drawerLoading" size="small">
          <template v-if="drawerNode">
            <div class="memory-graph-drawer__meta">
              <t-tag size="small" :theme="drawerTagTheme" variant="light-outline">
                {{ drawerTypeLabel }}
              </t-tag>
              <span v-if="drawerPage" class="memory-graph-drawer__version">
                {{ t('memory.graph.drawerVersion', { ver: drawerPage.version }) }}
              </span>
              <t-button v-if="drawerNode.kind === 'memory'" size="small" variant="outline" theme="primary"
                class="memory-graph-drawer__edit" @click="emit('edit', drawerNode.slug)">
                <template #icon><t-icon name="edit" /></template>
                {{ t('memory.graph.drawerEdit') }}
              </t-button>
              <t-button v-else-if="wikiGraphUrl" size="small" variant="outline" theme="default"
                class="memory-graph-drawer__edit" @click="openWikiGraph">
                <template #icon><t-icon name="jump" /></template>
                {{ t('memory.graph.drawerOpenWiki') }}
              </t-button>
            </div>

            <p v-if="drawerSummary && drawerNode.kind === 'memory'" class="memory-graph-drawer__summary">
              {{ drawerSummary }}
            </p>

            <div v-if="drawerBodyHtml" class="memory-graph-drawer__body" v-html="drawerBodyHtml"
              @click="handleDrawerClick" />
            <div v-else-if="!drawerLoading && drawerNode.kind !== 'memory'" class="memory-graph-drawer__satellite">
              <t-icon name="folder-open" class="memory-graph-drawer__satellite-icon" />
              <p class="memory-graph-drawer__satellite-title">{{ drawerNode.title }}</p>
              <p class="memory-graph-drawer__satellite-desc">{{ t('memory.graph.drawerSatelliteHint') }}</p>
            </div>
            <p v-else-if="!drawerLoading" class="memory-graph-drawer__empty">
              {{ t('memory.graph.drawerNoContent') }}
            </p>
          </template>
        </t-loading>
      </t-drawer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { marked } from 'marked'

import GraphCanvas, { type GraphCanvasNode, type GraphNodeStyle } from '@/components/graph/GraphCanvas.vue'
import { sanitizeMarkdownHTML } from '@/utils/security'
import {
  getMemoryGraph,
  getMemoryPage,
  type MemoryGraphData,
  type MemoryGraphNode,
  type MemoryPage,
} from '@/api/memory'

const { t } = useI18n()
const emit = defineEmits<{ (e: 'edit', slug: string): void }>()

const mode = ref<'personal' | 'bridged'>('personal')
const activeId = ref('')
const graph = ref<MemoryGraphData>({
  nodes: [],
  edges: [],
  meta: { mode: 'personal', total: 0, returned: 0, truncated: false },
})

const canvasRef = ref<InstanceType<typeof GraphCanvas> | null>(null)
const drawerVisible = ref(false)
const drawerLoading = ref(false)
const drawerNode = ref<MemoryGraphNode | null>(null)
const drawerPage = ref<MemoryPage | null>(null)

let pendingClickTimer: number | null = null
const DRAWER_PAN_OFFSET = 240

const helpRows = computed(() => [
  { action: t('memory.graph.helpClick'), desc: t('memory.graph.helpClickDesc') },
  { action: t('memory.graph.helpDblClick'), desc: t('memory.graph.helpDblClickDesc') },
  { action: t('memory.graph.helpDrag'), desc: t('memory.graph.helpDragDesc') },
  { action: t('memory.graph.helpZoom'), desc: t('memory.graph.helpZoomDesc') },
])

const hasSatellites = computed(() =>
  (graph.value.nodes || []).some((node) => node.kind === 'wiki' || node.kind === 'knowledge'),
)

const drawerTitle = computed(() => {
  if (drawerPage.value?.title) return drawerPage.value.title
  return drawerNode.value?.title || ''
})

const drawerSummary = computed(() => drawerPage.value?.summary || '')

const drawerTagTheme = computed(() => {
  const type = drawerPage.value?.page_type || drawerNode.value?.type || ''
  if (type === 'open_question') return 'warning'
  if (type === 'preference' || type === 'profile') return 'primary'
  if (type === 'project') return 'success'
  return 'default'
})

const drawerTypeLabel = computed(() => {
  if (drawerNode.value?.kind === 'wiki' || drawerNode.value?.kind === 'knowledge') {
    return t('memory.graph.legendWiki')
  }
  const type = drawerPage.value?.page_type || drawerNode.value?.type || 'episode'
  return t(`memory.types.${type}`)
})

const wikiGraphUrl = computed(() => {
  if (!drawerNode.value || drawerNode.value.kind === 'memory') return ''
  const kbId = drawerNode.value.knowledge_base_id
  if (!kbId || !drawerNode.value.slug) return ''
  return `/platform/knowledge-bases/${kbId}?tab=graph&slug=${encodeURIComponent(drawerNode.value.slug)}`
})

const drawerBodyHtml = computed(() => {
  const content = drawerPage.value?.content
  if (!content) return ''
  let preprocessed = content.replace(/\[\[([^\]]+)\]\]/g, (_, inner: string) => {
    const pipeIdx = inner.indexOf('|')
    const slug = pipeIdx > 0 ? inner.substring(0, pipeIdx).trim() : inner.trim()
    const display = pipeIdx > 0 ? inner.substring(pipeIdx + 1).trim() : slug
    return `<a href="#" class="wiki-content-link" data-slug="${slug}">${display}</a>`
  })
  const html = marked.parse(preprocessed, { breaks: true, async: false }) as string
  return sanitizeMarkdownHTML(html)
})

const MEMORY_TYPE_COLOURS: Record<string, string> = {
  profile: '#07c05f',
  preference: '#059e8a',
  project: '#0b8a45',
  entity: '#834ec2',
  topic: '#078d7a',
  episode: '#6b7280',
  open_question: '#e37318',
}

const FALLBACK_COLOUR = MEMORY_TYPE_COLOURS.profile

function nodeStyle(raw: GraphCanvasNode): GraphNodeStyle {
  const node = raw as MemoryGraphNode
  if (node.kind === 'wiki' || node.kind === 'knowledge') {
    return {
      radius: 9,
      fill: '#ffffff',
      stroke: '#e37318',
      strokeWidth: 1.5,
      dashed: true,
    }
  }
  const colour = MEMORY_TYPE_COLOURS[node.type || ''] || FALLBACK_COLOUR
  const radius = Math.min(20, 10 + Math.sqrt(node.link_count || 0) * 2.5)
  return { radius, fill: `${colour}28`, stroke: colour, strokeWidth: 2 }
}

async function load() {
  try {
    const res: any = await getMemoryGraph({ mode: mode.value, limit: 200 })
    graph.value = res?.data || {
      nodes: [],
      edges: [],
      meta: { mode: mode.value, total: 0, returned: 0, truncated: false },
    }
    if (drawerVisible.value && drawerNode.value) {
      const still = graph.value.nodes.find((n) => n.id === drawerNode.value?.id)
      if (!still) {
        drawerVisible.value = false
        drawerNode.value = null
        drawerPage.value = null
        activeId.value = ''
      }
    }
  } catch (error: any) {
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
    graph.value = {
      nodes: [],
      edges: [],
      meta: { mode: mode.value, total: 0, returned: 0, truncated: false },
    }
  }
}

function panToNode(nodeId: string) {
  canvasRef.value?.panToNode(nodeId, drawerVisible.value ? DRAWER_PAN_OFFSET : 0)
}

async function openDrawerForNode(nodeId: string) {
  const node = graph.value.nodes.find((item) => item.id === nodeId)
  if (!node) return

  activeId.value = nodeId
  drawerNode.value = node
  drawerVisible.value = true
  panToNode(nodeId)

  if (node.kind === 'memory') {
    drawerLoading.value = true
    drawerPage.value = null
    try {
      const res: any = await getMemoryPage(node.slug)
      drawerPage.value = res?.data || null
    } catch {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    } finally {
      drawerLoading.value = false
    }
  } else {
    drawerPage.value = null
    drawerLoading.value = false
  }
}

function onNodeSelect(id: string) {
  if (pendingClickTimer) clearTimeout(pendingClickTimer)
  pendingClickTimer = window.setTimeout(() => {
    openDrawerForNode(id)
    pendingClickTimer = null
  }, 220)
}

function onNodeCenter(id: string) {
  if (pendingClickTimer) {
    clearTimeout(pendingClickTimer)
    pendingClickTimer = null
  }
  const node = graph.value.nodes.find((item) => item.id === id)
  if (node?.kind === 'memory') {
    emit('edit', node.slug)
  }
}

function handleDrawerClick(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (target.classList.contains('wiki-content-link')) {
    event.preventDefault()
    const slug = target.getAttribute('data-slug')
    if (!slug) return
    const linked = graph.value.nodes.find((n) => n.kind === 'memory' && n.slug === slug)
    if (linked) {
      openDrawerForNode(linked.id)
      return
    }
    emit('edit', slug)
  }
}

function openWikiGraph() {
  if (!wikiGraphUrl.value) return
  window.open(wikiGraphUrl.value, '_blank', 'noopener')
}

watch(drawerVisible, (visible) => {
  if (!visible) {
    activeId.value = ''
    drawerNode.value = null
    drawerPage.value = null
  } else if (activeId.value) {
    panToNode(activeId.value)
  }
})

watch(mode, load)

onMounted(load)
</script>

<style scoped lang="less">
.memory-graph {
  height: calc(100vh - 248px);
  min-height: 440px;
  max-height: 680px;
}

.memory-graph__frame {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 100%;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  overflow: hidden;
  background: var(--td-bg-color-container);
}

.memory-graph__bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  padding: 8px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
}

.memory-graph__help {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--td-text-color-placeholder);
  cursor: pointer;

  &:hover {
    color: var(--td-brand-color);
    background: var(--td-bg-color-container);
  }
}

.memory-graph__truncated {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.memory-graph__bar-spacer {
  flex: 1;
  min-width: 8px;
}

.memory-graph__legend-inline {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-shrink: 0;
}

.legend-inline-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;

  &.is-memory {
    background: rgba(7, 192, 95, 0.3);
    border: 2px solid var(--td-brand-color);
  }

  &.is-wiki {
    background: #fff;
    border: 1.5px dashed #e37318;
  }
}

.memory-graph__canvas-wrap {
  position: relative;
  flex: 1;
  min-height: 0;

  :deep(.graph-canvas) {
    border-radius: 0;
    min-height: 100%;
    background: var(--td-bg-color-container);
  }
}

.memory-graph__empty-note {
  position: absolute;
  left: 50%;
  bottom: 14px;
  transform: translateX(-50%);
  z-index: 5;
  max-width: 88%;
  padding: 6px 12px;
  font-size: 12px;
  line-height: 1.45;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  box-shadow: var(--td-shadow-1);
  text-align: center;
}

:deep(.memory-graph-drawer) {
  box-shadow: -4px 0 16px rgba(0, 0, 0, 0.08);
}

.memory-graph-drawer__meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.memory-graph-drawer__version {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.memory-graph-drawer__edit {
  margin-left: auto;
}

.memory-graph-drawer__summary {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
}

.memory-graph-drawer__empty {
  margin: 0;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.memory-graph-drawer__satellite {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 28px 16px;
  text-align: center;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-secondarycontainer);
}

.memory-graph-drawer__satellite-icon {
  font-size: 28px;
  color: var(--td-warning-color);
}

.memory-graph-drawer__satellite-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.memory-graph-drawer__satellite-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
  max-width: 320px;
}

.memory-graph-drawer__body {
  line-height: 1.6;
  font-size: 14px;
  color: var(--td-text-color-primary);

  :deep(p) {
    margin: 0 0 12px;
  }

  :deep(a.wiki-content-link) {
    color: var(--td-brand-color);
    text-decoration: none;

    &:hover {
      text-decoration: underline;
    }
  }
}
</style>

<style lang="less">
.memory-graph-help-popup {
  .memory-graph-help {
    padding: 4px 2px;
    max-width: 280px;

    &__title {
      font-size: 13px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      margin-bottom: 8px;
    }

    &__rows {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }

    &__row {
      display: flex;
      gap: 10px;
      font-size: 12px;
      line-height: 1.45;
    }

    &__key {
      flex-shrink: 0;
      color: var(--td-text-color-primary);
      font-weight: 500;
      min-width: 72px;
    }

    &__desc {
      color: var(--td-text-color-secondary);
    }
  }
}
</style>
