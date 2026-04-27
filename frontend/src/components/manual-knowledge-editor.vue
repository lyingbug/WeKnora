<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'
import { useUIStore } from '@/stores/ui'
import { listKnowledgeBases, getKnowledgeDetails, createManualKnowledge, updateManualKnowledge } from '@/api/knowledge-base'
import { useI18n } from 'vue-i18n'
import MarkdownEditorCore from '@/components/MarkdownEditorCore.vue'

interface KnowledgeBaseOption {
  label: string
  value: string
}

interface KnowledgeDetailResponse {
  id: string
  knowledge_base_id: string
  title?: string
  file_name?: string
  metadata?: any
  parse_status?: string
}

type ManualStatus = 'draft' | 'publish'

const uiStore = useUIStore()
const router = useRouter()
const { t } = useI18n()

const visible = computed({
  get: () => uiStore.manualEditorVisible,
  set: (val: boolean) => {
    if (!val) {
      handleClose()
    }
  },
})

const mode = computed(() => uiStore.manualEditorMode)
const knowledgeId = computed(() => uiStore.manualEditorKnowledgeId)

const form = reactive({
  kbId: '' as string,
  title: '',
  content: '',
  status: 'draft' as ManualStatus,
})

const initialLoaded = ref(false)
const kbOptions = ref<KnowledgeBaseOption[]>([])
const kbLoading = ref(false)
const contentLoading = ref(false)
const saving = ref(false)
const savingAction = ref<ManualStatus>('draft')
const activeTab = ref<'edit' | 'preview'>('edit')
const lastUpdatedAt = ref<string>('')

const markdownCoreRef = ref<InstanceType<typeof MarkdownEditorCore> | null>(null)

const coreViewMode = computed<'edit' | 'preview'>({
  get: () => (activeTab.value === 'preview' ? 'preview' : 'edit'),
  set: (v) => {
    activeTab.value = v === 'preview' ? 'preview' : 'edit'
  },
})

const isPreviewMode = computed(() => activeTab.value === 'preview')
const viewToggleIcon = computed(() => (isPreviewMode.value ? 'edit' : 'view-module'))
const viewToggleTooltip = computed(() =>
  isPreviewMode.value
    ? t('manualEditor.view.toggleToEdit')
    : t('manualEditor.view.toggleToPreview'),
)
const viewToggleLabel = computed(() =>
  isPreviewMode.value ? t('manualEditor.view.editLabel') : t('manualEditor.view.previewLabel'),
)

const kbDisabled = computed(() => mode.value === 'edit' && !!form.kbId)

const dialogTitle = computed(() =>
  mode.value === 'edit' ? t('manualEditor.title.edit') : t('manualEditor.title.create'),
)

const lastUpdatedText = computed(() =>
  lastUpdatedAt.value ? t('manualEditor.status.lastUpdated', { time: lastUpdatedAt.value }) : '',
)

const canOpenInWindow = computed(() => mode.value === 'edit' && !!knowledgeId.value)

const openInNotesWindow = () => {
  if (!knowledgeId.value) return
  const id = knowledgeId.value
  handleClose()
  router.push(`/platform/notes/${id}`)
}

const loadKnowledgeBases = async () => {
  kbLoading.value = true
  try {
    const res: any = await listKnowledgeBases()
    const allKbs = Array.isArray(res?.data) ? res.data : []
    const list: KnowledgeBaseOption[] = allKbs
      .filter((item: any) => !item.type || item.type === 'document')
      .map((item: any) => ({ label: item.name, value: item.id }))
    kbOptions.value = list

    if (mode.value === 'create') {
      const presetKbId = uiStore.manualEditorKBId
      if (presetKbId) {
        const exists = list.find((item) => item.value === presetKbId)
        if (!exists) {
          kbOptions.value.unshift({
            label: t('manualEditor.labels.currentKnowledgeBase'),
            value: presetKbId,
          })
        }
        form.kbId = presetKbId
      } else {
        form.kbId = list[0]?.value ?? ''
      }
    }
  } catch (error) {
    console.error('[ManualEditor] Failed to load knowledge base list:', error)
    kbOptions.value = []
  } finally {
    kbLoading.value = false
  }
}

const parseManualMetadata = (
  metadata: any,
): { content: string; status: ManualStatus; updatedAt?: string } | null => {
  if (!metadata) {
    return null
  }
  try {
    let parsed = metadata
    if (typeof metadata === 'string') {
      parsed = JSON.parse(metadata)
    }
    if (parsed && typeof parsed === 'object') {
      const status = parsed.status === 'publish' ? 'publish' : 'draft'
      return {
        content: parsed.content || '',
        status,
        updatedAt: parsed.updated_at || parsed.updatedAt,
      }
    }
  } catch (error) {
    console.warn('[ManualEditor] Failed to parse manual metadata:', error)
  }
  return null
}

const loadKnowledgeContent = async () => {
  if (!knowledgeId.value) {
    return
  }
  contentLoading.value = true
  try {
    const res: any = await getKnowledgeDetails(knowledgeId.value)
    const data: KnowledgeDetailResponse | undefined = res?.data
    if (!data) {
      MessagePlugin.error(t('manualEditor.error.fetchDetailFailed'))
      return
    }

    form.kbId = data.knowledge_base_id || form.kbId
    const meta = parseManualMetadata(data.metadata)
    form.title =
      data.title ||
      data.file_name?.replace(/\.md$/i, '') ||
      uiStore.manualEditorInitialTitle ||
      ''
    form.content = meta?.content || uiStore.manualEditorInitialContent || ''
    form.status = meta?.status || (data.parse_status === 'completed' ? 'publish' : 'draft')
    if (meta?.updatedAt) {
      lastUpdatedAt.value = meta.updatedAt
    }

    if (form.kbId && !kbOptions.value.find((item) => item.value === form.kbId)) {
      kbOptions.value.unshift({
        label: t('manualEditor.labels.currentKnowledgeBase'),
        value: form.kbId,
      })
    }
  } catch (error) {
    console.error('[ManualEditor] Failed to load manual knowledge:', error)
    MessagePlugin.error(t('manualEditor.error.fetchDetailFailed'))
  } finally {
    contentLoading.value = false
  }
}

const resetForm = () => {
  form.kbId = uiStore.manualEditorKBId || ''
  form.title = uiStore.manualEditorInitialTitle || ''
  form.content = uiStore.manualEditorInitialContent || ''
  form.status = uiStore.manualEditorInitialStatus || 'draft'
  activeTab.value = 'edit'
  lastUpdatedAt.value = ''
  initialLoaded.value = false
}

const generateDefaultTitle = () => {
  if (uiStore.manualEditorInitialTitle) {
    return uiStore.manualEditorInitialTitle
  }
  return `${t('manualEditor.defaultTitlePrefix')}-${new Date().toLocaleString()}`
}

const initialize = async () => {
  resetForm()
  await loadKnowledgeBases()

  if (mode.value === 'edit') {
    await loadKnowledgeContent()
  } else {
    const presetKbId = uiStore.manualEditorKBId
    if (presetKbId) {
      form.kbId = presetKbId
    } else if (!form.kbId && kbOptions.value.length) {
      form.kbId = kbOptions.value[0].value
    }
    form.title = form.title || generateDefaultTitle()
    form.content = form.content || ''
  }

  initialLoaded.value = true
}

const validateForm = (targetStatus: ManualStatus): boolean => {
  if (!form.kbId) {
    MessagePlugin.warning(t('manualEditor.warning.selectKnowledgeBase'))
    return false
  }
  if (!form.title || !form.title.trim()) {
    MessagePlugin.warning(t('manualEditor.warning.enterTitle'))
    return false
  }
  if (!form.content || !form.content.trim()) {
    MessagePlugin.warning(t('manualEditor.warning.enterContent'))
    return false
  }
  if (targetStatus === 'publish' && form.content.trim().length < 10) {
    MessagePlugin.warning(t('manualEditor.warning.contentTooShort'))
    return false
  }
  return true
}

const handleSave = async (targetStatus: ManualStatus) => {
  if (saving.value || !validateForm(targetStatus)) {
    return
  }
  saving.value = true
  savingAction.value = targetStatus
  try {
    const payload: { title: string; content: string; status: string; tag_id?: string } = {
      title: form.title.trim(),
      content: form.content,
      status: targetStatus,
    }
    let response: any
    let knowledgeID = knowledgeId.value
    let kbId = form.kbId

    if (mode.value === 'edit' && knowledgeId.value) {
      response = await updateManualKnowledge(knowledgeId.value, payload)
    } else {
      const tagIdToUpload = uiStore.selectedTagId !== '__untagged__' ? uiStore.selectedTagId : undefined
      if (tagIdToUpload) {
        payload.tag_id = tagIdToUpload
      }
      response = await createManualKnowledge(form.kbId, payload)
      knowledgeID = response?.data?.id || knowledgeID
      kbId = form.kbId
    }

    if (response?.success) {
      MessagePlugin.success(
        targetStatus === 'draft'
          ? t('manualEditor.success.draftSaved')
          : t('manualEditor.success.published'),
      )
      if (knowledgeID) {
        uiStore.notifyManualEditorSuccess({
          kbId,
          knowledgeId: knowledgeID,
          status: targetStatus,
        })
      }
      uiStore.closeManualEditor()
    } else {
      const message = response?.message || t('manualEditor.error.saveFailed')
      MessagePlugin.error(message)
    }
  } catch (error: any) {
    const message = error?.error?.message || error?.message || t('manualEditor.error.saveFailed')
    MessagePlugin.error(message)
  } finally {
    saving.value = false
  }
}

const handleClose = () => {
  uiStore.closeManualEditor()
}

const toggleEditorView = () => {
  activeTab.value = isPreviewMode.value ? 'edit' : 'preview'
}

watch(visible, async (val) => {
  if (val) {
    await nextTick()
    await initialize()
    await nextTick()
    markdownCoreRef.value?.attachTextareaListeners()
    markdownCoreRef.value?.focusEnd()
  } else {
    resetForm()
  }
})

watch(activeTab, (val) => {
  if (val === 'edit') {
    nextTick(() => {
      markdownCoreRef.value?.attachTextareaListeners()
    })
  }
})
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    :header="dialogTitle"
    :closeBtn="true"
    :footer="false"
    width="880px"
    top="5%"
    class="manual-knowledge-editor"
    destroy-on-close
  >
    <div class="editor-body" v-if="initialLoaded">
      <div class="form-row form-row--title-row" v-if="canOpenInWindow">
        <t-link theme="primary" hover="color" @click="openInNotesWindow">
          {{ $t('manualEditor.openInWindow') }}
        </t-link>
      </div>

      <div class="form-row">
        <label class="form-label">{{ $t('manualEditor.form.knowledgeBaseLabel') }}</label>
        <t-select
          v-model="form.kbId"
          :disabled="kbDisabled"
          :loading="kbLoading"
          :options="kbOptions"
          :placeholder="$t('manualEditor.form.knowledgeBasePlaceholder')"
          :popup-props="{ attach: 'body' }"
        >
          <template #empty>
            <div style="padding: 20px; text-align: center; color: var(--td-text-color-placeholder);">
              {{ $t('manualEditor.noDocumentKnowledgeBases') }}
            </div>
          </template>
        </t-select>
      </div>

      <div class="form-row">
        <label class="form-label">{{ $t('manualEditor.form.titleLabel') }}</label>
        <t-input
          v-model="form.title"
          maxlength="100"
          :placeholder="$t('manualEditor.form.titlePlaceholder')"
          showLimitNumber
        />
      </div>

      <div class="status-row" v-if="mode === 'edit'">
        <t-tag theme="warning" v-if="form.status === 'draft'">{{ $t('manualEditor.status.draftTag') }}</t-tag>
        <t-tag theme="success" v-else>{{ $t('manualEditor.status.publishedTag') }}</t-tag>
        <span v-if="lastUpdatedText" class="status-timestamp">{{ lastUpdatedText }}</span>
      </div>

      <MarkdownEditorCore
        ref="markdownCoreRef"
        v-model="form.content"
        v-model:view-mode="coreViewMode"
        :disabled="saving"
        :content-loading="contentLoading"
        :autosize="{ minRows: 16, maxRows: 24 }"
      />

      <div class="dialog-footer">
        <div class="footer-left">
          <t-button variant="outline" theme="default" @click="handleClose">
            {{ $t('manualEditor.actions.cancel') }}
          </t-button>
        </div>
        <div class="footer-right">
          <t-tooltip :content="viewToggleTooltip" placement="top">
            <t-button
              variant="outline"
              theme="default"
              class="toggle-view-btn"
              :class="{ active: isPreviewMode }"
              @click="toggleEditorView"
            >
              <t-icon :name="viewToggleIcon" size="16px" />
              <span>{{ viewToggleLabel }}</span>
            </t-button>
          </t-tooltip>
          <t-button
            variant="outline"
            theme="default"
            @click="handleSave('draft')"
            :loading="saving && savingAction === 'draft'"
          >
            {{ $t('manualEditor.actions.saveDraft') }}
          </t-button>
          <t-button
            theme="primary"
            @click="handleSave('publish')"
            :loading="saving && savingAction === 'publish'"
          >
            {{ $t('manualEditor.actions.publish') }}
          </t-button>
        </div>
      </div>
    </div>
    <div v-else class="loading-wrapper">
      <t-loading size="medium" :text="$t('manualEditor.loading.preparing')" />
    </div>
  </t-dialog>
</template>

<style scoped lang="less">
.manual-knowledge-editor {
  :deep(.t-dialog__body) {
    padding: 20px 24px 12px;
    max-height: 80vh;
    overflow-y: auto;
  }
}

.editor-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-row--title-row {
  flex-direction: row;
  justify-content: flex-end;
  margin-bottom: -8px;
}

.form-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
}

.status-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.status-timestamp {
  font-size: 12px;
  color: var(--td-text-color-disabled);
}

.dialog-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
}

.footer-right {
  display: flex;
  gap: 16px;
}

.loading-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 240px;
}

:deep(.toggle-view-btn) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 16px;
  height: 32px;
  line-height: 32px;
  transition: all 0.18s ease;
}

:deep(.toggle-view-btn .t-button__content) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

:deep(.toggle-view-btn .t-button__text) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

:deep(.toggle-view-btn.active),
:deep(.toggle-view-btn:hover) {
  background: rgba(7, 192, 95, 0.12) !important;
  color: var(--td-brand-color-active) !important;
  border-color: rgba(7, 192, 95, 0.4) !important;
}
</style>

<style lang="less">
.t-popup {
  z-index: 2600 !important;
}
</style>
