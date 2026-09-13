import { reactive } from 'vue'

/*
 * 全局状态：字段名与旧页 viewer.js 的 state 一一对应，后续按块移植的
 * 渲染/交互函数都能按旧页同名引用。布局常量照抄旧页（源自 DSH 的
 * client-ui-layout）：视口 < 1024 侧栏自动折轨道，侧栏 264–420（默认
 * 280，折叠后 56 轨道），详情栏 300–520（默认 400），中栏最小 640。
 */
export const POLL_MS = 2000
export const LONG_TEXT_LINES = 20
export const SYSTEM_PREVIEW_LINES = 8
export const STORE_PREFIX = 'dsh.sessionview.'
export const SIDEBAR_AUTO_COLLAPSE = 1024
export const RAIL_W = 56
export const SIDEBAR_DEFAULT = 280
export const SIDEBAR_MIN = 264
export const SIDEBAR_MAX = 420
export const DETAILS_DEFAULT = 400
export const DETAILS_MIN = 300
export const DETAILS_MAX = 520
export const CENTER_MIN = 640
export const OVERFLOW_LIMIT = 8

/* ---------- localStorage 小助手（旧页 storeGet/storeSet/storeJSON 同名） ---------- */

export function storeGet(key: string): string | null {
  try {
    return window.localStorage.getItem(STORE_PREFIX + key)
  } catch {
    return null // file:// 下可能禁用 localStorage
  }
}

export function storeSet(key: string, value: string): void {
  try {
    window.localStorage.setItem(STORE_PREFIX + key, value)
  } catch {
    /* 同上：静默跳过 */
  }
}

export function storeJSON(key: string, fallback: Record<string, unknown>): Record<string, unknown> {
  const raw = storeGet(key)
  if (!raw) return fallback
  try {
    const v = JSON.parse(raw)
    return v && typeof v === 'object' ? v : fallback
  } catch {
    return fallback
  }
}

/* ---------- 文案短写（旧页同名函数） ---------- */

export function fmtSize(bytes: number | null | undefined): string {
  if (!bytes && bytes !== 0) return '—'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(2) + ' MB'
}

export function fmtClock(value: string | number | null | undefined): string {
  const d = new Date(value as string)
  if (isNaN(d.getTime())) return '—'
  const pad = (n: number) => (n < 10 ? '0' : '') + n
  return (
    d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' +
    pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds())
  )
}

export function relTime(value: string | number): string {
  const t = new Date(value).getTime()
  if (isNaN(t)) return '—'
  const secs = Math.round((Date.now() - t) / 1000)
  if (secs < 5) return '刚刚'
  if (secs < 60) return secs + ' 秒前'
  if (secs < 3600) return Math.floor(secs / 60) + ' 分钟前'
  if (secs < 86400) return Math.floor(secs / 3600) + ' 小时前'
  return Math.floor(secs / 86400) + ' 天前'
}

/* ---------- 全局 state（与旧页同名字段） ---------- */

// 类型先宽着走（any）；逐块移植到哪一块，哪一块再收紧类型。
/* eslint-disable @typescript-eslint/no-explicit-any */
export const state = reactive({
  root: '',
  generated: '',
  sessions: [] as any[],
  current: null as any,
  lines: [] as any[],
  nextFrom: 0,
  curSize: -1,
  curMtime: 0,
  filter: '',
  follow: true, // 旧页 live 模式默认开
  forceCollapse: false,
  onlyTools: false,
  msgSeq: 0,
  toolSeq: 0,
  calls: new Map<string, any>(),
  callNodes: {} as Record<string, any>,
  callEst: {} as Record<string, any>,
  imgAttr: null as any,
  anchors: {} as Record<string, any>,
  badLines: 0,
  pullError: '',
  polling: false,
  theme: 'light',
  view: 'chat',
  markdown: true,
  unit: 'token',
  sidebar: SIDEBAR_DEFAULT,
  details: 0,
  narrowExpanded: false,
  narrow: false,
  collapsed: {} as Record<string, boolean>,
  overflow: {} as Record<string, boolean>,
  listSig: '',
  meta: null as any,
  trajKinds: {} as Record<string, boolean>,
  trajOpen: {} as Record<string, boolean>,
})

/* ---------- 布局求解（computeColumns 逐行照旧页：纯函数、无迟滞） ---------- */

export function clampWidth(px: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, Math.round(px)))
}

export function computeColumns(viewport: number, sidebar: number, details: number) {
  const s = sidebar === 0 ? RAIL_W : clampWidth(sidebar, SIDEBAR_MIN, SIDEBAR_MAX)
  const d0 = details === 0 ? 0 : clampWidth(details, DETAILS_MIN, DETAILS_MAX)
  if (s + d0 + CENTER_MIN <= viewport) {
    return { sidebar: s, center: viewport - s - d0, details: d0 }
  }
  const d1 = d0 === 0 ? 0 : Math.max(DETAILS_MIN, viewport - s - CENTER_MIN)
  if (s + d1 + CENTER_MIN <= viewport) {
    return { sidebar: s, center: CENTER_MIN, details: d1 }
  }
  return { sidebar: s, center: Math.max(0, viewport - s), details: 0 }
}

/* 拖拽/布局落点：模板直接绑定这份解出的列宽。 */
export const layout = reactive({
  viewport: 0,
  cols: { sidebar: SIDEBAR_DEFAULT, center: 0, details: 0 },
  sidebarCollapsed: false,
  detailsCollapsed: true,
})

/* 把当前偏好解成三栏宽度并挂上骨架属性（旧页 applyLayout 的 Vue 形态）。 */
export function applyLayout(): void {
  const viewport = layout.viewport
  state.narrow = viewport < SIDEBAR_AUTO_COLLAPSE
  const sidebarCollapsed = state.narrow ? !state.narrowExpanded : state.sidebar === 0
  const sidebarPref = sidebarCollapsed ? 0 : state.sidebar === 0 ? SIDEBAR_DEFAULT : state.sidebar
  layout.cols = computeColumns(viewport, sidebarPref, state.details)
  layout.sidebarCollapsed = sidebarCollapsed
  layout.detailsCollapsed = layout.cols.details === 0
}

export function sidebarCollapsedNow(): boolean {
  return state.narrow ? !state.narrowExpanded : state.sidebar === 0
}

export function toggleSidebar(): void {
  if (state.narrow) {
    state.narrowExpanded = !state.narrowExpanded
  } else {
    state.sidebar = state.sidebar === 0 ? SIDEBAR_DEFAULT : 0
  }
  persistLayout()
  applyLayout()
}

export function toggleDetails(): void {
  if (state.details > 0) {
    state.details = 0
  } else {
    state.details = clampWidth(state.details || DETAILS_DEFAULT, DETAILS_MIN, DETAILS_MAX)
    // 让步链在中栏 640px 保不住时会直接放弃详情栏。点了没反应比"先把侧栏
    // 折成轨道腾地方"更糟：窄窗口下用户要的就是这块面板，所以放不下时
    // 顺手把侧栏折了，再解一次。
    const viewport = layout.viewport
    const collapsed = sidebarCollapsedNow()
    const pref = collapsed ? 0 : state.sidebar === 0 ? SIDEBAR_DEFAULT : state.sidebar
    if (computeColumns(viewport, pref, state.details).details === 0 && !collapsed) {
      if (state.narrow) {
        state.narrowExpanded = false
      } else {
        state.sidebar = 0
      }
    }
    renderDetails()
  }
  persistLayout()
  applyLayout()
}

export function openDetails(): void {
  if (state.details === 0) toggleDetails()
}

/* ---------- 布局持久化（旧页 persistLayout / loadState 同名） ---------- */

export function persistLayout(): void {
  storeSet('layout.sidebar', String(state.sidebar))
  storeSet('layout.details', String(state.details))
  storeSet('layout.narrowExpanded', state.narrowExpanded ? '1' : '0')
}

export function loadState(): void {
  state.collapsed = storeJSON('collapsed', {}) as Record<string, boolean>
  state.overflow = storeJSON('overflow', {}) as Record<string, boolean>
  state.markdown = storeGet('markdown') !== '0'
  state.unit = storeGet('unit') === 'char' ? 'char' : 'token'
  // Number(null) === 0：必须先判"存没存过"，否则首次访问会把侧栏读成折叠轨道。
  const rawSidebar = storeGet('layout.sidebar')
  if (rawSidebar !== null) {
    const sidebar = Number(rawSidebar)
    if (!isNaN(sidebar) && sidebar >= 0) state.sidebar = sidebar
  }
  const rawDetails = storeGet('layout.details')
  const details = rawDetails === null ? 0 : Number(rawDetails)
  state.details = !isNaN(details) && details > 0 ? details : 0
  state.narrowExpanded = storeGet('layout.narrowExpanded') === '1'
}

/* ---------- 主题（旧页 applyTheme/storedTheme/toggleTheme 同名） ---------- */

export const THEME_KEY = 'theme'
export const DEFAULT_THEME = 'light'

export function applyTheme(theme: string): void {
  state.theme = theme === 'dark' ? 'dark' : DEFAULT_THEME
  document.documentElement.setAttribute('data-theme', state.theme)
}

export function storedTheme(): string {
  const saved = storeGet(THEME_KEY)
  return saved === 'dark' || saved === 'light' ? saved : DEFAULT_THEME
}

export function toggleTheme(): void {
  const next = state.theme === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  storeSet(THEME_KEY, next)
}

/* ---------- 视图切换（对话 / 轨迹；旧页 switchView 同名） ---------- */

export function switchView(view: string): void {
  state.view = view === 'trajectory' ? 'trajectory' : 'chat'
  if (state.view === 'trajectory') {
    renderTrajectory() // 轨迹块注册的实现（未注册时空操作）
  }
}

/* ---------- 灯箱（旧页 openLightbox/closeLightbox 同名；点击接线在缩略图块） ---------- */

export const lightbox = reactive({ open: false, url: '', ref: '' })

export function openLightbox(url: string, ref: string): void {
  lightbox.open = true
  lightbox.url = url
  lightbox.ref = ref
}

export function closeLightbox(): void {
  lightbox.open = false
  lightbox.url = ''
  lightbox.ref = ''
}

/* ---------- 数据层占位（后续块替换成真实现） ---------- */

/* 旧页 scrollToBottom：把消息流滚到底部（自动跟随/切会话用）。 */
export function scrollToBottomOfTimeline(): void {
  window.setTimeout(() => {
    const el = document.getElementById('timeline')
    if (el) el.scrollTop = el.scrollHeight
  }, 0)
}

/* 旧页 setBanner：错误横幅（坏行提示由 updateBanner 逻辑接管，见 App.vue）。 */
export function setBannerText(text: string): void {
  state.pullError = text
}

export function refreshIndex(): Promise<void> {
  return Promise.resolve()
}

/* 侧栏块实现：展开装着该项目的分组并滚动到位。 */
export function revealProject(project: string): void {
  void project
}

export function renderTimeline(): void {}

/* ---------- 渲染入口注册表：各块组件实现，这里只留挂点（避免循环 import） ---------- */

let _renderTrajectory: () => void = () => {}
export function registerRenderTrajectory(fn: () => void): void { _renderTrajectory = fn }
export function renderTrajectory(): void { _renderTrajectory() }

let _renderDetails: () => void = () => {}
export function registerRenderDetails(fn: () => void): void { _renderDetails = fn }
export function renderDetails(): void { _renderDetails() }
