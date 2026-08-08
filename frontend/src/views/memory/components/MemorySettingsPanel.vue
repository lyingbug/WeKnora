<template>
  <t-loading :loading="loading" size="small" class="memory-settings-panel"
    :class="{ 'memory-settings-panel--policy': panelVariant === 'policy' }">
    <template v-for="(group, index) in visibleGroups" :key="group.name">
      <section v-if="sectionLayout" class="setting-drawer__section"
        :class="{ 'setting-drawer__section--first': index === 0 }">
        <h4 v-if="showGroupTitles" class="setting-drawer__section-title">
          {{ t(`memory.settings.groups.${group.name}.title`) }}
        </h4>
        <p v-if="showGroupTitles && groupDescription(group.name)" class="memory-settings-panel__section-desc">
          {{ groupDescription(group.name) }}
        </p>
        <SettingRow v-for="descriptor in group.descriptors" :key="descriptor.key" :descriptor="descriptor"
          :value="draft[descriptor.key]" :resolved="view?.values?.[descriptor.key]"
          :editable="isEditable(descriptor.key)" :level="level" :layout="rowLayout" :density="density"
          @update="onUpdate" />
      </section>

      <div v-else class="settings-block" :class="{ 'settings-block--first': index === 0 }">
        <h3 v-if="showGroupTitles" class="settings-block__title">
          {{ t(`memory.settings.groups.${group.name}.title`) }}
        </h3>
        <p v-if="showGroupTitles && groupDescription(group.name)" class="settings-block__desc">
          {{ groupDescription(group.name) }}
        </p>
        <div class="settings-group">
          <SettingRow v-for="descriptor in group.descriptors" :key="descriptor.key" :descriptor="descriptor"
            :value="draft[descriptor.key]" :resolved="view?.values?.[descriptor.key]"
            :editable="isEditable(descriptor.key)" :level="level" :layout="rowLayout" :density="density"
            @update="onUpdate" />
        </div>
      </div>
    </template>

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
    primaryKeys?: string[]
    showGroupTitles?: boolean
    /** Use SettingDrawer section chrome (drawer). */
    sectionLayout?: boolean
    rowLayout?: 'inline' | 'stacked'
    /** Only render settings from these groups (e.g. policy tabs). */
    groupFilter?: string[]
    /** Show every applicable setting instead of primary/advanced split. */
    showAll?: boolean
    density?: 'default' | 'comfortable'
    panelVariant?: 'default' | 'policy'
  }>(),
  {
    level: 'user',
    showGroupTitles: true,
    sectionLayout: false,
    rowLayout: 'inline',
    showAll: false,
    density: 'default',
    panelVariant: 'default',
  },
)
const emit = defineEmits<{ (e: 'changed'): void }>()

const { t, te } = useI18n()

const view = ref<MemorySettingsView | null>(null)
const draft = ref<Record<string, any>>({})
const loading = ref(false)
const showAdvanced = ref(false)

const defaultPrimaryKeys = [
  'memory.enabled',
  'memory.write.mode',
  'memory.write.require_review',
  'memory.write.allowed_types',
  'memory.recall.enabled',
]

const primary = computed(() => new Set(props.primaryKeys ?? defaultPrimaryKeys))

const groupOrder = ['general', 'write', 'recall', 'boost', 'anchor', 'lifecycle', 'privacy', 'insights']

const applicable = computed(() =>
  (view.value?.descriptors || []).filter((descriptor) => {
    if (descriptor.levels?.includes(props.level)) return true
    return descriptor.hard_locked && props.level === 'tenant'
  }),
)

const showAllSettings = computed(() => props.showAll || (props.groupFilter?.length ?? 0) > 0)

const advancedAvailable = computed(
  () => !showAllSettings.value && applicable.value.some((descriptor) => !primary.value.has(descriptor.key)),
)

const visibleGroups = computed(() => {
  const groups = new Map<string, MemorySettingDescriptor[]>()
  for (const descriptor of applicable.value) {
    if (props.groupFilter?.length && !props.groupFilter.includes(descriptor.group)) continue
    if (!showAllSettings.value && !showAdvanced.value && !primary.value.has(descriptor.key)) continue
    if (!groups.has(descriptor.group)) groups.set(descriptor.group, [])
    groups.get(descriptor.group)!.push(descriptor)
  }
  return groupOrder
    .filter((name) => groups.has(name))
    .map((name) => ({ name, descriptors: groups.get(name)! }))
})

function groupDescription(name: string): string {
  const key = `memory.settings.groups.${name}.description`
  return te(key) ? t(key) : ''
}

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
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
  } finally {
    loading.value = false
  }
}

let saveTimer: number | null = null
let queued: Record<string, any> = {}

function onUpdate(key: string, value: any) {
  draft.value = { ...draft.value, [key]: value }
  queued = { ...queued, [key]: value }
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    void flush()
  }, 500)
}

async function flush() {
  const patch = queued
  queued = {}
  if (!Object.keys(patch).length) return
  try {
    const res =
      props.level === 'tenant'
        ? await updateTenantMemorySettings(patch)
        : await updateMemorySettings(patch)

    const notes: string[] = res?.data?.notes || []
    applyView(res?.data?.view)
    if (notes.length) {
      MessagePlugin.warning({ content: notes.join('\n'), duration: 6000 })
    } else {
      MessagePlugin.success(t('memory.settings.saved'))
    }
    emit('changed')
  } catch {
    MessagePlugin.error(t('memory.errors.saveFailed'))
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

  &--policy {
    .settings-group {
      gap: 0;
    }
  }
}

.memory-settings-panel__section-desc {
  margin: 0 0 10px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
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
    margin: 0 0 4px;
  }

  &__desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin: 0 0 12px;
    line-height: 1.5;
  }
}

.settings-group {
  display: flex;
  flex-direction: column;
}

.settings-more {
  margin-top: 4px;
  padding-top: 8px;
}
</style>
