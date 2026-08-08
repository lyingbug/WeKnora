<template>
  <div class="memory-inbox">
    <div class="memory-inbox__intro" role="note">
      <p class="memory-inbox__intro-text">{{ t('memory.inbox.intro') }}</p>
    </div>

    <t-loading :loading="loading" :show-overlay="false">
      <div v-if="!notes.length && !loading" class="memory-inbox__empty">
        {{ t('memory.inbox.empty') }}
      </div>

      <div v-else class="memory-inbox__items">
        <article v-for="note in notes" :key="note.id" class="inbox-item">
          <div class="inbox-item__body">
            <div class="inbox-item__head">
              <t-tag size="small" variant="light">{{ t(`memory.types.${note.note_type}`) }}</t-tag>
              <span class="inbox-item__confidence">
                {{ t('memory.inbox.confidence', { value: Math.round(note.confidence * 100) }) }}
              </span>
              <span class="inbox-item__date">{{ formatDate(note.created_at) }}</span>
            </div>

            <!-- Editable in place: reviewing a proposed memory almost always
                 means adjusting the wording, and forcing that through a second
                 dialog would make people reject instead of correct. -->
            <t-textarea
              v-model="drafts[note.id]"
              class="inbox-item__statement"
              :autosize="{ minRows: 2, maxRows: 5 }"
            />

            <div v-if="note.session_id" class="inbox-item__source">
              <t-link theme="primary" size="small" hover="color" @click="openSession(note.session_id)">
                {{ t('memory.inbox.viewSource') }}
              </t-link>
            </div>
          </div>

          <div class="inbox-item__actions">
            <t-button size="small" theme="primary" @click="accept(note)">
              {{ t('memory.inbox.accept') }}
            </t-button>
            <t-button size="small" variant="outline" @click="reject(note)">
              {{ t('memory.inbox.reject') }}
            </t-button>
          </div>
        </article>
      </div>
    </t-loading>
  </div>
</template>

<script setup lang="ts">
/**
 * The review inbox.
 *
 * Automatic extraction proposes; the user disposes. Nothing reaches the model
 * from here until a person has looked at it, which is what makes it reasonable
 * to have memory on by default.
 */
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
  return value ? new Date(value).toLocaleString() : '-'
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
.memory-inbox {
  &__intro {
    margin-bottom: 16px;
    padding: 10px 12px;
    background: var(--td-bg-color-secondarycontainer);
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
  }

  &__intro-text {
    margin: 0;
    font-size: 13px;
    line-height: 1.55;
    color: var(--td-text-color-secondary);
  }

  &__empty {
    padding: 48px 0;
    text-align: center;
    color: var(--td-text-color-placeholder);
    font-size: 13px;
  }

  &__items {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
}

.inbox-item {
  display: flex;
  gap: 14px;
  align-items: flex-start;
  padding: 14px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  transition: border-color 0.18s ease, box-shadow 0.18s ease;

  &:hover {
    border-color: var(--td-brand-color-3, var(--td-brand-color));
    box-shadow: 0 4px 14px rgba(15, 23, 42, 0.06);
  }

  &__body {
    flex: 1;
    min-width: 0;
  }

  &__head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 8px;
    font-size: 12px;
    color: var(--td-text-color-placeholder, #999);
  }

  &__statement {
    margin-bottom: 6px;
  }

  &__source {
    font-size: 12px;
  }

  &__actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex-shrink: 0;
  }
}
</style>
