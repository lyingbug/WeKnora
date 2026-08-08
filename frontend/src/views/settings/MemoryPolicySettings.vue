<template>
  <div class="memory-policy-settings">
    <div class="section-header">
      <h2>{{ t('memory.policy.title') }}</h2>
      <p class="section-description">{{ t('memory.policy.description') }}</p>

      <div class="memory-hint" role="note">
        <p class="memory-hint__label">{{ t('memory.policy.hintLabel') }}</p>
        <p class="memory-hint__text">{{ t('memory.policy.hintText') }}</p>
      </div>
    </div>

    <t-tabs v-model="activeTab" class="policy-tabs">
      <t-tab-panel v-for="group in policyGroups" :key="group" :value="group"
        :label="t(`memory.settings.groups.${group}.title`)" />
    </t-tabs>

    <div class="policy-panel">
      <MemorySettingsPanel level="tenant" :group-filter="[activeTab]" show-all :show-group-titles="false"
        row-layout="inline" density="comfortable" panel-variant="policy" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import MemorySettingsPanel from '../memory/components/MemorySettingsPanel.vue'

const { t } = useI18n()

const policyGroups = [
  'general',
  'write',
  'recall',
  'boost',
  'anchor',
  'lifecycle',
  'privacy',
  'insights',
]

const activeTab = ref(policyGroups[0])
</script>

<style scoped lang="less">
.memory-policy-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 28px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px;
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.6;
  }
}

.memory-hint {
  margin-top: 16px;
  padding: 10px 12px;
  background: var(--td-bg-color-secondarycontainer);
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;

  &__label {
    margin: 0 0 4px;
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-placeholder);
    letter-spacing: 0.02em;
  }

  &__text {
    margin: 0;
    font-size: 14px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }
}

.policy-tabs {
  margin-bottom: 24px;

  :deep(.t-tabs__nav-item) {
    font-size: 14px;
  }

  :deep(.t-tabs__nav-item-wrapper) {
    padding: 0 14px;
    margin: 0;
  }

  :deep(.t-tabs__operations) {
    display: none;
  }

  :deep(.t-tabs__nav-scroll) {
    overflow-x: auto;
    scrollbar-width: none;

    &::-webkit-scrollbar {
      display: none;
    }
  }

  :deep(.t-tabs__content) {
    display: none;
  }
}

.policy-panel {
  margin-top: 4px;
}
</style>
