<template>
  <div class="setting-row">
    <div class="setting-info">
      <label>
        {{ label }}
        <t-tooltip v-if="lockReason" :content="lockReason">
          <t-icon name="lock-on" class="setting-info__lock" />
        </t-tooltip>
      </label>
      <p v-if="help" class="desc">{{ help }}</p>
      <p v-if="sourceHint" class="desc source">{{ sourceHint }}</p>
    </div>

    <div class="setting-control">
      <t-switch
        v-if="descriptor.kind === 'bool'"
        :value="Boolean(value)"
        :disabled="!editable"
        @change="(v: boolean) => $emit('update', descriptor.key, v)"
      />

      <t-select
        v-else-if="descriptor.kind === 'enum'"
        :value="value"
        :disabled="!editable"
        class="setting-control__select"
        @change="(v: any) => $emit('update', descriptor.key, v)"
      >
        <t-option
          v-for="option in descriptor.allowed || []"
          :key="option"
          :value="option"
          :label="optionLabel(option)"
        />
      </t-select>

      <t-select
        v-else-if="descriptor.kind === 'string_list' && (descriptor.allowed || []).length"
        :value="Array.isArray(value) ? value : []"
        :disabled="!editable"
        class="setting-control__select"
        multiple
        :min-collapsed-num="3"
        @change="(v: any) => $emit('update', descriptor.key, v)"
      >
        <t-option
          v-for="option in descriptor.allowed || []"
          :key="option"
          :value="option"
          :label="optionLabel(option)"
        />
      </t-select>

      <t-input-number
        v-else-if="descriptor.kind === 'int' || descriptor.kind === 'float'"
        :value="Number(value ?? 0)"
        :disabled="!editable"
        :min="descriptor.min"
        :max="descriptor.max"
        :step="descriptor.kind === 'float' ? 0.05 : 1"
        :decimal-places="descriptor.kind === 'float' ? 2 : 0"
        class="setting-control__number"
        theme="column"
        @change="(v: any) => $emit('update', descriptor.key, Number(v))"
      />

      <t-input
        v-else-if="descriptor.kind === 'string'"
        :value="String(value ?? '')"
        :disabled="!editable"
        class="setting-control__input"
        @change="(v: any) => $emit('update', descriptor.key, v)"
      />

      <!-- Free-form lists and maps (deny patterns, relation weights, per-type
           half-lives) are rare, structural, and dangerous to mistype, so they
           are shown read-only here rather than given a half-usable editor. -->
      <span v-else class="setting-control__readonly">{{ readonlyValue }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { MemorySettingDescriptor, MemorySettingValue } from '@/api/memory'

const props = defineProps<{
  descriptor: MemorySettingDescriptor
  value: any
  resolved?: MemorySettingValue
  editable: boolean
  level: string
}>()

defineEmits<{ (e: 'update', key: string, value: any): void }>()

const { t, te } = useI18n()

// Labels and help text are keyed by setting key, with the key itself as the
// fallback: a setting added on the backend renders immediately rather than
// showing an empty row while translations catch up.
//
// The dots in a setting key have to go, because i18n reads a dot as a path
// separator and would look for a "write" object inside "memory" rather than for
// the single entry named "memory.write.mode".
const translationKey = computed(() => props.descriptor.key.replace(/\./g, '_'))

const label = computed(() => {
  const key = `memory.settings.keys.${translationKey.value}.label`
  return te(key) ? t(key) : props.descriptor.key
})

const help = computed(() => {
  const key = `memory.settings.keys.${translationKey.value}.help`
  return te(key) ? t(key) : ''
})

function optionLabel(option: string): string {
  const key = `memory.settings.options.${option}`
  return te(key) ? t(key) : option
}

const lockReason = computed(() => {
  if (props.descriptor.hard_locked) {
    return t('memory.settings.lockedByPlatform')
  }
  if (props.editable) return ''
  const lockedBy = props.resolved?.locked_by
  if (!lockedBy) return t('memory.settings.notSettableHere')
  return t('memory.settings.lockedBy', { layer: layerName(lockedBy) })
})

// Showing where a value came from is what makes a layered configuration
// legible: "inherited from your workspace" is a very different message from
// "you set this".
const sourceHint = computed(() => {
  const source = props.resolved?.source
  if (!source || source === props.level) return ''
  if (source === 'default') return t('memory.settings.fromDefault')
  return t('memory.settings.fromLayer', { layer: layerName(source) })
})

function layerName(layer: string): string {
  const key = `memory.settings.layers.${layer}`
  return te(key) ? t(key) : layer
}

const readonlyValue = computed(() => {
  const value = props.value
  if (Array.isArray(value)) return value.length ? value.join(', ') : '-'
  if (value && typeof value === 'object') {
    return Object.entries(value)
      .map(([k, v]) => `${k}=${v}`)
      .join(', ')
  }
  return String(value ?? '-')
})
</script>

<style scoped lang="less">
/* Mirrors the row shape used across the settings dialog (see
   ChatHistorySettings.vue) so a generated row is indistinguishable from a
   hand-written one. */
.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 20px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 1;
  max-width: 65%;
  padding-right: 24px;

  label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 15px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    margin-bottom: 4px;
  }

  &__lock {
    color: var(--td-text-color-placeholder);
  }

  .desc {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }

  /* Where the value came from is secondary to what the setting does, so it
     sits below the description in the quieter placeholder tone. */
  .source {
    margin-top: 4px;
    color: var(--td-text-color-placeholder);
  }
}

.setting-control {
  flex-shrink: 0;
  min-width: 280px;
  display: flex;
  justify-content: flex-end;
  align-items: center;

  &__select,
  &__input {
    width: 100%;
  }

  &__number {
    width: 160px;
  }

  &__readonly {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
    text-align: right;
    word-break: break-word;
  }
}
</style>
