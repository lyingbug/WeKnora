<template>
  <div class="memory-list">
    <div class="memory-list__filters">
      <t-input
        v-model="query"
        class="memory-list__search"
        :placeholder="t('memory.list.searchPlaceholder')"
        clearable
      >
        <template #prefix-icon><t-icon name="search" /></template>
      </t-input>

      <t-select
        v-model="typeFilter"
        class="memory-list__select"
        :placeholder="t('memory.list.allTypes')"
        multiple
        clearable
        :min-collapsed-num="2"
      >
        <t-option v-for="type in MEMORY_TYPES" :key="type" :value="type" :label="typeLabel(type)" />
      </t-select>

      <t-radio-group v-model="statusFilter" variant="default-filled">
        <t-radio-button value="active">{{ t('memory.list.statusActive') }}</t-radio-button>
        <t-radio-button value="archived">{{ t('memory.list.statusArchived') }}</t-radio-button>
      </t-radio-group>
    </div>

    <t-loading :loading="loading" :show-overlay="false">
      <div v-if="!pages.length && !loading" class="memory-list__empty">
        {{ t('memory.list.empty') }}
      </div>

      <div v-else class="memory-list__items">
        <article
          v-for="page in pages"
          :key="page.id"
          class="memory-item"
          :class="{ 'is-archived': page.status === 'archived' }"
        >
          <div class="memory-item__main" @click="$emit('edit', page.slug)">
            <div class="memory-item__head">
              <t-tag size="small" variant="light" :theme="typeTheme(page.page_type)">
                {{ typeLabel(page.page_type) }}
              </t-tag>
              <h3 class="memory-item__title">{{ page.title }}</h3>
              <t-icon v-if="page.pinned" name="pin-filled" class="memory-item__pin" />
            </div>
            <p class="memory-item__summary">{{ page.summary || firstLine(page.content) }}</p>
            <div class="memory-item__meta">
              <span>{{ t('memory.list.updated', { date: formatDate(page.updated_at) }) }}</span>
              <span v-if="page.hit_count > 0">{{ t('memory.list.used', { count: page.hit_count }) }}</span>
              <!-- Strength is how close a memory is to being archived, which is
                   the one internal number a user genuinely benefits from seeing. -->
              <span class="memory-item__strength" :title="t('memory.list.strengthHint')">
                {{ t('memory.list.strength', { value: Math.round(page.strength * 100) }) }}
              </span>
            </div>
          </div>

          <div class="memory-item__actions">
            <t-tooltip :content="page.pinned ? t('memory.list.unpin') : t('memory.list.pin')">
              <t-button size="small" variant="text" shape="square" @click="togglePin(page)">
                <t-icon :name="page.pinned ? 'pin-filled' : 'pin'" />
              </t-button>
            </t-tooltip>
            <t-tooltip :content="t('memory.list.edit')">
              <t-button size="small" variant="text" shape="square" @click="$emit('edit', page.slug)">
                <t-icon name="edit" />
              </t-button>
            </t-tooltip>
            <t-tooltip :content="t('memory.list.forget')">
              <t-button size="small" variant="text" shape="square" theme="danger" @click="confirmDelete(page)">
                <t-icon name="delete" />
              </t-button>
            </t-tooltip>
          </div>
        </article>
      </div>
    </t-loading>

    <t-pagination
      v-if="total > pageSize"
      v-model="currentPage"
      class="memory-list__pagination"
      :total="total"
      :page-size="pageSize"
      :show-jumper="false"
      :show-page-size="false"
      @current-change="load"
    />
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

// Colour carries meaning here rather than decoration: the memories that shape
// answers most (who you are, how you want to be answered) read as neutral and
// stable, while the ones that need attention (open questions) stand out.
function typeTheme(type: string): string {
  switch (type) {
    case 'preference':
    case 'profile':
      return 'primary'
    case 'open_question':
      return 'warning'
    case 'project':
      return 'success'
    default:
      return 'default'
  }
}

function firstLine(content: string): string {
  return (content || '').split('\n').find((line) => line.trim()) || ''
}

function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
}

async function load() {
  loading.value = true
  try {
    const res: any = await listMemoryPages({
      query: query.value || undefined,
      type: typeFilter.value.length ? typeFilter.value.join(',') : undefined,
      status: statusFilter.value,
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
    await updateMemoryPage(page.slug, {
      title: page.title,
      page_type: page.page_type,
      content: page.content,
      summary: page.summary,
      pinned: !page.pinned,
      version: page.version,
    })
    page.pinned = !page.pinned
    MessagePlugin.success(page.pinned ? t('memory.list.pinned') : t('memory.list.unpinned'))
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

// Watching the filters is both simpler and safer than per-control @change
// handlers: TDesign fires those around the v-model write, so a handler could
// read the previous value and filter by it.
watch([query, typeFilter, statusFilter], reload)

onMounted(load)
</script>

<style scoped lang="less">
.memory-list {

  &__filters {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 16px;
  }

  &__search {
    max-width: 280px;
  }

  &__select {
    min-width: 200px;
    max-width: 280px;
  }

  &__empty {
    padding: 48px 0;
    text-align: center;
    color: var(--td-text-color-placeholder, #bbb);
    font-size: 13px;
  }

  &__items {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  &__pagination {
    margin-top: 16px;
  }
}

.memory-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;

  &:hover {
    border-color: var(--td-brand-color-3, var(--td-brand-color));
    box-shadow: 0 4px 14px rgba(15, 23, 42, 0.06);
  }

  &.is-archived {
    opacity: 0.6;
  }

  &__main {
    flex: 1;
    min-width: 0;
    cursor: pointer;
  }

  &__head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  &__title {
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    color: var(--td-text-color-primary, #000);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__pin {
    color: var(--td-warning-color, #e37318);
  }

  &__summary {
    margin: 0 0 8px;
    font-size: 13px;
    line-height: 1.6;
    color: var(--td-text-color-secondary, #666);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  &__meta {
    display: flex;
    gap: 14px;
    flex-wrap: wrap;
    font-size: 12px;
    color: var(--td-text-color-placeholder, #999);
  }

  &__strength {
    cursor: help;
  }

  &__actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }
}
</style>
