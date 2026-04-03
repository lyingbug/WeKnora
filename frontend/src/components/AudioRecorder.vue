<script setup lang="ts">
import { ref, computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { useAudioRecorder } from '@/composables/useAudioRecorder'

interface Props {
  kbId: string
  visible: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  'save': [text: string]
}>()

const { t } = useI18n()

const kbIdRef = toRef(props, 'kbId')
const {
  status,
  duration,
  interimText,
  finalText,
  error,
  volumeLevel,
  startRecording,
  stopRecording,
  cancelRecording,
  formatDuration,
} = useAudioRecorder(kbIdRef)

const isRecording = computed(() => status.value === 'recording')
const isConnecting = computed(() => status.value === 'connecting')
const isStopping = computed(() => status.value === 'stopping')
const isIdle = computed(() => status.value === 'idle')
const hasText = computed(() => finalText.value.trim().length > 0 || interimText.value.trim().length > 0)

const saving = ref(false)

// Volume bars for visualization
const volumeBars = computed(() => {
  const count = 20
  const level = volumeLevel.value
  return Array.from({ length: count }, (_, i) => {
    const threshold = i / count
    return level > threshold
  })
})

async function handleStart() {
  try {
    await startRecording()
  } catch (err: any) {
    if (err.name === 'NotAllowedError') {
      MessagePlugin.error(t('audioRecorder.permissionDenied'))
    } else {
      MessagePlugin.error(err.message || t('audioRecorder.connectionFailed'))
    }
  }
}

async function handleStop() {
  await stopRecording()
}

function handleCancel() {
  cancelRecording()
  emit('update:visible', false)
}

async function handleSave() {
  const text = finalText.value.trim()
  if (!text) return
  saving.value = true
  try {
    emit('save', text)
    emit('update:visible', false)
  } finally {
    saving.value = false
  }
}

function handleClose() {
  if (isRecording.value) {
    cancelRecording()
  }
  emit('update:visible', false)
}
</script>

<template>
  <t-dialog
    :visible="visible"
    :header="$t('audioRecorder.title')"
    :footer="false"
    :close-on-overlay-click="false"
    width="520px"
    @close="handleClose"
  >
    <div class="audio-recorder">
      <!-- Status & Duration -->
      <div class="recorder-status">
        <div class="status-left">
          <span v-if="isRecording" class="status-dot recording" />
          <span v-if="isConnecting" class="status-dot connecting" />
          <span class="status-text">
            <template v-if="isConnecting">{{ $t('audioRecorder.connecting') }}</template>
            <template v-else-if="isRecording">{{ $t('audioRecorder.recording') }}</template>
            <template v-else-if="isStopping">{{ $t('audioRecorder.stopping') }}</template>
            <template v-else>{{ $t('audioRecorder.readyToRecord') }}</template>
          </span>
        </div>
        <span v-if="isRecording || isStopping" class="duration">{{ formatDuration(duration) }}</span>
      </div>

      <!-- Volume Indicator -->
      <div v-if="isRecording" class="volume-indicator">
        <div
          v-for="(active, index) in volumeBars"
          :key="index"
          class="volume-bar"
          :class="{ active }"
        />
      </div>

      <!-- Transcription Text Area -->
      <div class="transcription-area">
        <div v-if="!hasText && isIdle" class="placeholder">
          {{ $t('audioRecorder.clickToStart') }}
        </div>
        <div v-else class="transcription-text">
          <span class="final-text">{{ finalText }}</span>
          <span v-if="interimText" class="interim-text">{{ interimText }}</span>
        </div>
      </div>

      <!-- Error Display -->
      <div v-if="error" class="error-msg">
        {{ error }}
      </div>

      <!-- Controls -->
      <div class="recorder-controls">
        <template v-if="isIdle && !hasText">
          <t-button
            theme="primary"
            @click="handleStart"
            :loading="isConnecting"
          >
            <template #icon><t-icon name="sound" /></template>
            {{ $t('audioRecorder.startRecording') }}
          </t-button>
        </template>

        <template v-if="isRecording">
          <t-button theme="danger" variant="outline" @click="handleCancel">
            {{ $t('audioRecorder.cancel') }}
          </t-button>
          <t-button theme="primary" @click="handleStop">
            <template #icon><t-icon name="stop-circle" /></template>
            {{ $t('audioRecorder.stopRecording') }}
          </t-button>
        </template>

        <template v-if="isIdle && hasText">
          <t-button variant="outline" @click="handleCancel">
            {{ $t('audioRecorder.cancel') }}
          </t-button>
          <t-button
            theme="primary"
            @click="handleSave"
            :loading="saving"
          >
            {{ $t('audioRecorder.saveToKB') }}
          </t-button>
        </template>
      </div>
    </div>
  </t-dialog>
</template>

<style scoped lang="less">
.audio-recorder {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.recorder-status {
  display: flex;
  align-items: center;
  justify-content: space-between;

  .status-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;

    &.recording {
      background: #e34d59;
      animation: pulse 1.5s infinite;
    }
    &.connecting {
      background: #ed7b2f;
      animation: pulse 1s infinite;
    }
  }

  .status-text {
    font-size: 14px;
    color: var(--td-text-color-primary);
  }

  .duration {
    font-size: 14px;
    font-variant-numeric: tabular-nums;
    color: var(--td-text-color-secondary);
  }
}

.volume-indicator {
  display: flex;
  gap: 2px;
  height: 24px;
  align-items: center;

  .volume-bar {
    flex: 1;
    height: 4px;
    border-radius: 2px;
    background: var(--td-bg-color-component);
    transition: height 0.1s ease, background 0.1s ease;

    &.active {
      height: 100%;
      background: var(--td-brand-color);
    }
  }
}

.transcription-area {
  min-height: 120px;
  max-height: 300px;
  overflow-y: auto;
  padding: 12px;
  border-radius: 6px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-border);

  .placeholder {
    color: var(--td-text-color-placeholder);
    font-size: 14px;
    text-align: center;
    line-height: 96px;
  }

  .transcription-text {
    font-size: 14px;
    line-height: 1.8;
    white-space: pre-wrap;
    word-break: break-word;

    .final-text {
      color: var(--td-text-color-primary);
    }

    .interim-text {
      color: var(--td-text-color-placeholder);
      font-style: italic;
    }
  }
}

.error-msg {
  color: var(--td-error-color);
  font-size: 12px;
  padding: 4px 0;
}

.recorder-controls {
  display: flex;
  justify-content: center;
  gap: 12px;
  padding-top: 4px;
}

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
