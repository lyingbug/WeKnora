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

    <MemorySettingsPanel level="tenant" :primary-keys="primaryKeys" />
  </div>
</template>

<script setup lang="ts">
/**
 * The workspace-level memory policy.
 *
 * Deliberately not the same form as personal settings, even though it is
 * generated from the same catalogue. What an administrator sets here is a
 * ceiling for everyone: members can be stricter with themselves but never more
 * permissive, so each value reads as "the most this workspace allows". Showing
 * the two screens as interchangeable copies of one form was what made it
 * impossible to tell which one was in charge.
 *
 * The primary set is therefore the policy questions — is memory available at
 * all, how far may capture go, must candidates be reviewed, what may be
 * recorded, and what happens to personal data — with the operational knobs
 * behind "more settings".
 */
import { useI18n } from 'vue-i18n'

import MemorySettingsPanel from '../memory/components/MemorySettingsPanel.vue'

const { t } = useI18n()

const primaryKeys = [
  'memory.enabled',
  'memory.write.mode',
  'memory.write.require_review',
  'memory.write.allowed_types',
  'memory.privacy.pii_redaction',
  'memory.retention.days',
]
</script>

<style scoped lang="less">
.memory-policy-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 4px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px 0;
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
    margin: 0 0 4px 0;
    font-size: 12px;
    font-weight: 500;
    color: var(--td-text-color-placeholder);
    letter-spacing: 0.02em;
  }

  &__text {
    margin: 0;
    font-size: 13px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }
}
</style>
