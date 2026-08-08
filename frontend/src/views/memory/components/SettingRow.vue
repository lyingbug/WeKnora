<template>
  <div class="setting-row" :class="{
    'setting-row--stacked': layout === 'stacked',
    'setting-row--block': isBlockControl,
    'setting-row--comfortable': density === 'comfortable',
    'setting-row--readonly': !editable,
  }">
    <div class="setting-info">
      <label>
        <span class="setting-info__label">{{ label }}</span>
        <t-tag v-if="inheritanceTag" size="small" variant="light" theme="default" class="setting-info__inherit">
          {{ inheritanceTag }}
        </t-tag>
        <t-tooltip v-if="lockReason" :content="lockReason">
          <t-icon name="lock-on" class="setting-info__lock" />
        </t-tooltip>
      </label>
      <p v-if="help" class="desc">{{ help }}</p>
    </div>

    <div class="setting-control">
      <template v-if="!editable">
        <t-tag v-if="descriptor.kind === 'bool'" size="medium" variant="light"
          :theme="Boolean(value) ? 'success' : 'default'" class="setting-readonly-tag">
          {{ Boolean(value) ? t('memory.settings.readonly.on') : t('memory.settings.readonly.off') }}
        </t-tag>

        <span v-else-if="descriptor.kind === 'enum'" class="setting-readonly-text">
          {{ enumDisplay }}
        </span>

        <div v-else-if="descriptor.kind === 'string_list'" class="setting-readonly-list"
          :class="{ 'setting-readonly-list--mono': !stringListUsesTags }">
          <template v-if="stringListUsesTags">
            <t-tag v-for="item in stringListValue" :key="item" size="small" variant="light" theme="default"
              class="setting-readonly-list__tag">
              {{ optionLabel(item) }}
            </t-tag>
            <span v-if="!stringListValue.length" class="setting-readonly-empty">
              {{ t('memory.settings.readonly.empty') }}
            </span>
          </template>
          <pre v-else class="setting-readonly-block">{{ readonlyValue }}</pre>
        </div>

        <div v-else-if="descriptor.kind === 'float_map' || descriptor.kind === 'int_map'" class="setting-map-grid">
          <div v-for="key in mapKeys" :key="key" class="setting-map-grid__item setting-map-grid__item--readonly">
            <span class="setting-map-grid__label">{{ mapKeyLabel(key) }}</span>
            <span class="setting-readonly-text">{{ mapValue(key) }}</span>
          </div>
        </div>

        <span v-else-if="descriptor.kind === 'int' || descriptor.kind === 'float'" class="setting-readonly-text">
          {{ numberDisplay }}
        </span>

        <span v-else-if="descriptor.kind === 'string'" class="setting-readonly-text">
          {{ stringDisplay }}
        </span>

        <pre v-else class="setting-readonly-block">{{ readonlyValue }}</pre>
      </template>

      <template v-else>
        <t-switch v-if="descriptor.kind === 'bool'" :value="Boolean(value)" @change="(v: boolean) => emitUpdate(v)" />

        <t-select v-else-if="descriptor.kind === 'enum'" :value="value" class="setting-control__select"
          @change="(v: any) => emitUpdate(v)">
          <t-option v-for="option in descriptor.allowed || []" :key="option" :value="option"
            :label="optionLabel(option)" />
        </t-select>

        <t-select v-else-if="descriptor.kind === 'string_list' && (descriptor.allowed || []).length"
          :value="Array.isArray(value) ? value : []" class="setting-control__select" multiple :min-collapsed-num="3"
          @change="(v: any) => emitUpdate(v)">
          <t-option v-for="option in descriptor.allowed || []" :key="option" :value="option"
            :label="optionLabel(option)" />
        </t-select>

        <t-textarea v-else-if="descriptor.kind === 'string_list' && !descriptor.allowed?.length" :value="stringListText"
          class="setting-control__textarea" :autosize="{ minRows: 3, maxRows: 8 }"
          :placeholder="t('memory.settings.patternPlaceholder')" @blur="onStringListTextBlur" />

        <div v-else-if="descriptor.kind === 'float_map'" class="setting-map-grid">
          <div v-for="key in mapKeys" :key="key" class="setting-map-grid__item">
            <label class="setting-map-grid__label">{{ mapKeyLabel(key) }}</label>
            <t-input-number :value="Number(mapValue(key))" :min="descriptor.min" :max="descriptor.max" :step="0.05"
              :decimal-places="2" theme="column" class="setting-map-grid__input"
              @change="(v: any) => updateMapKey(key, Number(v))" />
          </div>
        </div>

        <div v-else-if="descriptor.kind === 'int_map'" class="setting-map-grid">
          <div v-for="key in mapKeys" :key="key" class="setting-map-grid__item">
            <label class="setting-map-grid__label">{{ mapKeyLabel(key) }}</label>
            <t-input-number :value="Number(mapValue(key))" :min="descriptor.min" :max="descriptor.max" :step="1"
              :decimal-places="0" theme="column" class="setting-map-grid__input"
              @change="(v: any) => updateMapKey(key, Number(v))" />
          </div>
        </div>

        <t-input-number v-else-if="descriptor.kind === 'int' || descriptor.kind === 'float'" :value="Number(value ?? 0)"
          :min="descriptor.min" :max="descriptor.max" :step="descriptor.kind === 'float' ? 0.05 : 1"
          :decimal-places="descriptor.kind === 'float' ? 2 : 0" class="setting-control__number" theme="column"
          @change="(v: any) => emitUpdate(Number(v))" />

        <t-input v-else-if="descriptor.kind === 'string'" :value="String(value ?? '')" class="setting-control__input"
          @change="(v: any) => emitUpdate(v)" />

        <pre v-else class="setting-readonly-block">{{ readonlyValue }}</pre>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { MemorySettingDescriptor, MemorySettingValue } from '@/api/memory'

const props = withDefaults(
  defineProps<{
    descriptor: MemorySettingDescriptor
    value: any
    resolved?: MemorySettingValue
    editable: boolean
    level: string
    layout?: 'inline' | 'stacked'
    density?: 'default' | 'comfortable'
  }>(),
  { layout: 'inline', density: 'default' },
)

const emit = defineEmits<{ (e: 'update', key: string, value: any): void }>()

const { t, te } = useI18n()

const translationKey = computed(() => props.descriptor.key.replace(/\./g, '_'))

const isBlockControl = computed(() => {
  const kind = props.descriptor.kind
  if (kind === 'float_map' || kind === 'int_map') return true
  if (kind === 'string_list' && !(props.descriptor.allowed || []).length) return true
  return false
})

function localised(field: 'label' | 'help'): string {
  const scoped = `memory.settings.keys.${translationKey.value}.${field}_${props.level}`
  if (te(scoped)) return t(scoped)
  const shared = `memory.settings.keys.${translationKey.value}.${field}`
  if (te(shared)) return t(shared)
  return field === 'label' ? props.descriptor.key : ''
}

const label = computed(() => localised('label'))
const help = computed(() => localised('help'))

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

const inheritanceTag = computed(() => {
  const source = props.resolved?.source
  if (!source || source === props.level || source === 'default') return ''
  return t('memory.settings.fromLayer', { layer: layerName(source) })
})

function layerName(layer: string): string {
  const key = `memory.settings.layers.${layer}`
  return te(key) ? t(key) : layer
}

const mapKeys = computed(() => {
  const allowed = props.descriptor.allowed || []
  if (allowed.length) return allowed
  if (props.value && typeof props.value === 'object') {
    return Object.keys(props.value)
  }
  return []
})

function mapValue(key: string): number {
  if (props.value && typeof props.value === 'object' && key in props.value) {
    return Number(props.value[key])
  }
  return 0
}

function mapKeyLabel(key: string): string {
  if (te(`memory.types.${key}`)) return t(`memory.types.${key}`)
  return optionLabel(key)
}

function emitUpdate(value: any) {
  emit('update', props.descriptor.key, value)
}

function updateMapKey(key: string, value: number) {
  const base =
    props.value && typeof props.value === 'object' && !Array.isArray(props.value)
      ? { ...props.value }
      : {}
  base[key] = value
  emitUpdate(base)
}

const stringListValue = computed(() => (Array.isArray(props.value) ? props.value : []))

const stringListUsesTags = computed(() => (props.descriptor.allowed || []).length > 0)

const stringListText = computed(() => {
  if (!stringListValue.value.length) return ''
  return stringListValue.value.join('\n')
})

const enumDisplay = computed(() => {
  const raw = props.value
  if (raw == null || raw === '') return t('memory.settings.readonly.empty')
  return optionLabel(String(raw))
})

const numberDisplay = computed(() => {
  const raw = Number(props.value ?? 0)
  if (props.descriptor.kind === 'float') return raw.toFixed(2)
  return String(raw)
})

const stringDisplay = computed(() => {
  const raw = String(props.value ?? '').trim()
  return raw || t('memory.settings.readonly.empty')
})

function onStringListTextBlur(e: FocusEvent) {
  const target = e.target as HTMLTextAreaElement
  const lines = target.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  emitUpdate(lines)
}

const readonlyValue = computed(() => {
  const value = props.value
  if (Array.isArray(value)) return value.length ? value.join('\n') : '-'
  if (value && typeof value === 'object') {
    return Object.entries(value)
      .map(([k, v]) => `${mapKeyLabel(k)}: ${v}`)
      .join('\n')
  }
  return String(value ?? '-')
})
</script>

<style scoped lang="less">
.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  &--stacked,
  &--block {
    flex-direction: column;
    gap: 12px;

    .setting-info {
      max-width: none;
      padding-right: 0;
    }

    .setting-control {
      min-width: 0;
      width: 100%;
      justify-content: flex-start;
    }
  }

  &--stacked:not(&--block) {
    padding: 20px 0;
    border-bottom: 1px solid var(--td-component-stroke);

    &:last-child {
      border-bottom: none;
      padding-bottom: 0;
    }
  }

  &--block {
    padding: 20px 0;
    border-bottom: 1px solid var(--td-component-stroke);

    &:last-child {
      border-bottom: none;
      padding-bottom: 0;
    }
  }

  &--comfortable {
    gap: 32px;
    padding: 22px 0;

    .setting-info {
      flex: 1;
      max-width: none;
      padding-right: 0;
      display: flex;
      flex-direction: column;
      gap: 6px;

      label {
        margin-bottom: 0;
      }
    }

    .setting-info__label {
      font-size: 15px;
      font-weight: 500;
      line-height: 1.35;
    }

    .desc {
      font-size: 13px;
      line-height: 1.55;
      color: var(--td-text-color-placeholder);
      max-width: 52ch;
    }

    .setting-control {
      width: 280px;
      min-width: 280px;
      flex-shrink: 0;
      justify-content: flex-end;
      align-items: center;
      align-self: flex-start;
      padding-top: 1px;
    }

    .setting-map-grid__label {
      font-size: 13px;
    }

    .setting-readonly-block,
    .setting-readonly-text {
      font-size: 13px;
    }

    &.setting-row--block,
    &.setting-row--stacked.setting-row--block {
      gap: 12px;

      .setting-control {
        width: 100%;
        min-width: 0;
        justify-content: flex-start;
        align-items: stretch;
        padding-top: 0;
      }

      .desc {
        max-width: none;
      }
    }

    &.setting-row--readonly {
      .setting-control {
        justify-content: flex-end;
        align-items: center;
      }

      &.setting-row--block {
        .setting-control {
          justify-content: flex-start;
        }
      }
    }
  }

  &--readonly:not(&--comfortable) {
    .setting-control {
      justify-content: flex-start;
    }
  }
}

.setting-info {
  flex: 1;
  max-width: 58%;
  min-width: 0;
  padding-right: 16px;

  label {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 4px;
  }

  &__label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    line-height: 1.4;
  }

  &__inherit {
    flex-shrink: 0;
    font-size: 11px;
    line-height: 1.2;
  }

  &__lock {
    color: var(--td-text-color-placeholder);
    font-size: 14px;
  }

  .desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex-shrink: 0;
  min-width: 200px;
  display: flex;
  justify-content: flex-end;
  align-items: center;

  &__select,
  &__input,
  &__textarea {
    width: 100%;
  }

  &__number {
    width: 140px;
  }
}

.setting-map-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(148px, 1fr));
  gap: 10px 12px;
  width: 100%;

  &__item {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px 12px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    background: var(--td-bg-color-secondarycontainer);
  }

  &__label {
    font-size: 12px;
    font-weight: 500;
    color: var(--td-text-color-secondary);
    line-height: 1.3;
  }

  &__input {
    width: 100%;
  }
}

.setting-readonly-block {
  width: 100%;
  margin: 0;
  padding: 10px 12px;
  font-family: var(--app-font-family-mono);
  font-size: 12px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  white-space: pre-wrap;
  word-break: break-word;
  text-align: left;
}

.setting-readonly-text {
  font-size: 13px;
  line-height: 1.5;
  color: var(--td-text-color-primary);
  font-weight: 500;
}

.setting-readonly-tag {
  flex-shrink: 0;
}

.setting-readonly-empty {
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.setting-readonly-list {
  width: 100%;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;

  &--mono {
    display: block;
  }

  &__tag {
    max-width: 100%;
  }
}

.setting-map-grid__item--readonly {
  gap: 4px;
}
</style>
