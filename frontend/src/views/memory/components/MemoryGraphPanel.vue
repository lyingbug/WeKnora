<template>
  <div class="memory-graph">
    <div class="memory-graph__toolbar">
      <t-radio-group v-model="mode" variant="default-filled" @change="load">
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
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

import GraphCanvas, { type GraphCanvasNode, type GraphNodeStyle } from '@/components/graph/GraphCanvas.vue'
import { getMemoryGraph, type MemoryGraphData, type MemoryGraphNode } from '@/api/memory'

const { t } = useI18n()
const emit = defineEmits<{ (e: 'open', slug: string): void }>()

const mode = ref<'personal' | 'bridged'>('personal')
const activeId = ref('')
const graph = ref<MemoryGraphData>({ nodes: [], edges: [], meta: { mode: 'personal', total: 0, returned: 0, truncated: false } })

const typeColors: Record<string, string> = {
  profile: '#0052d9',
  preference: '#0594fa',
  project: '#2ba471',
  entity: '#834ec2',
  topic: '#078d7a',
  episode: '#5e5e5e',
  open_question: '#e37318',
}

// GraphCanvas is deliberately ignorant of what its nodes mean, so the styling
// callback narrows back to the memory node shape it was given.
function nodeStyle(raw: GraphCanvasNode): GraphNodeStyle {
  const node = raw as MemoryGraphNode
  if (node.kind === 'wiki') {
    // Knowledge-base pages are drawn as hollow, dashed satellites so they read
    // as "somewhere else that this connects to" rather than as memories.
    return { radius: 8, fill: '#ffffff', stroke: '#e37318', strokeWidth: 1.5, dashed: true }
  }
  const colour = typeColors[node.type || ''] || '#0052d9'
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

onMounted(load)
</script>

<style scoped lang="less">
.memory-graph {
  padding: 16px 4px 8px;

  &__toolbar {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

  &__hint {
    font-size: 12px;
    color: var(--td-text-color-secondary, #888);
  }

  &__legend {
    margin-left: auto;
    display: flex;
    gap: 14px;
  }

  &__canvas {
    height: 520px;
    border: 1px solid var(--td-component-stroke, #e7e7e7);
    border-radius: 10px;
    overflow: hidden;
  }

  &__truncated {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--td-text-color-placeholder, #999);
  }
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary, #888);
}

.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;

  &.is-memory {
    background: #0052d922;
    border: 2px solid #0052d9;
  }

  &.is-wiki {
    background: #fff;
    border: 1.5px dashed #e37318;
  }
}
</style>
