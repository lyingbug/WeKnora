<template>
  <t-loading :loading="loading" size="small" class="memory-settings-panel">
    <div v-for="(group, index) in visibleGroups" :key="group.name" class="settings-block"
      :class="{ 'settings-block--first': index === 0 }">
      <h3 v-if="showGroupTitles" class="settings-block__title">
        {{ t(`memory.settings.groups.${group.name}.title`) }}
      </h3>
      <p v-if="showGroupTitles" class="settings-block__desc">
        {{ t(`memory.settings.groups.${group.name}.description`) }}
      </p>

      <div class="settings-group">
        <SettingRow v-for="descriptor in group.descriptors" :key="descriptor.key" :descriptor="descriptor"
          :value="draft[descriptor.key]" :resolved="view?.values?.[descriptor.key]"
          :editable="isEditable(descriptor.key)" :level="level" @update="onUpdate" />
      </div>
    </div>

    <div v-if="advancedAvailable" class="settings-more">
      <t-link theme="primary" hover="color" @click="showAdvanced = !showAdvanced">
        {{ showAdvanced ? t('memory.settings.hideAdvanced') : t('memory.settings.showAdvanced') }}
        <template #suffix-icon>
          <t-icon :name="showAdvanced ? 'chevron-up' : 'chevron-down'" />
        </template>
      </t-link>
    </div>
  </t-loading>
</template>

<script setup lang="ts">
/**
 * The memory settings form.
 *
 * Every control is generated from the descriptor catalogue the API returns: its
 * type, bounds, allowed values and which layers may set it all come from the
 * server, so a new setting appears here without a frontend change and can never
 * claim to enforce something the backend does not.
 *
 * Two things this shows that most settings screens do not, because a layered
 * configuration is otherwise impossible to reason about: where each effective
 * value came from, and whether a wider layer has pinned it. A control the user
 * cannot change is read-only with the reason, rather than accepting a click that
 * does nothing.
 *
 * Changes save as they are made, which is how the rest of this dialog behaves.
 * A separate save step reads as safer and is not: it adds a state where what is
 * on screen and what is in effect disagree, with no indication which is which.
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

import SettingRow from './SettingRow.vue'
import {
  getMemorySettings,
  getTenantMemorySettings,
  updateMemorySettings,
  updateTenantMemorySettings,
  type MemorySettingDescriptor,
  type MemorySettingsView,
} from '@/api/memory'

const props = withDefaults(
  defineProps<{
    level?: 'user' | 'tenant'
    /** Keys to show before "more settings" is expanded. */
    primaryKeys?: string[]
    /** Group headings are noise when the primary set is a single short list. */
    showGroupTitles?: boolean
  }>(),
  { level: 'user', showGroupTitles: true },
)
const emit = defineEmits<{ (e: 'changed'): void }>()

const { t } = useI18n()

const view = ref<MemorySettingsView | null>(null)
const draft = ref<Record<string, any>>({})
const loading = ref(false)
const showAdvanced = ref(false)

// The questions most people actually have: is this on, what may it record, does
// it ask me first, and is it used in conversation. Everything else is real but
// rarely touched, and keeping it behind a link makes the first screen
// answerable at a glance.
const defaultPrimaryKeys = [
  'memory.enabled',
  'memory.write.mode',
  'memory.write.require_review',
  'memory.write.allowed_types',
  'memory.recall.enabled',
]

const primary = computed(() => new Set(props.primaryKeys ?? defaultPrimaryKeys))

const groupOrder = ['general', 'write', 'recall', 'boost', 'anchor', 'lifecycle', 'privacy', 'insights']

/** Descriptors this layer is allowed to have an opinion about.
 *
 * Platform invariants belong to no layer and are shown only in the workspace
 * policy view, where "this is fixed for everyone" is the subject. On a personal
 * screen they would be rows nobody can act on. */
const applicable = computed(() =>
  (view.value?.descriptors || []).filter((descriptor) => {
    if (descriptor.levels?.includes(props.level)) return true
    return descriptor.hard_locked && props.level === 'tenant'
  }),
)

const advancedAvailable = computed(() =>
  applicable.value.some((descriptor) => !primary.value.has(descriptor.key)),
)

const visibleGroups = computed(() => {
  const groups = new Map<string, MemorySettingDescriptor[]>()
  for (const descriptor of applicable.value) {
    if (!showAdvanced.value && !primary.value.has(descriptor.key)) continue
    if (!groups.has(descriptor.group)) groups.set(descriptor.group, [])
    groups.get(descriptor.group)!.push(descriptor)
  }
  return groupOrder
    .filter((name) => groups.has(name))
    .map((name) => ({ name, descriptors: groups.get(name)! }))
})

function isEditable(key: string): boolean {
  return Boolean(view.value?.editable?.[key])
}

function applyView(next: MemorySettingsView | null | undefined) {
  if (!next) return
  view.value = next
  const values: Record<string, any> = {}
  for (const [key, entry] of Object.entries(next.values || {})) {
    values[key] = entry.value
  }
  draft.value = values
}

async function load() {
  loading.value = true
  try {
    const res = props.level === 'tenant' ? await getTenantMemorySettings() : await getMemorySettings()
    applyView(res?.data)
  } catch (error: any) {
    // A 404 is the ordinary "memory is switched off for me" state, not a fault.
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
  } finally {
    loading.value = false
  }
}

async function onUpdate(key: string, value: any) {
  // Show the new value immediately; the server's reply is authoritative and
  // replaces it, which is what surfaces a clamp.
  draft.value = { ...draft.value, [key]: value }
  try {
    const res =
      props.level === 'tenant'
        ? await updateTenantMemorySettings({ [key]: value })
        : await updateMemorySettings({ [key]: value })

    const notes: string[] = res?.data?.notes || []
    applyView(res?.data?.view)
    if (notes.length) {
      // The server may clamp a value or refuse a key. Saying so is the
      // difference between a setting that quietly disagrees with the form and
      // one the user understands.
      MessagePlugin.warning({ content: notes.join('\n'), duration: 6000 })
    }
    emit('changed')
  } catch {
    MessagePlugin.error(t('memory.errors.saveFailed'))
    // Put the stored value back rather than leaving the form asserting
    // something that was never saved.
    await load()
  }
}

onMounted(load)
defineExpose({ reload: load })
</script>

<style scoped lang="less">
.memory-settings-panel {
  display: block;
  width: 100%;
}

.settings-block {
  margin-top: 28px;

  &--first {
    margin-top: 0;
  }

  &__title {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 4px 0;
  }

  &__desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.settings-group {
  display: flex;
  flex-direction: column;
}

.settings-more {
  margin-top: 16px;
  font-size: 13px;
}
</style>
