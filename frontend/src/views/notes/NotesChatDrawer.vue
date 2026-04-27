<template>
  <aside class="notes-chat-drawer" :style="{ width: drawerWidth + 'px' }">
    <div class="drawer-resizer" @mousedown="startResize"></div>
    <header class="drawer-head" style="--wails-draggable: drag">
      <div class="head-left" style="--wails-draggable: drag">
        <t-icon name="chat" class="head-icon" aria-hidden="true" />
        <div class="head-titles">
          <span class="head-title">{{ $t('notes.chat.title') }}</span>
        </div>
      </div>
      <div class="head-actions" style="--wails-draggable: no-drag">
        <t-tooltip :content="$t('notes.chat.newSession')" placement="bottom">
          <button class="head-btn" :disabled="creating" @click="createNewSession">
            <t-icon name="add" />
          </button>
        </t-tooltip>
        <t-tooltip :content="$t('common.close')" placement="bottom">
          <button class="head-btn" @click="close">
            <t-icon name="close" />
          </button>
        </t-tooltip>
      </div>
    </header>

    <div class="drawer-body">
      <div v-if="creating" class="drawer-loading">
        <t-loading size="medium" />
      </div>
      <ChatView
        v-else-if="sessionId"
        :key="sessionId"
        :session_id="sessionId"
        :embeddedMode="true"
        :showInputControls="true"
        :kbIds="currentKbIds"
      />
      <div v-else class="drawer-error">
        <t-icon name="info-circle" />
        <p>{{ $t('notes.chat.unavailable') }}</p>
        <t-button theme="primary" size="small" @click="createNewSession">
          {{ $t('notes.chat.retry') }}
        </t-button>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { useUIStore } from '@/stores/ui'
import { createSessions } from '@/api/chat/index'
import ChatView from '@/views/chat/index.vue'

const router = useRouter()
const { t } = useI18n()
const uiStore = useUIStore()

const creating = ref(false)
const sessionId = computed(() => uiStore.notesChatSessionId)
const currentKbIds = computed(() => uiStore.notesCurrentKbId ? [uiStore.notesCurrentKbId] : [])

const drawerWidth = ref(440)
const minWidth = 320
const maxWidth = 800

let startX = 0
let startWidth = 0

const startResize = (e: MouseEvent) => {
  e.preventDefault()
  startX = e.clientX
  startWidth = drawerWidth.value
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
  document.body.style.cursor = 'col-resize'
}

const onMouseMove = (e: MouseEvent) => {
  // 向左拖动增加宽度，向右拖动减少宽度
  const deltaX = startX - e.clientX
  let newWidth = startWidth + deltaX
  if (newWidth < minWidth) newWidth = minWidth
  if (newWidth > maxWidth) newWidth = maxWidth
  drawerWidth.value = newWidth
}

const onMouseUp = () => {
  document.removeEventListener('mousemove', onMouseMove)
  document.removeEventListener('mouseup', onMouseUp)
  document.body.style.cursor = ''
}

const ensureSession = async () => {
  if (sessionId.value) return
  creating.value = true
  try {
    const res: any = await createSessions({})
    if (res?.data?.id) {
      uiStore.setNotesChatSessionId(String(res.data.id))
    } else {
      MessagePlugin.error(t('notes.chat.createFailed'))
    }
  } catch (e) {
    console.error('[notes-chat] create session failed', e)
    MessagePlugin.error(t('notes.chat.createFailed'))
  } finally {
    creating.value = false
  }
}

const createNewSession = async () => {
  // 重置当前会话，让 ensureSession 创建新的
  uiStore.setNotesChatSessionId('')
  await ensureSession()
}

const close = () => uiStore.setNotesChatPanel(false)

onMounted(() => {
  void ensureSession()
})
</script>

<style scoped lang="less">
@accent: var(--td-brand-color);

.notes-chat-drawer {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  min-height: 0;
  position: relative;
}

.drawer-resizer {
  position: absolute;
  top: 0;
  left: -3px;
  width: 6px;
  height: 100%;
  cursor: col-resize;
  z-index: 10;
  background: transparent;
  transition: background 0.2s ease;

  &:hover,
  &:active {
    background: rgba(var(--td-brand-color-rgb), 0.2);
  }
}

.drawer-head {
  height: 44px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.head-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.head-icon {
  font-size: 16px;
  color: @accent;
}

.head-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}

.head-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.head-btn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 6px;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s ease, color 0.15s ease;

  &:hover:not(:disabled) {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  :deep(.t-icon) {
    font-size: 14px;
  }
}

.drawer-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  position: relative;
}

.drawer-loading {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.drawer-error {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--td-text-color-secondary);
  padding: 32px 24px;
  text-align: center;

  :deep(.t-icon) {
    font-size: 28px;
    color: var(--td-warning-color);
  }

  p {
    margin: 0;
    font-size: 13px;
    line-height: 1.6;
  }
}

/* 让嵌入的聊天页适配抽屉宽度 */
.drawer-body :deep(.chat) {
  height: 100%;
  width: 100%;
  background: var(--td-bg-color-container);
}

.drawer-body :deep(.chat_scroll_box) {
  padding: 0 12px !important;
}

.drawer-body :deep(.msg_list.is-embedded) {
  max-width: 100% !important;
  padding: 12px 4px !important;
}

.drawer-body :deep(.input-container.is-embedded) {
  padding: 6px 10px 8px !important;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
  min-height: 108px !important;
}

/* 为更矮的底部控件条预留空间（默认 56px 底 padding 对应 control-bar 偏高） */
.drawer-body :deep(.rich-input-container .t-textarea__inner) {
  padding-bottom: 40px !important;
}

/* 控件区紧凑化：保持 absolute 布局，靠尺寸/截断解决拥挤 */
.drawer-body :deep(.control-bar) {
  gap: 4px !important;
  left: 10px !important;
  right: 10px !important;
  bottom: 4px !important;
  max-height: 36px !important;
  padding-top: 2px !important;
  flex-wrap: nowrap !important;
}

.drawer-body :deep(.control-left) {
  gap: 4px !important;
  flex-wrap: nowrap !important;
  min-width: 0 !important;
  overflow: hidden;
}

.drawer-body :deep(.control-right) {
  flex-shrink: 0 !important;
  gap: 4px !important;
}

.drawer-body :deep(.control-btn) {
  padding: 2px 4px !important;
  min-height: 0 !important;
  height: 24px !important;
  flex-shrink: 0;
}

/* Agent 模式按钮：限制宽度并溢出省略 */
.drawer-body :deep(.agent-mode-btn) {
  height: 24px !important;
  padding: 0 4px !important;
  font-size: 11px !important;
  max-width: 120px !important;
  flex-shrink: 1 !important;
  min-width: 0 !important;

  .agent-mode-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    font-size: 11px !important;
  }

  .dropdown-arrow {
    flex-shrink: 0;
    margin-left: 2px;
  }
}

/* 模型选择按钮：保留靠右、可截短 */
.drawer-body :deep(.model-display) {
  margin-left: auto !important;
  flex-shrink: 1 !important;
  min-width: 0 !important;
}

.drawer-body :deep(.model-selector-trigger) {
  height: 22px !important;
  padding: 0 5px !important;
  font-size: 11px !important;
  max-width: 140px !important;
  min-width: 0 !important;
  flex-shrink: 1 !important;

  .model-selector-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
    font-size: 11px !important;
  }

  .model-dropdown-arrow {
    flex-shrink: 0;
    margin-left: 2px;
  }
}

/* 计数小徽标缩小 */
.drawer-body :deep(.image-count),
.drawer-body :deep(.attachment-count),
.drawer-body :deep(.kb-count) {
  font-size: 10px !important;
  padding: 0 3px !important;
  min-width: 14px !important;
}

/* 控件图标稍微缩小 */
.drawer-body :deep(.control-btn .control-icon) {
  width: 15px !important;
  height: 15px !important;
}

.drawer-body :deep(.send-btn),
.drawer-body :deep(.stop-btn) {
  width: 24px !important;
  height: 24px !important;
}

.drawer-body :deep(.send-btn img) {
  width: 14px !important;
  height: 14px !important;
}
</style>
