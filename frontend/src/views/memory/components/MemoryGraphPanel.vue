<template>
  <div class="memory-graph">
    <div class="memory-graph__toolbar">
      <t-radio-group v-model="mode" variant="default-filled">
        <t-radio-button value="personal">{{ t('memory.graph.modePersonal') }}</t-radio-button>
        <t-radio-button value="bridged">{{ t('memory.graph.modeBridged') }}</t-radio-button>
      </t-radio-group>

      <span class="memory-graph__hint">
        {{ mode === 'bridged' ? t('memory.graph.bridgedHint') : t('memory.graph.personalHint') }}
      </span>

      <div class="memory-graph__legend">
        <span class="legend-item"><i class="legend-dot is-memory" />{{ t('memory.graph.legendMemory') }}</span>
        <span v-if="mode === 'bridged'" class="legend-item">
          <i class="legend-dot is-wiki" />{{ t('memory.graph.legendWiki') }}
        </span>
      </div>
    </div>

    <div class="memory-graph__canvas">
      <GraphCanvas
        :nodes="graph.nodes"
        :edges="graph.edges"
        :node-style="nodeStyle"
        :active-id="activeId"
        :empty-text="t('memory.graph.empty')"
        @select="onSelect"
      />
    </div>

    <p v-if="mode === 'bridged' && !hasSatellites" class="memory-graph__note">
      {{ t('memory.graph.noAnchors') }}
    </p>

    <p v-if="graph.meta?.truncated" class="memory-graph__truncated">
      {{ t('memory.graph.truncated', { returned: graph.meta.returned, total: graph.meta.total }) }}
    </p>
  </div>
</template>

<script setup lang="ts">
/**
 * The personal memory graph, and the bridged view that shows where it attaches
 * to the organisation's knowledge base.
 *
 * The bridged mode is the point of the whole feature made visible: memory nodes
 * are the user's own understanding, the dashed satellites are the wiki pages
 * that understanding is anchored to.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

import GraphCanvas, { type GraphCanvasNode, type GraphNodeStyle } from '@/components/graph/GraphCanvas.vue'
import { getMemoryGraph, type MemoryGraphData, type MemoryGraphNode } from '@/api/memory'

const { t } = useI18n()
const emit = defineEmits<{ (e: 'open', slug: string): void }>()

const mode = ref<'personal' | 'bridged'>('personal')
const activeId = ref('')
const graph = ref<MemoryGraphData>({ nodes: [], edges: [], meta: { mode: 'personal', total: 0, returned: 0, truncated: false } })

// A categorical palette: seven memory types that have to stay apart at a
// glance. Deliberately literal hex rather than theme tokens, for two reasons —
// the values land in SVG presentation attributes where var() is not reliably
// resolved, and the fill is derived by appending an alpha pair, which only works
// on six-digit hex. Reading a token that returns rgba() produced solid black
// nodes. The hues are chosen around the product's green.
// Bridged mode draws memory → wiki-page edges. An ordinary knowledge base has
// no wiki pages, so its anchors — which are real, and do rank its content higher
// — have nothing to attach to here. Saying so beats an empty canvas.
const hasSatellites = computed(() =>
  (graph.value.nodes || []).some((node) => node.kind === 'wiki' || node.kind === 'knowledge'),
)

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

// GraphCanvas is deliberately ignorant of what its nodes mean, so the styling
// callback narrows back to the memory node shape it was given.
function nodeStyle(raw: GraphCanvasNode): GraphNodeStyle {
  const node = raw as MemoryGraphNode
  if (node.kind === 'wiki' || node.kind === 'knowledge') {
    // Knowledge-base items are hollow, dashed satellites so they read as
    // "somewhere else that this connects to" rather than as memories. Wiki pages
    // and ordinary documents share the treatment: from the reader's side they
    // are the same thing, something in the knowledge base they have engaged
    // with, and the label already says which.
    return {
      radius: 8,
      fill: '#ffffff',
      stroke: MEMORY_TYPE_COLOURS.open_question,
      strokeWidth: 1.5,
      dashed: true,
    }
  }
  const colour = MEMORY_TYPE_COLOURS[node.type || ''] || FALLBACK_COLOUR
  // Well-connected memories are drawn larger: size tracks how central a memory
  // is to the rest, which is the thing worth noticing at a glance.
  const radius = Math.min(18, 9 + Math.sqrt(node.link_count || 0) * 2.5)
  return { radius, fill: `${colour}22`, stroke: colour, strokeWidth: 2 }
}

async function load() {
  try {
    const res: any = await getMemoryGraph({ mode: mode.value, limit: 200 })
    graph.value = res?.data || { nodes: [], edges: [], meta: { mode: mode.value, total: 0, returned: 0, truncated: false } }
  } catch (error: any) {
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
    graph.value = { nodes: [], edges: [], meta: { mode: mode.value, total: 0, returned: 0, truncated: false } }
  }
}

function onSelect(id: string) {
  activeId.value = id
  const node = graph.value.nodes.find((item) => item.id === id)
  if (node?.kind === 'memory') {
    emit('open', node.slug)
  }
}

// Watch the model rather than handling @change: TDesign fires the change
// callback around the v-model write, so a handler that reads mode.value could
// see the previous selection and fetch the wrong graph.
watch(mode, load)

onMounted(load)
</script>

<style scoped lang="less">
.memory-graph {

  &__toolbar {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

  &__hint {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }

  &__legend {
    margin-left: auto;
    display: flex;
    gap: 14px;
  }

  &__canvas {
    height: 520px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 10px;
    overflow: hidden;
  }

  &__note {
    margin: 10px 0 0;
    font-size: 12px;
    line-height: 1.55;
    color: var(--td-text-color-placeholder);
  }

  &__truncated {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }
}

.legend-item {
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
  display: inline-block;

  &.is-memory {
    background: var(--td-brand-color-1);
    border: 2px solid var(--td-brand-color);
  }

  &.is-wiki {
    background: #fff;
    border: 1.5px dashed var(--td-warning-color);
  }
}
</style>
