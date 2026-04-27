import { defineStore } from 'pinia'

const NOTES_MODE_KEY = 'weknora_notes_mode_enabled'
const NOTES_CHAT_OPEN_KEY = 'weknora_notes_chat_open'
const NOTES_CHAT_SESSION_KEY = 'weknora_notes_chat_session'
const NOTES_WIKI_OPEN_KEY = 'weknora_notes_wiki_open'

export const useUIStore = defineStore('ui', {
  state: () => ({
    showSettingsModal: false,
    showKBEditorModal: false,
    kbEditorMode: 'create' as 'create' | 'edit',
    currentKBId: null as string | null,
    kbEditorType: 'document' as 'document' | 'faq' | 'notebook',
    kbEditorInitialName: '' as string,
    // 当前选中的分类ID，用于文件上传时传递
    selectedTagId: '__untagged__' as string,
    kbEditorInitialSection: null as string | null,
    settingsInitialSection: null as string | null,
    settingsInitialSubSection: null as string | null,
    manualEditorVisible: false,
    manualEditorMode: 'create' as 'create' | 'edit',
    manualEditorKBId: null as string | null,
    manualEditorKnowledgeId: null as string | null,
    manualEditorInitialTitle: '',
    manualEditorInitialContent: '',
    manualEditorInitialStatus: 'draft' as 'draft' | 'publish',
    manualEditorOnSuccess: null as null | ((payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => void),
    sidebarCollapsed: localStorage.getItem('sidebar_collapsed') === 'true',
    /** 设置中「笔记模式」：开启后侧栏精简、默认落地页为 /platform/notes */
    notesModeEnabled: typeof localStorage !== 'undefined' && localStorage.getItem(NOTES_MODE_KEY) === 'true',
    /** 笔记模式右侧对话面板是否展开 */
    notesChatPanelOpen: typeof localStorage !== 'undefined' && localStorage.getItem(NOTES_CHAT_OPEN_KEY) === 'true',
    /** 笔记模式对话面板复用的 session id */
    notesChatSessionId: typeof localStorage !== 'undefined' ? localStorage.getItem(NOTES_CHAT_SESSION_KEY) || '' : '',
    /** 当前编辑中的笔记所属知识库 ID（由 NotesEditor 维护，用于绑定 Chat KB 范围） */
    notesCurrentKbId: '' as string,
    /** 笔记模式 Wiki 面板是否展开 */
    notesWikiPanelOpen: typeof localStorage !== 'undefined' && localStorage.getItem(NOTES_WIKI_OPEN_KEY) === 'true',
  }),

  actions: {
    openSettings(section?: string, subSection?: string) {
      this.settingsInitialSection = section || null
      this.settingsInitialSubSection = subSection || null
      this.showSettingsModal = true
    },

    closeSettings() {
      this.showSettingsModal = false
      this.settingsInitialSection = null
      this.settingsInitialSubSection = null
    },

    toggleSettings() {
      this.showSettingsModal = !this.showSettingsModal
    },

    openKBSettings(kbId: string, initialSection?: string) {
      this.currentKBId = kbId
      this.kbEditorMode = 'edit'
       this.kbEditorType = 'document'
      this.kbEditorInitialSection = initialSection || null
      this.showKBEditorModal = true
    },

    openEditKB(kbId: string, initialSection?: string) {
      this.openKBSettings(kbId, initialSection)
    },

    openCreateKB(type: 'document' | 'faq' | 'notebook' = 'document', initialName?: string) {
      this.currentKBId = null
      this.kbEditorMode = 'create'
      this.kbEditorType = type
      this.kbEditorInitialName = initialName || ''
      this.kbEditorInitialSection = null
      this.showKBEditorModal = true
    },

    closeKBEditor() {
      this.showKBEditorModal = false
      this.currentKBId = null
      this.kbEditorInitialSection = null
      this.kbEditorType = 'document'
      this.kbEditorInitialName = ''
    },

    openManualEditor(options: {
      mode?: 'create' | 'edit'
      kbId?: string | null
      knowledgeId?: string | null
      title?: string
      content?: string
      status?: 'draft' | 'publish'
      onSuccess?: (payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) => void
    } = {}) {
      this.manualEditorMode = options.mode || 'create'
      this.manualEditorKBId = options.kbId ?? null
      this.manualEditorKnowledgeId = options.knowledgeId ?? null
      this.manualEditorInitialTitle = options.title || ''
      this.manualEditorInitialContent = options.content || ''
      this.manualEditorInitialStatus = options.status || 'draft'
      this.manualEditorOnSuccess = options.onSuccess || null
      this.manualEditorVisible = true
    },

    closeManualEditor() {
      this.manualEditorVisible = false
      this.manualEditorKnowledgeId = null
      this.manualEditorInitialContent = ''
      this.manualEditorInitialTitle = ''
      this.manualEditorInitialStatus = 'draft'
      this.manualEditorOnSuccess = null
    },

    notifyManualEditorSuccess(payload: { kbId: string; knowledgeId: string; status: 'draft' | 'publish' }) {
      if (typeof this.manualEditorOnSuccess === 'function') {
        try {
          this.manualEditorOnSuccess(payload)
        } catch (err) {
          console.error('Manual editor success callback error:', err)
        }
      }
      this.manualEditorOnSuccess = null
    },

    // 设置当前选中的分类ID
    setSelectedTagId(tagId: string) {
      this.selectedTagId = tagId
    },

    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed
      localStorage.setItem('sidebar_collapsed', String(this.sidebarCollapsed))
    },

    collapseSidebar() {
      this.sidebarCollapsed = true
      localStorage.setItem('sidebar_collapsed', 'true')
    },

    expandSidebar() {
      this.sidebarCollapsed = false
      localStorage.setItem('sidebar_collapsed', 'false')
    },

    setNotesChatPanel(open: boolean) {
      this.notesChatPanelOpen = open
      if (typeof localStorage !== 'undefined') {
        if (open) localStorage.setItem(NOTES_CHAT_OPEN_KEY, 'true')
        else localStorage.removeItem(NOTES_CHAT_OPEN_KEY)
      }
    },

    setNotesChatSessionId(id: string) {
      this.notesChatSessionId = id || ''
      if (typeof localStorage !== 'undefined') {
        if (id) localStorage.setItem(NOTES_CHAT_SESSION_KEY, id)
        else localStorage.removeItem(NOTES_CHAT_SESSION_KEY)
      }
    },

    setNotesCurrentKbId(kbId: string) {
      this.notesCurrentKbId = kbId || ''
    },

    setNotesWikiPanel(open: boolean) {
      this.notesWikiPanelOpen = open
      if (typeof localStorage !== 'undefined') {
        if (open) localStorage.setItem(NOTES_WIKI_OPEN_KEY, 'true')
        else localStorage.removeItem(NOTES_WIKI_OPEN_KEY)
      }
    },

    setNotesMode(enabled: boolean) {
      this.notesModeEnabled = enabled
      if (typeof localStorage !== 'undefined') {
        if (enabled) {
          localStorage.setItem(NOTES_MODE_KEY, 'true')
        } else {
          localStorage.removeItem(NOTES_MODE_KEY)
        }
      }
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('weknora:notes-mode-changed', { detail: { enabled } }))
      }
    },
  }
})

