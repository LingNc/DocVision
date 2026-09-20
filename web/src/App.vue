<script setup lang="ts">
// 迁移纪律：本组件（以及后续所有组件）不写任何样式——视觉全部来自
// 全局样式在 styles/base.css（token/共享词汇），本组件样式在文件尾 <style scoped>。模板结构照
// go/internal/sessionview/assets/viewer.html 的骨架逐节点复刻。
// 块 1：三栏骨架交互；块 2：侧栏（数据层 + 分组树）接上。
import { computed, onBeforeUnmount, onMounted, ref, watch, watchEffect } from 'vue'
import {
  applyLayout,
  applyTheme,
  closeLightbox,
  DETAILS_DEFAULT,
  DETAILS_MAX,
  DETAILS_MIN,
  fmtSize,
  layout,
  lightbox,
  loadState,
  resetLightbox,
  setLightboxScale,
  zoomLightbox,
  persistLayout,
  SIDEBAR_DEFAULT,
  SIDEBAR_MAX,
  SIDEBAR_MIN,
  state,
  storedTheme,
  storeSet,
  switchView,
  clampWidth,
  toggleDetails,
  toggleSidebar,
  toggleTheme,
} from './state'
import { bootData, refreshIndex, refreshImg2TextProgress, revealProject } from './data'
import { projectOf, sessionTitleOf, chainOfSession } from './legacy/sidebar'
import Sidebar from './components/Sidebar.vue'
import Timeline from './components/Timeline.vue'
import InlineMD from './components/InlineMD.vue'
import Trajectory from './components/Trajectory.vue'
import DetailsPanel from './components/DetailsPanel.vue'
import HistoryPanel from './components/HistoryPanel.vue'

const frame = ref<HTMLElement | null>(null)

/* 拖拽分隔条：8px 命中区、pointer capture、松手才落盘、双击回默认宽度。
 * 旧页 wireHandle 的 Vue 形态——同一个处理器给两侧用，base 记拖拽起点列宽。 */
const drag = ref<{ side: 'sidebar' | 'details' } | null>(null)
let dragOrigin = 0
let dragBase = 0

function onDragStart(ev: PointerEvent, side: 'sidebar' | 'details') {
  ev.preventDefault()
  dragOrigin = ev.clientX
  dragBase = side === 'sidebar' ? layout.cols.sidebar : layout.cols.details
  ;(ev.currentTarget as Element).setPointerCapture?.(ev.pointerId)
  drag.value = { side }
}

function onDragMove(ev: PointerEvent) {
  if (!drag.value) return
  const dx = ev.clientX - dragOrigin
  if (drag.value.side === 'sidebar') {
    state.sidebar = clampWidth(dragBase + dx, SIDEBAR_MIN, SIDEBAR_MAX)
    if (state.narrow) state.narrowExpanded = true
  } else {
    state.details = clampWidth(dragBase - dx, DETAILS_MIN, DETAILS_MAX)
  }
  applyLayout()
}

function onDragEnd() {
  if (!drag.value) return
  drag.value = null
  persistLayout()
}

function onDragDblClick(side: 'sidebar' | 'details') {
  if (side === 'sidebar') state.sidebar = SIDEBAR_DEFAULT
  else state.details = DETAILS_DEFAULT
  persistLayout()
  applyLayout()
}

/* 六个开关：跟随旧页 wiring 的行为（aria-pressed 走 :aria-pressed 绑定）。 */
function onClickFollow() {
  state.follow = !state.follow
  if (state.follow) scrollToBottom()
}

function onClickCollapseThinking() {
  // 思考块组件 watch forceCollapse 自动收起/恢复（含增量追加进来的）。
  state.forceCollapse = !state.forceCollapse
}

function onClickOnlyTools() {
  state.onlyTools = !state.onlyTools // Timeline.vue 的 watch 负责重渲
}

function onClickThumbs() {
  state.showThumbs = !state.showThumbs
  storeSet('showThumbs', state.showThumbs ? '1' : '0') // AssistantMsg 的 v-if 响应式跟随
}

function onClickMarkdown() {
  state.markdown = !state.markdown
  storeSet('markdown', state.markdown ? '1' : '0') // Timeline.vue 的 watch 负责重渲
}

function onClickUnit() {
  state.unit = state.unit === 'char' ? 'token' : 'char'
  storeSet('unit', state.unit) // Timeline.vue 的 watch 负责重渲对话流
}

function scrollToBottom() {
  window.setTimeout(() => {
    const el = document.getElementById('timeline')
    if (el) el.scrollTop = el.scrollHeight
  }, 0)
}

/* 徽标与面包屑：旧页 renderHeader 行为——选中会话后 #header-actions 整体
 * 重建为摘要徽标（mode-badge 从 DOM 移除）；未选中时才是 live 徽标。 */
const badgeText = computed(() => (state.polling ? '实时' : '实时（已断开）'))
const badCount = computed(() => state.lines.reduce((n: number, l: any) => n + (l && l.bad ? 1 : 0), 0))
const headerSummary = computed(() => {
  const cur = state.current
  if (!cur) return ''
  const parts: string[] = []
  parts.push(cur.messages + ' 条消息')
  if (state.lines.length) parts.push(state.lines.length + ' 行')
  parts.push(fmtSize(cur.size))
  if (badCount.value) parts.push('坏行 ' + badCount.value)
  return parts.join(' · ')
})
const curProject = computed(() => (state.current ? state.current.project || projectOf(state.current.id) : ''))
const sessionTitle = computed(() => (state.current ? sessionTitleOf(state.current) : ''))

/* 「历史」页签：当前会话属于某条成员数 > 1 的 img2text 调用链时出现。
 * computed 依赖 state.sessions（轮询整体换新数组）与当前会话 id，避免每帧
 * 全量重算；切到链外会话时视图回落「对话」。 */
const currentChain = computed(() => chainOfSession(state.sessions, state.current ? state.current.id : null))
watch(
  () => (state.current ? state.current.id : null),
  () => {
    if (state.view === 'history' && !currentChain.value) switchView('chat')
  },
)

/* 横幅：坏行提示与拉取失败共用一条（旧页 updateBanner/setBanner 的语义）。
 * 坏行优先；拉取失败的信息保留到坏行出现或下次成功渲染时。 */
const bannerText = computed(() => {
  if (badCount.value > 0) {
    return '已跳过 ' + badCount.value + ' 行坏数据（无法解析为 JSON，可能是一次写入中途读到的不完整行）'
  }
  return state.pullError
})

/* 按键：Esc 关灯箱；[ 折侧栏、] 折详情（输入时不触发）。 */
function onKeydown(ev: KeyboardEvent) {
  const tag = (ev.target as HTMLElement | null)?.tagName
  const typing = tag === 'INPUT' || tag === 'TEXTAREA' || (ev.target as HTMLElement | null)?.isContentEditable
  if (ev.key === 'Escape') closeLightbox()
  if (typing) return
  if (ev.key === '[') toggleSidebar()
  if (ev.key === ']') toggleDetails()
}

/* P6 灯箱交互：滚轮缩放（围绕鼠标点）、按住拖动平移、双击复位。
 * 「点击关闭」与「双击复位」天然冲突（双击 = 两次 click）——单击延迟 260ms
 * 再关，第二击落在窗口内就按双击复位处理；拖动过的 pointer 序列吞掉 click。
 * P14：pointer 事件天然统一鼠标/触屏——双指按下进入捏合缩放（以双指中点
 * 为锚、按初始距离比绝对缩放），触屏上连点两下在 1× 与 2.2× 间切换。 */
const lbPointers = new Map<number, { x: number; y: number }>()
let lbPan: { id: number; x: number; y: number; tx: number; ty: number; moved: boolean } | null = null
let lbPinch: { d0: number; s0: number; cx: number; cy: number; rect: DOMRect } | null = null
let lbLastTap = 0
let lbClickTimer: ReturnType<typeof setTimeout> | null = null
let lbSuppressClick = false

function lbImgRect(host: HTMLElement): DOMRect | null {
  const el = host.querySelector('#lightbox-img')
  return el ? (el as HTMLElement).getBoundingClientRect() : null
}

function onLbWheel(ev: WheelEvent) {
  ev.preventDefault()
  const rect = lbImgRect(ev.currentTarget as HTMLElement)
  if (!rect) return
  zoomLightbox(ev.deltaY > 0 ? 0.9 : 1 / 0.9, ev.clientX, ev.clientY, rect)
}

function onLbPointerdown(ev: PointerEvent) {
  if (ev.button !== 0 && ev.pointerType === 'mouse') return
  const host = ev.currentTarget as HTMLElement
  host.setPointerCapture(ev.pointerId)
  lbPointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY })
  if (lbPointers.size === 2) {
    // 双指：进入捏合，放弃平移候选。
    const [a, b] = [...lbPointers.values()]
    const rect = lbImgRect(host)
    if (rect) {
      lbPinch = { d0: Math.max(8, Math.hypot(a.x - b.x, a.y - b.y)), s0: lightbox.scale, cx: (a.x + b.x) / 2, cy: (a.y + b.y) / 2, rect }
      lbPan = null
    }
  } else if (lbPointers.size === 1) {
    lbPan = { id: ev.pointerId, x: ev.clientX, y: ev.clientY, tx: lightbox.tx, ty: lightbox.ty, moved: false }
  }
}

function onLbPointermove(ev: PointerEvent) {
  if (!lbPointers.has(ev.pointerId)) return
  lbPointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY })
  const host = ev.currentTarget as HTMLElement
  if (lbPinch && lbPointers.size >= 2) {
    const [a, b] = [...lbPointers.values()]
    const d = Math.max(8, Math.hypot(a.x - b.x, a.y - b.y))
    // 锚点跟手：双指中点当前位置。
    setLightboxScale(lbPinch.s0 * (d / lbPinch.d0), (a.x + b.x) / 2, (a.y + b.y) / 2, lbPinch.rect)
    return
  }
  if (lbPan && lbPan.id === ev.pointerId) {
    const dx = ev.clientX - lbPan.x
    const dy = ev.clientY - lbPan.y
    if (!lbPan.moved && Math.hypot(dx, dy) > 3) lbPan.moved = true
    if (lbPan.moved) {
      lightbox.tx = lbPan.tx + dx
      lightbox.ty = lbPan.ty + dy
    }
  }
}

function onLbPointerup(ev: PointerEvent) {
  const wasPan = lbPan && lbPan.id === ev.pointerId ? lbPan : null
  lbPointers.delete(ev.pointerId)
  if (lbPointers.size < 2) lbPinch = null
  if (wasPan) {
    lbSuppressClick = wasPan.moved === true
    // 触屏轻点（未拖动）：连点两下 = 缩放/复位切换（鼠标双击走 dblclick）。
    if (!wasPan.moved && ev.pointerType !== 'mouse') {
      const now = Date.now()
      const host = ev.currentTarget as HTMLElement
      const rect = lbImgRect(host)
      if (now - lbLastTap < 320 && rect) {
        lbLastTap = 0
        if (lightbox.scale > 1.5) resetLightbox()
        else setLightboxScale(2.2, ev.clientX, ev.clientY, rect)
      } else {
        lbLastTap = now
      }
    }
    lbPan = null
  }
}

let ro: ResizeObserver | null = null
let pollTimer = 0
let i2tTimer = 0

function onLbClick() {
  if (lbSuppressClick) {
    lbSuppressClick = false
    return
  }
  if (lbClickTimer) return // 双击的第二次 click——交给 dblclick 处理
  lbClickTimer = setTimeout(() => { lbClickTimer = null; closeLightbox() }, 260)
}

function onLbDblclick(ev: MouseEvent) {
  ev.preventDefault()
  if (lbClickTimer) { clearTimeout(lbClickTimer); lbClickTimer = null }
  resetLightbox()
}

/* P14：右下角缩放控件（触屏没有滚轮；按钮 @click.stop 避免触发「点空白关闭」）。 */
function lbZoomBtn(factor: number, ev: MouseEvent) {
  ev.stopPropagation()
  const host = document.getElementById('lightbox')
  const rect = host ? lbImgRect(host) : null
  if (!rect) return
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2
  zoomLightbox(factor, cx, cy, rect)
}

function lbReset(ev: MouseEvent) {
  ev.stopPropagation()
  resetLightbox()
}

onMounted(() => {
  loadState()
  applyTheme(storedTheme())
  const el = frame.value
  if (el) {
    layout.viewport = el.clientWidth || window.innerWidth
    if (window.ResizeObserver) {
      ro = new ResizeObserver(() => {
        layout.viewport = el.clientWidth || window.innerWidth
      })
      ro.observe(el)
    } else {
      window.addEventListener('resize', onResize)
    }
  }
  document.addEventListener('keydown', onKeydown)
  // 数据层：首拉 + 2 秒轮询（页面隐藏时跳过）。静态快照没有 /api，不轮询。
  bootData()
  if (!state.staticMode) {
    pollTimer = window.setInterval(() => {
      if (!document.hidden) void refreshIndex()
    }, 2000)
    // P18：img2text 进度概览走独立的 ~5s 轮询（节奏比会话列表慢一档）。
    i2tTimer = window.setInterval(() => {
      if (!document.hidden) void refreshImg2TextProgress()
    }, 5000)
  }
})

function onResize() {
  const el = frame.value
  if (el) layout.viewport = el.clientWidth || window.innerWidth
}

watchEffect(() => {
  // layout.viewport 变化驱动布局求解；state.sidebar/details/narrowExpanded 同理。
  void layout.viewport
  void state.sidebar
  void state.details
  void state.narrowExpanded
  applyLayout()
})

onBeforeUnmount(() => {
  ro?.disconnect()
  if (pollTimer) window.clearInterval(pollTimer)
  if (i2tTimer) window.clearInterval(i2tTimer)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div
    id="frame"
    ref="frame"
    class="frame"
    :style="{ gridTemplateColumns: `${layout.cols.sidebar}px minmax(0, 1fr) ${layout.cols.details}px` }"
    :data-sidebar-collapsed="layout.sidebarCollapsed ? '' : undefined"
    :data-details-collapsed="layout.detailsCollapsed ? '' : undefined"
    :data-dragging="drag ? '' : undefined"
  >
    <Sidebar></Sidebar>

    <div
      id="handle-sidebar"
      class="handle"
      data-side="sidebar"
      role="separator"
      aria-orientation="vertical"
      aria-label="调整侧栏宽度"
      :style="{ left: `${layout.cols.sidebar}px` }"
      :data-dragging="drag?.side === 'sidebar' ? 'true' : undefined"
      @pointerdown="onDragStart($event, 'sidebar')"
      @pointermove="onDragMove"
      @pointerup="onDragEnd"
      @pointercancel="onDragEnd"
      @dblclick="onDragDblClick('sidebar')"
    ></div>

    <main id="center-col" class="center-col">
      <header id="center-header" class="center-header">
        <div class="title-row">
          <button id="side-toggle" class="icon-btn" type="button" title="折叠 / 展开侧栏" aria-label="折叠或展开侧栏" :aria-pressed="layout.sidebarCollapsed ? 'false' : 'true'" @click="toggleSidebar">▤</button>
          <nav id="crumbs" class="crumbs" aria-label="面包屑">
            <span v-if="!state.current" class="crumb crumb-current">未选择会话</span>
            <template v-else>
              <button class="crumb is-link" type="button" title="在侧栏里定位到这个项目" @click="revealProject(curProject)">{{ curProject }}</button>
              <span class="crumb-sep">›</span>
              <!-- 会话名本身就是行内 Markdown 渲染根（旧页 nameNode('span','crumb crumb-current',…)），不再多包一层 -->
              <InlineMD :text="sessionTitle" tag="span" class="crumb crumb-current" :title="state.current.id"></InlineMD>
            </template>
          </nav>
          <div id="header-actions" class="header-actions">
            <span v-if="!state.current" id="mode-badge" class="badge badge-live">{{ badgeText }}</span>
            <span v-else class="badge">{{ headerSummary }}</span>
          </div>
          <button id="details-toggle" class="icon-btn" type="button" :title="layout.detailsCollapsed ? '详情面板（元信息 / 指标）' : '关闭详情面板'" aria-label="展开或收起详情面板" :aria-pressed="layout.detailsCollapsed ? 'false' : 'true'" @click="toggleDetails">ⓘ</button>
        </div>
        <div class="tabs-row">
          <div class="tabs" role="tablist" aria-label="视图">
            <button id="tab-chat" class="tab" :class="{ 'tab-active': state.view === 'chat' }" type="button" role="tab" :aria-selected="state.view === 'chat' ? 'true' : 'false'" data-view="chat" @click="switchView('chat')">对话</button>
            <button id="tab-traj" class="tab" :class="{ 'tab-active': state.view === 'trajectory' }" type="button" role="tab" :aria-selected="state.view === 'trajectory' ? 'true' : 'false'" data-view="trajectory" @click="switchView('trajectory')">轨迹</button>
            <button v-if="currentChain" id="tab-history" class="tab" :class="{ 'tab-active': state.view === 'history' }" type="button" role="tab" :aria-selected="state.view === 'history' ? 'true' : 'false'" data-view="history" @click="switchView('history')">历史 {{ currentChain.length }}</button>
          </div>
          <div class="tab-tools" role="group" aria-label="显示选项">
            <button id="follow" class="tab-toggle" type="button" :aria-pressed="state.follow ? 'true' : 'false'" title="新消息到达时自动滚动到底部" @click="onClickFollow">自动跟随</button>
            <button id="collapse-thinking" class="tab-toggle" type="button" :aria-pressed="state.forceCollapse ? 'true' : 'false'" title="把所有消息的思考过程折叠起来" @click="onClickCollapseThinking">折叠全部思考</button>
            <button id="only-tools" class="tab-toggle" type="button" :aria-pressed="state.onlyTools ? 'true' : 'false'" title="只显示工具调用与工具结果" @click="onClickOnlyTools">仅看工具调用</button>
            <button id="md-toggle" class="tab-toggle" type="button" :aria-pressed="state.markdown ? 'true' : 'false'" :title="state.markdown ? '消息正文按 Markdown 渲染（标题 / 列表 / 代码块 / 表格），点击回到纯文本' : '消息正文按纯文本显示（pre-wrap），点击改用 Markdown 渲染'" @click="onClickMarkdown">Markdown</button>
            <button id="thumb-toggle" class="tab-toggle" type="button" :aria-pressed="state.showThumbs ? 'true' : 'false'" :title="state.showThumbs ? '显示看图调用下的缩略图预览行，点击隐藏' : '已隐藏缩略图预览行，点击显示'" @click="onClickThumbs">缩略图</button>
            <button id="unit-toggle" class="tab-toggle" type="button" :aria-pressed="state.unit === 'char' ? 'false' : 'true'" :title="state.unit === 'char' ? '计数按字符数显示（精确值），点击改为 token' : '计数按 token 显示（本地估算，带 ≈），点击改为字符'" @click="onClickUnit">{{ state.unit === 'char' ? '字符' : 'token' }}</button>
            <button id="theme-toggle" class="tab-toggle" type="button" :aria-pressed="state.theme === 'dark' ? 'true' : 'false'" :title="state.theme === 'dark' ? '切换为白天模式（浅色，默认）' : '切换为夜间模式（深色）'" @click="toggleTheme">{{ state.theme === 'dark' ? '☀️ 浅色' : '🌙 深色' }}</button>
          </div>
        </div>
      </header>
      <div id="banner" class="banner" :class="{ hidden: !bannerText }">{{ bannerText }}</div>
      <div class="view-area">
        <Timeline></Timeline>
        <Trajectory />
        <HistoryPanel />
      </div>
    </main>

    <div
      id="handle-details"
      class="handle"
      data-side="details"
      role="separator"
      aria-orientation="vertical"
      aria-label="调整详情栏宽度"
      :style="{ left: `${Math.max(0, layout.viewport - layout.cols.details)}px` }"
      :data-dragging="drag?.side === 'details' ? 'true' : undefined"
      :data-hidden="layout.detailsCollapsed ? 'true' : undefined"
      @pointerdown="onDragStart($event, 'details')"
      @pointermove="onDragMove"
      @pointerup="onDragEnd"
      @pointercancel="onDragEnd"
      @dblclick="onDragDblClick('details')"
    ></div>

    <aside id="details-col" class="details-col" aria-label="详情">
      <div class="details-head">
        <span class="details-title">详情</span>
        <button id="details-close" class="icon-btn" type="button" title="关闭详情面板" aria-label="关闭详情面板" @click="state.details > 0 && toggleDetails()">✕</button>
      </div>
      <DetailsPanel />
    </aside>
  </div>
  <div id="lightbox" class="lightbox" :class="{ hidden: !lightbox.open }" @click="onLbClick"
    @wheel="onLbWheel" @pointerdown="onLbPointerdown" @pointermove="onLbPointermove"
    @pointerup="onLbPointerup" @pointercancel="onLbPointerup" @dblclick="onLbDblclick">
    <!-- 旧页 img 不拦冒泡：点图也会冒到灯箱背景关闭（行为保持一致；P6 拖动过则不关） -->
    <img id="lightbox-img" :src="lightbox.open ? lightbox.url : undefined" :alt="lightbox.ref"
      :style="{ transform: 'translate(' + lightbox.tx + 'px,' + lightbox.ty + 'px) scale(' + lightbox.scale + ')' }">
    <div class="lightbox-hint">滚轮 / 双指缩放 · 拖动平移 · 双击复位 · 点击空白或 Esc 关闭</div>
    <div class="lightbox-zoom" @click.stop @dblclick.stop>
      <button class="icon-btn" type="button" title="缩小" aria-label="缩小" @click="lbZoomBtn(1.25, $event)">−</button>
      <span class="lightbox-scale">{{ Math.round(lightbox.scale * 100) }}%</span>
      <button class="icon-btn" type="button" title="放大" aria-label="放大" @click="lbZoomBtn(0.8, $event)">＋</button>
      <button class="icon-btn" type="button" title="复位 100%" aria-label="复位缩放" @click="lbReset($event)">1:1</button>
    </div>
  </div>
</template>

<style scoped>
/* ============================ 三栏骨架（DSH AppFrame） ============================ */

.frame {
  position: relative;
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr) 0px;
  grid-template-rows: 100%;
  height: 100vh;
  overflow: hidden;
  background: var(--bg);
  transition: grid-template-columns .18s var(--ease);
}

.frame[data-dragging] {
 transition: none; cursor: col-resize; user-select: none; 
}

@media (prefers-reduced-motion: reduce) {
 .frame { transition: none; } 
}

.sidebar-col {
  grid-column: 1;
  min-width: 0;
  overflow: hidden;
  background: var(--sidebar-bg);
  border-right: .5px solid var(--border);
  display: flex;
  flex-direction: column;
}

.frame[data-sidebar-collapsed] .sidebar-col {
 border-right-color: transparent; 
}

.center-col {
  grid-column: 2;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  background: var(--bg);
}

/* 详情栏头部（元素在本组件渲染；样式曾在 DetailsPanel 的 scoped 里丢匹配） */
.details-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 8px 0 14px;
  border-bottom: .5px solid var(--border);
}

.details-title {
 flex: 1; min-width: 0; font-size: 13px; font-weight: 600; color: var(--muted); letter-spacing: .2px;
}

.details-col {
  grid-column: 3;
  min-width: 0;
  overflow: hidden;
  border-left: .5px solid var(--border);
  background: var(--bg);
  display: flex;
  flex-direction: column;
}

.frame[data-details-collapsed] .details-col {
 border-left-color: transparent; 
}

/* 8px 命中区、居中 4px 偏移 —— 与 DSH 的 DragHandle 一致 */
.handle {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 8px;
  margin-left: -4px;
  z-index: 6;
  cursor: col-resize;
  touch-action: none;
  transition: left .18s var(--ease), right .18s var(--ease);
}

.frame[data-dragging] .handle {
 transition: none; 
}

@media (prefers-reduced-motion: reduce) {
 .handle { transition: none; } 
}

.handle::after {
  content: "";
  position: absolute;
  top: 50%;
  left: 50%;
  width: 3px;
  height: 34px;
  transform: translate(-50%, -50%);
  border-radius: 3px;
  background: var(--border-strong);
  opacity: 0;
  transition: opacity .14s var(--ease);
}

.handle:hover::after, .handle[data-dragging="true"]::after {
 opacity: 1; background: var(--accent); 
}

.handle[data-hidden="true"] {
 display: none; 
}

/* ============================ 中栏头部：面包屑 + 标签页 ============================ */

.center-header {
  flex: none;
  position: relative;
  padding: 12px 28px 0 20px;
  background: var(--bg);
}

.center-header::after {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: .5px;
  background: var(--border);
  pointer-events: none;
}

.title-row {
 display: flex; align-items: center; min-height: 32px; gap: 0; 
}

.crumbs {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
  overflow: hidden;
}

.crumb {
  max-width: 220px;
  padding: 4px 8px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  color: var(--dim);
  font: inherit;
  font-size: 14px;
  line-height: 20px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.crumb.is-link {
 cursor: pointer; 
}

.crumb.is-link:hover {
 background: var(--hover); 
}

.crumb-current {
 color: var(--text); font-weight: 600; 
}

.crumb-sep {
 flex: none; color: var(--caption); font-size: 14px; line-height: 20px; 
}

.header-actions {
 flex: none; display: flex; align-items: center; gap: 8px; margin-left: 20px; 
}

.header-actions:empty {
 display: none; 
}

.tabs-row {
 display: flex; align-items: flex-end; gap: 12px; 
}

.tabs {
 display: flex; gap: 36px; padding-left: 8px; margin-top: 4px; 
}

.tab {
  position: relative;
  padding: 0 0 11px;
  border: 0;
  background: transparent;
  color: var(--dim);
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  line-height: 16px;
  cursor: pointer;
}

.tab::after {
  content: "";
  position: absolute;
  left: 0;
  right: 0;
  bottom: 1px;
  height: 2px;
  border-radius: 2px;
  background: transparent;
}

.tab:hover {
 color: var(--muted); 
}

.tab-active {
 color: var(--accent); 
}

.tab-active::after {
 background: var(--accent); 
}

.tab:focus-visible {
 outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 3px; 
}

.tab-tools {
 flex: none; display: flex; align-items: center; gap: 2px; margin-left: auto; padding-bottom: 6px; 
}

.tab-toggle {
  height: 20px;
  padding: 0 7px;
  border: 0;
  border-radius: 3px;
  background: transparent;
  color: var(--dim);
  font: inherit;
  font-size: 12px;
  line-height: 20px;
  white-space: nowrap;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.tab-toggle:hover {
 color: var(--text); background: var(--hover); 
}

.tab-toggle[aria-pressed="true"] {
 color: var(--accent); background: var(--accent-soft); 
}

.tab-toggle:focus-visible {
 outline: 1px solid var(--accent); outline-offset: 1px; 
}

.banner {
  flex: none;
  margin: 8px 28px 0 20px;
  padding: 6px 10px;
  border: .5px solid var(--border);
  border-left: 3px solid var(--warn);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  color: var(--muted);
  font-size: 12px;
}

.view-area {
 flex: 1; min-height: 0; display: flex; flex-direction: column; overflow: hidden; 
}

@media (max-width: 1023px) {
  .center-header { padding: 10px 16px 0 12px; }
  .tab-tools { gap: 0; }
}
</style>
