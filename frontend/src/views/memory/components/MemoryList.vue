<template>
  <div class="memory-list">
    <div class="memory-list__toolbar">
      <t-input v-model="query" class="memory-list__search" :placeholder="t('memory.list.searchPlaceholder')" clearable
        size="medium">
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>

      <t-select v-model="typeFilter" class="memory-list__type" :placeholder="t('memory.list.allTypes')" multiple
        clearable size="medium" :min-collapsed-num="2">
        <t-option v-for="type in MEMORY_TYPES" :key="type" :value="type" :label="typeLabel(type)" />
      </t-select>

      <t-radio-group v-model="statusFilter" variant="default-filled" size="medium" class="memory-list__status">
        <t-radio-button value="active">{{ t('memory.list.statusActive') }}</t-radio-button>
        <t-radio-button value="archived">{{ t('memory.list.statusArchived') }}</t-radio-button>
      </t-radio-group>
    </div>

    <t-loading :loading="loading" size="small" class="memory-list__body">
      <div v-if="!pages.length && !loading" class="memory-list__empty">
        <t-empty :description="t(props.saved === false ? 'memory.list.historyEmpty' : 'memory.list.empty')" />
      </div>

      <ul v-else class="memory-list__rows" role="list">
        <li v-for="page in pages" :key="page.id" class="memory-item"
          :class="{ 'memory-item--archived': page.status === 'archived' }" role="button" tabindex="0"
          @click="emit('edit', page.slug)" @keydown.enter="emit('edit', page.slug)">
          <div class="memory-item__content">
            <div class="memory-item__head">
              <span class="memory-item__type">{{ typeLabel(page.page_type) }}</span>
              <span class="memory-item__origin">{{ t(page.saved ? 'memory.list.savedOrigin' : 'memory.list.historyOrigin') }}</span>
              <h3 class="memory-item__title">{{ page.title }}</h3>
              <t-icon v-if="page.pinned" name="pin-filled" class="memory-item__pin" />
            </div>

            <p v-if="displaySummary(page)" class="memory-item__summary">{{ displaySummary(page) }}</p>

            <div class="memory-item__meta">
              <span>{{ t('memory.list.updated', { date: formatDate(page.updated_at) }) }}</span>
              <span v-if="page.hit_count > 0" class="memory-item__meta-dot">
                {{ t('memory.list.used', { count: page.hit_count }) }}
              </span>
            </div>
          </div>

          <div class="memory-item__actions" @click.stop>
            <t-dropdown :options="menuOptions(page)" placement="bottom-right" attach="body" trigger="click"
              @click="(data: any) => runAction(page, data.value)">
              <t-button variant="text" shape="square" size="small" class="memory-item__more">
                <t-icon name="ellipsis" />
              </t-button>
            </t-dropdown>
          </div>
        </li>
      </ul>
    </t-loading>

    <t-pagination v-if="total > pageSize" v-model="currentPage" class="memory-list__pagination" :total="total"
      :page-size="pageSize" :show-jumper="false" :show-page-size="false" @current-change="load" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'

import {
  MEMORY_TYPES,
  deleteMemoryPage,
  listMemoryPages,
  updateMemoryPage,
  type MemoryPage,
} from '@/api/memory'

const props = defineProps<{ saved?: boolean }>()
const { t } = useI18n()
const emit = defineEmits<{ (e: 'edit', slug: string): void; (e: 'changed'): void }>()

const pages = ref<MemoryPage[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = 20
const loading = ref(false)
const query = ref('')
const typeFilter = ref<string[]>([])
const statusFilter = ref<'active' | 'archived'>('active')

function typeLabel(type: string): string {
  return t(`memory.types.${type}`)
}

function menuOptions(page: MemoryPage) {
  return [
    { content: t('memory.list.edit'), value: 'edit' },
    {
      content: page.pinned ? t('memory.list.unpin') : t('memory.list.pin'),
      value: 'pin',
    },
    { content: t('memory.list.forget'), value: 'delete', theme: 'error' },
  ]
}

function runAction(page: MemoryPage, action: string) {
  if (action === 'edit') emit('edit', page.slug)
  else if (action === 'pin') togglePin(page)
  else if (action === 'delete') confirmDelete(page)
}

function firstLine(content: string): string {
  return (content || '').split('\n').find((line) => line.trim()) || ''
}

function displaySummary(page: MemoryPage): string {
  const title = (page.title || '').trim()
  const summary = (page.summary || firstLine(page.content)).trim()
  if (!summary || summary === title) return ''
  return summary
}

function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString(undefined, {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

async function load() {
  loading.value = true
  try {
    const res = await listMemoryPages({
      query: query.value || undefined,
      type: typeFilter.value.length ? typeFilter.value.join(',') : undefined,
      status: statusFilter.value,
      saved: props.saved,
      page: currentPage.value,
      page_size: pageSize,
    })
    pages.value = res?.data?.pages || []
    total.value = res?.data?.total || 0
  } catch (error: any) {
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
    pages.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function reload() {
  currentPage.value = 1
  load()
}

async function togglePin(page: MemoryPage) {
  try {
    const res = await updateMemoryPage(page.slug, {
      title: page.title,
      page_type: page.page_type,
      status: page.status,
      content: page.content,
      summary: page.summary,
      pinned: !page.pinned,
      version: page.version,
    })
    const updated = res?.data
    page.pinned = updated?.pinned ?? !page.pinned
    page.version = updated?.version ?? page.version + 1
    MessagePlugin.success(page.pinned ? t('memory.list.pinned') : t('memory.list.unpinned'))
    if (updated?.saved !== undefined && updated.saved !== props.saved) {
      await load()
    }
    emit('changed')
  } catch {
    MessagePlugin.error(t('memory.errors.saveFailed'))
  }
}

function confirmDelete(page: MemoryPage) {
  const dialog = DialogPlugin.confirm({
    header: t('memory.list.forgetHeader'),
    body: t('memory.list.forgetBody', { title: page.title }),
    confirmBtn: { content: t('memory.list.forget'), theme: 'danger' },
    onConfirm: async () => {
      try {
        await deleteMemoryPage(page.slug)
        MessagePlugin.success(t('memory.list.forgotten'))
        load()
        emit('changed')
      } catch {
        MessagePlugin.error(t('memory.errors.forgetFailed'))
      } finally {
        dialog.destroy()
      }
    },
  })
}

let searchTimer: number | null = null
watch([typeFilter, statusFilter, () => props.saved], reload)
watch(query, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(reload, 300)
})

onMounted(load)
</script>

<style scoped lang="less">
.memory-list {
  min-height: 0;
}

.memory-list__toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 20px;
}

.memory-list__search {
  flex: 1;
  min-width: 200px;
  max-width: 360px;
}

.memory-list__type {
  width: 168px;
  flex-shrink: 0;
}

.memory-list__status {
  flex-shrink: 0;
}

.memory-list__body {
  min-height: 200px;
}

.memory-list__empty {
  padding: 56px 16px;
}

.memory-list__rows {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--td-component-stroke);
}

.memory-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 4px 20px;
  align-items: start;
  padding: 18px 2px;
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background 0.15s ease;

  &:hover {
    background: var(--td-bg-color-container-hover);

    .memory-item__more {
      opacity: 1;
    }
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: -2px;
  }

  &--archived {
    opacity: 0.76;
  }

  &__content {
    min-width: 0;
  }

  &__head {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }

  &__type {
    flex-shrink: 0;
    padding: 2px 8px;
    font-size: 12px;
    font-weight: 500;
    line-height: 1.35;
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 4px;
  }

  &__title {
    margin: 0;
    min-width: 0;
    font-size: 15px;
    font-weight: 500;
    line-height: 1.4;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__pin {
    flex-shrink: 0;
    color: var(--td-warning-color);
    font-size: 14px;
  }

  &__summary {
    margin: 6px 0 0;
    font-size: 13px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  &__meta {
    margin-top: 8px;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px 10px;
    font-size: 12px;
    line-height: 1.4;
    color: var(--td-text-color-placeholder);
    font-variant-numeric: tabular-nums;
  }

  &__meta-dot::before {
    content: '·';
    margin-right: 10px;
    color: var(--td-text-color-disabled);
  }

  &__actions {
    padding-top: 2px;
  }

  &__more {
    opacity: 0;
    color: var(--td-text-color-secondary);
    transition: opacity 0.15s ease;
  }
}

.memory-item__origin {
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  background: var(--td-bg-color-secondarycontainer);
}

.memory-list__pagination {
  margin-top: 16px;
  padding-top: 4px;
}
</style>
