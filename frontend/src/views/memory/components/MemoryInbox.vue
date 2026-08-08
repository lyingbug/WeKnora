<template>
  <div class="memory-inbox">
    <p class="memory-inbox__intro">{{ t('memory.inbox.intro') }}</p>

    <t-loading :loading="loading" :show-overlay="false" class="memory-inbox__body">
      <div v-if="!notes.length && !loading" class="memory-inbox__empty">
        <t-empty :description="t('memory.inbox.empty')" />
      </div>

      <ul v-else class="memory-inbox__list" role="list">
        <li v-for="note in notes" :key="note.id" class="inbox-card">
          <div class="inbox-card__main">
            <div class="inbox-card__head">
              <span class="inbox-card__type">{{ t(`memory.types.${note.note_type}`) }}</span>
              <span class="inbox-card__confidence">
                {{ t('memory.inbox.confidence', { value: Math.round(note.confidence * 100) }) }}
              </span>
              <span class="inbox-card__date">{{ formatDate(note.created_at) }}</span>
            </div>

            <t-textarea v-model="drafts[note.id]" class="inbox-card__statement"
              :autosize="{ minRows: 2, maxRows: 6 }" />

            <div v-if="note.session_id" class="inbox-card__source">
              <t-link theme="primary" size="small" hover="color" @click="openSession(note.session_id)">
                {{ t('memory.inbox.viewSource') }}
              </t-link>
            </div>
          </div>

          <div class="inbox-card__actions">
            <t-button size="medium" theme="primary" @click="accept(note)">
              {{ t('memory.inbox.accept') }}
            </t-button>
            <t-button size="medium" variant="outline" @click="reject(note)">
              {{ t('memory.inbox.reject') }}
            </t-button>
          </div>
        </li>
      </ul>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { MessagePlugin } from 'tdesign-vue-next'

import { listMemoryNotes, promoteMemoryNote, rejectMemoryNote, type MemoryNote } from '@/api/memory'

const { t } = useI18n()
const router = useRouter()
const emit = defineEmits<{ (e: 'changed'): void }>()

const notes = ref<MemoryNote[]>([])
const drafts = reactive<Record<string, string>>({})
const loading = ref(false)

function formatDate(value: string): string {
  return value
    ? new Date(value).toLocaleString(undefined, {
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
    : '-'
}

async function load() {
  loading.value = true
  try {
    const res = await listMemoryNotes({ status: 'pending', page_size: 50 })
    notes.value = res?.data?.notes || []
    for (const note of notes.value) {
      drafts[note.id] = note.statement
    }
  } catch (error: any) {
    if (error?.response?.status !== 404) {
      MessagePlugin.error(t('memory.errors.loadFailed'))
    }
    notes.value = []
  } finally {
    loading.value = false
  }
}

async function accept(note: MemoryNote) {
  try {
    await promoteMemoryNote(note.id, { statement: drafts[note.id], note_type: note.note_type })
    MessagePlugin.success(t('memory.inbox.accepted'))
    notes.value = notes.value.filter((item) => item.id !== note.id)
    emit('changed')
  } catch {
    MessagePlugin.error(t('memory.errors.saveFailed'))
  }
}

async function reject(note: MemoryNote) {
  try {
    await rejectMemoryNote(note.id)
    notes.value = notes.value.filter((item) => item.id !== note.id)
    emit('changed')
  } catch {
    MessagePlugin.error(t('memory.errors.saveFailed'))
  }
}

function openSession(sessionId: string) {
  router.push(`/platform/chat/${sessionId}`)
}

onMounted(load)
</script>

<style scoped lang="less">
.memory-inbox__intro {
  margin: 0 0 20px;
  font-size: 14px;
  line-height: 1.55;
  color: var(--td-text-color-secondary);
  max-width: 56ch;
}

.memory-inbox__body {
  min-height: 160px;
}

.memory-inbox__empty {
  padding: 56px 16px;
}

.memory-inbox__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.inbox-card {
  display: flex;
  gap: 20px;
  align-items: flex-start;
  padding: 16px 18px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  transition: border-color 0.15s ease, box-shadow 0.15s ease;

  &:hover {
    border-color: var(--td-component-border);
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);
  }

  &__main {
    flex: 1;
    min-width: 0;
  }

  &__head {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px 12px;
    margin-bottom: 10px;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    font-variant-numeric: tabular-nums;
  }

  &__type {
    padding: 2px 8px;
    font-size: 12px;
    font-weight: 500;
    line-height: 1.35;
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 4px;
  }

  &__statement {
    margin-bottom: 8px;
  }

  &__source {
    font-size: 13px;
  }

  &__actions {
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex-shrink: 0;
    padding-top: 2px;
  }
}
</style>
