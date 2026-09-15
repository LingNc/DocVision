import { reactive } from 'vue'
import { type Line, type ImageAttr, type Session, type PartialInfo } from './legacy/types'

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
/*
 * 行号锚点注册表（**非响应式**，故意不放进 reactive(state)——把 DOM 节点
 * 包进响应式代理纯属浪费还容易踩坑）。各消息组件挂载时注册自己的根元素、
 * 卸载时注销；轨迹页 jumpToLine 用它跳回对话。切会话时 clearAnchors()。
 */
const anchors = new Map<number, HTMLElement>()
export function registerAnchor(n: number, el: HTMLElement): void { anchors.set(n, el) }
export function unregisterAnchor(n: number, el: HTMLElement): void { if (anchors.get(n) === el) anchors.delete(n) }
export function anchorOf(n: number): HTMLElement | null { return anchors.get(n) || null }
export function clearAnchors(): void { anchors.clear() }

export const state = reactive({
  root: '',
  generated: '',
  sessions: [] as Session[],
  current: null as Session | null,
  lines: [] as Line[],
  nextFrom: 0,
  curSize: -1,
  curMtime: 0,
  filter: '',
  follow: true, // 旧页 live 模式默认开
  forceCollapse: false,
  onlyTools: false,
  callEst: {} as Record<string, number>,
  imgAttr: null as { sig: string; map: Record<number, ImageAttr>; candRound: Record<string, number>; roundLast: Record<string, string> } | null,
  pullError: '',
  polling: false,
  theme: 'light',
  view: 'chat',
  markdown: true,
  showThumbs: true, // P13：工具卡下的缩略图预览行显隐（页签行「缩略图」开关，记忆 showThumbs）
  unit: 'token',
  sidebar: SIDEBAR_DEFAULT,
  details: 0,
  narrowExpanded: false,
  narrow: false,
  collapsed: {} as Record<string, boolean>,
  overflow: {} as Record<string, boolean>,
  listSig: '',
  meta: null as unknown,
  trajKinds: {} as Record<string, boolean>,
  trajOpen: {} as Record<string, boolean>,
  // P9：轨迹页选中的行（transcript 行号）。选中后右侧详情栏顶部显示该步的
  // 完整输入/输出/图片；再点同一行取消；跳对话按钮仍走 jumpToLine。
  trajSelected: null as string | null,
  // P7：当前会话的流式快照（<转录>.partial；消息完整落盘即消失）。
  // 轮询时只在「当前会话正在 live」时读取，切会话/非 live 清空。
  partial: null as PartialInfo | null,
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
  state.showThumbs = storeGet('showThumbs') !== '0'
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
}

/* ---------- 灯箱（旧页 openLightbox/closeLightbox 同名；点击接线在缩略图块） ----------
 * P6：滚轮缩放（围绕鼠标点）+ 按住拖动平移 + 双击复位。transform 走
 * translate(tx,ty) scale(s)、origin 默认中心；锚点换算见 zoomLightbox。 */

export const lightbox = reactive({ open: false, url: '', ref: '', scale: 1, tx: 0, ty: 0 })

export function openLightbox(url: string, ref: string): void {
  lightbox.open = true
  lightbox.url = url
  lightbox.ref = ref
  lightbox.scale = 1
  lightbox.tx = 0
  lightbox.ty = 0
}

export function closeLightbox(): void {
  lightbox.open = false
  lightbox.url = ''
  lightbox.ref = ''
  lightbox.scale = 1
  lightbox.tx = 0
  lightbox.ty = 0
}

/** 滚轮缩放：factor<1 放大。锚点 = 鼠标相对图像中心的偏移 p，
 *  要求缩放后同一内容点仍停在鼠标下：t' = t + p·(s − s')。 */
export function zoomLightbox(factor: number, mx: number, my: number, rect: DOMRect): void {
  const s0 = lightbox.scale
  const s1 = Math.min(8, Math.max(0.15, s0 * factor))
  if (s1 === s0) return
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2
  const px = (mx - cx - lightbox.tx) / s0
  const py = (my - cy - lightbox.ty) / s0
  lightbox.tx += px * (s0 - s1)
  lightbox.ty += py * (s0 - s1)
  lightbox.scale = s1
}

export function resetLightbox(): void {
  lightbox.scale = 1
  lightbox.tx = 0
  lightbox.ty = 0
}

/* ---------- 数据层接口 ---------- */

/* 旧页 setBanner：错误横幅（坏行提示由 updateBanner 逻辑接管，见 App.vue）。 */
export function setBannerText(text: string): void {
  state.pullError = text
}



