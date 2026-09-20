<script setup lang="ts">
/*
 * 侧栏（块 2）：三层分组（项目 → 阶段 → 会话），结构/类名/文案照旧页
 * buildGroup/sessionRow/renderSessions/renderTotals。折叠记忆走
 * v-collapse 指令（旧页 bindCollapse：程序化改 open 不写记忆，只有用户
 * 点击才写回）；选中高亮与行内文案走响应式，轮询不重建 DOM。
 */
import { computed, ref, watchEffect } from 'vue'
import { state, openDetails, fmtSize, storeGet, storeSet, setBoard } from '../state'
import {
  bookDominantState, bookTip, booksHeadText,
  buildGroups, buildImg2TextChains, filterByBoard, groupKey, groupWantOpen,
  imageChipText, imageTipText, img2textMemberTag,
  projectProgressLine, projectOf,
  isOverflowOpen, relTime, sessionTip, sessionTitleOf, setCollapsed, setOverflowOpen,
  stageRank, stageStatusText, stageTitleOf, usageChipText, fmtTokens,
  type SessionChain, type StageStatus,
} from '../legacy/sidebar'
import { selectSession, refreshIndex } from '../data'
import InlineMD from './InlineMD.vue'
import type { Img2TextBook } from '../legacy/types'

const OVERFLOW_LIMIT = 8

const listEl = ref<HTMLElement | null>(null)

/*
 * P18：板块切换（LaTeX / img2text）。仅当存在 img2text 会话或 progress books
 * 非空时才显示切换器，否则隐藏保持现状；板块本身在 state.board，切换只影响
 * 侧栏显示哪些项目组（过滤在 buildGroups 里做）。
 */
const showBoardSwitch = computed(() => {
  if (state.img2textProgress.length > 0) return true
  return state.sessions.some((s: any) => s.board === 'img2text')
})

/* 板块被隐藏时（比如 preview.img2text 关掉后重开页面）强制回落 latex，
 * 否则用户会停在一张永远空白的 img2text 板块上。 */
watchEffect(() => {
  if (!showBoardSwitch.value && state.board !== 'latex') setBoard('latex')
})

/* 当前板块下的会话总数（脚注计数按板块口径）。 */
const boardSessionCount = computed(() => filterByBoard(state.sessions, state.board).length)

/* img2text 进度概览：只在 img2text 板块且有数据时显示。 */
const progressBooks = computed<Img2TextBook[]>(() =>
  state.board === 'img2text' ? state.img2textProgress : []
)

function barPct(part: number, total: number): string {
  if (!total) return '0'
  return ((part * 100) / total).toFixed(1)
}

/*
 * T54：进度概览整块可折叠（像项目组的 <details> 一样），记忆 key
 * 「overview:img2text」，首次无记忆默认折叠（书多时省空间），用户展开后记
 * 住。头部行 = 标题 + 聚合「N/M 书完成」。
 */
function overviewWantOpen(): boolean {
  return groupWantOpen('overview:img2text', false)
}

const overviewHeadText = computed(() => booksHeadText(progressBooks.value))

/*
 * T54：概览区内的列表/方块两种视图，与侧栏头部 ▦ 同一图标词汇但状态独立
 * 记忆（i2t.overviewBlocks）。块视图一书一块，颜色取主导状态
 * （escalated>pending>fixed>done），点块把搜索框填上书名、借既有过滤跳到
 * 该书的会话组（比"选中该书第一个会话"简单：不依赖进度条目与会话的对齐）。
 */
const overviewBlocks = ref(storeGet('i2t.overviewBlocks') === '1')
function toggleOverviewBlocks() {
  overviewBlocks.value = !overviewBlocks.value
  storeSet('i2t.overviewBlocks', overviewBlocks.value ? '1' : '0')
}

function onOverviewBookClick(b: Img2TextBook) {
  const stem = b.book.replace(/\.md$/i, '')
  state.filter = stem.toLowerCase()
}

/*
 * 方块视图（P8）：矢量图/章节转换动辄几十个会话，列表扫不过来。开了之后
 * 会话渲染成一排 20px 小方块（逐图会话显示书内序号），悬浮出完整说明，
 * 记忆落在 localStorage 与折叠记忆同库。
 */
const blockView = ref(storeGet('side.blockView') === '1')
function toggleBlockView() {
  blockView.value = !blockView.value
  storeSet('side.blockView', blockView.value ? '1' : '0')
}

interface StageView {
  stage: string; title: string; items: any[]; live: number
  status: StageStatus | null
  overflowKey: string; needOverflow: boolean; shown: any[]
  hiddenCount: number
}
interface GroupView {
  name: string; prefix: string; title: string; legacy: boolean
  items: any[]; live: number; matched: boolean
  progress: string | null
  stages: StageView[]; multi: boolean
  /* T53：img2text 板块（无过滤词时）按图聚合的调用链；null = 走阶段层。 */
  chains: SessionChain[] | null
  chainSingles: any[]
}

const groups = computed<GroupView[]>(() => {
  // 搜索时保持平铺（历史会话能被搜到、照常命中），不做链聚合——取舍见 T53。
  const chaining = state.board === 'img2text' && !state.filter
  return buildGroups().map((g) => {
    const stages0 = g.items.length ? (g.items[0] as any).projectStages : null
    const stageKeys = Object.keys(g.stages).sort((a, b) => {
      const d = stageRank(a) - stageRank(b)
      return d !== 0 ? d : a < b ? -1 : 1
    })
    const multi = stageKeys.length > 1
    const stages: StageView[] = stageKeys.map((stg) => {
      const sg = g.stages[stg]
      // 逐图会话按"书里的顺序"排，而不是按修改时间。
      const items = sg.items.slice().sort((x: any, y: any) => {
        if (x.imageOrder && y.imageOrder && x.imageOrder !== y.imageOrder) return x.imageOrder - y.imageOrder
        return y.mtime > x.mtime ? 1 : -1
      })
      const okey = groupKey('overflow', g.name + '/' + stg)
      const needOverflow = items.length > OVERFLOW_LIMIT
      const openAll = !needOverflow || isOverflowOpen(okey) || !!state.filter
      const shown = openAll ? items : items.slice(0, OVERFLOW_LIMIT)
      return {
        stage: stg, title: stageTitleOf(stg), items, live: sg.live,
        status: stageStatusText(stages0, stg, sg.live),
        overflowKey: okey, needOverflow, shown, hiddenCount: items.length - shown.length,
      }
    })
    let chains: SessionChain[] | null = null
    let chainSingles: any[] = []
    if (chaining) {
      const built = buildImg2TextChains(g.items)
      chains = built.chains
      chainSingles = built.singles
    }
    return {
      name: g.name, prefix: g.prefix, title: g.title, legacy: g.legacy,
      items: g.items, live: g.live, matched: g.matched,
      progress: projectProgressLine(stages0),
      stages, multi, chains, chainSingles,
    }
  })
})

const shownCount = computed(() => groups.value.reduce((n, g) => n + g.items.length, 0))
const emptyText = computed(() => (state.sessions.length ? '没有匹配的会话' : '没有找到 *.jsonl 会话转录'))

const footText = computed(() => {
  const live = filterByBoard(state.sessions, state.board).filter((s: any) => s.live).length
  return groups.value.length + ' 个项目 · ' + boardSessionCount.value + ' 个会话' +
    (live ? ' · ' + live + ' 个活跃' : '') + (state.filter ? ' · 匹配 ' + shownCount.value : '')
})

/* 合计：所有会话的输入/输出 token 与平均缓存命中率（按 token 加权）+ 费用。 */
const totals = computed(() => {
  const tot = { requests: 0, prompt: 0, cached: 0, completion: 0, cost: 0, costCurrency: '', unpriced: 0 }
  state.sessions.forEach((s: any) => {
    const st = s.stats
    if (!st || !st.requests) return
    tot.requests += st.requests
    tot.prompt += st.promptTokens
    tot.cached += st.cachedTokens
    tot.completion += st.completionTokens
    if (s.cost) {
      tot.cost += Number(s.cost.total) || 0
      tot.costCurrency = s.cost.currency || tot.costCurrency
    } else {
      tot.unpriced += st.requests
    }
  })
  return tot
})
const totalsText = computed(() => {
  const tot = totals.value
  if (!tot.requests) return ''
  let money = ''
  if (tot.cost > 0 || tot.costCurrency) {
    money = ' · 费用 ' + (tot.unpriced ? '≥' : '') + tot.costCurrency + tot.cost.toFixed(2)
  }
  return '合计 ' + tot.requests + ' 次请求 · 输入 ' + fmtTokens(tot.prompt) +
    ' / 输出 ' + fmtTokens(tot.completion) + ' tokens' +
    (tot.prompt ? ' · 缓存命中 ' + ((tot.cached * 100) / tot.prompt).toFixed(0) + '%' : '') + money
})
const totalsTitle = computed(() => {
  const tot = totals.value
  return tot.unpriced
    ? '有 ' + tot.unpriced + ' 次请求的模型没配价格（models.<条目>.price），金额只是下界'
    : ''
})

/* 折叠指令：旧页 bindCollapse + setGroupOpen 的合成。
 * mounted：设 open + 记基准；用户 toggle 且基准变化 → 写回记忆（frozen 除外）。
 * updated：按最新 want 程序化同步 open（记忆/选中会话变化时），不动记忆。 */
const vCollapse = {
  mounted(el: HTMLDetailsElement, binding: any) {
    const { key, want, frozen } = binding.value
    el.open = !!want
    el.dataset.open = want ? '1' : '0'
    el.addEventListener('toggle', () => {
      const now = el.open ? '1' : '0'
      if (el.dataset.open === now) return
      el.dataset.open = now
      if (frozen) return // 过滤时强制展开，不要把"被强制"当成用户选择写回记忆
      setCollapsed(key, !el.open)
    })
  },
  updated(el: HTMLDetailsElement, binding: any) {
    const { want } = binding.value
    const flag = want ? '1' : '0'
    if (el.dataset.open === flag) return
    el.dataset.open = flag
    el.open = !!want
  },
}

function projWantOpen(g: GroupView): boolean {
  const cur: any = state.current
  const curProj = cur ? cur.project || projectOf(cur.id) : ''
  return groupWantOpen(groupKey('proj', g.name), !!curProj && curProj === g.name)
}

function stageWantOpen(g: GroupView, sv: StageView): boolean {
  const cur: any = state.current
  const curProj = cur ? cur.project || projectOf(cur.id) : ''
  const curStage = cur ? cur.stage || 'session' : ''
  return groupWantOpen(groupKey('stage', g.name + '/' + sv.stage),
    !!curProj && curProj === g.name && curStage === sv.stage)
}

function rowTitle(s: any): string {
  return sessionTip(s)
}

/* ---------- T53：img2text 调用链 ---------- */

/* 历史折叠默认收起；当前选中的会话在链里（含历史）时自动展开，手动收起过则不展开。 */
function chainWantOpen(g: GroupView, c: SessionChain): boolean {
  const cur: any = state.current
  const holds = !!cur && c.items.some((s) => s.id === cur.id)
  return groupWantOpen(groupKey('chain', g.name + '/' + c.key), holds)
}

/* 方块视图一块一链：悬浮说明列出链上全部会话（新 → 旧）。 */
function chainTip(c: SessionChain): string {
  const lines = [sessionTip(c.main)]
  if (c.rest.length) {
    lines.push('—— 调用链（共 ' + c.items.length + ' 次会话，新 → 旧）——')
    c.items.forEach((s) => lines.push(img2textMemberTag(s.id) + ' · ' + sessionTitleOf(s)))
  }
  return lines.join('\n')
}

function onRowClick(s: any) {
  selectSession(s.id)
}

function onInfoClick(ev: MouseEvent, s: any) {
  ev.stopPropagation()
  selectSession(s.id)
  openDetails()
}

function onMoreClick(okey: string) {
  setOverflowOpen(okey, true)
}
</script>

<template>
  <aside id="sidebar-col" class="sidebar-col">
    <div class="side-head">
      <span class="side-title">工作区</span>
      <button
        id="side-view-toggle" class="icon-btn" :class="{ on: blockView }" type="button"
        :title="blockView ? '切回列表视图' : '切到方块视图（会话多时好扫；悬浮看说明）'"
        @click="toggleBlockView"
      >{{ blockView ? '☰' : '▦' }}</button>
      <button id="refresh" class="icon-btn" type="button" title="重新扫描会话" @click="refreshIndex">⟳</button>
    </div>
    <div v-if="showBoardSwitch" class="board-tabs" role="group" aria-label="板块">
      <button
        id="board-latex" class="tab-toggle" type="button"
        :aria-pressed="state.board === 'latex' ? 'true' : 'false'"
        title="LaTeX 板块：主根（latex）会话"
        @click="setBoard('latex')"
      >LaTeX</button>
      <button
        id="board-img2text" class="tab-toggle" type="button"
        :aria-pressed="state.board === 'img2text' ? 'true' : 'false'"
        title="img2text 板块：逐图文字化会话与进度"
        @click="setBoard('img2text')"
      >img2text</button>
    </div>
    <div class="side-search">
      <input id="search" v-model="state.filter" type="search" placeholder="过滤：会话名 / 阶段 / 项目…" autocomplete="off">
    </div>
    <div class="side-list-wrap">
      <div id="session-list" ref="listEl" class="session-list" role="tree" aria-label="会话列表">
        <details
          v-if="progressBooks.length"
          v-collapse="{ key: 'overview:img2text', want: overviewWantOpen(), frozen: false }"
          class="i2t-overview"
          aria-label="img2text 进度概览"
        >
          <summary class="i2t-ov-head" title="img2text 逐书进度概览（点击折叠/展开，状态会被记住）">
            <span class="row-slot"><span class="row-caret"></span></span>
            <span class="i2t-ov-title">进度概览</span>
            <span class="i2t-ov-agg">{{ overviewHeadText }}</span>
            <button
              class="icon-btn" :class="{ on: overviewBlocks }" type="button"
              :title="overviewBlocks ? '概览切回列表视图' : '概览切到方块视图（一书一块，色块按主导状态；点块搜该书的会话）'"
              @click.stop="toggleOverviewBlocks"
            >{{ overviewBlocks ? '☰' : '▦' }}</button>
          </summary>
          <div v-if="overviewBlocks" class="i2t-blocks">
            <button
              v-for="b in progressBooks"
              :key="b.book"
              class="i2t-block"
              :class="'i2t-' + bookDominantState(b)"
              type="button"
              :title="bookTip(b)"
              :aria-label="b.book + '：完成 ' + b.done + ' · 修复 ' + b.fixed + ' · 升级 ' + b.escalated + ' · 进行中 ' + b.running + ' · 待处理 ' + b.pending"
              @click="onOverviewBookClick(b)"
            ></button>
          </div>
          <template v-else>
            <div v-for="b in progressBooks" :key="b.book" class="i2t-book">
              <div class="i2t-book-head">
                <span class="i2t-book-name" :title="b.book">{{ b.book }}</span>
                <span class="i2t-book-count">{{ b.done }}/{{ b.total }}</span>
              </div>
              <div class="i2t-bar" role="img" :aria-label="'完成 ' + b.done + ' · 修复 ' + b.fixed + ' · 升级 ' + b.escalated + ' · 进行中 ' + b.running + ' · 待处理 ' + b.pending">
                <span class="i2t-seg i2t-done" :style="{ width: barPct(b.done, b.total) + '%' }"></span>
                <span class="i2t-seg i2t-fixed" :style="{ width: barPct(b.fixed, b.total) + '%' }"></span>
                <span class="i2t-seg i2t-esc" :style="{ width: barPct(b.escalated, b.total) + '%' }"></span>
                <span class="i2t-seg i2t-run" :style="{ width: barPct(b.running, b.total) + '%' }"></span>
                <span class="i2t-seg i2t-pend" :style="{ width: barPct(b.pending, b.total) + '%' }"></span>
              </div>
              <div class="i2t-book-meta">
                <span class="i2t-meta-done">完成 {{ b.done }}</span>
                <span v-if="b.fixed" class="i2t-meta-fixed">修复 {{ b.fixed }}</span>
                <span v-if="b.escalated" class="i2t-meta-esc">升级 {{ b.escalated }}</span>
                <span v-if="b.running" class="i2t-meta-run">进行中 {{ b.running }}</span>
                <span v-if="b.pending" class="i2t-meta-pend">待处理 {{ b.pending }}</span>
              </div>
            </div>
          </template>
        </details>
        <details
          v-for="g in groups"
          :key="g.name"
          v-collapse="{ key: 'proj:' + g.name, want: g.matched ? true : projWantOpen(g), frozen: g.matched }"
          class="proj-group"
          :data-project="g.name"
        >
          <summary class="proj-row" :title="g.name">
            <span class="row-slot row-folder">
              <svg class="folder closed" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M1.5 3.5h4l1.5 2h7.5v7a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1v-9Z" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/></svg>
              <!-- T19：旧 open 图标背板左斜边与前盖左斜边交叉，选中态左侧看着像被糊住；
     重画——背板左缘垂直直线（底部只留短 stub），前盖斜边整个在背板内侧，互不穿插。 -->
<svg class="folder open" viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M14.5 8V5.5a1 1 0 0 0-1-1H7.2L5.7 3.5H2.5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h2.2" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" stroke-linecap="round"/><path d="M4.9 14.5 6.7 7.5h7.7l-1.8 7Z" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/></svg>
            </span>
            <span class="row-body">
              <span v-if="g.prefix" class="proj-prefix">{{ g.prefix }}</span>
              <span class="proj-title">{{ g.title }}</span>
            </span>
            <span class="row-meta">{{ g.items.length }} 个会话</span>
            <span v-if="g.legacy" class="proj-note" title="这个输出根本身就是一个工程（work/、progress.json 等直接挂在它下面），是引入多项目布局之前的形态；新布局是「输出根/书名/」。">旧版单项目</span>
            <span v-if="g.live" class="dot live"></span>
          </summary>
          <div>
            <div v-if="g.progress" class="proj-progress" title="progress.json 里各阶段的当前状态">{{ g.progress }}</div>
            <!-- T53：img2text 板块按图聚合成调用链——主行是最新一次会话，
                 旁边的「历史 N」展开器点开看该图的全部历史会话。 -->
            <template v-if="g.chains">
              <div v-if="blockView" class="session-blocks">
                <button
                  v-for="c in g.chains"
                  :key="c.key"
                  class="session-block"
                  :class="{ active: state.current && c.items.some((s) => s.id === (state.current as any).id), live: c.live > 0, err: !c.live && c.main.endState === 'error', pend: !c.live && !c.main.endState }"
                  type="button"
                  role="treeitem"
                  :data-id="c.main.id"
                  :title="chainTip(c)"
                  @click="onRowClick(c.main)"
                >{{ c.main.imageOrder || c.main.chapterOrder || '' }}</button>
              </div>
              <template v-else>
                <div v-for="c in g.chains" :key="c.key" class="chain">
                  <button
                    class="session-row sub1"
                    :class="{ active: state.current && state.current.id === c.main.id }"
                    type="button"
                    role="treeitem"
                    :data-id="c.main.id"
                    :title="chainTip(c)"
                    @click="onRowClick(c.main)"
                  >
                    <span class="row-slot"><span class="dot" :class="{ live: c.live > 0 }"></span></span>
                    <InlineMD tag="span" class="row-title" :text="sessionTitleOf(c.main)"></InlineMD>
                    <span v-if="usageChipText(c.main)" class="row-chip usage-chip">{{ usageChipText(c.main) }}</span>
                    <span class="row-time">{{ relTime(c.main.mtime) }}</span>
                    <span class="row-actions">
                      <button class="icon-btn" type="button" title="打开详情面板（元信息 / 指标）" @click.stop="onInfoClick($event, c.main)">ⓘ</button>
                    </span>
                  </button>
                  <details
                    v-if="c.rest.length"
                    v-collapse="{ key: 'chain:' + g.name + '/' + c.key, want: chainWantOpen(g, c), frozen: false }"
                    class="chain-hist"
                  >
                    <summary class="chain-hist-row" :title="'同一张图的全部历史会话（新 → 旧）：逐图分析 → stage0 → stage1 → prev 轮转'">
                      <span class="row-slot"><span class="row-caret"></span></span>
                      <span class="chain-hist-label">历史 {{ c.rest.length }}</span>
                    </summary>
                    <button
                      v-for="s in c.rest"
                      :key="s.id"
                      class="session-row sub2"
                      :class="{ active: state.current && state.current.id === s.id }"
                      type="button"
                      role="treeitem"
                      :data-id="s.id"
                      :title="rowTitle(s)"
                      @click="onRowClick(s)"
                    >
                      <span class="row-slot"><span class="dot" :class="{ live: s.live }"></span></span>
                      <InlineMD tag="span" class="row-title" :text="sessionTitleOf(s)"></InlineMD>
                      <span class="row-chip chain-tag">{{ img2textMemberTag(s.id) }}</span>
                      <span class="row-time">{{ relTime(s.mtime) }}</span>
                      <span class="row-actions">
                        <button class="icon-btn" type="button" title="打开详情面板（元信息 / 指标）" @click.stop="onInfoClick($event, s)">ⓘ</button>
                      </span>
                    </button>
                  </details>
                </div>
                <button
                  v-for="s in g.chainSingles"
                  :key="s.id"
                  class="session-row sub1"
                  :class="{ active: state.current && state.current.id === s.id }"
                  type="button"
                  role="treeitem"
                  :data-id="s.id"
                  :title="rowTitle(s)"
                  @click="onRowClick(s)"
                >
                  <span class="row-slot"><span class="dot" :class="{ live: s.live }"></span></span>
                  <InlineMD tag="span" class="row-title" :text="sessionTitleOf(s)"></InlineMD>
                  <span v-if="usageChipText(s)" class="row-chip usage-chip">{{ usageChipText(s) }}</span>
                  <span class="row-time">{{ relTime(s.mtime) }}</span>
                  <span class="row-actions">
                    <button class="icon-btn" type="button" title="打开详情面板（元信息 / 指标）" @click.stop="onInfoClick($event, s)">ⓘ</button>
                  </span>
                </button>
              </template>
            </template>
            <template v-if="!g.chains">
            <template v-for="sv in g.stages" :key="sv.stage">
              <details
                v-if="g.multi"
                v-collapse="{ key: 'stage:' + g.name + '/' + sv.stage, want: stageWantOpen(g, sv), frozen: false }"
                class="stage-group"
                :data-project="g.name"
                :data-stage="sv.stage"
              >
                <summary class="stage-row">
                  <span class="row-slot"><span class="row-caret"></span></span>
                  <span class="row-body">{{ sv.title }}</span>
                  <span class="row-meta">{{ sv.items.length }} 个会话</span>
                  <span v-if="sv.status" class="stage-progress" :class="sv.status.state" :title="sv.status.title">{{ sv.status.text }}</span>
                </summary>
                <div v-if="blockView" class="session-blocks">
                  <button
                    v-for="s in sv.items"
                    :key="s.id"
                    class="session-block"
                    :class="{ active: state.current && state.current.id === s.id, live: s.live, err: !s.live && s.endState === 'error', pend: !s.live && !s.endState }"
                    type="button"
                    role="treeitem"
                    :data-id="s.id"
                    :title="rowTitle(s)"
                    @click="onRowClick(s)"
                  >{{ s.imageOrder || s.chapterOrder || '' }}</button>
                </div>
                <div v-else>
                  <button
                    v-for="s in sv.shown"
                    :key="s.id"
                    class="session-row sub2"
                    :class="{ active: state.current && state.current.id === s.id }"
                    type="button"
                    role="treeitem"
                    :data-id="s.id"
                    :title="rowTitle(s)"
                    @click="onRowClick(s)"
                  >
                    <span class="row-slot"><span class="dot" :class="{ live: s.live }"></span></span>
                    <InlineMD tag="span" class="row-title" :text="sessionTitleOf(s)"></InlineMD>
                    <span v-if="usageChipText(s)" class="row-chip usage-chip">{{ usageChipText(s) }}</span>
                    <span v-if="s.imageName" class="row-chip image-chip" :title="imageTipText(s)">{{ imageChipText(s) }}</span>
                    <span class="row-time">{{ relTime(s.mtime) }}</span>
                    <span class="row-actions">
                      <button class="icon-btn" type="button" title="打开详情面板（元信息 / 指标）" @click.stop="onInfoClick($event, s)">ⓘ</button>
                    </span>
                  </button>
                  <button v-if="sv.needOverflow" class="session-overflow" type="button" @click="onMoreClick(sv.overflowKey)">
                    更多会话（还有 {{ sv.hiddenCount }} 个）
                  </button>
                </div>
              </details>
              <template v-else>
                <div v-if="blockView" class="session-blocks">
                  <button
                    v-for="s in sv.items"
                    :key="s.id"
                    class="session-block"
                    :class="{ active: state.current && state.current.id === s.id, live: s.live, err: !s.live && s.endState === 'error', pend: !s.live && !s.endState }"
                    type="button"
                    role="treeitem"
                    :data-id="s.id"
                    :title="rowTitle(s)"
                    @click="onRowClick(s)"
                  >{{ s.imageOrder || s.chapterOrder || '' }}</button>
                </div>
                <template v-else>
                  <button
                    v-for="s in sv.shown"
                    :key="s.id"
                    class="session-row sub1"
                    :class="{ active: state.current && state.current.id === s.id }"
                    type="button"
                    role="treeitem"
                    :data-id="s.id"
                    :title="rowTitle(s)"
                    @click="onRowClick(s)"
                  >
                    <span class="row-slot"><span class="dot" :class="{ live: s.live }"></span></span>
                    <InlineMD tag="span" class="row-title" :text="sessionTitleOf(s)"></InlineMD>
                    <span v-if="usageChipText(s)" class="row-chip usage-chip">{{ usageChipText(s) }}</span>
                    <span v-if="s.imageName" class="row-chip image-chip" :title="imageTipText(s)">{{ imageChipText(s) }}</span>
                    <span class="row-time">{{ relTime(s.mtime) }}</span>
                    <span class="row-actions">
                      <button class="icon-btn" type="button" title="打开详情面板（元信息 / 指标）" @click.stop="onInfoClick($event, s)">ⓘ</button>
                    </span>
                  </button>
                  <button v-if="sv.needOverflow" class="session-overflow" type="button" @click="onMoreClick(sv.overflowKey)">
                    更多会话（还有 {{ sv.hiddenCount }} 个）
                  </button>
                </template>
              </template>
            </template>
            </template>
          </div>
        </details>
        <div v-if="!shownCount" class="empty">{{ emptyText }}</div>
      </div>
      <div class="list-fade" aria-hidden="true"></div>
    </div>
    <div id="side-totals" class="side-totals" :class="{ hidden: !totalsText }">{{ totalsText }}</div>
    <div class="side-status">
      <div id="root-path" class="root-path" title="扫描根目录">{{ state.root || '—' }}</div>
      <div id="side-foot" class="side-foot">{{ footText }}</div>
    </div>
  </aside>
</template>

<style scoped>
/* ============================ 侧栏 ============================ */

.side-head {
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  height: 44px;
  padding: 0 8px 0 12px;
}

.side-title {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: .2px;
  color: var(--muted);
  white-space: nowrap;
  overflow: hidden;
}

.frame[data-sidebar-collapsed] .side-title,
.frame[data-sidebar-collapsed] .side-search,
.frame[data-sidebar-collapsed] .side-status,
.frame[data-sidebar-collapsed] .side-totals {
 display: none; 
}

.frame[data-sidebar-collapsed] .side-head {
 padding: 0; justify-content: center; 
}

.side-search {
 flex: none; padding: 0 10px 8px; 
}

/* P18：板块切换行（LaTeX / img2text）——按钮本体沿用 .tab-toggle 词汇。 */
.board-tabs {
  flex: none;
  display: flex;
  gap: 2px;
  padding: 0 10px 8px;
}

.board-tabs .tab-toggle {
  flex: 1;
  justify-content: center;
  height: 22px;
  border: .5px solid var(--border);
  border-radius: 6px;
}

.frame[data-sidebar-collapsed] .board-tabs {
  display: none;
}

.side-search input {
  width: 100%;
  height: 28px;
  background: transparent;
  border: .5px solid var(--border);
  border-radius: 10px;
  color: var(--text);
  font: inherit;
  font-size: 13px;
  padding: 0 9px;
}

.side-search input::placeholder {
 color: var(--caption); 
}

.side-search input:focus {
 outline: none; border-color: var(--accent); background: var(--bg); 
}

.side-list-wrap {
 position: relative; flex: 1; min-height: 0; display: flex; 
}

.session-list {
 flex: 1; overflow-y: auto; padding: 0 6px 20px; 
}

/* 列表底部的渐隐遮罩（DSH 的 list fade） */
.list-fade {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 24px;
  pointer-events: none;
  background: linear-gradient(to bottom, transparent, var(--sidebar-bg));
}

.frame[data-sidebar-collapsed] .list-fade {
 display: none; 
}

/* 组行（项目 / 阶段）= DSH 的 projectRow：34px、padding 0 8px、圆角 8px、
   左边一个 16px 固定插槽；**没有按层级递增的缩进**。 */
.proj-group {
 margin: 0 0 4px; 
}

.proj-row,
.stage-row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 34px;
  padding: 0 8px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  user-select: none;
  list-style: none;
  color: var(--text);
}

.proj-row::-webkit-details-marker, .stage-row::-webkit-details-marker {
 display: none; 
}

.proj-row:hover, .stage-row:hover {
 background: var(--hover); 
}

.stage-row {
 height: 28px; 
}

.row-slot {
  flex: none;
  width: 16px;
  height: 20px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--caption);
}

.row-caret {
  width: 0; height: 0;
  border-left: 4px solid currentColor;
  border-top: 3.5px solid transparent;
  border-bottom: 3.5px solid transparent;
  transition: transform .15s var(--ease);
}

details[open] > summary .row-caret {
 transform: rotate(90deg); 
}

.row-body {
 flex: 1; min-width: 0; display: flex; align-items: baseline; gap: 4px; 
}

.row-title {
  min-width: 0;
  font-size: 14px;
  line-height: 20px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.stage-row .row-title {
 font-size: 12.5px; line-height: 20px; font-weight: 600; color: var(--muted); 
}

.proj-prefix {
 flex: none; color: var(--caption); font-family: var(--mono); font-size: 11px; 
}

.proj-title {
 min-width: 0; font-size: 14px; line-height: 20px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; 
}

.row-meta {
  flex: none;
  color: var(--caption);
  font-size: 12px;
  line-height: 20px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.proj-note {
  flex: none;
  color: var(--caption);
  font-size: 10.5px;
  line-height: 16px;
  border: .5px solid var(--border);
  border-radius: 999px;
  padding: 0 6px;
}

.stage-progress {
 flex: none; color: var(--caption); font-family: var(--mono); font-size: 10.5px; 
}

.stage-progress.done {
 color: var(--ok); 
}

.proj-progress {
  padding: 2px 8px 4px;
  color: var(--caption);
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 1.7;
  word-break: break-word;
}

.frame[data-sidebar-collapsed] .proj-progress {
 display: none; 
}

/* 会话行 = DSH 的 sessionRow：32px、padding 0 8px、圆角 8px、
   16px 固定插槽（状态点）、标题 14px/20px 省略号、次要信息与时间 12px 靠右，
   hover 时时间让位给行内操作按钮。 */
.session-row {
  display: flex;
  align-items: center;
  gap: 0;
  width: 100%;
  height: 32px;
  padding: 0 8px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background .12s;
}

.session-row:hover {
 background: var(--hover); 
}

.session-row.active {
 background: var(--active); 
}

.session-row .row-title {
 flex: 1; margin: 0 6px 0 4px; 
}

.session-row.active .row-title {
 font-weight: 500; 
}

.session-row .row-time {
 flex: none; margin-left: 8px; color: var(--caption); font-size: 12px; line-height: 20px; 
}

.session-row .row-chip + .row-chip {
 margin-left: 6px; 
}

.session-row .row-actions {
 display: none; flex: none; align-items: center; gap: 10px; 
}

.session-row:hover .row-actions {
 display: inline-flex; 
}

.session-row:hover .row-time {
 display: none; 
}

.session-row .row-chip {
 color: var(--caption); font-size: 11px; white-space: nowrap; 
}

.session-row .row-chip.usage-chip {
 font-family: var(--mono); 
}

.session-row.active .row-slot {
 color: var(--accent); 
}

/* ---- P8 侧栏改版（仿 DSH 的树状侧栏）------------------------------------
 * 层级递进：项目行不缩进（文件夹图标打头），阶段行缩进一层，会话行再进
 * 一层；展开的项目用「打开的文件夹」图标。会话方块视图：一格 20px、逐图
 * 会话带书内序号，悬浮看完整说明，活跃会话实心、运行中有绿圈。 */
.row-folder .folder {
 display: none; color: var(--dim); 
}

.row-folder .folder.closed {
 display: inline-block; 
}

details[open] > .proj-row .row-folder .folder.closed {
 display: none; 
}

details[open] > .proj-row .row-folder .folder.open {
 display: inline-block; color: var(--accent); 
}

.stage-row {
 padding-left: 24px; 
}

.stage-progress.running {
 color: var(--ok); font-weight: 600; 
}

.stage-progress.busy {
 color: var(--warn); 
}

.stage-progress.idle {
 color: var(--caption); opacity: .72; 
}

.session-row.sub1 {
 padding-left: 24px; 
}

.session-row.sub2 {
 padding-left: 40px; 
}

.session-overflow.sub2, .session-overflow.sub1 {
 padding-left: 40px; 
}

.session-row.sub1 .session-overflow, .stage-group .session-overflow {
 padding-left: 40px; 
}

.session-blocks {
  display: flex;
  flex-wrap: wrap;
  gap: 5px;
  padding: 4px 10px 8px 40px;
}

.session-block {
  position: relative;
  width: 20px;
  height: 20px;
  padding: 0;
  border: .5px solid var(--border);
  border-radius: 6px;
  background: var(--hover);
  color: var(--muted);
  font-family: var(--mono);
  font-size: 9px;
  line-height: 19px;
  text-align: center;
  cursor: pointer;
  transition: background .12s, border-color .12s;
}

.session-block:hover {
 background: var(--active); border-color: var(--border-strong); 
}

.session-block.active {
 background: var(--accent); border-color: var(--accent); color: #fff; 
}

.session-block.live {
 border-color: var(--ok); box-shadow: 0 0 0 2px rgba(34, 197, 94, .22); 
}

.session-block.live.active {
 border-color: var(--accent); box-shadow: 0 0 0 2px rgba(65, 118, 230, .3); 
}

/* T22：方块状态着色——红=有过回执但从没提交（错误终止），暗=还没开工；
   正常结束就是默认样子，运行中绿圈见 .live。 */
.session-block.err {
 background: rgba(239, 68, 68, .14);
 border-color: rgba(239, 68, 68, .55);
 color: #ef4444;
}

.session-block.err:hover {
 background: rgba(239, 68, 68, .24);
 border-color: #ef4444;
}

.session-block.err.active {
 background: var(--accent);
 border-color: var(--accent);
 color: #fff;
}

.session-block.pend {
 opacity: .42;
}

.session-block.pend:hover, .session-block.pend.active {
 opacity: 1;
}

.icon-btn.on {
 color: var(--accent); 
}

/* ---- T53：img2text 调用链（同图会话聚合） ------------------------------
 * 主行是最新一次会话；下面挂一个「历史 N」展开器（details），点开出该图的
 * 全部历史会话（逐图分析 → stage0 → stage1 → prev 轮转），行内带来源标签。 */
.chain-hist {
  margin: 0;
}

.chain-hist-row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 24px;
  padding: 0 8px 0 40px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  user-select: none;
  list-style: none;
  color: var(--caption);
  font-size: 12px;
}

.chain-hist-row::-webkit-details-marker {
  display: none;
}

.chain-hist-row:hover {
  background: var(--hover);
  color: var(--muted);
}

.chain-hist-row .row-slot {
  height: 16px;
}

.chain-hist-label {
  min-width: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chain-hist .session-row.sub2 {
  padding-left: 56px;
}

.row-chip.chain-tag {
  flex: none;
  color: var(--caption);
  font-size: 11px;
  white-space: nowrap;
  border: .5px solid var(--border);
  border-radius: 999px;
  padding: 0 6px;
  line-height: 16px;
  margin-left: 6px;
}

.frame[data-sidebar-collapsed] .chain-hist {
  display: none;
}

/* 组内会话过多时只显示前 N 条 + 一个「更多会话」按钮（28px、左内边距 28px） */
.session-overflow {
  display: block;
  width: 100%;
  height: 28px;
  padding: 0 12px 0 28px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--caption);
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.session-overflow:hover {
 color: var(--muted); background: var(--hover); 
}

.empty {
 padding: 16px 12px; color: var(--caption); font-size: 13px; 
}

/* 侧栏折叠成 56px 轨道：只留插槽（点 / 箭头），标题与次要信息全隐藏。 */
.frame[data-sidebar-collapsed] .session-list {
 padding: 0 8px 20px; 
}

.frame[data-sidebar-collapsed] .proj-row,
.frame[data-sidebar-collapsed] .stage-row,
.frame[data-sidebar-collapsed] .session-row {
 justify-content: center; padding: 0; overflow: hidden; 
}

.frame[data-sidebar-collapsed] .proj-row .row-body,
.frame[data-sidebar-collapsed] .stage-row .row-body,
.frame[data-sidebar-collapsed] .proj-row .row-meta,
.frame[data-sidebar-collapsed] .stage-row .row-meta,
.frame[data-sidebar-collapsed] .stage-progress,
.frame[data-sidebar-collapsed] .proj-note,
.frame[data-sidebar-collapsed] .session-row .row-title,
.frame[data-sidebar-collapsed] .session-row .row-time,
.frame[data-sidebar-collapsed] .session-row .row-actions,
.frame[data-sidebar-collapsed] .session-row .row-chip {
 display: none; 
}

.frame[data-sidebar-collapsed] .session-row .row-slot {
 width: 20px;
}

/* P14：折叠轨道里行的可发现性——行留一点纵向呼吸、hover/active 圆角高亮，
   当前会话保持 var(--active) 底，扫一眼就能定位（悬浮仍有 title 全文）。 */
.frame[data-sidebar-collapsed] .proj-row,
.frame[data-sidebar-collapsed] .stage-row,
.frame[data-sidebar-collapsed] .session-row {
 min-height: 28px;
 border-radius: 8px;
}

.frame[data-sidebar-collapsed] .proj-row:hover,
.frame[data-sidebar-collapsed] .stage-row:hover,
.frame[data-sidebar-collapsed] .session-row:hover {
 background: var(--hover);
}

.frame[data-sidebar-collapsed] .session-row.active {
 background: var(--active);
}

.frame[data-sidebar-collapsed] .session-blocks {
 display: none;
}

.frame[data-sidebar-collapsed] .row-folder .folder {
 width: 16px; height: 16px; 
}

.side-totals {
  flex: none;
  border-top: .5px solid var(--border);
  padding: 8px 12px;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.7;
}

.side-status {
 flex: none; border-top: .5px solid var(--border); padding: 6px 12px 10px; 
}

.root-path {
  color: var(--caption);
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  direction: rtl;
  text-align: left;
}

.side-foot {
  color: var(--caption);
  font-size: 10.5px;
  line-height: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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

/* 组内会话过多时只显示前 N 条 + 一个「更多会话」按钮（28px、左内边距 28px） */
.session-overflow {
  display: block;
  width: 100%;
  height: 28px;
  padding: 0 12px 0 28px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--caption);
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.session-overflow:hover {
 color: var(--muted); background: var(--hover); 
}

.empty {
 padding: 16px 12px; color: var(--caption); font-size: 13px; 
}

/* 侧栏折叠成 56px 轨道：只留插槽（点 / 箭头），标题与次要信息全隐藏。 */
.frame[data-sidebar-collapsed] .session-list {
 padding: 0 8px 20px; 
}

.frame[data-sidebar-collapsed] .proj-row,
.frame[data-sidebar-collapsed] .stage-row,
.frame[data-sidebar-collapsed] .session-row {
 justify-content: center; padding: 0; overflow: hidden; 
}

.frame[data-sidebar-collapsed] .proj-row .row-body,
.frame[data-sidebar-collapsed] .stage-row .row-body,
.frame[data-sidebar-collapsed] .proj-row .row-meta,
.frame[data-sidebar-collapsed] .stage-row .row-meta,
.frame[data-sidebar-collapsed] .stage-progress,
.frame[data-sidebar-collapsed] .proj-note,
.frame[data-sidebar-collapsed] .session-row .row-title,
.frame[data-sidebar-collapsed] .session-row .row-time,
.frame[data-sidebar-collapsed] .session-row .row-actions,
.frame[data-sidebar-collapsed] .session-row .row-chip {
 display: none; 
}

.frame[data-sidebar-collapsed] .session-row .row-slot {
 width: 20px;
}

/* P14：折叠轨道里行的可发现性——行留一点纵向呼吸、hover/active 圆角高亮，
   当前会话保持 var(--active) 底，扫一眼就能定位（悬浮仍有 title 全文）。 */
.frame[data-sidebar-collapsed] .proj-row,
.frame[data-sidebar-collapsed] .stage-row,
.frame[data-sidebar-collapsed] .session-row {
 min-height: 28px;
 border-radius: 8px;
}

.frame[data-sidebar-collapsed] .proj-row:hover,
.frame[data-sidebar-collapsed] .stage-row:hover,
.frame[data-sidebar-collapsed] .session-row:hover {
 background: var(--hover);
}

.frame[data-sidebar-collapsed] .session-row.active {
 background: var(--active);
}

.frame[data-sidebar-collapsed] .session-blocks {
 display: none;
}

.frame[data-sidebar-collapsed] .row-folder .folder {
 width: 16px; height: 16px; 
}

.side-totals {
  flex: none;
  border-top: .5px solid var(--border);
  padding: 8px 12px;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.7;
}

.side-status {
 flex: none; border-top: .5px solid var(--border); padding: 6px 12px 10px; 
}

.root-path {
  color: var(--caption);
  font-family: var(--mono);
  font-size: 10.5px;
  line-height: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  direction: rtl;
  text-align: left;
}

.side-foot {
  color: var(--caption);
  font-size: 10.5px;
  line-height: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
