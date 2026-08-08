<template>
  <SettingDrawer :visible="visible" :title="t('memory.actions.openSettings')"
    :description="t('memory.settingsDrawer.description')" icon="setting" width="560px" :min-width="480" :max-width="880"
    storage-key="setting-drawer:width:memory-personal-settings" hide-footer
    @update:visible="(v: boolean) => emit('update:visible', v)">
    <MemorySettingsPanel ref="panel" level="user" section-layout row-layout="stacked" :show-group-titles="true"
      @changed="emit('changed')" />

    <section class="setting-drawer__section">
      <h4 class="setting-drawer__section-title">{{ t('memory.data.title') }}</h4>
      <div class="memory-data-row">
        <div class="memory-data-row__info">
          <label class="form-label">{{ t('memory.data.exportLabel') }}</label>
          <p class="form-desc">{{ t('memory.data.exportDesc') }}</p>
        </div>
        <t-button size="small" variant="outline" @click="exportMemory">
          {{ t('memory.actions.export') }}
        </t-button>
      </div>
      <div class="memory-data-row">
        <div class="memory-data-row__info">
          <label class="form-label">{{ t('memory.data.forgetLabel') }}</label>
          <p class="form-desc">{{ t('memory.data.forgetDesc') }}</p>
        </div>
        <t-button size="small" theme="danger" variant="outline" @click="confirmForgetAll">
          {{ t('memory.actions.forgetAll') }}
        </t-button>
      </div>
    </section>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'

import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import { exportMemories, forgetMemories } from '@/api/memory'
import MemorySettingsPanel from './MemorySettingsPanel.vue'

defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
  (e: 'forgot-all'): void
}>()

const { t } = useI18n()
const panel = ref<InstanceType<typeof MemorySettingsPanel> | null>(null)

async function exportMemory() {
  try {
    const blob = await exportMemories()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `weknora-memory-${new Date().toISOString().slice(0, 10)}.json`
    link.click()
    URL.revokeObjectURL(url)
  } catch {
    MessagePlugin.error(t('memory.errors.exportFailed'))
  }
}

function confirmForgetAll() {
  const dialog = DialogPlugin.confirm({
    header: t('memory.forgetAll.header'),
    body: t('memory.forgetAll.body'),
    confirmBtn: { content: t('memory.forgetAll.confirm'), theme: 'danger' },
    onConfirm: async () => {
      try {
        await forgetMemories({ scope: 'all' })
        MessagePlugin.success(t('memory.forgetAll.done'))
        emit('forgot-all')
        emit('changed')
      } catch {
        MessagePlugin.error(t('memory.errors.forgetFailed'))
      } finally {
        dialog.destroy()
      }
    },
  })
}

defineExpose({
  reload: () => panel.value?.reload(),
})
</script>

<style scoped lang="less">
.memory-data-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 0;

  &:not(:last-child) {
    border-bottom: 1px solid var(--td-component-stroke);
  }

  &__info {
    flex: 1;
    min-width: 0;
  }
}

.form-label {
  display: block;
  margin-bottom: 4px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.form-desc {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
}
</style>
