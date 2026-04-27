<template>
  <div class="notes-welcome">
    <t-tooltip :content="$t('notes.chat.title')" placement="bottom">
      <button
        class="welcome-chat-btn"
        :class="{ 'is-active': uiStore.notesChatPanelOpen }"
        @click="uiStore.setNotesChatPanel(!uiStore.notesChatPanelOpen)"
      >
        <t-icon name="chat" />
      </button>
    </t-tooltip>

    <div class="welcome-inner">
      <div class="welcome-hero">
        <span class="hero-eyebrow">WeKnora · Notes</span>
        <h1 class="hero-title">{{ $t('notes.list.title') }}</h1>
        <p class="hero-sub">{{ $t('notes.list.subtitle') }}</p>
        <div class="hero-actions">
          <t-button theme="primary" size="large" @click="goNew">
            <template #icon><t-icon name="add" /></template>
            {{ $t('notes.list.newNote') }}
          </t-button>
        </div>
        <p class="hero-hint">
          <t-icon name="upload" />
          {{ $t('notes.welcome.dragHint') }}
        </p>
      </div>

      <div class="welcome-feats">
        <div class="feat">
          <div class="feat-icon edit">
            <t-icon name="edit-1" />
          </div>
          <div class="feat-content">
            <div class="feat-title">{{ $t('notes.welcome.featWriteTitle') }}</div>
            <div class="feat-desc">{{ $t('notes.welcome.featWriteDesc') }}</div>
          </div>
        </div>
        <div class="feat">
          <div class="feat-icon publish">
            <t-icon name="refresh" />
          </div>
          <div class="feat-content">
            <div class="feat-title">{{ $t('notes.welcome.featPublishTitle') }}</div>
            <div class="feat-desc">{{ $t('notes.welcome.featPublishDesc') }}</div>
          </div>
        </div>
        <div class="feat">
          <div class="feat-icon kbd">
            <t-icon name="keyboard" />
          </div>
          <div class="feat-content">
            <div class="feat-title">{{ $t('notes.welcome.featShortcutsTitle') }}</div>
            <div class="feat-desc shortcuts-list">
              <span class="kbd-line"><kbd>⌘S</kbd><span class="kbd-text">{{ $t('notes.list.tipSave') }}</span></span>
              <span class="kbd-line"><kbd>⌘B</kbd><span class="kbd-text">{{ $t('notes.list.tipBold') }}</span></span>
              <span class="kbd-line"><kbd>⇧P</kbd><span class="kbd-text">{{ $t('notes.list.tipPreview') }}</span></span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useUIStore } from '@/stores/ui'

const router = useRouter()
const uiStore = useUIStore()
const goNew = () => router.push('/platform/notes/new')
</script>

<style scoped lang="less">
@accent: var(--td-brand-color);

.notes-welcome {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow-y: auto;
  padding: 48px 32px;
  position: relative;
  background:
    radial-gradient(900px 360px at 85% -10%, rgba(7, 192, 95, 0.05), transparent 60%),
    radial-gradient(600px 280px at 5% 110%, rgba(7, 192, 95, 0.04), transparent 60%);
}

.welcome-chat-btn {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;
  z-index: 5;

  &:hover {
    background: var(--td-bg-color-container-hover);
    color: var(--td-text-color-primary);
  }

  &.is-active {
    background: var(--td-brand-color-light);
    color: var(--td-brand-color);
  }

  :deep(.t-icon) {
    font-size: 16px;
  }
}

.welcome-inner {
  width: 100%;
  max-width: 760px;
  display: flex;
  flex-direction: column;
  gap: 36px;
}

.welcome-hero {
  text-align: left;
}

.hero-eyebrow {
  display: inline-block;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 1.2px;
  color: @accent;
  text-transform: uppercase;
  background: rgba(7, 192, 95, 0.1);
  padding: 4px 10px;
  border-radius: 999px;
  margin-bottom: 16px;
}

.hero-title {
  margin: 0 0 10px;
  font-size: 36px;
  font-weight: 700;
  letter-spacing: -0.5px;
  color: var(--td-text-color-primary);
  line-height: 1.2;
}

.hero-sub {
  margin: 0 0 24px;
  font-size: 15px;
  line-height: 1.7;
  color: var(--td-text-color-secondary);
  max-width: 560px;
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.hero-hint {
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  display: inline-flex;
  align-items: center;
  gap: 6px;

  :deep(.t-icon) {
    font-size: 14px;
  }
}

.welcome-feats {
  display: grid;
  gap: 20px;
  grid-template-columns: repeat(3, 1fr);
  margin-top: 16px;
}

.feat {
  position: relative;
  padding: 24px;
  border: 1px solid var(--td-component-border);
  border-radius: 16px;
  background: var(--td-bg-color-container);
  transition: all 0.3s cubic-bezier(0.2, 0.8, 0.2, 1);
  display: flex;
  flex-direction: column;
  overflow: hidden;

  /* Subtle inner glow / reflection */
  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 100%;
    background: linear-gradient(180deg, rgba(255,255,255,0.05) 0%, transparent 100%);
    opacity: 0;
    transition: opacity 0.3s ease;
    pointer-events: none;
  }

  &:hover {
    border-color: var(--td-brand-color-light);
    box-shadow: 0 12px 32px -8px rgba(7, 192, 95, 0.12), 0 4px 12px -4px rgba(0, 0, 0, 0.04);
    transform: translateY(-4px);

    &::before {
      opacity: 1;
    }

    .feat-icon {
      transform: scale(1.05);
    }
  }
}

.feat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 20px;
  transition: transform 0.3s cubic-bezier(0.2, 0.8, 0.2, 1);
  flex-shrink: 0;

  :deep(.t-icon) {
    font-size: 22px;
  }

  &.edit {
    background: linear-gradient(135deg, rgba(7, 192, 95, 0.15), rgba(7, 192, 95, 0.05));
    color: @accent;
    border: 1px solid rgba(7, 192, 95, 0.1);
  }
  &.publish {
    background: linear-gradient(135deg, rgba(0, 82, 217, 0.15), rgba(0, 82, 217, 0.05));
    color: var(--td-brand-color-7, #0052d9);
    border: 1px solid rgba(0, 82, 217, 0.1);
  }
  &.kbd {
    background: linear-gradient(135deg, rgba(255, 152, 0, 0.15), rgba(255, 152, 0, 0.05));
    color: var(--td-warning-color);
    border: 1px solid rgba(255, 152, 0, 0.1);
  }
}

.feat-content {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.feat-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 8px;
  letter-spacing: -0.2px;
}

.feat-desc {
  font-size: 14px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
  
  &.shortcuts-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-top: 4px;
  }
}

.kbd-line {
  display: flex;
  align-items: center;
  gap: 10px;

  .kbd-text {
    font-size: 13px;
    color: var(--td-text-color-secondary);
  }

  kbd {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 32px;
    height: 24px;
    padding: 0 6px;
    border-radius: 6px;
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
    font-family: 'JetBrains Mono', 'SF Mono', Consolas, monospace;
    font-size: 12px;
    font-weight: 500;
    border: 1px solid var(--td-component-border);
    border-bottom-width: 2px;
    box-shadow: 0 1px 2px rgba(0,0,0,0.02);
  }
}

/* 容器查询：当画布宽度被抽屉挤压到很窄时，优雅降级欢迎页 */
@container (max-width: 540px) {
  .notes-welcome {
    padding: 24px 16px;
    align-items: flex-start;
  }
  
  .welcome-inner {
    margin-top: 4vh;
    gap: 24px;
  }

  .hero-title {
    font-size: 26px;
  }

  .hero-sub {
    font-size: 14px;
    margin-bottom: 16px;
  }

  .welcome-feats {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .feat {
    padding: 16px;
    flex-direction: row;
    align-items: flex-start;
    gap: 16px;
  }

  .feat-icon {
    width: 36px;
    height: 36px;
    margin-bottom: 0;

    :deep(.t-icon) {
      font-size: 18px;
    }
  }

  .feat-title {
    font-size: 15px;
    margin-bottom: 4px;
  }

  .feat-desc {
    font-size: 13px;
  }
}
</style>
