<template>
  <t-tooltip v-if="state && state !== 'unlit'" :content="tooltip" placement="top">
    <span class="memory-state-dot" :class="`memory-state-dot--${state}`" :aria-label="tooltip" />
  </t-tooltip>
</template>

<script setup lang="ts">
/**
 * How much of this item the reader has engaged with, as one dot.
 *
 * Nothing is drawn for an item nobody has touched. That is the common case, and
 * a column of grey dots would be a wall of noise that also drowns the few marks
 * that mean something — absence is the clearest way to say "not yet".
 *
 * The vocabulary is deliberately the same one the wiki graph legend uses, since
 * it is the same feature seen from a different angle: a reader should not have
 * to learn two sets of colours for one idea.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  state?: string
  /** Number of anchors behind this state, for the tooltip. */
  anchorCount?: number
  lastSeenAt?: string
}>()

const { t } = useI18n()

const tooltip = computed(() => {
  const label = t(`memory.illuminate.${props.state}`)
  const parts = [label]
  if (props.anchorCount && props.anchorCount > 0) {
    parts.push(t('memory.illuminate.interactions', { count: props.anchorCount }))
  }
  const seen = relativeLastSeen()
  if (seen) parts.push(seen)
  return parts.join(' · ')
})

/** "3 days ago" reads better than a timestamp for something this incidental. */
function relativeLastSeen(): string {
  if (!props.lastSeenAt) return ''
  const then = new Date(props.lastSeenAt).getTime()
  if (Number.isNaN(then)) return ''
  const days = Math.floor((Date.now() - then) / 86_400_000)
  if (days <= 0) return t('memory.illuminate.seenToday')
  return t('memory.illuminate.seenDaysAgo', { days })
}
</script>

<style scoped lang="less">
.memory-state-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  // A ring rather than a disc for the lighter states, so engagement reads as
  // filling up rather than as five unrelated colours.
  box-sizing: border-box;

  &--touched {
    border: 1.5px solid var(--td-brand-color-3, #08dd6e);
    background: transparent;
  }

  &--familiar {
    border: 1.5px solid var(--td-brand-color);
    background: var(--td-brand-color-1, #e9f8ec);
  }

  &--mastered {
    border: 1.5px solid var(--td-brand-color);
    background: var(--td-brand-color);
  }

  &--flagged {
    border: 1.5px solid var(--td-warning-color);
    background: var(--td-warning-color);
  }
}
</style>
