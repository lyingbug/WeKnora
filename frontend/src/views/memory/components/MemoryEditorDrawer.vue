<template>
  <SettingDrawer
    :visible="visible"
    :title="slug ? t('memory.editor.editHeader') : t('memory.editor.createHeader')"
    icon="bookmark"
    width="640px"
    :min-width="520"
    :max-width="960"
    storage-key="setting-drawer:width:memory-editor"
    :confirm-loading="saving"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    @confirm="save"
    @cancel="close"
  >
    <t-loading :loading="loading" :show-overlay="false">
      <section class="setting-drawer__section">
        <div class="form-item">
          <label class="form-label">{{ t('memory.editor.type') }}</label>
          <t-select v-model="form.page_type" :disabled="Boolean(slug)">
            <t-option
              v-for="type in MEMORY_TYPES"
              :key="type"
              :value="type"
              :label="t(`memory.types.${type}`)"
            />
          </t-select>
          <p v-if="slug" class="form-desc">{{ t('memory.editor.typeLocked') }}</p>
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('memory.editor.title') }}</label>
          <t-input v-model="form.title" :placeholder="t('memory.editor.titlePlaceholder')" />
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('memory.editor.summary') }}</label>
          <t-input v-model="form.summary" :placeholder="t('memory.editor.summaryPlaceholder')" />
          <p class="form-desc">{{ t('memory.editor.summaryHint') }}</p>
        </div>

        <div class="form-item">
          <label class="form-label">{{ t('memory.editor.content') }}</label>
          <t-textarea
            v-model="form.content"
            :autosize="{ minRows: 6, maxRows: 16 }"
            :placeholder="t('memory.editor.contentPlaceholder')"
          />
          <p class="form-desc">{{ t('memory.editor.linkHint') }}</p>
        </div>
      </section>

      <section v-if="form.page_type === 'preference'" class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ t('memory.editor.preferenceSection') }}</h4>
        <div class="preference-grid">
          <div class="form-item">
            <label class="form-label">{{ t('memory.editor.language') }}</label>
            <t-input v-model="form.structured.language" placeholder="zh / en / ja" />
          </div>
          <div class="form-item">
            <label class="form-label">{{ t('memory.editor.verbosity') }}</label>
            <t-select v-model="form.structured.verbosity" clearable>
              <t-option
                v-for="v in ['concise', 'balanced', 'detailed']"
                :key="v"
                :value="v"
                :label="t(`memory.settings.options.${v}`)"
              />
            </t-select>
          </div>
          <div class="form-item">
            <label class="form-label">{{ t('memory.editor.format') }}</label>
            <t-select v-model="form.structured.format" clearable>
              <t-option
                v-for="v in ['prose', 'bullets', 'markdown']"
                :key="v"
                :value="v"
                :label="t(`memory.settings.options.${v}`)"
              />
            </t-select>
          </div>
          <div class="form-item">
            <label class="form-label">{{ t('memory.editor.codeStyle') }}</label>
            <t-select v-model="form.structured.code_style" clearable>
              <t-option
                v-for="v in ['always', 'minimal', 'when_asked']"
                :key="v"
                :value="v"
                :label="t(`memory.settings.options.${v}`)"
              />
            </t-select>
          </div>
        </div>
      </section>

      <section class="setting-drawer__section">
        <div class="form-item form-item--inline">
          <t-checkbox v-model="form.pinned">{{ t('memory.editor.pinned') }}</t-checkbox>
          <span class="form-desc form-desc--inline">{{ t('memory.editor.pinnedHint') }}</span>
        </div>

        <div v-if="revisions.length" class="form-item">
          <label class="form-label">{{ t('memory.editor.history') }}</label>
          <div class="memory-editor__revisions">
            <div v-for="revision in revisions" :key="revision.id" class="revision-row">
              <span class="revision-row__meta">
                v{{ revision.version }} ·
                {{ t(`memory.editSource.${revision.edit_source || 'pipeline'}`) }} ·
                {{ formatDate(revision.edited_at) }}
              </span>
              <t-link theme="primary" size="small" hover="color" @click="revert(revision.version)">
                {{ t('memory.editor.revert') }}
              </t-link>
            </div>
          </div>
        </div>
      </section>
    </t-loading>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import {
  MEMORY_TYPES,
  createMemoryPage,
  getMemoryPage,
  listMemoryRevisions,
  revertMemoryPage,
  updateMemoryPage,
  type MemoryPageRevision,
  type MemoryPreference,
} from '@/api/memory'

const props = defineProps<{ visible: boolean; slug: string }>()
const emit = defineEmits<{ (e: 'update:visible', value: boolean): void; (e: 'saved'): void }>()

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const revisions = ref<MemoryPageRevision[]>([])
const version = ref(0)
const status = ref('')

const form = reactive<{
  page_type: string
  title: string
  summary: string
  content: string
  pinned: boolean
  structured: MemoryPreference
}>({
  page_type: 'episode',
  title: '',
  summary: '',
  content: '',
  pinned: false,
  structured: {},
})

function reset() {
  form.page_type = 'episode'
  form.title = ''
  form.summary = ''
  form.content = ''
  form.pinned = false
  form.structured = {}
  revisions.value = []
  version.value = 0
  status.value = ''
}

function formatDate(value: string): string {
  return value ? new Date(value).toLocaleString() : '-'
}

async function load() {
  if (!props.slug) {
    reset()
    return
  }
  loading.value = true
  try {
    const res: any = await getMemoryPage(props.slug)
    const page = res?.data
    form.page_type = page.page_type
    form.title = page.title
    form.summary = page.summary
    form.content = page.content
    form.pinned = page.pinned
    form.structured = { ...(page.structured || {}) }
    version.value = page.version
    status.value = page.status || ''

    const revisionRes: any = await listMemoryRevisions(props.slug)
    revisions.value = revisionRes?.data || []
  } catch {
    MessagePlugin.error(t('memory.errors.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!form.content.trim() && !form.summary.trim()) {
    MessagePlugin.warning(t('memory.editor.emptyWarning'))
    return
  }
  saving.value = true
  try {
    const body: Record<string, any> = {
      page_type: form.page_type,
      title: form.title,
      summary: form.summary,
      content: form.content,
      pinned: form.pinned,
    }
    if (status.value) {
      body.status = status.value
    }
    if (form.page_type === 'preference') {
      body.structured = form.structured
    }
    if (props.slug) {
      body.version = version.value
      await updateMemoryPage(props.slug, body)
    } else {
      await createMemoryPage(body as any)
    }
    MessagePlugin.success(t('memory.editor.saved'))
    emit('saved')
  } catch (error: any) {
    if (error?.response?.status === 409) {
      MessagePlugin.warning(t('memory.editor.conflict'))
      load()
      return
    }
    MessagePlugin.error(t('memory.errors.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function revert(targetVersion: number) {
  try {
    await revertMemoryPage(props.slug, targetVersion, version.value)
    MessagePlugin.success(t('memory.editor.reverted'))
    await load()
    emit('saved')
  } catch (error: any) {
    if (error?.response?.status === 409) {
      MessagePlugin.warning(t('memory.editor.conflict'))
      await load()
      return
    }
    MessagePlugin.error(t('memory.errors.saveFailed'))
  }
}

function close() {
  emit('update:visible', false)
}

watch(
  () => [props.visible, props.slug],
  ([visible]) => {
    if (visible) load()
  },
)
</script>

<style scoped lang="less">
.form-item {
  margin-bottom: 0;
}

.form-label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.4;
}

.form-desc {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);

  &--inline {
    margin: 0;
  }
}

.form-item--inline {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.preference-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 16px;
}

.memory-editor__revisions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-secondarycontainer);
}

.revision-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary);

  &__meta {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

@media (max-width: 560px) {
  .preference-grid {
    grid-template-columns: 1fr;
  }
}
</style>
