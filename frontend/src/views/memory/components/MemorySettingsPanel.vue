<template>
  <div class="memory-settings">
    <div class="memory-settings__head">
      <t-switch v-model="showAdvanced" size="small" />
      <span>{{ t('memory.settings.showAdvanced') }}</span>
      <span class="memory-settings__spacer" />
      <t-button size="small" theme="primary" :loading="saving" :disabled="!dirty" @click="save">
        {{ t('memory.settings.save') }}
      </t-button>
    </div>

    <t-loading :loading="loading" :show-overlay="false">
      <section v-for="group in visibleGroups" :key="group.name" class="settings-group">
        <header class="settings-group__head">
          <h3>{{ t(`memory.settings.groups.${group.name}.title`) }}</h3>
          <p>{{ t(`memory.settings.groups.${group.name}.description`) }}</p>
        </header>

        <div class="settings-group__rows">
          <SettingRow
            v-for="descriptor in group.descriptors"
            :key="descriptor.key"
            :descriptor="descriptor"
            :value="draft[descriptor.key]"
            :resolved="view?.values?.[descriptor.key]"
            :editable="isEditable(descriptor.key)"
            :level="level"
            @update="onUpdate"
          />
        </div>
      </section>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
/**
 * The memory settings panel.
 *
 * Every control here is generated from the descriptor catalogue the API
 * returns: its type, bounds, allowed values and which layers may set it all
 * come from the server, so a new setting appears in the UI without a frontend
 * change and can never drift from what the backend actually enforces.
 *
 * The panel shows two things most settings screens do not, because without them
 * a layered configuration is impossible to reason about: where each effective
 * value came from, and whether a wider layer has pinned it. A control the user
 * cannot change is shown read-only with the reason, rather than accepting a
 * click that does nothing.
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

const props = withDefaults(defineProps<{ level?: 'user' | 'tenant' }>(), { level: 'user' })
const emit = defineEmits<{ (e: 'saved'): void }>()

const { t } = useI18n()

const view = ref<MemorySettingsView | null>(null)
const draft = ref<Record<string, any>>({})
const changed = ref<Record<string, any>>({})
const loading = ref(false)
const saving = ref(false)
const showAdvanced = ref(false)

// The basics are the handful of questions most people actually have: is this
// on, what may it record, does it ask me first, and does it show its work.
// Everything else is real but rarely touched, and burying it keeps the first
// screen answerable at a glance.
const basicKeys = new Set([
  'memory.enabled',
  'memory.write.mode',
  'memory.write.require_review',
  'memory.write.allowed_types',
  'memory.recall.enabled',
  'memory.recall.show_used_memories',
])

const groupOrder = ['general', 'write', 'recall', 'boost', 'anchor', 'lifecycle', 'privacy', 'insights']

const dirty = computed(() => Object.keys(changed.value).length > 0)

const visibleGroups = computed(() => {
  const descriptors = view.value?.descriptors || []
  const groups = new Map<string, MemorySettingDescriptor[]>()

  for (const descriptor of descriptors) {
    // A key this layer can never set is noise here; it belongs on the screen
    // of whoever owns it.
    if (!descriptor.hard_locked && !descriptor.levels?.includes(props.level)) continue
    if (!showAdvanced.value && !basicKeys.has(descriptor.key)) continue
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

function onUpdate(key: string, value: any) {
  draft.value = { ...draft.value, [key]: value }
  changed.value = { ...changed.value, [key]: value }
}

async function load() {
  loading.value = true
  try {
    const res: any = props.level === 'tenant' ? await getTenantMemorySettings() : await getMemorySettings()
    const data: MemorySettingsView = res?.data
    view.value = data
    const next: Record<string, any> = {}
    for (const [key, entry] of Object.entries(data?.values || {})) {
      next[key] = entry.value
    }
    draft.value = next
    changed.value = {}
  } catch (error: any) {
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const res: any =
      props.level === 'tenant'
        ? await updateTenantMemorySettings(changed.value)
        : await updateMemorySettings(changed.value)

    const notes: string[] = res?.data?.notes || []
    if (notes.length) {
      // The server may clamp a value or refuse a key. Saying so is the
      // difference between a setting that quietly disagrees with the form and
      // one the user understands.
      MessagePlugin.warning({ content: notes.join('\n'), duration: 6000 })
    } else {
      MessagePlugin.success(t('memory.settings.saved'))
    }
    view.value = res?.data?.view || view.value
    changed.value = {}
    if (view.value) {
      const next: Record<string, any> = {}
      for (const [key, entry] of Object.entries(view.value.values || {})) {
        next[key] = entry.value
      }
      draft.value = next
    }
    emit('saved')
  } catch {
    MessagePlugin.error(t('memory.errors.saveFailed'))
  } finally {
    saving.value = false
  }
}

onMounted(load)
defineExpose({ reload: load })
</script>

<style scoped lang="less">
.memory-settings {
  padding: 16px 4px 8px;

  &__head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--td-text-color-secondary, #666);
    margin-bottom: 16px;
  }

  &__spacer {
    flex: 1;
  }
}

.settings-group {
  margin-bottom: 24px;

  &__head {
    margin-bottom: 10px;

    h3 {
      margin: 0;
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary, #000);
    }

    p {
      margin: 4px 0 0;
      font-size: 12px;
      line-height: 1.6;
      color: var(--td-text-color-secondary, #888);
    }
  }

  &__rows {
    border: 1px solid var(--td-component-stroke, #e7e7e7);
    border-radius: 10px;
    overflow: hidden;
  }
}
</style>
