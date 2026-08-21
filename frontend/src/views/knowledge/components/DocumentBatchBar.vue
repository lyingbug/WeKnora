<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import FolderPickerMenu, { type FolderOption } from './FolderPickerMenu.vue';
import { canSelectAllFiltered as canSelectAllFilteredMatches } from '../batchReparseSelection';

const props = defineProps<{
  count: number;
  deleteLoading?: boolean;
  reparseLoading?: boolean;
  tagLoading?: boolean;
  // When true the bar stays visible even with 0 selections, so users can exit
  // batch mode from here without selecting anything first.
  visible?: boolean;
  /** Hidden when the knowledge base has no folder structure to file into. */
  showMoveToFolder?: boolean;
  folderOptions?: FolderOption[];
  /** True while any document filter (status, tag, keyword, ...) is applied. */
  filterActive?: boolean;
  /** How many documents match the active filter across all pages. */
  filteredTotal?: number;
  /** True once the user opted into the whole filtered result set. */
  allFilteredSelected?: boolean;
}>();

const emit = defineEmits<{
  (e: 'cancel'): void;
  (e: 'delete'): void;
  (e: 'reparse'): void;
  (e: 'batchTag'): void;
  (e: 'moveToFolder', folderPath: string): void;
  (e: 'selectAllFiltered'): void;
}>();

const { t } = useI18n();

const folderPickerVisible = ref(false);

const busy = computed(() => !!props.deleteLoading || !!props.reparseLoading || !!props.tagLoading);
// Rebuilding the whole filtered set targets every match, not just loaded rows.
const targetCount = computed(() =>
  props.allFilteredSelected ? props.filteredTotal || 0 : props.count,
);
const canSelectAllFiltered = computed(() =>
  canSelectAllFilteredMatches({
    filterActive: !!props.filterActive,
    allFilteredSelected: !!props.allFilteredSelected,
    filteredTotal: props.filteredTotal || 0,
    selectedCount: props.count,
  }),
);
// Selecting all matches is a rebuild-only affordance. Delete / tag / move keep
// operating on explicitly checked rows so a cross-page selection can never
// remove or relabel documents the user has not seen.
const otherActionsDisabled = computed(
  () => props.count === 0 || busy.value || !!props.allFilteredSelected,
);
</script>

<template>
  <transition name="batch-bar-fade">
    <div v-if="visible || count > 0" class="doc-batch-bar" role="region"
      :aria-label="t('knowledgeBase.selectedCount', { count })">
      <div class="batch-bar-inner">
        <div class="batch-bar-left">
          <span class="batch-bar-count">
            {{ allFilteredSelected
              ? t('knowledgeBase.allFilteredSelected', { count: filteredTotal || 0 })
              : t('knowledgeBase.selectedCount', { count }) }}
          </span>
          <t-button v-if="canSelectAllFiltered" variant="text" theme="primary" size="small"
            class="batch-bar-select-all" @click="emit('selectAllFiltered')">
            {{ t('knowledgeBase.selectAllFiltered', { count: filteredTotal || 0 }) }}
          </t-button>
          <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="emit('cancel')">
            {{ t('knowledgeBase.clearSelection') }}
          </t-button>
        </div>
        <div class="batch-bar-actions">
          <t-popconfirm theme="warning"
            :content="t('knowledgeBase.confirmBatchReparseDocument', { count: targetCount })"
            :confirm-btn="{ content: t('knowledgeBase.confirmBatchReparse'), theme: 'warning' }"
            :cancel-btn="{ content: t('common.cancel') }" placement="top" @confirm="emit('reparse')">
            <t-button theme="default" variant="outline" size="small"
              :disabled="targetCount === 0 || busy" :loading="reparseLoading" @click.stop>
              <template #icon><t-icon name="refresh" size="14px" /></template>
              {{ t('knowledgeBase.rebuildDocument') }}
            </t-button>
          </t-popconfirm>

          <t-button theme="default" variant="outline" size="small"
            :disabled="otherActionsDisabled" :loading="tagLoading"
            @click="emit('batchTag')">
            <template #icon><t-icon name="discount" size="14px" /></template>
            {{ t('knowledgeBase.batchTag') }}
          </t-button>

          <t-popup v-if="showMoveToFolder" v-model:visible="folderPickerVisible" trigger="click"
            placement="top" overlay-class-name="card-more" destroy-on-close>
            <t-button theme="default" variant="outline" size="small" :disabled="otherActionsDisabled">
              <template #icon><t-icon name="folder" size="14px" /></template>
              {{ t('knowledgeBase.moveToFolder.action') }}
            </t-button>
            <template #content>
              <div class="card-menu">
                <FolderPickerMenu :options="folderOptions || []"
                  @confirm="(path: string) => { folderPickerVisible = false; emit('moveToFolder', path) }" />
              </div>
            </template>
          </t-popup>

          <t-popconfirm theme="warning" :content="t('knowledgeBase.confirmBatchDeleteDocument', { count })"
            :confirm-btn="{ content: t('knowledgeBase.confirmDelete'), theme: 'danger' }"
            :cancel-btn="{ content: t('common.cancel') }" placement="top" @confirm="emit('delete')">
            <t-button theme="danger" variant="outline" size="small"
              :disabled="otherActionsDisabled" :loading="deleteLoading" @click.stop>
              <template #icon><t-icon name="delete" size="14px" /></template>
              {{ t('knowledgeBase.batchDelete') }}
            </t-button>
          </t-popconfirm>
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped lang="less">
.doc-batch-bar {
  position: relative;
  z-index: 5;
  width: 100%;
  // Wide enough for the extra "select all matches" affordance without pushing
  // the clear-selection button out of view.
  max-width: 680px;
  margin: 0 auto;
  padding: 0 4px;
  box-sizing: border-box;
}

.batch-bar-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
  padding: 8px 12px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.08);
}

.batch-bar-left {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  flex: 1;
}

.batch-bar-count {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
}

.batch-bar-select-all {
  flex-shrink: 0;
  padding: 0 6px !important;
  height: 28px !important;
  font-size: 12px;
}

.batch-bar-clear {
  flex-shrink: 0;
  padding: 0 6px !important;
  height: 28px !important;
  font-size: 12px;
  color: var(--td-text-color-secondary) !important;

  &:hover {
    color: var(--td-brand-color) !important;
  }
}

.batch-bar-actions {
  flex-shrink: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active {
  transition: transform 0.2s ease, opacity 0.2s ease;
}

.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to {
  opacity: 0;
  transform: translateY(6px);
}
</style>
