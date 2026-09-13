/*
 * 侧栏（与列表数据）的旧页函数照搬区：分组、排序、搜索、行文案、悬浮说明、
 * 折叠/溢出记忆、列表签名。函数名与旧页一致，方便对照与逐块复验。
 * 视觉规则在整卷样式表里；这里只产数据与文案。
 */
import { state, storeSet, fmtSize, fmtClock, relTime } from '../state'

export const ROOT_PROJECT = '（根目录）'

/* 项目 = 相对路径的第一段；根目录直挂的转录归到"（根目录）"。 */
export function projectOf(id: string): string {
  const i = String(id || '').indexOf('/')
  return i <= 0 ? ROOT_PROJECT : String(id).slice(0, i)
}

/* 项目内的子目录（work/sessions 之类），让人一眼看出这个会话是什么阶段落的盘。 */
export function subPathOf(id: string): string {
  const parts = String(id || '').split('/')
  parts.pop()
  parts.shift()
  return parts.join('/')
}

export function joinPath(...args: string[]): string {
  const parts: string[] = []
  for (const a of args) {
    const p = String(a || '').replace(/^\/+|\/+$/g, '')
    if (p) parts.push(p)
  }
  return parts.join('/')
}

export function shortSHA(value: unknown): string {
  const s = String(value || '')
  if (!s) return '—'
  return s.length > 12 ? s.slice(0, 12) : s
}

export function firstLine(text: unknown, limit?: number): string {
  const s = String(text === undefined || text === null ? '' : text)
  const lines = s.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const t = lines[i].replace(/\s+/g, ' ').trim()
    if (t) {
      const max = limit || 96
      return t.length > max ? t.slice(0, max) + '…' : t
    }
  }
  return ''
}

/*
 * The wire format keeps the provider's own field name (reasoning_content);
 * normalising it once here means the render code reads one spelling only.
 */
export function normalizeLine(line: any): any {
  if (line && line.reasoning === undefined && line.reasoning_content !== undefined) {
    line.reasoning = line.reasoning_content
  }
  return line
}

export function normalizeLines(lines: any[] | null | undefined): any[] {
  return (lines || []).map(normalizeLine)
}

/* ---------- 指标短写 ---------- */

export function fmtTokens(n: unknown): string {
  const v = Number(n) || 0
  if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(v >= 1e4 ? 0 : 1) + 'k'
  return String(v)
}

export function fmtDur(ms: unknown): string {
  const v = Number(ms) || 0
  if (v < 1000) return v + 'ms'
  if (v < 60000) return (v / 1000).toFixed(1) + 's'
  const m = Math.floor(v / 60000)
  const sec = Math.round((v % 60000) / 1000)
  return m + 'm' + (sec < 10 ? '0' : '') + sec + 's'
}

/*
 * 一处计数的**唯一**文案来源：按显示单位开关给出字符数或 token 数。
 * chars 是精确字符数；tokens 是 Go 侧本地估算（由页面数据下发，前端不自己
 * 算），所以 token 形态一律带 ≈。
 */
export function countText(chars: unknown, tokens: unknown): string {
  if (state.unit === 'char') return (Number(chars) || 0) + ' 字符'
  return '≈ ' + fmtTokens(tokens) + ' tokens'
}

/* 轨迹表 / 列里的短形态：单位由列名写明，值只有数字。 */
export function countValue(chars: unknown, tokens: unknown): string {
  if (state.unit === 'char') return String(Number(chars) || 0)
  return '≈ ' + fmtTokens(tokens)
}

export function unitLabel(): string {
  return state.unit === 'char' ? '字符' : 'tokens'
}

/* t="usage" 行集合（指标聚合的输入）。 */
export function usageLines(lines: any[]): any[] {
  const out: any[] = []
  ;(lines || []).forEach((l) => {
    if (l && l.t === 'usage' && l.stats) out.push(l)
  })
  return out
}

export function aggregate(usages: any[]): any {
  if (!usages.length) return null
  const agg: any = {
    requests: 0, promptTokens: 0, cachedTokens: 0, completionTokens: 0, reasoningTokens: 0,
    durationMs: 0, ttftMs: 0, genMs: 0, streamed: false, firstTs: '', lastTs: '', perRequest: usages,
  }
  usages.forEach((l) => {
    const st = l.stats
    agg.requests += 1
    agg.promptTokens += Number(st.promptTokens) || 0
    agg.cachedTokens += Number(st.cachedTokens) || 0
    agg.completionTokens += Number(st.completionTokens) || 0
    agg.reasoningTokens += Number(st.reasoningTokens) || 0
    const dur = Number(st.durationMs) || 0
    const ttft = Number(st.ttftMs) || 0
    agg.durationMs += dur
    agg.ttftMs += ttft
    agg.genMs += Math.max(dur - ttft, 1)
    if (st.streamed) agg.streamed = true
    const ts = l.ts || ''
    if (ts && (!agg.firstTs || ts < agg.firstTs)) agg.firstTs = ts
    if (ts && ts > agg.lastTs) agg.lastTs = ts
  })
  agg.cacheHitPct = agg.promptTokens ? (agg.cachedTokens * 100) / agg.promptTokens : 0
  agg.avgTtftMs = agg.ttftMs / agg.requests
  agg.avgDurationMs = agg.durationMs / agg.requests
  agg.outputTps = agg.genMs ? (agg.completionTokens * 1000) / agg.genMs : 0
  agg.spanMs = 0
  if (agg.firstTs && agg.lastTs) {
    const t0 = Date.parse(agg.firstTs)
    const t1 = Date.parse(agg.lastTs)
    if (!isNaN(t0) && !isNaN(t1) && t1 > t0) agg.spanMs = t1 - t0
  }
  return agg
}

/* 会话列表里的 sessions 项已经带后端聚合好的 stats（不用再读转录）。 */
export function scanStats(session: any): any {
  const st = session && session.stats
  if (!st || !st.requests) return null
  return st
}

/* 费用：后端算好放在 session.cost 里（没配价格就是 null → 不显示）。 */
export function fmtCost(cost: any, unpriced?: boolean): string {
  if (!cost) return ''
  return (unpriced ? '≥' : '') + (cost.currency || '') + cost.total.toFixed(2)
}

export function statsSummary(st: any, session?: any): string {
  if (!st) return ''
  const parts = ['输入 ' + fmtTokens(st.promptTokens) + ' · 输出 ' + fmtTokens(st.completionTokens)]
  if (st.promptTokens) parts.push('缓存 ' + st.cacheHitPct.toFixed(0) + '%')
  if (st.avgTtftMs) parts.push('首字 ' + fmtDur(st.avgTtftMs))
  const money = session ? fmtCost(session.cost, false) : ''
  if (money) parts.push(money)
  return parts.join(' · ')
}

/* 工具调用的参数估算按 call id 索引一次：折叠行、输入段、轨迹表都从这里取。 */
export function indexCallEstimates(lines: any[]): void {
  ;(lines || []).forEach((line) => {
    const est = line.est || {}
    ;(line.tool_calls || []).forEach((c: any, i: number) => {
      if (c && c.id) state.callEst[c.id] = (est.calls && est.calls[i]) || 0
    })
  })
}

/* 一行的本地估算（Go 侧算好下发）：缺字段时全 0，不在这里兜算。 */
export const EMPTY_EST = { text: 0, reasoning: 0, calls: [] as number[], images: 0, imageCount: 0 }

export function estOf(line: any): { text: number; reasoning: number; calls: number[]; images: number; imageCount: number } {
  return line && line.est ? line.est : EMPTY_EST
}

/* ---------- 阶段分组与排序 ---------- */

export const STAGE_ORDER = ['vector', 'style', 'chapters', 'convert', 'checker', 'style-fix', 'figure-check']

export function stageRank(stage: string): number {
  const i = STAGE_ORDER.indexOf(stage)
  return i < 0 ? STAGE_ORDER.length : i
}

export function stageTitleOf(stage: string): string {
  const map: Record<string, string> = {
    vector: '矢量图', style: '样式', chapters: '章节划分', convert: '章节转换',
    checker: '章节核对', 'style-fix': '样式修复', 'figure-check': '逐图校验',
  }
  return map[stage] || stage || '其他会话'
}

/* progress.json 里的键是档位级的，阶段名与它并不一一对应。 */
export function progressKeyFor(stage: string): string {
  if (stage === 'vector') return 'images'
  if (stage === 'checker' || stage === 'style-fix') return 'convert'
  return stage
}

export function stageStatusText(stages: any, stage: string): { text: string; done: boolean; title: string } | null {
  if (!stages) return null
  const key = progressKeyFor(stage)
  const v = stages[key]
  if (v === undefined || v === null || v === '') return null
  const text = String(v)
  const done = /^(done|ok|true|finished|complete[d]?)$/i.test(text)
  return { text: done ? '✓ 完成' : text, done, title: 'progress.json: ' + key + ' = ' + text }
}

/* 书级进展：progress.json 原样列出各阶段状态（档位1/2 的键略有不同）。 */
export function projectProgressLine(stages: any): string | null {
  if (!stages) return null
  const order = ['images', 'style', 'chapters', 'convert', 'assemble']
  const parts: string[] = []
  order.forEach((k) => {
    if (stages[k] === undefined) return
    const done = /^(done|ok|true|finished|complete[d]?)$/i.test(String(stages[k]))
    parts.push(k + (done ? ' ✓' : ' ' + stages[k]))
  })
  if (!parts.length) return null
  return parts.join(' · ')
}

export function sessionHaystack(s: any): string {
  // 逐图会话要能按**图片名 / PDF 页 / 顺序 / 图片类型**检索（p12/#12/页12 都能命中）。
  const parts = [s.title, s.label, s.id, s.name, s.project, s.stage, s.stageTitle,
    s.imageName, s.imageType, s.imageCaption, s.imageShort, s.imageFile, s.imageLabel]
  if (s.page) parts.push('p' + s.page, 'p.' + s.page, '页' + s.page, String(s.page))
  if (s.imageOrder) parts.push('#' + s.imageOrder, '第' + s.imageOrder + '张', String(s.imageOrder))
  return parts.filter(Boolean).join(' ').toLowerCase()
}

/*
 * 逐图会话显示哪个名字：**有图注/标签就用它**，没有就用**短写文件名**。
 * 口径与 Go 侧 imageSessionTitle 一致（--list 与侧栏显示同一个名字）。
 */
export function imageDisplayName(s: any): string {
  return s.imageCaption || s.imageLabel || s.imageShort || s.imageName
}

export function sessionTitleOf(s: any): string {
  if (s.imageName && (s.page || s.imageOrder || s.imageShort || s.imageLabel || s.imageCaption)) {
    // 逐图会话的标题用**图注 / 图片名**，页与序号在从属信息里。
    return (s.stageTitle || '矢量图') + ' · ' + imageDisplayName(s)
  }
  return s.title || s.label || s.name
}

/*
 * 会话行的悬浮说明：**完整**的用量口径、元信息、图片身份与路径。
 * 行里放不下，这里承载全部细节，口径只有一处实现。
 */
export function sessionTip(s: any): string {
  const tip: string[] = [sessionTitleOf(s), s.id]
  const st = scanStats(s)
  if (st) {
    tip.push(statsSummary(st, s))
    tip.push(st.requests + ' 次 API 请求 · 输入 ' + st.promptTokens +
      ' tokens（其中 ' + st.cachedTokens + ' 命中前缀缓存）· 输出 ' + st.completionTokens +
      (st.reasoningTokens ? '（思考 ' + st.reasoningTokens + '）' : '') +
      ' · 平均耗时 ' + fmtDur(st.avgDurationMs) + ' · 平均首字 ' + fmtDur(st.avgTtftMs) +
      ' · 输出 ' + (st.outputTps || 0).toFixed(1) + ' tok/s')
  }
  const m = metaOf(s)
  if (m) {
    tip.push('含系统提示词快照：' + m.count + ' 条 t=meta 元信息行，最新一条 ' +
      countText(m.promptChars || 0, m.promptTokenEst) + (m.model ? '（模型 ' + m.model + '）' : '') +
      (m.tools ? '，含 ' + m.tools + ' 个工具定义' : ''))
  }
  if (s.imageName) {
    tip.push('来源图片: ' + imageDisplayName(s) + (s.imageType ? '（' + s.imageType + '）' : '') +
      (s.page ? '\n第 ' + s.page + ' 页' : '') + (s.imageOrder ? '\n书内第 ' + s.imageOrder + ' 张' : '') +
      (s.imageCaption ? '\n图注: ' + s.imageCaption : ''))
    if (s.imageFile) tip.push('图片文件: ' + s.imageFile)
    if (s.imageName) tip.push('图片哈希: ' + s.imageName)
    if (s.imagePath) tip.push('图片路径: ' + s.imagePath)
  }
  tip.push('消息 ' + s.messages + ' 条 · ' + fmtSize(s.size) + ' · 最后写入 ' + fmtClock(s.mtime))
  const sub = subPathOf(s.id)
  if (sub) tip.push('目录 ' + sub)
  return tip.join('\n')
}

export function metaOf(session: any): any {
  return session && session.meta && session.meta.count ? session.meta : null
}

export function usageChipText(s: any): string {
  const st = scanStats(s)
  return st ? fmtTokens(st.promptTokens) + ' / ' + fmtTokens(st.completionTokens) : ''
}

export function imageChipText(s: any): string {
  const bits: string[] = []
  if (s.page) bits.push('p' + s.page)
  if (s.imageOrder) bits.push('#' + s.imageOrder)
  if (s.imageType) bits.push(s.imageType)
  return bits.join('·')
}

/* 图片身份小标签的悬浮说明：短名之外把完整文件名 / 哈希 / 路径给全。 */
export function imageTipText(s: any): string {
  const bits = [imageDisplayName(s)]
  if (s.imageFile && s.imageFile !== imageDisplayName(s)) bits.push('文件 ' + s.imageFile)
  if (s.imageName) bits.push('哈希 ' + s.imageName)
  if (s.imagePath) bits.push(s.imagePath)
  if (s.page) bits.push('第 ' + s.page + ' 页')
  if (s.imageOrder) bits.push('书内第 ' + s.imageOrder + ' 张')
  if (s.imageType) bits.push(s.imageType)
  return bits.join('\n')
}

/* ---------- 分组构建（旧页 buildGroups 同名同序） ---------- */

export interface StageGroup { stage: string; title: string; items: any[]; live: number }
export interface ProjectGroup {
  name: string; prefix: string; title: string; legacy: boolean
  items: any[]; live: number; matched: boolean
  stages: Record<string, StageGroup>
}

export function buildGroups(): ProjectGroup[] {
  const q = state.filter
  let groups: ProjectGroup[] = []
  const byName: Record<string, ProjectGroup> = {}
  state.sessions.forEach((s: any) => {
    const name = s.project || projectOf(s.id)
    let g = byName[name]
    if (!g) {
      // 「输出根/项目名」（多项目布局）与「输出根」（旧版单项目布局）区别对待：
      // 前者以书名为标题、容器名弱化成前缀，后者标一句「旧版单项目」。
      const cut = name.lastIndexOf('/')
      g = byName[name] = {
        name,
        prefix: cut > 0 ? name.slice(0, cut + 1) : '',
        title: cut > 0 ? name.slice(cut + 1) : name,
        legacy: !!s.projectLegacy,
        items: [], live: 0, matched: false,
        stages: {},
      }
      groups.push(g)
    }
    if (s.projectLegacy) g.legacy = true
    if (q && sessionHaystack(s).indexOf(q) < 0) return
    g.items.push(s)
    if (s.live) g.live++
    // 阶段子组：项目内按流程阶段再分一层（组头带会话数与 progress.json 状态）。
    const stage = s.stage || 'session'
    let sg = g.stages[stage]
    if (!sg) sg = g.stages[stage] = { stage, title: stageTitleOf(stage), items: [], live: 0 }
    sg.items.push(s)
    if (s.live) sg.live++
  })
  if (q) {
    groups = groups.filter((g) => g.items.length > 0)
    groups.forEach((g) => { g.matched = true })
  }
  return groups
}

export function findSession(id: string): any {
  let found: any = null
  state.sessions.forEach((s: any) => {
    if (s.id === id) found = s
  })
  return found
}

/* ---------- 折叠 / 溢出记忆（旧页同名） ---------- */

export function groupKey(kind: string, name: string): string {
  return kind + ':' + name
}

export function isCollapsed(key: string): boolean {
  return state.collapsed[key] === true
}

export function hasCollapseMemory(key: string): boolean {
  return Object.prototype.hasOwnProperty.call(state.collapsed, key)
}

export function setCollapsed(key: string, collapsed: boolean): void {
  state.collapsed[key] = !!collapsed
  storeSet('collapsed', JSON.stringify(state.collapsed))
}

/*
 * 一个组要不要展开：记忆里有记录就按记忆；没记录默认**收起**，只有这一组
 * 装着当前选中的会话才展开。过滤命中时由调用方强制展开（frozen），不进这里。
 */
export function groupWantOpen(key: string, holdsCurrent: boolean): boolean {
  if (hasCollapseMemory(key)) return !isCollapsed(key)
  return !!holdsCurrent
}

export function isOverflowOpen(key: string): boolean {
  return state.overflow[key] === true
}

export function setOverflowOpen(key: string, open: boolean): void {
  if (open) state.overflow[key] = true
  else delete state.overflow[key]
  storeSet('overflow', JSON.stringify(state.overflow))
}

/* ---------- 列表签名（旧页 listSignature 同名） ---------- */

export function listSignature(): string {
  const parts: (string | number)[] = [state.filter, state.sessions.length]
  state.sessions.forEach((s: any) => {
    parts.push([s.id, s.messages,
      s.cost ? s.cost.total + (s.cost.currency || '') : '',
      s.stats ? s.stats.requests : 0,
      s.projectStages ? JSON.stringify(s.projectStages) : ''].join('~'))
  })
  return parts.join('|')
}

export { relTime }
