<template>
  <t-dialog
    :visible="visible"
    :header="slug ? t('memory.editor.editHeader') : t('memory.editor.createHeader')"
    :confirm-btn="{ content: t('memory.editor.save'), loading: saving }"
    :cancel-btn="t('memory.editor.cancel')"
    width="720px"
    @close="close"
    @cancel="close"
    @confirm="save"
  >
    <t-loading :loading="loading" :show-overlay="false">
      <t-form label-align="top" class="memory-editor">
        <t-form-item :label="t('memory.editor.type')">
          <t-select v-model="form.page_type" :disabled="Boolean(slug)">
            <t-option
              v-for="type in MEMORY_TYPES"
              :key="type"
              :value="type"
              :label="t(`memory.types.${type}`)"
            />
          </t-select>
          <!-- The type is fixed after creation because it is part of the slug,
               and silently re-addressing a memory would orphan its links. -->
          <p v-if="slug" class="memory-editor__hint">{{ t('memory.editor.typeLocked') }}</p>
        </t-form-item>

        <t-form-item :label="t('memory.editor.title')">
          <t-input v-model="form.title" :placeholder="t('memory.editor.titlePlaceholder')" />
        </t-form-item>

        <t-form-item :label="t('memory.editor.summary')">
          <t-input v-model="form.summary" :placeholder="t('memory.editor.summaryPlaceholder')" />
          <p class="memory-editor__hint">{{ t('memory.editor.summaryHint') }}</p>
        </t-form-item>

        <t-form-item :label="t('memory.editor.content')">
          <t-textarea
            v-model="form.content"
            :autosize="{ minRows: 6, maxRows: 16 }"
            :placeholder="t('memory.editor.contentPlaceholder')"
          />
          <p class="memory-editor__hint">{{ t('memory.editor.linkHint') }}</p>
        </t-form-item>

        <!-- Structured preferences are the only memory that steers generation,
             so they get typed controls rather than free text. -->
        <template v-if="form.page_type === 'preference'">
          <div class="memory-editor__preferences">
            <t-form-item :label="t('memory.editor.language')">
              <t-input v-model="form.structured.language" placeholder="zh / en / ja" />
            </t-form-item>
            <t-form-item :label="t('memory.editor.verbosity')">
              <t-select v-model="form.structured.verbosity" clearable>
                <t-option v-for="v in ['concise', 'balanced', 'detailed']" :key="v" :value="v"
                  :label="t(`memory.settings.options.${v}`)" />
              </t-select>
            </t-form-item>
            <t-form-item :label="t('memory.editor.format')">
              <t-select v-model="form.structured.format" clearable>
                <t-option v-for="v in ['prose', 'bullets', 'markdown']" :key="v" :value="v"
                  :label="t(`memory.settings.options.${v}`)" />
              </t-select>
            </t-form-item>
            <t-form-item :label="t('memory.editor.codeStyle')">
              <t-select v-model="form.structured.code_style" clearable>
                <t-option v-for="v in ['always', 'minimal', 'when_asked']" :key="v" :value="v"
                  :label="t(`memory.settings.options.${v}`)" />
              </t-select>
            </t-form-item>
          </div>
        </template>

        <t-form-item>
          <t-checkbox v-model="form.pinned">{{ t('memory.editor.pinned') }}</t-checkbox>
          <span class="memory-editor__hint">{{ t('memory.editor.pinnedHint') }}</span>
        </t-form-item>

        <t-form-item v-if="revisions.length" :label="t('memory.editor.history')">
          <div class="memory-editor__revisions">
            <div v-for="revision in revisions" :key="revision.id" class="revision-row">
              <span class="revision-row__meta">
                v{{ revision.version }} · {{ t(`memory.editSource.${revision.edit_source || 'pipeline'}`) }} ·
                {{ formatDate(revision.edited_at) }}
              </span>
              <t-link theme="primary" size="small" hover="color" @click="revert(revision.version)">
                {{ t('memory.editor.revert') }}
              </t-link>
            </div>
          </div>
        </t-form-item>
      </t-form>
    </t-loading>
  </t-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'

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
// Held so a save can send it back: the server reads an absent status as
// "active", which would un-archive whatever is being edited.
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
    // The server reads an absent status as "active", so editing an archived
    // memory without saying so silently brings it back.
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
    // A version conflict means someone (or an agent) changed the memory while
    // this dialog was open; reloading is more useful than a generic error.
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
    await revertMemoryPage(props.slug, targetVersion)
    MessagePlugin.success(t('memory.editor.reverted'))
    await load()
    emit('saved')
  } catch {
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
.memory-editor {
  &__hint {
    margin: 4px 0 0;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }

  &__preferences {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0 16px;
    padding: 12px 16px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 8px;
    margin-bottom: 8px;
  }

  &__revisions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    width: 100%;
  }
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
</style>
