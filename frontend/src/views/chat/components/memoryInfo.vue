<template>
    <!-- The memories that influenced this answer. Showing them is not a nicety:
         a memory system users cannot inspect is one they cannot trust or
         correct, and every previous attempt at this feature shipped without it. -->
    <div class="memory-info" v-if="memories.length">
        <div class="memory-header" @click="expanded = !expanded">
            <t-icon name="bookmark" class="memory-header-icon" />
            <span>{{ t('chat.memoryUsedCount', { count: memories.length }) }}</span>
            <div class="memory-toggle">
                <t-icon :name="expanded ? 'chevron-down' : 'chevron-right'" />
            </div>
        </div>
        <div class="memory-body" v-show="expanded">
            <div v-for="memory in memories" :key="memory.id" class="memory-row">
                <t-tag size="small" variant="light" :theme="kindTheme(memory.kind)">
                    {{ kindLabel(memory.kind) }}
                </t-tag>
                <span class="memory-text">{{ memory.content }}</span>
                <t-tooltip :content="t('chat.memoryForget')">
                    <t-button
                        size="small"
                        variant="text"
                        shape="square"
                        theme="danger"
                        :disabled="forgetting === memory.id"
                        @click.stop="forget(memory)"
                    >
                        <template #icon><t-icon name="delete" /></template>
                    </t-button>
                </t-tooltip>
            </div>
            <p class="memory-hint">{{ t('chat.memoryHint') }}</p>
        </div>
    </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { deleteMemoryItem, type MemoryKind } from '@/api/memory'

interface UsedMemory {
    id: string
    kind: MemoryKind
    content: string
}

const props = defineProps<{ session: Record<string, any> }>()

const { t } = useI18n()
const expanded = ref(false)
const forgetting = ref('')
const removed = ref<string[]>([])

const memories = computed<UsedMemory[]>(() => {
    const list = (props.session?.used_memories || []) as UsedMemory[]
    return list.filter((memory) => memory && memory.id && !removed.value.includes(memory.id))
})

const kindLabel = (kind: MemoryKind) => t(`memorySettings.kinds.${kind}`)

const kindTheme = (kind: MemoryKind) => {
    switch (kind) {
        case 'profile':
            return 'primary'
        case 'preference':
            return 'success'
        case 'task':
            return 'warning'
        default:
            return 'default'
    }
}

// Forgetting from the chat is the shortest path from "that memory is wrong" to
// it being gone, which is where users actually notice a bad memory.
const forget = async (memory: UsedMemory) => {
    forgetting.value = memory.id
    try {
        await deleteMemoryItem(memory.id)
        removed.value.push(memory.id)
        MessagePlugin.success(t('chat.memoryForgotten'))
    } catch (error: any) {
        MessagePlugin.error(error?.message || t('chat.memoryForgetFailed'))
    } finally {
        forgetting.value = ''
    }
}
</script>

<style lang="less" scoped>
.memory-info {
    border: 1px solid var(--td-component-stroke);
    border-radius: 8px;
    background: var(--td-bg-color-container);
    overflow: hidden;
}

.memory-header {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 12px;
    cursor: pointer;
    font-size: 13px;
    color: var(--td-text-color-secondary);
    user-select: none;

    &:hover {
        background: var(--td-bg-color-container-hover);
    }
}

.memory-header-icon {
    color: var(--td-brand-color);
}

.memory-toggle {
    display: flex;
    align-items: center;
    margin-left: 2px;
}

.memory-body {
    padding: 4px 12px 10px;
    border-top: 1px solid var(--td-component-stroke);
}

.memory-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 0;
}

.memory-text {
    flex: 1;
    min-width: 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--td-text-color-primary);
    word-break: break-word;
}

.memory-hint {
    margin: 6px 0 0;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
}
</style>
