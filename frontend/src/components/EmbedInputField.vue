<template>
  <div class="embed-input-box" :class="{ 'is-replying': isReplying }">
    <t-textarea
      v-model="query"
      class="embed-input-box__textarea"
      :placeholder="t('input.placeholder')"
      :autosize="{ minRows: 2, maxRows: 6 }"
      @keydown="onKeydown"
      @compositionstart="isComposing = true"
      @compositionend="isComposing = false"
    />
    <div class="embed-input-box__bar">
      <t-tooltip v-if="isReplying" :content="t('input.stopGeneration')" placement="top">
        <button type="button" class="embed-stop-btn" @click="emit('stop-generation')">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            <rect x="5" y="5" width="6" height="6" rx="1" />
          </svg>
        </button>
      </t-tooltip>
      <button
        v-else
        type="button"
        class="embed-send-btn"
        :class="{ disabled: !query.trim() }"
        :aria-label="t('input.send')"
        @click="submit"
      >
        <img src="@/assets/img/sending-aircraft.svg" :alt="t('input.send')" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  isReplying: boolean
}>()

const emit = defineEmits<{
  (e: 'send-msg', query: string): void
  (e: 'stop-generation'): void
}>()

const { t } = useI18n()
const query = ref('')
const isComposing = ref(false)

const submit = () => {
  if (props.isReplying) return
  const val = query.value.trim()
  if (!val) return
  emit('send-msg', val)
  query.value = ''
}

/** TDesign t-textarea: (value, { e }) — same as Input-field.vue */
const onKeydown = (_val: string, ctx: { e: KeyboardEvent }) => {
  const e = ctx?.e
  if (!e || e.keyCode !== 13) return
  if (isComposing.value) return
  // Shift+Enter / Ctrl+Enter: newline
  if (e.shiftKey || e.ctrlKey) return
  e.preventDefault()
  submit()
}
</script>

<style scoped lang="less">
.embed-input-box {
  position: relative;
  width: 100%;
  max-width: 800px;
  margin: 0 auto;
  background: var(--td-bg-color-container, #fff);
  border-radius: 12px;
  border: 0.5px solid var(--td-component-border, #e7e7e7);
  box-shadow: 0 6px 6px rgba(0, 0, 0, 0.04), 0 12px 12px -1px rgba(0, 0, 0, 0.08);
  transition: border-color 0.15s ease;

  &:focus-within {
    border-color: var(--embed-primary, var(--td-brand-color, #07c05f));
  }

  &__textarea {
    width: 100%;

    :deep(.t-textarea__inner) {
      border: none;
      box-shadow: none;
      background: transparent;
      padding: 14px 16px 52px;
      font-size: 14px;
      line-height: 1.5;
      resize: none;
    }
  }

  &__bar {
    position: absolute;
    right: 12px;
    bottom: 12px;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    pointer-events: none;

    > * {
      pointer-events: auto;
    }
  }
}

.embed-send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  background: var(--embed-primary, var(--td-brand-color, #07c05f));
  transition: background 0.15s ease, opacity 0.15s ease;

  &:hover:not(.disabled) {
    filter: brightness(0.94);
  }

  &.disabled {
    cursor: not-allowed;
    opacity: 0.45;
  }

  img {
    width: 16px;
    height: 16px;
  }
}

.embed-stop-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);

  &:hover {
    background: var(--td-bg-color-component-hover);
  }
}
</style>
