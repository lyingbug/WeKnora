import { getCurrentInstance, ref, onUnmounted, readonly } from 'vue'

/**
 * 响应式断点检测 composable
 *
 * 断点定义：
 *  - isMobile:  < 768px  (安卓手机)
 *  - isTablet:  768-1023px (平板)
 *  - isDesktop: ≥ 1024px (PC)
 *
 * 使用方式：
 *   const { isMobile, isTablet, isDesktop } = useBreakpoint()
 *
 * 组件卸载时自动移除监听器。
 */

const MOBILE_MAX = 767
const TABLET_MAX = 1023

const MOBILE_QUERY = `(max-width: ${MOBILE_MAX}px)`
const TABLET_QUERY = `(min-width: ${MOBILE_MAX + 1}px) and (max-width: ${TABLET_MAX}px)`

// 全局共享状态：全应用保持唯一 matchMedia 监听器
// 避免每个组件都创建自己的 MediaQueryList
let listeners = 0
let mobileMQL: MediaQueryList | null = null
let tabletMQL: MediaQueryList | null = null

const isMobile = ref(false)
const isTablet = ref(false)
const isDesktop = ref(true)

export interface BreakpointFlags {
  isMobile: boolean
  isTablet: boolean
  isDesktop: boolean
}

/** 由视口宽度推断断点；SSR / 无 matchMedia 环境下的回退路径。 */
export function resolveBreakpoint(width: number): BreakpointFlags {
  return {
    isMobile: width <= MOBILE_MAX,
    isTablet: width > MOBILE_MAX && width <= TABLET_MAX,
    isDesktop: width > TABLET_MAX,
  }
}

function evalBreakpoint() {
  // SSR / 测试环境安全回退
  if (typeof window === 'undefined') {
    isMobile.value = false
    isTablet.value = false
    isDesktop.value = true
    return
  }

  // 优先用 matchMedia：window.innerWidth 含经典滚动条宽度，媒体查询不含。
  // 在断点边界附近两者会差 0～15px，导致 JS 判定的形态与 CSS 实际渲染的形态不一致。
  let flags: BreakpointFlags
  if (typeof window.matchMedia === 'function') {
    const mobile = (mobileMQL ?? window.matchMedia(MOBILE_QUERY)).matches
    const tablet = (tabletMQL ?? window.matchMedia(TABLET_QUERY)).matches
    flags = { isMobile: mobile, isTablet: tablet, isDesktop: !mobile && !tablet }
  } else {
    flags = resolveBreakpoint(window.innerWidth)
  }

  isMobile.value = flags.isMobile
  isTablet.value = flags.isTablet
  isDesktop.value = flags.isDesktop
}

// 模块加载即求值：platform/index.vue 与 stores 在挂载前就会读这些 ref，
// 若等到 App.vue 的 onMounted 才首次求值，手机上第一帧会先渲染桌面侧栏再切换。
evalBreakpoint()

function onMobileChange(e: MediaQueryListEvent) {
  evalBreakpoint()
}

function onTabletChange(e: MediaQueryListEvent) {
  evalBreakpoint()
}

function startListening() {
  if (typeof window === 'undefined' || !window.matchMedia) return

  if (listeners === 0) {
    mobileMQL = window.matchMedia(MOBILE_QUERY)
    tabletMQL = window.matchMedia(TABLET_QUERY)

    mobileMQL.addEventListener('change', onMobileChange)
    tabletMQL.addEventListener('change', onTabletChange)
  }
  listeners++

  // 立即评估一次
  evalBreakpoint()
}

function stopListening() {
  if (typeof window === 'undefined') return

  listeners--
  if (listeners <= 0) {
    listeners = 0
    mobileMQL?.removeEventListener('change', onMobileChange)
    tabletMQL?.removeEventListener('change', onTabletChange)
    mobileMQL = null
    tabletMQL = null
  }
}

export function useBreakpoint() {
  // 每个使用者都持有一个监听引用，卸载时成对释放。
  startListening()

  // 组件外调用（store、工具函数）没有卸载时机，注册 onUnmounted 只会告警并让计数永久泄漏。
  if (getCurrentInstance()) {
    onUnmounted(() => {
      stopListening()
    })
  }

  return {
    isMobile: readonly(isMobile),
    isTablet: readonly(isTablet),
    isDesktop: readonly(isDesktop),
    // 暴露手动刷新，供 App.vue 初始化 / resize 兜底
    refresh: evalBreakpoint,
    startListening,
  } as const
}

/**
 * 供 store 等非组件代码使用的直接访问接口。
 * 模块加载时已按当前视口求值，因此首帧读到的就是正确形态；
 * 之后由 App.vue 启动的全局 matchMedia 监听保持同步。
 */
export { isMobile, isTablet, isDesktop, evalBreakpoint, startListening, stopListening }

export { MOBILE_MAX, TABLET_MAX }
