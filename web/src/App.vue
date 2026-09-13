<script setup lang="ts">
// 迁移纪律：本组件（以及后续所有组件）不写任何样式——视觉全部来自
// 整卷沿用的旧页样式表（src/styles/viewer.css）。模板结构照
// go/internal/sessionview/assets/viewer.html 的骨架逐节点复刻。
// 块 1：三栏骨架的布局求解、拖拽分隔条、侧栏/详情栏折叠、页签切换、
// 六个开关的状态与落盘、主题、灯箱常驻 DOM。
import { computed, onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'
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
  persistLayout,
  refreshIndex,
  renderDetails,
  renderTimeline,
  renderTrajectory,
  ROOT_PROJECT,
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

const frame = ref<HTMLElement | null>(null)
const search = ref('')

/* 拖拽分隔条：8px 命中区、pointer capture、松手才落盘、双击回默认宽度。
 * 旧页 wireHandle 的 Vue 形态——同一个处理器给两侧用，base 记拖拽起点列宽。 */
const drag = ref<{ side: 'sidebar' | 'details'; handle: Element } | null>(null)
let dragOrigin = 0
let dragBase = 0

function onDragStart(ev: PointerEvent, side: 'sidebar' | 'details') {
  ev.preventDefault()
  dragOrigin = ev.clientX
  dragBase = side === 'sidebar' ? layout.cols.sidebar : layout.cols.details
  drag.value = { side, handle: ev.currentTarget as Element }
  ;(ev.currentTarget as Element).setPointerCapture?.(ev.pointerId)
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
  state.forceCollapse = !state.forceCollapse
  if (state.forceCollapse) {
    // 作用于消息流里所有思考行（含增量追加进来的）。
    document.querySelectorAll('details.disclosure-thinking').forEach((d) => {
      ;(d as HTMLDetailsElement).open = false
    })
  }
}

function onClickOnlyTools() {
  state.onlyTools = !state.onlyTools
  renderTimeline()
}

function onClickMarkdown() {
  state.markdown = !state.markdown
  storeSet('markdown', state.markdown ? '1' : '0')
  renderTimeline()
}

function onClickUnit() {
  state.unit = state.unit === 'char' ? 'token' : 'char'
  storeSet('unit', state.unit)
  renderTimeline()
  renderTrajectory()
  renderDetails()
  // 侧栏提示里也有计数：签名作废，强制整份重建（轮询守卫不该拦用户换单位）。
  state.listSig = ''
  refreshList()
}

function scrollToBottom() {
  window.setTimeout(() => {
    const el = document.getElementById('timeline')
    if (el) el.scrollTop = el.scrollHeight
  }, 0)
}

function onClickSearch() {
  /* 搜索框 v-model 绑 state.filter；列表刷新在侧栏块接线。 */
}

function onClickRefresh() {
  refreshIndex()
}

/* 徽标：旧页 renderHeader 行为——选中会话后 #header-actions 整体重建为
 * 摘要徽标（mode-badge 从 DOM 移除）；未选中时才是 live 徽标。 */
const badgeText = computed(() => (state.polling ? '实时' : '实时（已断开）'))
const headerSummary = computed(() => {
  const cur = state.current
  if (!cur) return ''
  const parts: string[] = []
  parts.push(cur.messages + ' 条消息')
  if (state.lines.length) parts.push(state.lines.length + ' 行')
  parts.push(fmtSize(cur.size))
  if (state.badLines) parts.push('坏行 ' + state.badLines)
  return parts.join(' · ')
})
/* 面包屑：未选中给占位；选中后项目名可点（revealProject 在侧栏块接线）。 */
const curProject = computed(() => (state.current ? state.current.project || ROOT_PROJECT : ''))
const sessionTitle = computed(() => (state.current ? state.current.name || state.current.id : ''))

/* 按键：Esc 关灯箱；[ 折侧栏、] 折详情（输入时不触发）。 */
function onKeydown(ev: KeyboardEvent) {
  const tag = (ev.target as HTMLElement | null)?.tagName
  const typing = tag === 'INPUT' || tag === 'TEXTAREA' || (ev.target as HTMLElement | null)?.isContentEditable
  if (ev.key === 'Escape') closeLightbox()
  if (typing) return
  if (ev.key === '[') toggleSidebar()
  if (ev.key === ']') toggleDetails()
}

let ro: ResizeObserver | null = null

onMounted(() => {
  loadState()
  applyTheme(storedTheme())
  // 视口宽度进 reactive：布局求解（watchEffect → applyLayout）随之重跑。
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
    <aside id="sidebar-col" class="sidebar-col">
      <div class="side-head">
        <span class="side-title">工作区</span>
        <button id="refresh" class="icon-btn" type="button" title="重新扫描会话" @click="onClickRefresh">⟳</button>
      </div>
      <div class="side-search">
        <input id="search" v-model="state.filter" type="search" placeholder="过滤：会话名 / 阶段 / 项目…" autocomplete="off" @input="onClickSearch">
      </div>
      <div class="side-list-wrap">
        <div id="session-list" class="session-list" role="tree" aria-label="会话列表"></div>
        <div class="list-fade" aria-hidden="true"></div>
      </div>
      <div id="side-totals" class="side-totals hidden"></div>
      <div class="side-status">
        <div id="root-path" class="root-path" title="扫描根目录">—</div>
        <div id="side-foot" class="side-foot"></div>
      </div>
    </aside>

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
              <span class="crumb crumb-current" :title="state.current.id">{{ sessionTitle }}</span>
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
          </div>
          <div class="tab-tools" role="group" aria-label="显示选项">
            <button id="follow" class="tab-toggle" type="button" :aria-pressed="state.follow ? 'true' : 'false'" title="新消息到达时自动滚动到底部" @click="onClickFollow">自动跟随</button>
            <button id="collapse-thinking" class="tab-toggle" type="button" :aria-pressed="state.forceCollapse ? 'true' : 'false'" title="把所有消息的思考过程折叠起来" @click="onClickCollapseThinking">折叠全部思考</button>
            <button id="only-tools" class="tab-toggle" type="button" :aria-pressed="state.onlyTools ? 'true' : 'false'" title="只显示工具调用与工具结果" @click="onClickOnlyTools">仅看工具调用</button>
            <button id="md-toggle" class="tab-toggle" type="button" :aria-pressed="state.markdown ? 'true' : 'false'" :title="state.markdown ? '消息正文按 Markdown 渲染（标题 / 列表 / 代码块 / 表格），点击回到纯文本' : '消息正文按纯文本显示（pre-wrap），点击改用 Markdown 渲染'" @click="onClickMarkdown">Markdown</button>
            <button id="unit-toggle" class="tab-toggle" type="button" :aria-pressed="state.unit === 'char' ? 'false' : 'true'" :title="state.unit === 'char' ? '计数按字符数显示（精确值），点击改为 token' : '计数按 token 显示（本地估算，带 ≈），点击改为字符'" @click="onClickUnit">{{ state.unit === 'char' ? '字符' : 'token' }}</button>
            <button id="theme-toggle" class="tab-toggle" type="button" :aria-pressed="state.theme === 'dark' ? 'true' : 'false'" :title="state.theme === 'dark' ? '切换为白天模式（浅色，默认）' : '切换为夜间模式（深色）'" @click="toggleTheme">{{ state.theme === 'dark' ? '☀️ 浅色' : '🌙 深色' }}</button>
          </div>
        </div>
      </header>
      <div id="banner" class="banner" :class="{ hidden: !state.badLines }"></div>
      <div class="view-area">
        <div id="timeline" class="timeline" :class="{ hidden: state.view !== 'chat' }"></div>
        <div id="trajectory" class="trajectory" :class="{ hidden: state.view === 'chat' }"></div>
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
      <div id="details-body" class="details-body"></div>
    </aside>
  </div>
  <div id="lightbox" class="lightbox" :class="{ hidden: !lightbox.open }" @click="closeLightbox">
    <img id="lightbox-img" :src="lightbox.open ? lightbox.url : undefined" :alt="lightbox.ref" @click.stop>
    <div class="lightbox-hint">点击空白处或按 Esc 关闭</div>
  </div>
</template>
