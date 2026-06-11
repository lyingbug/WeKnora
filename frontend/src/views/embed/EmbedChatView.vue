<template>
  <EmbedChatCore
    :session-id="sessionId"
    :channel-id="channelId"
    :token="token"
    :agent-id="agentId"
    :kb-ids="kbIds"
    :welcome-message="welcomeMessage"
    :show-suggested-questions="showSuggestedQuestions"
    :use-session-header-title="useSessionHeaderTitle"
    :host-context="hostContext"
    @session-title="(title: string) => emit('session-title', title)"
  />
</template>

<script setup lang="ts">
import { defineAsyncComponent } from 'vue'

const EmbedChatCore = defineAsyncComponent(() => import('@/views/embed/EmbedChatCore.vue'))

defineProps<{
  sessionId: string
  channelId: string
  token: string
  agentId: string
  kbIds: string[]
  welcomeMessage?: string
  showSuggestedQuestions?: boolean
  useSessionHeaderTitle?: boolean
  hostContext?: Record<string, unknown>
}>()

const emit = defineEmits<{
  (e: 'session-title', title: string): void
}>()
</script>
