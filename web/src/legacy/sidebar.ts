/*
 * 侧栏（与列表数据）的旧页函数照搬区：分组、排序、搜索、行文案、悬浮说明、
 * 折叠/溢出记忆、列表签名。函数名与旧页一致，方便对照与逐块复验。
 * 视觉规则在整卷样式表里；这里只产数据与文案。
 */
import { state, storeSet, fmtSize, fmtClock, relTime } from '../state'
import { type Line, type LineEst, type Img2TextBook } from './types'

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
export function normalizeLine(line: Line): Line {
  if (line && line.reasoning === undefined && line.reasoning_content !== undefined) {
    line.reasoning = line.reasoning_content
  }
  return line
}

export function normalizeLines(lines: Line[] | null | undefined): Line[] {
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
  if (v < 1000) return Math.round(v) + 'ms' // 平均值是浮点，别把 991.4705882ms 漏给用户（T12）
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

/* 收紧成非可选的数字面：调用方拿到的永远是数字（缺失按 0，不往下游漏 undefined）。 */
export function estOf(line: Line | null | undefined): { text: number; reasoning: number; calls: number[]; images: number; imageCount: number } {
  const e = line && line.est
  return {
    text: (e && e.text) || 0,
    reasoning: (e && e.reasoning) || 0,
    calls: (e && e.calls) || [],
    images: (e && e.images) || 0,
    imageCount: (e && e.imageCount) || 0,
  }
}

/* ---------- 阶段分组与排序 ---------- */

export const STAGE_ORDER = ['dedupe', 'vector', 'style', 'chapters', 'convert', 'checker', 'style-fix', 'figure-check', 'fix', 'final-review']

/* 旧预览索引/静态导出可能还没有新阶段字段；已知核验转录名是稳定身份，
 * 不能因旧服务下发 stage=会话 而永远落进最后的「其他会话」组。 */
export function sessionStageOf(s: { stage?: string; label?: string; id?: string; name?: string }): string {
  const name = (s.name || s.id || '').replace(/\\/g, '/').split('/').pop() || ''
  if (/^motif_dedupe_.+\.jsonl$/.test(name) || /^(?:dedupe|motif-dedupe):/.test(s.label || '')) return 'dedupe'
  return s.stage || 'session'
}

export function stageRank(stage: string): number {
  const i = STAGE_ORDER.indexOf(stage)
  return i < 0 ? STAGE_ORDER.length : i
}

export function stageTitleOf(stage: string): string {
  const map: Record<string, string> = {
    dedupe: '整组核验', vector: '矢量图', style: '样式', chapters: '章节划分', convert: '章节转换',
    checker: '章节核对', 'style-fix': '样式修复', 'figure-check': '逐图校验',
    fix: '全书修复', 'final-review': '终审',
  }
  return map[stage] || stage || '其他会话'
}

/* progress.json 里的键是档位级的，阶段名与它并不一一对应。 */
export function progressKeyFor(stage: string): string {
  if (stage === 'dedupe' || stage === 'vector') return 'images'
  if (stage === 'checker' || stage === 'style-fix') return 'convert'
  // T88：fix/final-review 是 assemble 阶段内的会话，progress.json 没有
  // 它们自己的键——不映射的话阶段状态永远是「未开始」（其实早做完了）。
  if (stage === 'fix' || stage === 'final-review') return 'assemble'
  return stage
}

/* T110：样式阶段内试点会话的执行顺序（主样式会话 → 选材 → 试转 → 修订），
 * 未知标签回 -1（交给 mtime 兜底）。 */
export function stylePilotOrder(label: string): number {
  const m: Record<string, number> = { style: 0, 'style:pilot_select': 1, 'style:pilot_trial': 2, 'style:pilot_revise': 3 }
  return m[label || 'style'] ?? -1
}

/* 阶段状态三态：done=绿 / running=绿点脉冲 / else=进行中文本（P8 状态色）。 */
export interface StageStatus {
  text: string; state: 'done' | 'running' | 'idle' | 'busy'; title: string
}

export function stageStatusText(stages: any, stage: string, live?: number): StageStatus | null {
  // 会话活着 = 这一阶段正在跑，优先于 progress.json 的静态值。
  if (live) return { text: '运行中', state: 'running', title: '这一阶段还有 ' + live + ' 个会话在实时写入' }
  if (!stages) return null
  const key = progressKeyFor(stage)
  const v = stages[key]
  if (v === undefined || v === null || v === '') {
    return { text: '未开始', state: 'idle', title: 'progress.json: ' + key + ' 还没有记录' }
  }
  const text = String(v)
  const done = /^(done|ok|true|finished|complete[d]?)$/i.test(text)
  if (done) return { text: '✓ 完成', state: 'done', title: 'progress.json: ' + key + ' = ' + text }
  return { text, state: 'busy', title: 'progress.json: ' + key + ' = ' + text }
}

/* ---------- T63：convert/checker 阶段聚合 ----------
 * 逐章/逐章核对会话几十个，做完之后一行行列着全是噪音。聚合规则：
 *  - 该阶段有 live 会话（正在跑）→ 只显示 live 行，并发一目了然；
 *  - 全部结束 → 不显示会话行，只留阶段头的「N 章 · 完成 M」；
 *  - 有错误收场的会话永远保留（错误不能藏）；
 *  - 搜索过滤时不聚合（命中的历史会话要能点进去）。
 * 返回 null = 该阶段不聚合（原样显示）。 */
export interface StageAggregate {
  total: number; done: number; live: number; err: number
  rows: any[]            // 聚合后实际显示的会话行
  head: string           // 阶段头文案（替代「N 个会话」）
}
export function stageChapterAggregate(stage: string, items: any[], filtered: boolean): StageAggregate | null {
  if (stage !== 'convert' && stage !== 'checker') return null
  if (filtered) return null
  const chapterItems = (items || []).filter((s) => !s?.chapterRole || s.chapterRole === 'convert')
  const total = chapterItems.length
  if (!total) return null
  const live = chapterItems.filter((s) => s && s.live)
  const done = chapterItems.filter((s) => s && s.endState === 'done').length
  const errs = chapterItems.filter((s) => s && !s.live && s.endState === 'error')
  const rows = live.length ? live : errs
  const bits = [total + ' 章', '完成 ' + done]
  if (live.length) bits.push('运行 ' + live.length)
  if (errs.length) bits.push('错误 ' + errs.length)
  return { total, done, live: live.length, err: errs.length, rows, head: bits.join(' · ') }
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
  if (s.chapterOrder) parts.push('第' + s.chapterOrder + '章', 'chapter ' + s.chapterOrder, String(s.chapterOrder))
  if (s.endState === 'error') parts.push('错误', '未提交', 'error')
  if (s.endState === 'done') parts.push('已提交', '完成', 'done')
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
  // T22：状态语义与方块上色一致（绿圈=运行中 / 红=干过活没交 / 暗=没开工）。
  if (s.chapterOrder) tip.push('第 ' + s.chapterOrder + ' 章')
  if (s.live) tip.push('状态: 运行中（正在实时写入）')
  else if (s.endState === 'done') tip.push('状态: 已提交（正常结束）')
  else if (s.endState === 'error') tip.push('状态: 错误终止（有过工具回执但从没提交成功）')
  else tip.push('状态: 未开始（还没有任何工具回执）')
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

export interface ChapterTreeNode {
  id: string
  convert: any | null
  checker: any | null
  children: ChapterTreeNode[]
}

/* P59：核对会话的锚点是它核对的那份**转换转录的文件名**（`convert_<章>` /
 * `convert_<章>.tryN`），不含扩展名。转换与核对自己的 `.tryN` 号互相独立——
 * 手动重新核对只让核对的号加一、核对的还是同一份产物——所以树与历史只按这个
 * 口径配对，不再拿两套号互相猜。 */
export function convertBasename(s: any): string {
  const id = String((s && s.id) || '')
  const seg = id.slice(id.lastIndexOf('/') + 1)
  return seg.replace(/\.jsonl$/, '')
}

function newestOf(items: any[]): any | null {
  let best: any = null
  ;(items || []).forEach((s) => { best = newerAttempt(best, s) })
  return best
}

/* 同一个锚点下的多次核对：用户口径是"按 mtime 取最新"（重核号只是它自己的
 * 历史序号），mtime 并列才退回 newerAttempt 的确定性比较。 */
function newestByMtime(items: any[]): any | null {
  let best: any = null
  ;(items || []).forEach((s) => {
    if (!best) { best = s; return }
    const d = mtimeMs(s) - mtimeMs(best)
    if (d > 0) best = s
    else if (d === 0) best = newerAttempt(best, s)
  })
  return best
}

/*
 * P59：给一个节点选核对会话——**核对挂在它锚定的转换下面**。
 *  · 有锚点的节点（新转录）只认锚点：锚点对不上就不显示，绝不退回"最近那条
 *    核对"，否则另一次尝试的结论会摆到这一行旁边；
 *  · 全都还是没有锚点的旧转录时才走老规则；
 *  · 归档世代参与配对：上一代的核对不能挂到这一代的转换上（P58）。
 */
function checkerForConvert(checkers: any[], convert: any, pinnedConvert: boolean): any | null {
  if (!convert) return newestOf(checkers)
  const anchored = (checkers || []).filter((c) => String(c.chapterAnchor || '') !== '')
  if (anchored.length) {
    const key = convertBasename(convert)
    const hit = anchored.filter((c) => String(c.chapterAnchor) === key && !!c.archived === !!convert.archived)
    return newestByMtime(hit)
  }
  // 旧规则：钉住转换时只跟同一轮（同尝试号 + 同世代）；默认取最新一条，归档
  // 世代不同、或核对号小于转换号（上一次尝试的结论）都不算这一次的（T187）。
  const attempt = Number(convert.chapterAttempt) || 1
  if (pinnedConvert) {
    const peers = (checkers || []).filter((k: any) =>
      (Number(k.chapterAttempt) || 1) === attempt && !!k.archived === !!convert.archived)
    return newestOf(peers)
  }
  const best = newestOf(checkers)
  if (!best) return null
  if (!!best.archived !== !!convert.archived) return null
  if ((Number(best.chapterAttempt) || 1) < attempt) return null
  return best
}

/*
 * T170：同一章被 checker 打回后会换全新会话重来（convert_<章>.try2.jsonl…），
 * 几次尝试是**一件事的历史版本**。章节树一行代表"这一章现在的样子"，所以默认
 * 打开**最新一次**尝试（序号大者优先，序号相同才比修改时间）——旧实现是遍历
 * 列表时后者覆盖前者，而列表按时间新→旧，结果反而停在第一次尝试上；用户点开
 * 这一行看到的是被作废的那一次。更早的尝试仍在「历史 N」页签里逐个可点。
 */
function newerAttempt(cur: any, next: any): any {
  if (!cur) return next
  if (!next) return cur
  // 归档里的转录来自**更早一次根尝试**（`work/attempts/<根章>/tryN/`）。根章重来
  // 一次时它的子章节尝试号又从 1 数起，所以归档里的 `convert_<子章>.try2` 尝试号
  // 比工作树里这一次的 `convert_<子章>`（#1）还大——只比号会把"上一代的通过结果"
  // 当成这一章的现状显示（用户看到"下面都通过了、却还有一个在排队"）。先比是不是
  // 归档，工作树那条永远优先；两边同为归档/同时活体时才比尝试号。
  if (!!cur.archived !== !!next.archived) return cur.archived ? next : cur
  const a = Number(cur.chapterAttempt) || 1
  const b = Number(next.chapterAttempt) || 1
  if (a !== b) return b > a ? next : cur
  return mtimeMs(next) > mtimeMs(cur) ? next : cur
}

/*
 * T214：子行的**世代**跟着父行走。父行的转换是工作树里的（正在跑的这一代），
 * 子行就只在工作树里挑；父行是归档里的（你在历史里切到了上一代），子行也回到
 * 归档——同一章的两个世代各有一套子章节，混着取就会"父行是旧的那一代、子行是
 * 新的那一代"。父行连转换都没有（只有核对）时不设限，退回各取最新；那一代里
 * 没有这个子章节的转换（这一代根本没转过它）时也不留空行，退回该节点最新一次。
 */
function convertInGeneration(candidates: any[], archived: boolean | null): any | null {
  if (!candidates || !candidates.length) return null
  if (archived === null) return newestOf(candidates)
  const sameGen = candidates.filter((c) => !!c.archived === archived)
  return newestOf(sameGen.length ? sameGen : candidates)
}

/*
 * 章节树里"这一行显示哪一次尝试"（T173，T175 收窄，T214 补子行世代）。
 *
 * 默认每个节点各取自己最新的一次：同一章下各节点的重试次数是**互相独立**
 * 的（有的只转过一次、有的被打回五次），拿一个章节的 #2 去套全树，别的章节
 * 就会显示一个不相干的旧版本。
 *
 * 唯一的例外是**你正在看的那一条**（`pinId`）：它所在的行必须显示它，否则
 * 点开历史里的 #2、树里还是 #3。同一节点的另一种角色（convert ↔ checker）
 * 按**核对会话的锚点**配（P59，见 checkerForConvert）：钉住核对时转换由锚点
 * 决定，钉住转换时核对挂在它锚定的那次转换下面；旧转录没有锚点才退回"同一轮"。
 * 子会话跟着父行的那一代走（T214，见 convertInGeneration），别的章节不受影响。
 */
export function buildChapterTree(items: any[], pinId = ''): ChapterTreeNode[] {
  const byId: Record<string, ChapterTreeNode> = {}
  const parents: Record<string, string> = {}
  const byRole: Record<string, any[]> = {}
  let pinned: { node: ChapterTreeNode; role: 'convert' | 'checker'; session: any } | null = null
  ;(items || []).forEach((session) => {
    if (!session?.chapterId) return
    const id = String(session.chapterId)
    const node = byId[id] || (byId[id] = { id, convert: null, checker: null, children: [] })
    parents[id] = String(session.chapterParent || '')
    if (session.chapterRole !== 'checker' && session.chapterRole !== 'convert') return
    const role: 'convert' | 'checker' = session.chapterRole === 'checker' ? 'checker' : 'convert'
    const key = id + '|' + role
    ;(byRole[key] || (byRole[key] = [])).push(session)
    if (pinId && session.id === pinId) pinned = { node, role, session }
  })
  // 先搭出父子关系（T214 要按"父行定了哪一代"给子行选行，得从根往下走）。
  const roots: ChapterTreeNode[] = []
  Object.keys(byId).forEach((id) => {
    const parent = parents[id] && byId[parents[id]]
    if (parent) parent.children.push(byId[id])
    else roots.push(byId[id])
  })
  const ordinal = (id: string): number => {
    const m = /_child_(\d+)$/.exec(id)
    return m ? Number(m[1]) : 0
  }
  const sort = (nodes: ChapterTreeNode[]) => {
    nodes.sort((a, b) => ordinal(a.id) - ordinal(b.id) || a.id.localeCompare(b.id))
    nodes.forEach((node) => sort(node.children))
  }
  sort(roots)
  // P59：先定每一行的转换，再由**锚点**选核对。钉住核对会话时，转换由它的
  // 锚点决定（不是按尝试号，也不是"最新那条"）；钉住转换时，转换仍取那一条，
  // 核对照锚点选。定完把这一行的世代交给子行（T214）。
  const pin = pinned as { node: ChapterTreeNode; role: 'convert' | 'checker'; session: any } | null
  const resolve = (node: ChapterTreeNode, parentArchived: boolean | null): void => {
    const here = pin && pin.node === node ? pin : null
    if (here && here.role === 'checker') {
      node.checker = here.session
      const anchor = String(here.session.chapterAnchor || '')
      const converts = byRole[node.id + '|convert'] || []
      if (anchor) {
        const sameGen = converts.filter((c: any) => !!c.archived === !!here.session.archived)
        const hit = (sameGen.length ? sameGen : converts).filter((c: any) => convertBasename(c) === anchor)
        // 锚点对应的转换不存在（例如只剩归档的核对）时留空：不拿别的尝试凑。
        node.convert = hit.length ? newestOf(hit) : null
      } else {
        // 旧转录没有锚点：按老规则把转换切到"同一轮"；那一轮不在就保持最新。
        const want = Number(here.session.chapterAttempt) || 1
        const peers = converts.filter((c: any) =>
          (Number(c.chapterAttempt) || 1) === want && !!c.archived === !!here.session.archived)
        node.convert = peers.length ? newestOf(peers) : newestOf(converts)
      }
    } else {
      node.convert = here
        ? here.session
        : convertInGeneration(byRole[node.id + '|convert'] || [], parentArchived)
      node.checker = checkerForConvert(byRole[node.id + '|checker'] || [], node.convert, !!here)
    }
    const archived = node.convert ? !!node.convert.archived : parentArchived
    node.children.forEach((child) => resolve(child, archived))
  }
  roots.forEach((root) => resolve(root, null))
  return roots
}

export interface StageGroup { stage: string; title: string; items: any[]; live: number }
export interface ProjectGroup {
  name: string; prefix: string; title: string; legacy: boolean
  items: any[]; live: number; matched: boolean
  stages: Record<string, StageGroup>
}

/* ---------- P18：板块（board）过滤 ----------

 * latex 板块 = board 字段为空/缺失的会话；img2text 板块 = board==="img2text"。
 * 其余未知取值按 latex 归位（Go 侧目前只有这一种标记）。 */

export function sessionBoard(s: { board?: string } | null | undefined): string {
  return s && s.board === 'img2text' ? 'img2text' : 'latex'
}

export function filterByBoard<T extends { board?: string }>(sessions: T[] | null | undefined, board: string): T[] {
  const b = board === 'img2text' ? 'img2text' : 'latex'
  return (sessions || []).filter((s) => sessionBoard(s) === b)
}

/* ---------- T53：img2text 调用链（同一张图的多次会话聚合成一组） ----------
 *
 * img2text 板块的会话 ID 两种形态：
 *   img2text:sessions/<md>/<图名>.jsonl                        —— 逐图分析
 *   img2text:mermaid_fix/<md>_<图名>/session-stageN[.prevM].jsonl —— 升级修复/轮转
 * 链键 = 归一后的图片文件名（<hash>.<ext>）：sessions 的去掉 images_ 前缀和
 * <md>_ 前缀；mermaid_fix 的按最后一个下划线拆（图片名是内容哈希、本身不含
 * 下划线，书/图分界就是最后一个 _，md 名含下划线也不怕）。归一失败退回原始段，
 * 保证同图的 stage0/stage1/prevN 至少按来源各成一条链，绝不散列。 */

export interface Img2TextRef {
  kind: 'analyze' | 'fix'
  img: string   // 归一后的图片文件名（链键）
  stage: number // fix: 0/1…；analyze: -1
  prev: number  // 轮转序号：0=当前，1=prev1（上一次），2=prev2（上上次）…
}

export function parseImg2TextId(id: string): Img2TextRef | null {
  const PREFIX = 'img2text:'
  if (!id || id.indexOf(PREFIX) !== 0) return null
  const segs = id.slice(PREFIX.length).split('/')
  if (segs.length !== 3) return null
  if (segs[0] === 'mermaid_fix') {
    const m = /^session-stage(\d+)(?:\.prev(\d+))?\.jsonl$/.exec(segs[2])
    if (!m) return null
    const dir = segs[1]
    let img = dir
    const i = dir.lastIndexOf('_')
    if (i > 0 && i < dir.length - 1) img = dir.slice(i + 1)
    return { kind: 'fix', img, stage: parseInt(m[1], 10), prev: m[2] ? parseInt(m[2], 10) : 0 }
  }
  if (segs[0] === 'sessions') {
    // T57：逐图转录也做 prevN 轮转（<图>.prev1.jsonl = 上一次运行，与
    // mermaid_fix 的 session-stage0.prev1.jsonl 同款规则；无后缀 = 当前）。
    const m = /^(.*?)(?:\.prev(\d+))?\.jsonl$/i.exec(segs[2])
    if (!m || !m[1]) return null
    const stem = m[1]
    const prev = m[2] ? parseInt(m[2], 10) : 0
    const dot = segs[1].lastIndexOf('.')
    const md = dot > 0 ? segs[1].slice(0, dot) : segs[1]
    let img = stem
    if (img.indexOf('images_') === 0) img = img.slice('images_'.length)
    if (md && img.indexOf(md + '_') === 0) img = img.slice(md.length + 1)
    if (!img) img = stem
    return { kind: 'analyze', img, stage: -1, prev }
  }
  return null
}

/*
 * 章节会话（convert / checker）的标题是**流水线里的标识符**（`chapter_004_child_01`），
 * 不是作者写的文案：走行内 markdown 会把下划线当成强调吃掉
 * （`chapter_004_child_01` → `chapter004child01`，树里还缩成 `chapter004child01chil…`），
 * 所以这一类标题一律纯文本显示（T176）；img2text/样式等会话照旧走 markdown。
 */
export function plainTitle(s: any): boolean {
  return !!(s && s.chapterId)
}

/*
 * 章节树里那句状态文案（T178）：同一个 `chapter_activity` 在转换会话与核对
 * 会话里意思相反，不能共用一张表——
 *  · 转换会话跑完（checking_result）= 这一轮转换交上去了、正等核对
 *    → 「转换完成待核对」；
 *  · 核对会话跑完（checking_result）= 核对会话结束了、正在读它的结论
 *    → 「核对完成待处理」；
 *  · 转换会话又排到 queued、而这一章已经有核对结论（上一轮被打回）→ 它等的
 *    其实是**重转换**的并发槽 → 「核对完成待并发槽」。
 *
 * T181：核对结论一出来，转换行就不能再挂着「待核对」——它自己在等自己的核对，
 * 而核对已经说话了。所以转换行的 checking_result 要看同一行核对的结论：发现问题
 * →「核对发现问题」（马上要被重新喂回去改），已通过 →「已通过」。
 */
const CHAPTER_ACTIVITY: Record<string, string> = {
  running: '运行中', waiting_children: '等待子会话',
  pass: '已通过', issues: '发现问题', error: '会话错误',
  // T171：转录里已有 run_end 却停在 running 的尝试被放弃了，不能再显示"运行中"。
  interrupted: '已中断',
}

export function chapterActivityLabel(
  state: string | undefined,
  role: 'convert' | 'checker' = 'convert',
  peerVerdict = '',
  live = false,
): string {
  if (!state) return live ? '运行中' : ''
  // peerVerdict 是同一行**另一个角色**的最新结论（转换行看核对的结论）。
  const judged = peerVerdict === 'issues' || peerVerdict === 'pass'
  if (state === 'queued') {
    return role === 'convert' && judged ? '核对完成待并发槽' : '等待并发槽'
  }
  if (state === 'checking_result') {
    if (role === 'checker') return '核对完成待处理'
    if (peerVerdict === 'issues') return '核对发现问题'
    if (peerVerdict === 'pass') return '已通过'
    return '转换完成待核对'
  }
  return CHAPTER_ACTIVITY[state] || (live ? '运行中' : '')
}

/*
 * 章节会话的尝试号（T165，T173 改标法）：同一章每次重开一条转录
 * （`convert_<章>.try2.jsonl`），条目标 `#N`。用户口径：不要在中栏/侧栏
 * 上写中文「第 N 次」——它在正常浏览时不提供信息，只在「历史」列表里用来
 * 区分同一件事的几次尝试，所以只保留紧凑的 `#N`，且只在历史列表里出现。
 */
export function chapterAttemptTag(s: any): string {
  const n = Number(s && s.chapterAttempt) || 1
  return n > 1 ? '#' + n : ''
}

/* 历史列表里每条会话的来源小标签：逐图分析 / stage0 / stage1 · 上一次 … */
export function img2textMemberTag(id: string): string {
  const r = parseImg2TextId(id)
  if (!r) return ''
  let t = r.kind === 'analyze' ? '逐图分析' : 'stage' + r.stage
  if (r.prev === 1) t += ' · 上一次'
  else if (r.prev === 2) t += ' · 上上次'
  else if (r.prev > 2) t += ' · prev' + r.prev
  return t
}

function mtimeMs(s: any): number {
  const t = Date.parse((s && s.mtime) || '')
  return isNaN(t) ? 0 : t
}

/*
 * 链内排序：最新在前。主键 mtime 降序；并列时 stage 编号大的新、
 * 轮转序号小的新（当前运行 > prev1 > prev2），最后按 id 保底确定性。
 */
export function sortChainMembers(items: any[]): any[] {
  return (items || []).slice().sort((a, b) => {
    const d = mtimeMs(b) - mtimeMs(a)
    if (d) return d
    const ra = parseImg2TextId(a && a.id)
    const rb = parseImg2TextId(b && b.id)
    const sa = ra ? ra.stage : -2
    const sb = rb ? rb.stage : -2
    if (sa !== sb) return sb - sa
    const pa = ra ? ra.prev : 0
    const pb = rb ? rb.prev : 0
    if (pa !== pb) return pa - pb
    const ia = String((a && a.id) || '')
    const ib = String((b && b.id) || '')
    return ia < ib ? -1 : ia > ib ? 1 : 0
  })
}

export interface SessionChain {
  key: string      // 链键（归一图片名，项目内唯一）
  img: string
  main: any        // 最新一次会话（主行）
  rest: any[]      // 历史（次新 → 最旧）
  items: any[]     // main + rest
  live: number
}

function chainBookOrder(c: SessionChain): number {
  // 书内序号取全体成员的**最小**正 chapterOrder（逐图分析的编号是书的扫描
  // 顺序；修复段另有一套编号，取最小值让同图各段挨在一起、链间按书序排）。
  let best = Infinity
  c.items.forEach((s) => {
    const n = Number(s && s.chapterOrder) || 0
    if (n > 0 && n < best) best = n
  })
  return best
}

/* 把一组 img2text 会话按图片聚成链；解析不了 ID 的进 singles（原样罗列）。 */
export function buildImg2TextChains(items: any[]): { chains: SessionChain[]; singles: any[] } {
  const byImg: Record<string, any[]> = {}
  const singles: any[] = []
  ;(items || []).forEach((s) => {
    const r = s && parseImg2TextId(s.id)
    if (!r) { singles.push(s); return }
    ;(byImg[r.img] = byImg[r.img] || []).push(s)
  })
  const chains: SessionChain[] = Object.keys(byImg).map((img) => {
    const members = sortChainMembers(byImg[img])
    let live = 0
    members.forEach((s) => { if (s.live) live++ })
    return { key: img, img, main: members[0], rest: members.slice(1), items: members, live }
  })
  chains.sort((a, b) => {
    const oa = chainBookOrder(a)
    const ob = chainBookOrder(b)
    if (oa !== ob) return oa - ob
    return a.img < b.img ? -1 : a.img > b.img ? 1 : 0
  })
  singles.sort((a, b) => mtimeMs(b) - mtimeMs(a))
  return { chains, singles }
}

export function buildGroups(): ProjectGroup[] {
  const q = state.filter
  let groups: ProjectGroup[] = []
  const byName: Record<string, ProjectGroup> = {}
  filterByBoard(state.sessions, state.board).forEach((s: any) => {
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
        legacy: false,
        items: [], live: 0, matched: false,
        stages: {},
      }
      groups.push(g)
    }
    // 「旧版单项目」徽标只属于**根组**（Go 的 RootProject"（根目录）"：输出
    // 根本身就是工作区）。书组里只要有一笔旧式落点的转录（如
    // <书>/work/style_session.jsonl 直接收在 work/ 下），组级 OR 会把整组
    // 误标——新布局的书不该挂这个徽标（T13）。
    if (s.projectLegacy && name === '（根目录）') g.legacy = true
    if (q && sessionHaystack(s).indexOf(q) < 0) return
    g.items.push(s)
    if (s.live) g.live++
    // 阶段子组：项目内按流程阶段再分一层（组头带会话数与 progress.json 状态）。
    const stage = sessionStageOf(s)
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

/* 中栏「历史」页签的数据源：给定会话 id，返回它所在 img2text 调用链的全部
 * 成员（sortChainMembers 排序：新 → 旧）；不属于任何链 / 单成员链 / 不可
 * 解析的 id 一律返回 null（此时「历史」页签不出现）。纯函数：直接对传入的
 * sessions 数组扫描，不读全局 state，调用方用 computed 按引用缓存。 */
export function chainOfSession(sessions: any[], id: string | null | undefined): any[] | null {
  if (!id) return null
  // 章节转换的尝试链优先：同一章被 checker 打回后换会话重来，几次尝试是
  // 同一件事的历史版本（键 = 项目 + 角色 + 章节 ID，成员 = 各次尝试）。
  const attemptChain = chapterAttemptChain(sessions, id)
  if (attemptChain) return attemptChain
  const self = parseImg2TextId(id)
  if (!self) return null
  // 链是按项目聚的（侧栏 buildGroups 里每组各自 buildImg2TextChains）：
  // 同一内容哈希的图出现在两本书里时各成一条链，不能跨项目混并。
  let proj = ''
  ;(sessions || []).forEach((s) => {
    if (s && s.id === id) proj = s.project || projectOf(s.id)
  })
  const members: any[] = []
  ;(sessions || []).forEach((s) => {
    const r = s && parseImg2TextId(s.id)
    if (!r || r.img !== self.img) return
    if ((s.project || projectOf(s.id)) !== proj) return
    members.push(s)
  })
  if (members.length < 2) return null
  return sortChainMembers(members)
}

/*
 * 章节转换/核对的尝试链：给定会话 id，把它所在「同一章节 ID」的各次尝试聚成
 * 一条链。只认带 chapterId 的会话；只尝试过一次的返回 null（此时不出现
 * 「历史」页签）。
 */
export function chapterAttemptChain(sessions: any[], id: string | null | undefined): any[] | null {
  if (!id) return null
  let self: any = null
  ;(sessions || []).forEach((s) => {
    if (s && s.id === id) self = s
  })
  if (!self || !self.chapterId) return null
  const proj = self.project || projectOf(self.id)
  const members: any[] = []
  ;(dedupeChapterAttempts(sessions || [])).forEach((s) => {
    if (!s || !s.chapterId) return
    if (s.chapterId !== self.chapterId) return
    // T194：**不再按角色分链**——转换与同一轮的核对是同一件事的两个角色（用户口径
    // "这俩是一体的"）。原来按角色过滤时，点一条核对会话会让整个列表从「#2 #1」
    // 变成「#1」（核对只有一次尝试），看起来像"点一下 #2 就变成了 #1"。
    if ((s.project || projectOf(s.id)) !== proj) return
    members.push(s)
  })
  // P59：链只列**当前这条会话所在角色**的历史——
  //  · 转换会话下面 = 这一章的各次转换（不看核对）；
  //  · 核对会话下面 = **挂在同一个转换名下**的各次核对（不看转换）。
  // 锚点就是转换转录名（`chapterAnchor`）；没有锚点的老转录退回"同章同类"。
  const wantRole = self.chapterRole === 'checker' ? 'checker' : 'convert'
  let rows = members.filter((m: any) => (m.chapterRole === 'checker' ? 'checker' : 'convert') === wantRole)
  if (wantRole === 'checker' && self.chapterAnchor) {
    rows = rows.filter((m: any) => m.chapterAnchor === self.chapterAnchor)
  }
  if (rows.length < 2) return null
  // 转换：各次尝试（新 → 旧）；核对：同一转换名下的各次核对（新 → 旧）。
  return wantRole === 'checker'
    ? rows.slice().sort((a: any, b: any) => mtimeMs(b) - mtimeMs(a))
    : sortChainMembers(rows)
}

/* P59：历史列表以转换会话为行首锚点——每个转换尝试一行，紧跟它**锚定的**核对
 * （新 → 旧），然后是上一个更老的转换。旧转录没有锚点时才按"同世代里尝试号
 * 不大于自己的最近那次转换"归位；归不进去的排在最后，不藏掉任何一条。 */
export function orderChapterChain(members: any[]): any[] {
  const converts = (members || []).filter((s) => s && s.chapterRole !== 'checker').sort(convertChainOrder)
  const checkers = (members || []).filter((s) => s && s.chapterRole === 'checker')
  const owned: Record<string, any[]> = {}
  const leftover: any[] = []
  converts.forEach((c) => { owned[String(c.id)] = [] })
  checkers.forEach((k) => {
    const anchor = String((k && k.chapterAnchor) || '')
    if (anchor) {
      const hit = converts.find((c) => convertBasename(c) === anchor && !!c.archived === !!k.archived)
      if (hit) { owned[String(hit.id)].push(k); return }
      leftover.push(k)
      return
    }
    const sameGen = converts.filter((c) => !!c.archived === !!k.archived)
    const older = sameGen.filter((c) => (Number(c.chapterAttempt) || 1) <= (Number(k.chapterAttempt) || 1))
    const owner = (older.length ? older : sameGen).slice().sort(convertChainOrder)[0]
    if (owner) owned[String(owner.id)].push(k)
    else leftover.push(k)
  })
  const byThen = (a: any, b: any) => mtimeMs(b) - mtimeMs(a)
  const out: any[] = []
  converts.forEach((c) => {
    out.push(c)
    owned[String(c.id)].sort(byThen).forEach((k) => out.push(k))
  })
  leftover.sort(byThen).forEach((k) => out.push(k))
  return out
}

/* 转换自己的先后：工作树优先于归档（P58：归档是上一代），再比尝试号、mtime。 */
function convertChainOrder(a: any, b: any): number {
  if (!!a.archived !== !!b.archived) return a.archived ? 1 : -1
  const aa = Number(a.chapterAttempt) || 1
  const ab = Number(b.chapterAttempt) || 1
  if (aa !== ab) return ab - aa
  const d = mtimeMs(b) - mtimeMs(a)
  if (d) return d
  const ia = String((a && a.id) || '')
  const ib = String((b && b.id) || '')
  return ia < ib ? -1 : ia > ib ? 1 : 0
}

/* T194：同一章同一角色可能扫到**两份同号转录**——工作树里这次尝试的，加上
 * `work/attempts/<章>/tryN/` 里更早一次父尝试留下的。两种情形要分开处理：
 *  · 内容完全相同（同一个转录被扫了两遍）→ 只留一份，工作树那份优先；
 *  · 内容不同（同一个子章节在两次父尝试里各转过一次，文件名里的尝试号都从 1
 *    重新数）→ **两条都要留**，否则历史会把一次真实发生过的对话藏掉。上层用
 *    `chainMemberTag` 把归档那份标成 `旧#N`，行上另有「归档」徽标。
 * 所以这里的判据是「同一键 + 同一内容指纹」才去重，键相同而内容不同不算重复。
 * 非章节会话原样返回。 */
export function dedupeChapterAttempts(items: any[]): any[] {
  const out: any[] = []
  const byKey: Record<string, any[]> = {}
  ;(items || []).forEach((s) => {
    if (!s || !s.chapterId) { out.push(s); return }
    const key = [s.project || projectOf(s.id), s.chapterId, s.chapterRole || '', Number(s.chapterAttempt) || 1].join('|')
    const sha = String(s.sha || '')
    const peers = byKey[key] || (byKey[key] = [])
    // 没有内容指纹（旧索引 / 静态快照）时不动手：宁可多一行，也不要藏掉一次对话。
    const twin = sha ? peers.find((o) => String(o.sha || '') === sha) : null
    if (!twin) { peers.push(s); out.push(s); return }
    // 同一份内容被扫了两遍：工作树那份胜出；两份都在归档里就留较新的。
    if (!!twin.archived && !s.archived) {
      const i = out.indexOf(twin)
      if (i >= 0) out[i] = s
      peers[peers.indexOf(twin)] = s
    }
  })
  return out
}

/* 历史行的小标签：img2text 链路用阶段标签，章节会话统一给 `#N`（含第 1 次，
 * 否则同章的几条历史行看起来一模一样）。 */
export function chainMemberTag(s: any): string {
  // 归档里的同号尝试来自更早一次父尝试（内容不同），标「旧」才不会和这一次的
  // 同号行混成"重复行"。
  // P58 观察：只给 `#N` 时，同一条链里会出现两个 `#1`（转换 #1 + 核对 #1），
  // 加了"重新核对"以后又出现 `#1/#2` 对不上号——用户看不出哪一行是核对。
  // 核对行标「核对#N」，转换行仍是 `#N`；归档前缀照旧。
  if (s && s.chapterId) {
    const tag = (s.chapterRole === 'checker' ? '核对' : '') + '#' + (Number(s.chapterAttempt) || 1)
    return (s.archived ? '旧' : '') + tag
  }
  return img2textMemberTag(s && s.id)
}

export function findSession(id: string): any {
  let found: any = null
  state.sessions.forEach((s) => {
    if (s.id === id) found = s
  })
  return found
}

/* ---------- T54：img2text 进度概览（块主导色 / 聚合文案） ---------- */

export type I2tDominantState = 'running' | 'escalated' | 'pending' | 'fixed' | 'done'

/*
 * 块视图里一本书一个色块：取"最重"的状态——T58 起正在跑(running，蓝)最优先，
 * 正在处理中的书一眼可见；之后才是 升级未修好(琥珀) > 待处理(暗) >
 * 修复(青) > 全完成(绿)。
 */
export function bookDominantState(b: Pick<Img2TextBook, 'done' | 'fixed' | 'escalated' | 'pending' | 'running'>): I2tDominantState {
  if (b.running > 0) return 'running'
  if (b.escalated > 0) return 'escalated'
  if (b.pending > 0) return 'pending'
  if (b.fixed > 0) return 'fixed'
  return 'done'
}

export const I2T_STATE_TEXT: Record<I2tDominantState, string> = {
  done: '全部完成', fixed: '有修复', escalated: '有升级未修好', pending: '有待处理', running: '正在处理中',
}

/* 块悬浮说明：书名 + 五态计数 + done/total。 */
export function bookTip(b: Img2TextBook): string {
  return b.book + '\n' +
    '完成 ' + b.done + ' · 修复 ' + b.fixed + ' · 升级 ' + b.escalated + ' · 进行中 ' + b.running + ' · 待处理 ' + b.pending +
    '（' + b.done + '/' + b.total + '）\n主导状态：' + I2T_STATE_TEXT[bookDominantState(b)] +
    '\n点击在侧栏里搜这本书的会话'
}

/* 概览头部聚合：「12/15 书完成」（完成 = done+fixed 覆盖了 total）。 */
export function booksHeadText(books: Img2TextBook[]): string {
  const list = books || []
  const full = list.filter((b) => b.total > 0 && b.done + b.fixed >= b.total).length
  return full + '/' + list.length + ' 书完成'
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

/* ---------- P34 递归章节树的展开记忆 ---------- */

/*
 * 章节节点在折叠记忆里的键。带 scope（项目 + 阶段）是必须的：不同书、甚至
 * 同一本书的不同阶段里都可能有同名节点（chapter_001），不带 scope 会互相
 * 串状态——在 A 书里拉开一章，B 书里同名那章跟着开。
 */
export function chapterNodeKey(scope: string, nodeId: string): string {
  return 'chapter:' + scope + '/' + nodeId
}

/*
 * 一个章节节点要不要展开：记忆优先，没记忆时只展开"装着当前会话"的那一支。
 *
 * 这条规则和侧栏三层分组共用一套语义（groupWantOpen）：用户手动拉开的节点
 * 一直开着，与看哪个会话无关；从没碰过的节点随选中会话自动露出。关键是**它
 * 只读自己的键**——选择会话、展开别的节点都不会把这一支关上（旧实现把
 * `:open` 绑成"是否含当前会话"，每次重渲染都会强制把其它节点关上）。
 */
export function chapterWantOpen(scope: string, nodeId: string, holdsCurrent: boolean): boolean {
  const key = chapterNodeKey(scope, nodeId)
  /* 这里刻意用**属性读**而不是 hasCollapseMemory：state.collapsed 是 Vue 的
     reactive 代理，hasOwnProperty 不走 get 陷阱、建不起响应式依赖——照它写会
     出现"点了展开按钮、记忆写进去了，但视图不重绘"（要等别的 reactive 变化才
     显示）。读一次 state.collapsed[key] 就把这个键登记成依赖，写入即触发重绘。 */
  const mem = state.collapsed[key]
  if (mem !== undefined) return !mem
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

/* ---------- T78：style-fix 阶段内排序 ----------
 * 修复循环的环节顺序 = 问题整合(1) → 样式修复(2) → 评估分派(3) → 分块修复(4)；
 * 同环节内按轮次、块号升序（历史轮在前），再按修改时间新→旧兜底。
 * Go 侧 label 形态：style-fix:integrate / :style_r1 / :assess_r1 / :blk-r1-3。 */
export interface StyleFixPhaseKey { phase: number; round: number; blk: number }

export function styleFixPhaseKey(label: string | undefined | null): StyleFixPhaseKey | null {
  const m = /^(?:style-fix:)?(integrate|style_r(\d+)|assess_r(\d+)|blk-r(\d+)-(\d+))$/.exec(String(label || ''))
  if (!m) return null
  if (m[1] === 'integrate') return { phase: 1, round: 0, blk: 0 }
  if (m[2] !== undefined) return { phase: 2, round: +m[2], blk: 0 }
  if (m[3] !== undefined) return { phase: 3, round: +m[3], blk: 0 }
  return { phase: 4, round: +m[4], blk: +m[5] }
}

export function stylefixCompare(a: any, b: any): number {
  const ka = styleFixPhaseKey(a && a.label)
  const kb = styleFixPhaseKey(b && b.label)
  if (ka && kb) {
    if (ka.phase !== kb.phase) return ka.phase - kb.phase
    if (ka.round !== kb.round) return ka.round - kb.round
    if (ka.blk !== kb.blk) return ka.blk - kb.blk
    return 0
  }
  // 一边能解析一边不能：解析得到的按环节排前面，未知的排后面
  if (ka) return -1
  if (kb) return 1
  return 0
}
