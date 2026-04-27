import { reactive, ref, computed, watch } from 'vue'
import { defineStore } from 'pinia'
import i18n from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'

type MenuChild = Record<string, any>

interface MenuItem {
  title: string
  titleKey?: string
  icon: string
  path: string
  childrenPath?: string
  children?: MenuChild[]
}

const createMenuChildren = () => reactive<MenuChild[]>([])

export const useMenuStore = defineStore('menuStore', () => {
  const menuArr = reactive<MenuItem[]>([
    { title: '', titleKey: 'menu.knowledgeBase', icon: 'zhishiku', path: 'knowledge-bases' },
    { title: '', titleKey: 'menu.knowledgeSearch', icon: 'search', path: 'knowledge-search' },
    { title: '', titleKey: 'menu.notes', icon: 'edit', path: 'notes' },
    { title: '', titleKey: 'menu.agents', icon: 'agent', path: 'agents' },
    { title: '', titleKey: 'menu.organizations', icon: 'organization', path: 'organizations' },
    {
      title: '',
      titleKey: 'menu.chat',
      icon: 'prefixIcon',
      path: 'creatChat',
      childrenPath: 'chat',
      children: createMenuChildren()
    },
    { title: '', titleKey: 'menu.settings', icon: 'setting', path: 'settings' },
    { title: '', titleKey: 'menu.logout', icon: 'logout', path: 'logout' }
  ])

  const isFirstSession = ref(false)
  const firstQuery = ref('')
  const firstMentionedItems = ref<any[]>([])
  const firstModelId = ref('')
  const firstImageFiles = ref<any[]>([])
  const firstAttachmentFiles = ref<any[]>([])
  const prefillQuery = ref('')

  const applyMenuTranslations = () => {
    menuArr.forEach(item => {
      if (item.titleKey) {
        item.title = i18n.global.t(item.titleKey)
      }
    })
  }

  applyMenuTranslations()

  watch(
    () => i18n.global.locale.value,
    () => {
      applyMenuTranslations()
    }
  )

  const liteHiddenPaths = new Set(['logout', 'organizations'])
  /** 笔记模式开启时侧栏仅保留这些 path */
  const notesModeKeepPaths = new Set(['notes', 'knowledge-search', 'creatChat', 'settings', 'logout'])

  const visibleMenuArr = computed(() => {
    const authStore = useAuthStore()
    const uiStore = useUIStore()
    let list: MenuItem[] = menuArr
    if (authStore.isLiteMode) {
      list = list.filter(item => !liteHiddenPaths.has(item.path))
    }
    // 笔记模式开启：侧栏精简为白名单项（含笔记）
    if (uiStore.notesModeEnabled) {
      list = list.filter(item => notesModeKeepPaths.has(item.path))
    } else {
      // 非笔记模式：左侧不展示「笔记」，改从头像菜单「进入笔记模式」进入
      list = list.filter(item => item.path !== 'notes')
    }
    return list
  })

  const chatMenuIndex = menuArr.findIndex(item => item.path === 'creatChat')

  const clearMenuArr = () => {
    const chatMenu = menuArr[chatMenuIndex]
    if (chatMenu && chatMenu.children) {
      chatMenu.children = createMenuChildren()
    }
  }

  const updatemenuArr = (obj: any) => {
    const chatMenu = menuArr[chatMenuIndex]
    if (!chatMenu.children) {
      chatMenu.children = createMenuChildren()
    }
    const exists = chatMenu.children.some((item: MenuChild) => item.id === obj.id)
    if (!exists) {
      chatMenu.children.push(obj)
    }
  }

  const updataMenuChildren = (item: MenuChild) => {
    const chatMenu = menuArr[chatMenuIndex]
    if (!chatMenu.children) {
      chatMenu.children = createMenuChildren()
    }
    chatMenu.children.unshift(item)
  }

  const updatasessionTitle = (sessionId: string, title: string) => {
    const chatMenu = menuArr[chatMenuIndex]
    chatMenu.children?.forEach((item: MenuChild) => {
      if (item.id === sessionId) {
        item.title = title
        item.isNoTitle = false
      }
    })
  }

  const changeIsFirstSession = (payload: boolean) => {
    isFirstSession.value = payload
  }

  const changeFirstQuery = (payload: string, mentionedItems: any[] = [], modelId: string = '', imageFiles: any[] = [], attachmentFiles: any[] = []) => {
    firstQuery.value = payload
    firstMentionedItems.value = mentionedItems
    firstModelId.value = modelId
    firstImageFiles.value = imageFiles
    firstAttachmentFiles.value = attachmentFiles
  }

  const setPrefillQuery = (q: string) => {
    prefillQuery.value = q
  }

  const consumePrefillQuery = () => {
    const q = prefillQuery.value
    prefillQuery.value = ''
    return q
  }

  return {
    menuArr,
    visibleMenuArr,
    isFirstSession,
    firstQuery,
    firstMentionedItems,
    firstModelId,
    firstImageFiles,
    firstAttachmentFiles,
    prefillQuery,
    clearMenuArr,
    updatemenuArr,
    updataMenuChildren,
    updatasessionTitle,
    changeIsFirstSession,
    changeFirstQuery,
    setPrefillQuery,
    consumePrefillQuery
  }
})
