/*
 * 对话时间线的**共享算法层**（块 C 组件化后）：图片归属（FIFO 配对）、工具
 * 家族/摘要、结果分类、媒体路径、元信息状态。DOM 一概不建——消息流条目由
 * legacy/stream.ts 的 streamModel() 扫出，渲染在 Timeline.vue 与各消息组件
 * 里；富文本（renderMarkdown / machineScroll / copyButton）在 richtext.ts。
 */
import { state } from '../state'
import { estOf, metaOf } from './sidebar'
import { type Line, type ToolCall, type ImageAttr, type ImageCallEntry } from './types'

/* ---------- 图片轮 / 任务块 ---------- */

// 图片引用（file://media/<sha>.jpg 之类）的**文件名**：长哈希只留前 8 位显示，
// 完整名字在缩略图的 title 与 alt 上（沿用 Go 侧"哈希当不了名字"的同一口径）。
function refBaseName(ref: unknown): string {
  const s = String(ref || '').split('?')[0]
  const i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'))
  return i >= 0 ? s.slice(i + 1) : s
}

function shortFileName(name: unknown): string {
  const s = String(name || '')
  const dot = s.lastIndexOf('.')
  const stem = dot > 0 ? s.slice(0, dot) : s
  const ext = dot > 0 ? s.slice(dot) : ''
  if (/^[0-9a-f]{32,}$/i.test(stem)) return stem.slice(0, 8) + ext
  return s
}

// 作图任务把原图印刷尺寸写在正文里（ORIGINAL FIGURE SIZE: 36.9mm x 20.1mm）
function originalFigureSize(text: unknown): string {
  const m = /ORIGINAL\s+FIGURE\s+SIZE\s*:\s*([0-9.]+\s*mm\s*[x×]\s*[0-9.]+\s*mm)/i.exec(String(text || ''))
  return m ? m[1].replace(/\s+/g, ' ') : ''
}

// 图片轮的一行摘要：几张 + 原图尺寸（取得到就写）+ 文件名
export function imageTurnSummary(line: Line | null | undefined): string {
  const imgs = (line && line.images) || []
  const bits: string[] = [imgs.length + ' 张图片']
  const size = originalFigureSize(line && line.text)
  if (size) bits.push(size)
  const names = imgs.map((r: unknown) => shortFileName(refBaseName(r)))
  if (names.length) {
    bits.push(names.slice(0, 2).join('、') + (names.length > 2 ? ' 等 ' + names.length + ' 个文件' : ''))
  }
  return bits.join(' · ')
}

/*
 * 带图 user 轮的**工具归属**。wire 上图片只能走 user 消息（tool 消息的 content 只能
 * 是文本），所以归属只能从**句柄文本 + 顺序**里推断，绝不新增字段。
 *
 * 顺序那一半是 **FIFO 配对**：先把转录里"会产生图片的工具调用"按出现次序排成队列，
 * 再按顺序扫带图 user 行，每条**还没归属**的图片行领走队列里**最早的那个还没被
 * 认领**的调用（一对一）。理由就是转录本身的次序——一轮里多次看图调用，图片回执
 * 也是按同样的次序依次回来的，所以第 N 条图配第 N 次产图调用。
 *
 *   · 句柄 `Tool image output` 开头：有 `(call <id>)` → **精确匹配**优先，FIFO 让位；
 *     有 `from <tool>` → 队列里最早的同名、还没被认领的那次调用；
 *     都没有（旧转录）→ 纯 FIFO。
 *   · 不是句柄但**有正文**（真实矢量图会话：任务提示里头就带着原图）→ 这一轮就是
 *     任务本身，归「本会话任务」，**FIFO 不许抢**。
 *   · 不是句柄也没正文（纯图片行）：先按 FIFO 配；队列空/配不上才当任务投喂的原图。
 *   · 什么都配不上 → kind 'none'，页面标「归属：未识别」并让它单独成行。
 */
const IMAGE_HANDLE_RE = /^Tool image output\b/i
const IMAGE_CALL_RE = /\(call\s+([A-Za-z0-9_.:-]+)\)/
const IMAGE_FROM_RE = /\bfrom\s+([A-Za-z0-9_.:-]+)/i
// 哪些调用"会产生图片"——两条依据都用上：
//   · **回执里的图片证据最硬**：`Image <路径> … attached.` / `PDF page <文件> … attached.`。
//     有回执就看回执——失败的 view_pdf 回执是 TOOL ERROR，它没产图，不该占着队列
//     位置把后面那条图认错。
//   · 没有回执（被压缩截断、或还在跑）才退回**工具名**当推定：看图就这两个工具。
const IMAGE_TOOLS: Record<string, number> = { view_image: 1, view_pdf: 1 }
const IMAGE_RESULT_RE = /^(?:Image\s+\S+|PDF page\s+\S+)[^\n]*\battached\b/im

export const IMAGE_WIRE_TITLE = 'user 消息承载图片（tool 消息的 content 只能文本，OpenAI 兼容 schema 限制）' +
  ' · text + image_url(data:image/jpeg;base64,…)'

// 归属结果按"会话 + 行数"缓存：行数变了（追加/切会话）就重算一遍。
export function imageAttributions(): Record<number, ImageAttr> {
  const sig = (state.current ? state.current.id : '') + ':' + state.lines.length
  if (state.imgAttr && state.imgAttr.sig === sig) return state.imgAttr.map

  /*
   * 第一步：按转录顺序记下**每一次调用**（id / 工具名 / 所在行，以及它的回执文本），
   * 再挑出"会产生图片"的那些排成 FIFO 队列。用**条目**而不是 callId 当队列元素，
   * 因为续跑/重放的转录里同一个 call id 会出现两次，按 id 去重会让重放段的图片
   * 全部认不出归属。
   */
  const calls: ImageCallEntry[] = []
  const idxById: Record<string, number[]> = {}
  state.lines.forEach((line: Line) => {
    if (!line || line.bad || (line.t && line.t !== 'msg')) return
    if (line.role === 'assistant') {
      ;(line.tool_calls || []).forEach((c: ToolCall) => {
        ;(idxById[c.id || ''] = idxById[c.id || ''] || []).push(calls.length)
        calls.push({ id: c.id || '', name: (c.function || {}).name || '', lineN: line.n,
          receipt: null, claimed: false, qpos: -1 })
      })
      return
    }
    if (line.role === 'tool') {
      const idxs = idxById[line.tool_call_id || ''] || []
      for (let k = 0; k < idxs.length; k++) {
        if (calls[idxs[k]].receipt === null) {
          calls[idxs[k]].receipt = String(line.text || '')
          break
        }
      }
    }
  })

  const queue: ImageCallEntry[] = []
  const candRound: Record<string, number> = {}
  const roundLast: Record<string, string> = {}
  calls.forEach((c) => {
    const img = c.receipt === null
      ? !!IMAGE_TOOLS[c.name]
      : IMAGE_RESULT_RE.test(c.receipt.trim())
    if (img) {
      c.qpos = queue.length
      queue.push(c)
      candRound[c.id] = c.lineN
      roundLast[c.lineN] = c.id
    }
  })

  let firstTask = 0
  state.lines.forEach((l: Line) => {
    if (!l || l.bad || (l.t && l.t !== 'msg')) return
    if (l.role === 'user' && !(l.images && l.images.length) && !firstTask) firstTask = l.n
  })

  const map: Record<number, ImageAttr> = {}
  let qi = 0
  // 只能领**这一行之前**发生过的调用——图片是回执之后才回来的；排在后面的调用
  // 还没发生（会话开头先投图的纯图片行就是这么落到「本会话任务」上的）。
  const nextUnclaimed = (lineN: number): ImageCallEntry | null => {
    while (qi < queue.length && queue[qi].claimed) qi++
    if (qi >= queue.length || queue[qi].lineN >= lineN) return null
    return queue[qi]
  }

  const claim = (entry: ImageAttr, pick: ImageCallEntry, how: ImageAttr['how']) => {
    pick.claimed = true
    entry.kind = 'call'
    entry.how = how
    entry.callId = pick.id
    if (!entry.name) entry.name = pick.name
  }

  state.lines.forEach((line: Line) => {
    if (!line || line.bad || (line.t && line.t !== 'msg')) return
    if (line.role !== 'user' || !(line.images && line.images.length)) return

    const text = String(line.text || '').trim()
    const entry: ImageAttr = { kind: 'none', how: '', callId: '', name: '', lineN: line.n, taskLineN: firstTask }

    // 有正文又不是句柄 = 这一轮就是**任务本身** → 归本会话任务，FIFO 不许抢它。
    if (text && !IMAGE_HANDLE_RE.test(text)) {
      entry.kind = 'task'
      entry.how = 'task'
      entry.taskLineN = line.n
      map[line.n] = entry
      return
    }

    let pick: ImageCallEntry | null = null
    if (IMAGE_HANDLE_RE.test(text)) {
      const idm = IMAGE_CALL_RE.exec(text)
      const frm = IMAGE_FROM_RE.exec(text)
      entry.name = frm ? frm[1] : ''
      if (idm) {
        // 精确匹配优先：同 id 的条目里取"这一行之前、还没被认领"的最后一个
        // （重放时同一个 call id 会出现两次）。
        const cands = idxById[idm[1]] || []
        for (let k = cands.length - 1; k >= 0; k--) {
          const ex = calls[cands[k]]
          if (ex.qpos >= 0 && !ex.claimed && ex.lineN < line.n) { pick = ex; break }
        }
        if (!pick && idxById[idm[1]]) {
          // 不在队列里（那条调用没产图/还没回执）也要认句柄写明的归属。
          pick = { id: idm[1], name: entry.name || '', lineN: 0, receipt: null, claimed: false, qpos: -1 }
        }
        if (pick) claim(entry, pick, 'call-id')
      }
      if (!pick && entry.name) {
        for (let j = qi; j < queue.length; j++) {
          if (queue[j].lineN >= line.n) break
          if (!queue[j].claimed && queue[j].name === entry.name) {
            pick = queue[j]
            claim(entry, pick, 'tool-name')
            break
          }
        }
      }
    }
    if (!pick) {
      pick = nextUnclaimed(line.n)
      if (pick) claim(entry, pick, 'order')
    }
    if (!pick) {
      // 队列里没有可领的产图调用：纯图片行当**任务投喂的原图**；句柄却配不上 → 未识别。
      if (!IMAGE_HANDLE_RE.test(text)) {
        entry.kind = 'task'
        entry.how = 'task'
        entry.taskLineN = text ? line.n : (firstTask || line.n)
      }
    }
    map[line.n] = entry
  })

  state.imgAttr = { sig, map, candRound, roundLast }
  // 返回缓存里的引用（而非局部 map）：命中/未命中两条路径交给调用方的是
  // 同一个对象（reactive 包装后引用一致），调用方才能安全做"是否重算"判断。
  return state.imgAttr.map
}

// 归属依据的中文说明（对话页的附件脚注与轨迹页共用同一套说法）。
export function attributionText(attr: ImageAttr | null | undefined): string {
  if (!attr) return ''
  switch (attr.how) {
    case 'call-id':
      return '归属：call ' + attr.callId + (attr.name ? '（' + attr.name + '）' : '')
    case 'tool-name':
      return '归属：call ' + attr.callId + '（按工具名 ' + attr.name + ' 匹配）'
    case 'order':
      return '归属：由顺序推断（本轮的 call ' + attr.callId + '）'
    case 'task':
      return '归属：本会话任务（这一段是投喂给任务的原图）'
    default:
      return '归属：未识别'
  }
}

/* ---------- 媒体路径 ---------- */

function sessionDir(id: string): string {
  const i = String(id || '').lastIndexOf('/')
  return i < 0 ? '' : String(id).slice(0, i)
}

export function mediaURL(ref: unknown): string {
  const tail = String(ref || '').replace(/^file:\/\//, '')
  const rel = joinPath(sessionDir(state.current ? state.current.id : ''), tail)
  return '/media/' + rel
}

function joinPath(...args: string[]): string {
  const parts: string[] = []
  for (const a of args) {
    const p = String(a || '').replace(/^\/+|\/+$/g, '')
    if (p) parts.push(p)
  }
  return parts.join('/')
}

/* ---------- 工具卡算法 ---------- */

/*
 * 工具家族：DSH 的调用行一眼能分出"读/写/跑命令/检索"，这里也用同一个
 * 思路——把工具名归成几族，只给工具名上色（不用左侧色条）。
 */
export function toolFamily(name: unknown): string {
  const n = String(name || '').toLowerCase()
  if (n === 'bash' || n === 'compile' || n === 'python') return 'shell'
  if (n === 'submit') return 'submit'
  if (n.indexOf('write') === 0 || n.indexOf('edit') === 0) return 'write'
  if (n.indexOf('grep') === 0 || n.indexOf('doc_search') === 0 ||
    n.indexOf('list_') === 0 || n.indexOf('search') >= 0) return 'search'
  if (n.indexOf('read') === 0 || n.indexOf('view_') === 0 || n.indexOf('image_context') === 0) return 'view'
  return 'other'
}

/*
 * 调用的"一行摘要"：参数里最能说明这次调用在干什么的那一个（命令/path/
 * pattern/query），折叠状态下也能读懂调用过程，不必逐个展开 JSON。
 */
export function toolSummary(name: unknown, argsText: unknown): string {
  let obj: Record<string, unknown> | null = null
  try { obj = JSON.parse(String(argsText || '{}')) as Record<string, unknown> } catch { obj = null }
  if (!obj || typeof obj !== 'object') return ''
  const n = String(name || '').toLowerCase()
  const pick = (v: unknown): string => {
    if (typeof v === 'string') return v
    if (v === undefined || v === null) return ''
    try { return JSON.stringify(v) } catch { return '' }
  }
  let v = ''
  if (n === 'bash' || n === 'python') v = pick(obj.command || obj.code || obj.script)
  else if (n === 'compile') v = pick(obj.path)
  else if (n.indexOf('grep') === 0 || n.indexOf('search') >= 0 || n === 'doc_search') v = pick(obj.pattern || obj.query)
  else if (n.indexOf('write') === 0 || n.indexOf('read') === 0 || n === 'view_pdf' || n === 'view_image') v = pick(obj.path)
  else if (n === 'submit') v = pick(obj.path || obj.status)
  if (!v) {
    // 兜底：参数里第一个非空字符串（顺序与模型给的参数顺序一致）。
    const keys = Object.keys(obj)
    for (let i = 0; i < keys.length; i++) {
      const cand = pick(obj[keys[i]])
      if (cand) { v = cand; break }
    }
  }
  v = String(v).split('\n')[0].replace(/\s+/g, ' ').trim()
  return v.length > 90 ? v.slice(0, 90) + '…' : v
}

/*
 * 展开后的"命令行首行"强调：JSON 高亮之外，把这次调用真正执行的那一行
 * 加一个 `$ ` 前导提示符并加重——输入侧只做这一点，别比输出更花。
 */
export function toolPromptLine(name: unknown, argsText: unknown): string {
  let obj: Record<string, unknown> | null = null
  try { obj = JSON.parse(String(argsText || '{}')) as Record<string, unknown> } catch { obj = null }
  if (!obj || typeof obj !== 'object') return ''
  const n = String(name || '').toLowerCase()
  let v: unknown = ''
  if (n === 'bash' || n === 'python') v = obj.command || obj.code || ''
  else if (n.indexOf('grep') === 0 || n === 'doc_search' || n.indexOf('search') >= 0) {
    v = [obj.pattern || obj.query || '', obj.path || ''].filter(Boolean).join('  ')
  } else if (n === 'compile' || n.indexOf('write') === 0 || n.indexOf('edit') === 0 ||
    n.indexOf('read') === 0 || n === 'view_pdf' || n === 'view_image') {
    v = obj.path || ''
  }
  const out = String(v).split('\n')[0].trim()
  return out.length > 160 ? out.slice(0, 160) + '…' : out
}

export function classifyResult(text: unknown): string {
  const s = String(text || '')
  if (/REJECTED|文件不存在|失败|error|not found|traceback/i.test(s)) return 'error'
  if (/\bok\s*\(/.test(s)) return 'ok'
  return 'plain'
}

/* ---------- 元信息状态（streamSummary 用；详情栏块复用） ---------- */

interface MetaLineInfo {
  line: Line
  count: number
  promptChars: number
  /** 本地估算（Go 侧下发）：显示单位是 token 时用它，单位是字符时用
   *  promptChars 的精确字符数——两个口径都留着，切换开关不用重新拉数据。 */
  promptTokenEst: number
}

function metaLines(): Line[] {
  const found: Line[] = []
  state.lines.forEach((l: Line) => { if (l.t === 'meta') found.push(l) })
  return found
}

export function metaState(): MetaLineInfo | null {
  const metas = metaLines()
  if (!metas.length && !metaOf(state.current)) return null
  const line = metas.length ? metas[metas.length - 1] : null
  if (!line) return null
  const scanned = metaOf(state.current) as { count?: number } | null
  return {
    line,
    count: Math.max(metas.length, scanned ? scanned.count || 0 : 0),
    promptChars: String(line.text || '').length,
    promptTokenEst: estOf(line).text,
  }
}
