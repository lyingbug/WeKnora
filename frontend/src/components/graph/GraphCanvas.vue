<template>
  <div ref="containerRef" class="graph-canvas" :class="{ 'is-empty': !nodes.length }">
    <div v-if="!nodes.length" class="graph-canvas__empty">
      <slot name="empty">{{ emptyText }}</slot>
    </div>
    <svg v-else ref="svgRef" class="graph-canvas__svg" @wheel.prevent="onWheel" @mousedown="onCanvasDown">
      <g :transform="`translate(${transform.x} ${transform.y}) scale(${transform.k})`">
        <line
          v-for="(edge, index) in renderedEdges"
          :key="`e-${index}`"
          class="graph-canvas__edge"
          :class="[`is-${edge.kind}`, { 'is-dimmed': isDimmed(edge) }]"
          :x1="edge.x1"
          :y1="edge.y1"
          :x2="edge.x2"
          :y2="edge.y2"
        />
        <g
          v-for="node in layoutNodes"
          :key="node.id"
          class="graph-canvas__node"
          :class="{ 'is-dimmed': isNodeDimmed(node.id), 'is-active': node.id === activeId }"
          :transform="`translate(${node.x} ${node.y})`"
          @mousedown.stop="onNodeDown(node, $event)"
          @mouseenter="hoveredId = node.id"
          @mouseleave="hoveredId = ''"
          @click.stop="$emit('select', node.id)"
        >
          <circle
            :r="styleFor(node).radius"
            :fill="styleFor(node).fill"
            :stroke="styleFor(node).stroke"
            :stroke-width="styleFor(node).strokeWidth"
            :stroke-dasharray="styleFor(node).dashed ? '3 2' : undefined"
          />
          <text
            class="graph-canvas__label"
            :y="styleFor(node).radius + 12"
            text-anchor="middle"
          >{{ truncate(node.title || node.id) }}<title>{{ node.title || node.id }}</title></text>
        </g>
      </g>
    </svg>

    <div v-if="nodes.length" class="graph-canvas__controls">
      <t-button size="small" variant="outline" shape="square" @click="zoomBy(1.2)">
        <t-icon name="add" />
      </t-button>
      <t-button size="small" variant="outline" shape="square" @click="zoomBy(1 / 1.2)">
        <t-icon name="remove" />
      </t-button>
      <t-button size="small" variant="outline" shape="square" @click="resetView">
        <t-icon name="fullscreen" />
      </t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * A small, self-contained force-directed graph.
 *
 * Node appearance is entirely delegated to the `nodeStyle` prop, so the same
 * canvas renders a personal memory graph, a bridged memory-and-knowledge graph,
 * or anything else, without this component knowing what any of it means.
 *
 * Written from scratch rather than pulling in a graph library: the wiki browser
 * already ships a hand-rolled SVG force layout, and adding d3 or cytoscape to
 * the bundle to draw a few hundred circles would cost far more than it saves.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export interface GraphCanvasNode {
  id: string
  title?: string
  [key: string]: any
}

export interface GraphCanvasEdge {
  source: string
  target: string
  kind?: string
}

export interface GraphNodeStyle {
  radius: number
  fill: string
  stroke: string
  strokeWidth: number
  dashed?: boolean
}

const props = withDefaults(
  defineProps<{
    nodes: GraphCanvasNode[]
    edges: GraphCanvasEdge[]
    nodeStyle?: (node: GraphCanvasNode) => GraphNodeStyle
    activeId?: string
    emptyText?: string
  }>(),
  {
    activeId: '',
    emptyText: '',
  },
)

defineEmits<{ (e: 'select', id: string): void }>()

interface SimNode {
  id: string
  title?: string
  raw: GraphCanvasNode
  x: number
  y: number
  vx: number
  vy: number
  pinned: boolean
}

const containerRef = ref<HTMLElement | null>(null)
const svgRef = ref<SVGSVGElement | null>(null)
const layoutNodes = ref<SimNode[]>([])
const hoveredId = ref('')
const transform = ref({ x: 0, y: 0, k: 1 })

let frame = 0
let dragging: SimNode | null = null
let panning: { x: number; y: number } | null = null
let width = 800
let height = 600

const defaultStyle: GraphNodeStyle = {
  radius: 10,
  // Literal rather than var(): these land in SVG presentation attributes, where
  // a custom property is not reliably resolved. Callers that care pass their own
  // colours; this is only the fallback, and it follows the product's green.
  fill: '#e9f8ec',
  stroke: '#07c05f',
  strokeWidth: 1.5,
}

function styleFor(node: SimNode): GraphNodeStyle {
  return props.nodeStyle ? props.nodeStyle(node.raw) : defaultStyle
}

const adjacency = computed(() => {
  const map = new Map<string, Set<string>>()
  for (const edge of props.edges) {
    if (!map.has(edge.source)) map.set(edge.source, new Set())
    if (!map.has(edge.target)) map.set(edge.target, new Set())
    map.get(edge.source)!.add(edge.target)
    map.get(edge.target)!.add(edge.source)
  }
  return map
})

const renderedEdges = computed(() => {
  const byId = new Map(layoutNodes.value.map((n) => [n.id, n]))
  const out: Array<{ x1: number; y1: number; x2: number; y2: number; kind: string; source: string; target: string }> = []
  for (const edge of props.edges) {
    const a = byId.get(edge.source)
    const b = byId.get(edge.target)
    if (!a || !b) continue
    out.push({ x1: a.x, y1: a.y, x2: b.x, y2: b.y, kind: edge.kind || 'link', source: edge.source, target: edge.target })
  }
  return out
})

function isNodeDimmed(id: string): boolean {
  if (!hoveredId.value) return false
  if (id === hoveredId.value) return false
  return !adjacency.value.get(hoveredId.value)?.has(id)
}

function isDimmed(edge: { source: string; target: string }): boolean {
  if (!hoveredId.value) return false
  return edge.source !== hoveredId.value && edge.target !== hoveredId.value
}

function truncate(text: string): string {
  return text.length > 14 ? `${text.slice(0, 13)}…` : text
}

// Approximate rendered label width at 11px. CJK glyphs are full-width and latin
// ones roughly half that; being a few pixels out is fine, since this only feeds
// a separation constraint.
function labelHalfWidth(text: string): number {
  let width = 0
  for (const ch of truncate(text)) {
    width += ch.charCodeAt(0) > 0x2e80 ? 11 : 6
  }
  return width / 2
}

/**
 * Seed positions on a circle and let the simulation sort them out.
 *
 * Existing coordinates are carried over when a node survives a data refresh, so
 * expanding a neighbourhood nudges the layout instead of throwing the whole
 * canvas into the air and making the user re-find what they were looking at.
 */
function buildLayout() {
  const previous = new Map(layoutNodes.value.map((n) => [n.id, n]))
  const count = props.nodes.length || 1
  const radius = Math.min(width, height) / 3

  layoutNodes.value = props.nodes.map((node, index) => {
    const existing = previous.get(node.id)
    if (existing) {
      return { ...existing, raw: node, title: node.title }
    }
    const angle = (index / count) * Math.PI * 2
    return {
      id: node.id,
      title: node.title,
      raw: node,
      x: width / 2 + Math.cos(angle) * radius,
      y: height / 2 + Math.sin(angle) * radius,
      vx: 0,
      vy: 0,
      pinned: false,
    }
  })
  restartSimulation()
}

const REPULSION = 4200
const SPRING = 0.012
const SPRING_LENGTH = 90
const GRAVITY = 0.008
const DAMPING = 0.86
// The simulation stops itself rather than spinning forever: a graph that keeps
// drifting under the cursor is harder to read than one that has settled.
const MAX_TICKS = 400
// Space a caption needs beside its neighbour, and how close two captions have to
// be vertically before they can collide at all.
const LABEL_GAP = 12
const LABEL_LINE_HEIGHT = 16

let ticks = 0

function restartSimulation() {
  ticks = 0
  if (!frame) frame = requestAnimationFrame(tick)
}

function tick() {
  frame = 0
  const nodes = layoutNodes.value
  if (!nodes.length) return

  const byId = new Map(nodes.map((n) => [n.id, n]))

  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      const a = nodes[i]
      const b = nodes[j]
      let dx = a.x - b.x
      let dy = a.y - b.y
      let distSq = dx * dx + dy * dy
      if (distSq < 1) {
        // Perfectly coincident nodes have no direction to separate along, so
        // give them one instead of dividing by zero.
        dx = Math.random() - 0.5
        dy = Math.random() - 0.5
        distSq = 1
      }
      const force = REPULSION / distSq
      const dist = Math.sqrt(distSq)
      const fx = (dx / dist) * force
      const fy = (dy / dist) * force
      a.vx += fx
      a.vy += fy
      b.vx -= fx
      b.vy -= fy
    }
  }

  for (const edge of props.edges) {
    const a = byId.get(edge.source)
    const b = byId.get(edge.target)
    if (!a || !b) continue
    const dx = b.x - a.x
    const dy = b.y - a.y
    const dist = Math.sqrt(dx * dx + dy * dy) || 1
    const force = (dist - SPRING_LENGTH) * SPRING
    const fx = (dx / dist) * force
    const fy = (dy / dist) * force
    a.vx += fx
    a.vy += fy
    b.vx -= fx
    b.vy -= fy
  }

  const cx = width / 2
  const cy = height / 2
  let motion = 0
  for (const node of nodes) {
    if (node.pinned) {
      node.vx = 0
      node.vy = 0
      continue
    }
    node.vx += (cx - node.x) * GRAVITY
    node.vy += (cy - node.y) * GRAVITY
    node.vx *= DAMPING
    node.vy *= DAMPING
    node.x += node.vx
    node.y += node.vy
    motion += Math.abs(node.vx) + Math.abs(node.vy)
  }

  // Keep labels off each other. Repulsion balances gravity at roughly a hundred
  // pixels, which is about how wide a dozen Chinese characters render, so two
  // memories with ordinary titles settled with their captions overlapping into
  // one unreadable line. Resolving it positionally rather than with more
  // repulsion keeps dense graphs from flying apart.
  for (let i = 0; i < nodes.length; i++) {
    for (let j = i + 1; j < nodes.length; j++) {
      const a = nodes[i]
      const b = nodes[j]
      const needed =
        labelHalfWidth(a.title || a.id) + labelHalfWidth(b.title || b.id) + LABEL_GAP
      const dx = b.x - a.x
      const dy = b.y - a.y
      // Only captions on roughly the same line can collide.
      if (Math.abs(dy) > LABEL_LINE_HEIGHT) continue
      const gap = Math.abs(dx)
      if (gap >= needed) continue
      const push = (needed - gap) / 2
      const dir = dx === 0 ? (i % 2 === 0 ? 1 : -1) : Math.sign(dx)
      if (!a.pinned) a.x -= push * dir
      if (!b.pinned) b.x += push * dir
    }
  }

  layoutNodes.value = [...nodes]
  ticks++
  if (motion > 0.5 && ticks < MAX_TICKS) {
    frame = requestAnimationFrame(tick)
  }
}

function measure() {
  const el = containerRef.value
  if (!el) return
  width = el.clientWidth || 800
  height = el.clientHeight || 600
}

function onWheel(event: WheelEvent) {
  const factor = event.deltaY < 0 ? 1.1 : 1 / 1.1
  zoomBy(factor, event.offsetX, event.offsetY)
}

function zoomBy(factor: number, originX = width / 2, originY = height / 2) {
  const next = Math.min(4, Math.max(0.25, transform.value.k * factor))
  const scale = next / transform.value.k
  transform.value = {
    k: next,
    x: originX - (originX - transform.value.x) * scale,
    y: originY - (originY - transform.value.y) * scale,
  }
}

function resetView() {
  transform.value = { x: 0, y: 0, k: 1 }
  measure()
  buildLayout()
}

function onCanvasDown(event: MouseEvent) {
  panning = { x: event.clientX - transform.value.x, y: event.clientY - transform.value.y }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

function onNodeDown(node: SimNode, event: MouseEvent) {
  dragging = node
  node.pinned = true
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
  event.preventDefault()
}

function onMove(event: MouseEvent) {
  if (dragging) {
    const rect = svgRef.value?.getBoundingClientRect()
    if (!rect) return
    dragging.x = (event.clientX - rect.left - transform.value.x) / transform.value.k
    dragging.y = (event.clientY - rect.top - transform.value.y) / transform.value.k
    layoutNodes.value = [...layoutNodes.value]
    restartSimulation()
    return
  }
  if (panning) {
    transform.value = {
      ...transform.value,
      x: event.clientX - panning.x,
      y: event.clientY - panning.y,
    }
  }
}

function onUp() {
  if (dragging) {
    // Releasing a node hands it back to the simulation; keeping it pinned
    // would slowly turn the graph into a manual layout nobody asked for.
    dragging.pinned = false
    dragging = null
    restartSimulation()
  }
  panning = null
  window.removeEventListener('mousemove', onMove)
  window.removeEventListener('mouseup', onUp)
}

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  measure()
  buildLayout()
  if (typeof ResizeObserver !== 'undefined' && containerRef.value) {
    resizeObserver = new ResizeObserver(() => {
      measure()
      restartSimulation()
    })
    resizeObserver.observe(containerRef.value)
  }
})

onBeforeUnmount(() => {
  if (frame) cancelAnimationFrame(frame)
  resizeObserver?.disconnect()
  window.removeEventListener('mousemove', onMove)
  window.removeEventListener('mouseup', onUp)
})

watch(
  () => [props.nodes, props.edges],
  () => {
    measure()
    buildLayout()
  },
  { deep: false },
)
</script>

<style scoped lang="less">
.graph-canvas {
  position: relative;
  width: 100%;
  height: 100%;
  min-height: 320px;
  background: var(--td-bg-color-container);
  border-radius: 8px;
  overflow: hidden;
  cursor: grab;

  &.is-empty {
    cursor: default;
  }

  &__svg {
    width: 100%;
    height: 100%;
    display: block;
  }

  &__empty {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-placeholder);
    font-size: 13px;
  }

  &__edge {
    stroke: var(--td-component-border);
    stroke-width: 1;
    transition: opacity 0.15s ease;

    &.is-anchor {
      stroke: var(--td-warning-color);
      stroke-dasharray: 4 3;
    }

    &.is-dimmed {
      opacity: 0.15;
    }
  }

  &__node {
    cursor: pointer;
    transition: opacity 0.15s ease;

    &.is-dimmed {
      opacity: 0.2;
    }

    &.is-active circle {
      filter: drop-shadow(0 0 6px var(--td-brand-color));
    }
  }

  &__label {
    font-size: 11px;
    fill: var(--td-text-color-secondary);
    pointer-events: none;
    user-select: none;
  }

  &__controls {
    position: absolute;
    right: 12px;
    bottom: 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
}
</style>
