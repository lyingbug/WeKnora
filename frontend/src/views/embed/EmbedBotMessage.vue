<template>
  <div class="embed-bot-msg" :class="{ 'is-embedded': embeddedMode }">
    <AgentStreamDisplay
      v-if="session?.isAgentMode"
      :session="session"
      :session-id="sessionId"
      :user-query="userQuery"
      :embedded-mode="embeddedMode"
      :embed-channel-id="embedChannelId"
      :embed-token="embedToken"
    />
    <div v-else-if="!session?.hideContent" ref="parentMd">
      <div v-if="hasActualContent" class="content-wrapper">
        <div class="ai-markdown-template markdown-content" v-html="renderedHTML" />
      </div>
      <div v-if="hasActualContent && !session?.is_completed" class="loading-indicator">
        <div class="loading-typing">
          <span></span>
          <span></span>
          <span></span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onMounted, onUpdated, ref, watch } from 'vue'
import { marked } from 'marked'
import markedKatex from 'marked-katex-extension'
import 'katex/dist/katex.min.css'
import {
  sanitizeHTML,
  safeMarkdownToHTML,
  createSafeImage,
  isValidImageURL,
  hydrateProtectedFileImages,
} from '@/utils/security'
import { replaceIncompleteImageWithPlaceholder } from '@/utils/chatMessageShared'
import {
  createMermaidCodeRenderer,
  ensureMermaidInitialized,
  renderMermaidInContainer,
} from '@/utils/mermaidShared'

const AgentStreamDisplay = defineAsyncComponent(
  () => import('@/views/chat/components/AgentStreamDisplay.vue'),
)

marked.use({ breaks: true })
marked.use(markedKatex({ throwOnError: false, nonStandard: true }))
ensureMermaidInitialized()

const preprocessMathDelimiters = (rawText: string): string => {
  if (!rawText || typeof rawText !== 'string') return ''
  return rawText
    .replace(/\\\[([\s\S]*?)\\\]/g, '$$$$$1$$$$')
    .replace(/\\\(([\s\S]*?)\\\)/g, '$$$1$$')
}

const customRenderer = new marked.Renderer()
customRenderer.image = function ({ href, title, text }) {
  if (!isValidImageURL(href)) return ''
  return createSafeImage(href, text || '', title || '')
}
customRenderer.code = createMermaidCodeRenderer('mermaid-embed-botmsg')

const props = withDefaults(
  defineProps<{
    content?: string
    session?: Record<string, unknown>
    sessionId?: string
    userQuery?: string
    embeddedMode?: boolean
    embedChannelId?: string
    embedToken?: string
  }>(),
  {
    content: '',
    session: () => ({}),
    sessionId: '',
    userQuery: '',
    embeddedMode: true,
    embedChannelId: '',
    embedToken: '',
  },
)

const parentMd = ref<HTMLElement | null>(null)

const renderedHTML = computed(() => {
  const text = String(props.content || props.session?.content || '')
  if (!text.trim()) return ''
  const processed = replaceIncompleteImageWithPlaceholder(text)
  const safeText = preprocessMathDelimiters(processed)
  const safeMarkdown = safeMarkdownToHTML(safeText)
  const html = marked.parse(safeMarkdown, { renderer: customRenderer, breaks: true })
  return sanitizeHTML(html as string)
})

const hasActualContent = computed(() => {
  const text = String(props.content || props.session?.content || '')
  return text.trim().length > 0
})

const hydrateImages = async () => {
  const embedCtx =
    props.embedChannelId && props.embedToken
      ? { channelId: props.embedChannelId, token: props.embedToken }
      : undefined
  await hydrateProtectedFileImages(parentMd.value, embedCtx)
}

const renderMermaidDiagrams = async () => {
  await renderMermaidInContainer(parentMd.value)
}

watch(renderedHTML, () => {
  nextTick(async () => {
    await hydrateImages()
    if (props.session?.is_completed) {
      await renderMermaidDiagrams()
    }
  })
})

onUpdated(() => {
  nextTick(async () => {
    await hydrateImages()
    if (props.session?.is_completed) {
      await renderMermaidDiagrams()
    }
  })
})

onMounted(() => {
  nextTick(async () => {
    await hydrateImages()
    await renderMermaidDiagrams()
  })
})
</script>

<style scoped lang="less">
@import '../../components/css/markdown.less';

.embed-bot-msg {
  border-radius: 4px;
  color: var(--td-text-color-primary);
  font-size: 16px;
  margin-right: auto;
  max-width: 100%;
  box-sizing: border-box;

  &.is-embedded {
    width: 100%;

    :deep(.agent-stream-display) {
      width: 100%;
    }
  }
}

.content-wrapper {
  background: var(--td-bg-color-container);
  border-radius: 6px;
  padding: 8px 0;
}

.ai-markdown-template {
  font-size: 15px;
  color: var(--td-text-color-primary);
  line-height: 1.6;
}

.markdown-content {
  :deep(p) {
    margin: 6px 0;
    line-height: 1.6;
  }

  :deep(code) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 2px 5px;
    border-radius: 3px;
    font-family: var(--app-font-family-mono);
    font-size: 11px;
  }

  :deep(pre) {
    background: var(--td-bg-color-secondarycontainer);
    padding: 10px;
    border-radius: 4px;
    overflow-x: auto;
    margin: 6px 0;

    code {
      background: none;
      padding: 0;
    }
  }

  :deep(ul),
  :deep(ol) {
    margin: 6px 0;
    padding-left: 20px;
  }

  :deep(li) {
    margin: 3px 0;
  }

  :deep(blockquote) {
    border-left: 2px solid var(--td-brand-color);
    padding-left: 10px;
    margin: 6px 0;
    color: var(--td-text-color-secondary);
  }

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4),
  :deep(h5),
  :deep(h6) {
    margin: 10px 0 6px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  :deep(a) {
    color: var(--td-brand-color);
    text-decoration: none;

    &:hover {
      text-decoration: underline;
    }
  }

  :deep(table) {
    border-collapse: collapse;
    margin: 6px 0;
    font-size: 11px;
    width: 100%;

    th,
    td {
      border: 1px solid var(--td-component-stroke);
      padding: 5px 8px;
      text-align: left;
    }

    th {
      background: var(--td-bg-color-secondarycontainer);
      font-weight: 600;
    }

    tbody tr:nth-child(even) {
      background: var(--td-bg-color-secondarycontainer);
    }
  }

  :deep(img) {
    max-width: 80%;
    max-height: 300px;
    width: auto;
    height: auto;
    border-radius: 8px;
    display: block;
    margin: 8px 0;
    border: 0.5px solid var(--td-component-stroke);
    object-fit: contain;
  }

  :deep(.mermaid) {
    margin: 16px 0;
    padding: 16px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 8px;
    overflow-x: auto;
    text-align: center;

    svg {
      max-width: 100%;
      height: auto;
    }
  }
}

.loading-indicator {
  padding: 8px 0;
}

.loading-typing {
  display: flex;
  align-items: center;
  gap: 4px;

  span {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--embed-primary, var(--td-brand-color));
    animation: typingBounce 1.4s ease-in-out infinite;

    &:nth-child(1) {
      animation-delay: 0s;
    }

    &:nth-child(2) {
      animation-delay: 0.2s;
    }

    &:nth-child(3) {
      animation-delay: 0.4s;
    }
  }
}

@keyframes typingBounce {
  0%,
  60%,
  100% {
    transform: translateY(0);
  }

  30% {
    transform: translateY(-8px);
  }
}
</style>
