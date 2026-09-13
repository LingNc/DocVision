<script setup lang="ts">
/*
 * 侧栏（块 2）：三层分组（项目 → 阶段 → 会话），结构/类名/文案照旧页
 * buildGroup/sessionRow/renderSessions/renderTotals。折叠记忆走
 * v-collapse 指令（旧页 bindCollapse：程序化改 open 不写记忆，只有用户
 * 点击才写回）；选中高亮与行内文案走响应式，轮询不重建 DOM。
 */
import { computed, ref } from 'vue'
import { state, openDetails, fmtSize } from '../state'
import {
  buildGroups, groupKey, groupWantOpen, imageChipText, imageTipText,
  projectProgressLine, projectOf,
  relTime, sessionTip, sessionTitleOf, setCollapsed, setOverflowOpen,
  stageRank, stageStatusText, stageTitleOf, usageChipText, fmtTokens,
} from '../legacy/sidebar'
import { selectSession, refreshIndex } from '../data'
import InlineMD from './InlineMD.vue'

const OVERFLOW_LIMIT = 8

const listEl = ref<HTMLElement | null>(null)

interface StageView {
  stage: string; title: string; items: any[]; live: number
  status: { text: string; done: boolean; title: string } | null
  overflowKey: string; needOverflow: boolean; shown: any[]
  hiddenCount: number
}
interface GroupView {
  name: string; prefix: string; title: string; legacy: boolean
  items: any[]; live: number; matched: boolean
  progress: string | null
  stages: StageView[]; multi: boolean
}

const groups = computed<GroupView[]>(() => {
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
        status: stageStatusText(stages0, stg),
        overflowKey: okey, needOverflow, shown, hiddenCount: items.length - shown.length,
      }
    })
    return {
      name: g.name, prefix: g.prefix, title: g.title, legacy: g.legacy,
      items: g.items, live: g.live, matched: g.matched,
      progress: projectProgressLine(stages0),
      stages, multi,
    }
  })
})

const shownCount = computed(() => groups.value.reduce((n, g) => n + g.items.length, 0))
const emptyText = computed(() => (state.sessions.length ? '没有匹配的会话' : '没有找到 *.jsonl 会话转录'))

const footText = computed(() => {
  const live = state.sessions.filter((s: any) => s.live).length
  return groups.value.length + ' 个项目 · ' + state.sessions.length + ' 个会话' +
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
      <button id="refresh" class="icon-btn" type="button" title="重新扫描会话" @click="refreshIndex">⟳</button>
    </div>
    <div class="side-search">
      <input id="search" v-model="state.filter" type="search" placeholder="过滤：会话名 / 阶段 / 项目…" autocomplete="off">
    </div>
    <div class="side-list-wrap">
      <div id="session-list" ref="listEl" class="session-list" role="tree" aria-label="会话列表">
        <details
          v-for="g in groups"
          :key="g.name"
          v-collapse="{ key: 'proj:' + g.name, want: g.matched ? true : projWantOpen(g), frozen: g.matched }"
          class="proj-group"
          :data-project="g.name"
        >
          <summary class="proj-row" :title="g.name">
            <span class="row-slot"><span class="row-caret"></span></span>
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
                  <span v-if="sv.status" class="stage-progress" :class="{ done: sv.status.done }" :title="sv.status.title">{{ sv.status.text }}</span>
                  <span v-if="sv.live" class="dot live"></span>
                </summary>
                <div>
                  <button
                    v-for="s in sv.shown"
                    :key="s.id"
                    class="session-row"
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
                <button
                  v-for="s in sv.shown"
                  :key="s.id"
                  class="session-row"
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
