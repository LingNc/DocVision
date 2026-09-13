/*
 * 对话时间线（旧页 renderLine 一族照搬区，块 3）：消息分节、思考块、工具卡、
 * 图片归属、系统消息、增量追加。这里是**命令式 DOM 构建**——与旧页逐函数
 * 对应，因为图片归属要做跨节点手术（attachImages 往别的行的卡片里塞内容），
 * 虚拟 DOM 不适合表达这种引用；视觉规则全部在整卷样式表里。
 *
 * 富文本（renderMarkdown / machineScroll / prettyJSON / copyButton）在
 * richtext.ts；数据与文案助手在 sidebar.ts / state.ts。
 */
import { state, layout, storeGet, storeSet, setBannerText, LONG_TEXT_LINES, SYSTEM_PREVIEW_LINES, renderDetails, renderTrajectory } from '../state'
import {
  aggregate, countText, estOf, firstLine, fmtCost, fmtDur, fmtTokens, indexCallEstimates,
  metaOf, usageLines,
} from './sidebar'
import { el, clear } from './dom'
import { renderMarkdown, machineScroll, prettyJSON, copyButton, foldLabel } from './richtext'
import { openLightbox, toggleDetails } from '../state'

/* ---------- 图片轮 / 任务块 ---------- */

// 图片引用（file://media/<sha>.jpg 之类）的**文件名**：长哈希只留前 8 位显示，
// 完整名字在缩略图的 title 与 alt 上（沿用 Go 侧"哈希当不了名字"的同一口径）。
export function refBaseName(ref: unknown): string {
  const s = String(ref || '').split('?')[0]
  const i = Math.max(s.lastIndexOf('/'), s.lastIndexOf('\\'))
  return i >= 0 ? s.slice(i + 1) : s
}

export function shortFileName(name: unknown): string {
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
function imageTurnSummary(line: any): string {
  const imgs = (line && line.images) || []
  const bits: string[] = [imgs.length + ' 张图片']
  const size = originalFigureSize(line && line.text)
  if (size) bits.push(size)
  const names = imgs.map((r: any) => shortFileName(refBaseName(r)))
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

// 归属结果按"会话 + 行数"缓存：渲染是增量的，行数变了就重算一遍。
function imageAttributions(): Record<number, any> {
  const sig = (state.current ? state.current.id : '') + ':' + state.lines.length
  if (state.imgAttr && state.imgAttr.sig === sig) return state.imgAttr.map

  /*
   * 第一步：按转录顺序记下**每一次调用**（id / 工具名 / 所在行，以及它的回执文本），
   * 再挑出"会产生图片"的那些排成 FIFO 队列。用**条目**而不是 callId 当队列元素，
   * 因为续跑/重放的转录里同一个 call id 会出现两次，按 id 去重会让重放段的图片
   * 全部认不出归属。
   */
  const calls: any[] = []
  const idxById: Record<string, number[]> = {}
  state.lines.forEach((line: any) => {
    if (!line || line.bad || (line.t && line.t !== 'msg')) return
    if (line.role === 'assistant') {
      ;(line.tool_calls || []).forEach((c: any) => {
        ;(idxById[c.id] = idxById[c.id] || []).push(calls.length)
        calls.push({ id: c.id, name: (c.function || {}).name || '', lineN: line.n,
          receipt: null, claimed: false, qpos: -1 })
      })
      return
    }
    if (line.role === 'tool') {
      const idxs = idxById[line.tool_call_id] || []
      for (let k = 0; k < idxs.length; k++) {
        if (calls[idxs[k]].receipt === null) {
          calls[idxs[k]].receipt = String(line.text || '')
          break
        }
      }
    }
  })

  const queue: any[] = []
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
  state.lines.forEach((l: any) => {
    if (!l || l.bad || (l.t && l.t !== 'msg')) return
    if (l.role === 'user' && !(l.images && l.images.length) && !firstTask) firstTask = l.n
  })

  const map: Record<number, any> = {}
  let qi = 0
  // 只能领**这一行之前**发生过的调用——图片是回执之后才回来的；排在后面的调用
  // 还没发生（会话开头先投图的纯图片行就是这么落到「本会话任务」上的）。
  const nextUnclaimed = (lineN: number) => {
    while (qi < queue.length && queue[qi].claimed) qi++
    if (qi >= queue.length || queue[qi].lineN >= lineN) return null
    return queue[qi]
  }

  const claim = (entry: any, pick: any, how: string) => {
    pick.claimed = true
    entry.kind = 'call'
    entry.how = how
    entry.callId = pick.id
    if (!entry.name) entry.name = pick.name
  }

  state.lines.forEach((line: any) => {
    if (!line || line.bad || (line.t && line.t !== 'msg')) return
    if (line.role !== 'user' || !(line.images && line.images.length)) return

    const text = String(line.text || '').trim()
    const entry: any = { kind: 'none', how: '', callId: '', name: '', lineN: line.n, taskLineN: firstTask }

    // 有正文又不是句柄 = 这一轮就是**任务本身** → 归本会话任务，FIFO 不许抢它。
    if (text && !IMAGE_HANDLE_RE.test(text)) {
      entry.kind = 'task'
      entry.how = 'task'
      entry.taskLineN = line.n
      map[line.n] = entry
      return
    }

    let pick: any = null
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
        if (!pick && state.callNodes[idm[1]]) {
          // 不在队列里（那条调用没产图/还没回执）也要认句柄写明的归属。
          pick = { id: idm[1], name: entry.name, claimed: false }
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
  return map
}

// 归属依据的中文说明（对话页的附件脚注与轨迹页共用同一套说法）。
export function attributionText(attr: any): string {
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

/* ---------- 折叠正文 / 缩略图 ---------- */

function collapsibleText(text: unknown, previewLines: number, key: string, extraClass?: string, tokens?: number): HTMLElement {
  const wrap = el('div', 'text-wrap')
  const lines = String(text === undefined || text === null ? '' : text).split('\n')
  const long = lines.length > previewLines
  const pre = el('pre', 'body-text' + (extraClass ? ' ' + extraClass : ''))
  let expanded = storeGet('text.' + key) === '1'
  const paint = () => {
    pre.textContent = (expanded || !long) ? lines.join('\n') : lines.slice(0, previewLines).join('\n')
    pre.classList.toggle('clamped', long && !expanded)
  }
  paint()
  wrap.appendChild(pre)
  if (long) {
    const toggle = el('button', 'text-toggle') as HTMLButtonElement
    const label = () => {
      toggle.textContent = foldLabel(expanded, lines.length, String(text).length, tokens)
    }
    label()
    toggle.type = 'button'
    toggle.addEventListener('click', () => {
      expanded = !expanded
      storeSet('text.' + key, expanded ? '1' : '0')
      paint()
      label()
    })
    wrap.appendChild(toggle)
  }
  return wrap
}

// 一张缩略图：点它进灯箱看原图（图片条与"看图类调用的预览行"共用）。
function thumbImg(ref: string, cls?: string): HTMLImageElement {
  const url = mediaURL(ref)
  const img = el('img', cls || 'thumb') as HTMLImageElement
  img.src = url
  img.alt = ref
  img.loading = 'lazy'
  img.title = ref
  img.addEventListener('click', () => { openLightbox(url, ref) })
  return img
}

function imageStrip(line: any): HTMLElement {
  const strip = el('div', 'images')
  ;(line.images || []).forEach((ref: string) => { strip.appendChild(thumbImg(ref)) })
  return strip
}

/*
 * Markdown 正文：行数超过 previewLines 时先夹住（max-height + 渐隐遮罩），
 * 展开/收起沿用与纯文本路径同一套按钮与记忆键。
 */
function markdownText(text: unknown, previewLines: number, key: string, extraClass?: string, tokens?: number): HTMLElement {
  const wrap = el('div', 'text-wrap')
  const raw = String(text === undefined || text === null ? '' : text)
  const body = el('div', 'md-body' + (extraClass ? ' ' + extraClass : ''))
  body.appendChild(renderMarkdown(raw))
  const lines = raw.split('\n')
  const long = lines.length > previewLines
  let expanded = storeGet('text.' + key) === '1'
  const paint = () => {
    const clamped = long && !expanded
    body.classList.toggle('clamped', clamped)
    body.style.maxHeight = clamped ? (previewLines * 24) + 'px' : ''
  }
  paint()
  wrap.appendChild(body)
  if (long) {
    const toggle = el('button', 'text-toggle') as HTMLButtonElement
    const label = () => {
      toggle.textContent = foldLabel(expanded, lines.length, raw.length, tokens)
    }
    label()
    toggle.type = 'button'
    toggle.addEventListener('click', () => {
      expanded = !expanded
      storeSet('text.' + key, expanded ? '1' : '0')
      paint()
      label()
    })
    wrap.appendChild(toggle)
  }
  return wrap
}

function bodyBlock(text: unknown, previewLines: number, key: string, extraClass?: string, tokens?: number): HTMLElement {
  if (!state.markdown) return collapsibleText(text, previewLines, key, extraClass, tokens)
  return markdownText(text, previewLines, key, extraClass, tokens)
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

/* ---------- 折叠行（思考 / 工具调用 / 图片轮共用骨架） ---------- */

/*
 * 折叠行（思考 / 工具调用）折叠时只占一行：
 *   [16px 图标位（含折叠箭头）] 名称 13px ·(2px 圆点) 一行摘要 13px 省略号
 */
function disclosureLine(cls: string, name: string, summary: string, tail?: string, open?: boolean): HTMLDetailsElement {
  const details = document.createElement('details')
  details.className = 'disclosure ' + cls
  details.open = !!open
  const head = el('summary')
  const slot = el('span', 'line-slot')
  slot.appendChild(el('span', 'line-caret'))
  head.appendChild(slot)
  head.appendChild(el('span', 'line-name', name))
  if (summary) {
    head.appendChild(el('span', 'line-sep'))
    const s = el('span', 'line-summary', summary)
    s.title = summary
    head.appendChild(s)
  }
  if (tail) head.appendChild(el('span', 'line-tail', tail))
  details.appendChild(head)
  return details
}

function thinkingDisclosure(line: any): HTMLDetailsElement {
  const remembered = storeGet('thinking.' + state.current.id + '.' + line.n) === '1'
  const d = disclosureLine('disclosure-thinking', '思考', firstLine(line.reasoning),
    countText(line.reasoning.length, estOf(line).reasoning), !state.forceCollapse && remembered)
  const body = el('div', 'thinking-body')
  const scroll = el('div', 'reasoning-scroll')
  scroll.appendChild(el('pre', 'body-text reasoning-text', line.reasoning))
  body.appendChild(scroll)
  d.appendChild(body)
  d.addEventListener('toggle', () => {
    if (state.forceCollapse) return
    storeSet('thinking.' + state.current.id + '.' + line.n, d.open ? '1' : '0')
  })
  return d
}

/* ---------- 工具卡 ---------- */

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
  let obj: any = null
  try { obj = JSON.parse(String(argsText || '{}')) } catch { obj = null }
  if (!obj || typeof obj !== 'object') return ''
  const n = String(name || '').toLowerCase()
  const pick = (v: any): string => {
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
function toolPromptLine(name: unknown, argsText: unknown): string {
  let obj: any = null
  try { obj = JSON.parse(String(argsText || '{}')) } catch { obj = null }
  if (!obj || typeof obj !== 'object') return ''
  const n = String(name || '').toLowerCase()
  let v: any = ''
  if (n === 'bash' || n === 'python') v = obj.command || obj.code || ''
  else if (n.indexOf('grep') === 0 || n === 'doc_search' || n.indexOf('search') >= 0) {
    v = [obj.pattern || obj.query || '', obj.path || ''].filter(Boolean).join('  ')
  } else if (n === 'compile' || n.indexOf('write') === 0 || n.indexOf('edit') === 0 ||
    n.indexOf('read') === 0 || n === 'view_pdf' || n === 'view_image') {
    v = obj.path || ''
  }
  v = String(v).split('\n')[0].trim()
  return v.length > 160 ? v.slice(0, 160) + '…' : v
}

function cmdPreview(cmd: string): HTMLElement {
  const div = el('div', 'io-cmd')
  div.appendChild(el('span', 'cmd-prompt', '$ '))
  div.appendChild(el('span', 'cmd-line', cmd))
  return div
}

export function classifyResult(text: unknown): string {
  const s = String(text || '')
  if (/REJECTED|文件不存在|失败|error|not found|traceback/i.test(s)) return 'error'
  if (/\bok\s*\(/.test(s)) return 'ok'
  return 'plain'
}

function ioSection(label: string, text: unknown, isError: boolean, key: string, tokens?: number): HTMLElement {
  const section = el('div', 'io-section')
  section.appendChild(el('div', 'io-label', label))
  if (text === undefined || text === null || text === '') {
    section.appendChild(el('div', 'io-empty', '（无内容）'))
    return section
  }
  // 输入输出都走同一套机器文本渲染：是 JSON 就做 JSON 高亮（k/s/n/b），
  // 否则按终端输出/diff/日志着色；超过限高给「展开全文」，与思考块同一套。
  const wrap = machineScroll(text, key, undefined, tokens)
  if (isError) {
    const pre = wrap.querySelector('.io-text')
    if (pre) pre.setAttribute('data-error', 'true')
  }
  section.appendChild(wrap)
  return section
}

interface CallNode {
  details: HTMLDetailsElement
  card: HTMLElement
  summary: string
  tail: HTMLElement | null
  argsChars: number
  argsTokens: number
  result: { line: any; status: string } | null
  attachments?: number
  preview?: HTMLElement | null
}

/*
 * 工具调用行：折叠时一行（图标位 + 工具名 + 圆点 + 摘要），展开时是
 * 「输入 / 输出」两段的卡片。输出来自配对的那条 role="tool" 行——两种模式
 * 共用同一条路径：直播模式里结果晚到，就补进同一个节点。
 */
function toolDisclosure(call: any): CallNode {
  const fn = call.function || {}
  const name = fn.name || '(未命名工具)'
  state.toolSeq++
  state.calls.set(call.id, { name, seq: state.toolSeq })

  const argsText = String(fn.arguments || '')
  const brief = toolSummary(name, fn.arguments)
  const argsTokens = callTokensOf(call)
  const d = disclosureLine('disclosure-tool fam-' + toolFamily(name), name, brief,
    countText(argsText.length, argsTokens), storeGet('call.' + state.current.id + '.' + call.id) === '1')
  d.setAttribute('data-call-id', call.id || '')
  const tail = d.querySelector('.line-tail') as HTMLElement | null

  const card = el('div', 'io-card')
  const cmdLine = toolPromptLine(name, argsText)
  if (cmdLine) card.appendChild(cmdPreview(cmdLine))
  card.appendChild(ioSection('输入', prettyJSON(argsText) || '(无参数)', false, 'in.' + (call.id || ''), argsTokens))
  const actions = el('div', 'io-actions')
  actions.appendChild(copyButton(argsText))
  card.appendChild(actions)
  d.appendChild(card)
  d.addEventListener('toggle', () => {
    storeSet('call.' + state.current.id + '.' + call.id, d.open ? '1' : '0')
  })

  const node: CallNode = { details: d, card, summary: brief, tail,
    argsChars: argsText.length, argsTokens, result: null }
  if (call.id) state.callNodes[call.id] = node
  return node
}

/*
 * 一次调用的参数估算：由 Go 侧下发（line.est.calls，与 calls 一一对应），
 * 前端只取用、不自己算——两套公式必然漂移。
 */
function callTokensOf(call: any): number {
  const est = state.callEst[call.id]
  return est === undefined ? 0 : est
}

// 折叠行的尾巴：`输入 → 输出 计数 · ok/error`（再加附件张数）。
// 一行里就能看出这次调用吃了多少、回了多少、成没成、带了几张图，不必展开。
function updateCallTail(node: CallNode): void {
  if (!node.tail) return
  let tail = countText(node.argsChars, node.argsTokens)
  if (node.result) {
    const text = String(node.result.line.text || '')
    tail = countText(node.argsChars, node.argsTokens) + ' → ' +
      countText(text.length, estOf(node.result.line).text) +
      (node.result.status === 'error' ? ' · error' : node.result.status === 'ok' ? ' · ok' : '')
  }
  if (node.attachments) tail += ' · 附件 ' + node.attachments + ' 张（user 轮）'
  node.tail.textContent = tail
}

// 把一条工具回执补进它对应的调用卡片（同一次调用 = **一行**；配不上的回执
// 另起一行，见 standaloneResult）。
function attachResult(node: CallNode, line: any): CallNode {
  const text = String(line.text || '')
  const status = classifyResult(text)
  node.result = { line, status }
  node.card.appendChild(el('div', 'io-divider'))
  const out = ioSection('输出', text, status === 'error', 'result.' + line.n, estOf(line).text)
  // 精确匹配时图片就是这个工具的**输出**，要贴在输出正文正下方；
  // 用类名把这一段认出来（ioSection 只认标签文本，没法从外面找）。
  out.classList.add('out-section')
  node.card.appendChild(out)
  const actions = el('div', 'io-actions')
  actions.appendChild(copyButton(text))
  node.card.appendChild(actions)
  node.details.classList.add('status-' + status)
  updateCallTail(node)
  return node
}

/*
 * 看图类调用的**缩略图预览行**：紧跟在那一行工具调用下面，**折叠态就可见**
 * （所以它是 <details> 的兄弟节点，不能塞进卡片里）。一轮里调了多次 view 时，
 * 这一轮的缩略图**全部并排挂在最后一个 view 行下面**，顺序按转录里的先后
 * ——所以每次都按行号重排一遍再重画。
 */
function addPreview(host: CallNode, line: any): HTMLElement | null {
  if (!host || !host.details || !host.details.parentNode) return null
  let strip = host.preview
  if (!strip || !strip.parentNode) {
    strip = el('div', 'preview-strip')
    ;(strip as any).__items = []
    host.preview = strip
    const sec = host.details.parentNode
    sec.insertBefore(strip, host.details.nextSibling)
  }
  ;(strip as any).__items.push(line)
  ;(strip as any).__items.sort((a: any, b: any) => a.n - b.n)
  clear(strip)
  ;(strip as any).__items.forEach((l: any) => {
    ;(l.images || []).forEach((ref: string) => { strip!.appendChild(thumbImg(ref, 'preview-thumb')) })
  })
  return strip
}

// 缩略图预览挂哪一行：这一轮里**最后一个**能产图的看图调用（"一轮多 view"）。
function previewHost(host: CallNode, attr: any): CallNode {
  const info = state.imgAttr
  if (!info || !attr || !attr.callId) return host
  const round = info.candRound ? info.candRound[attr.callId] : undefined
  const lastId = (round !== undefined && info.roundLast) ? info.roundLast[round] : ''
  const node = lastId && state.callNodes ? state.callNodes[lastId] : null
  return (node as CallNode) || host
}

/*
 * 图片轮到哪一行去：精确匹配时图片是那次调用的**输出**，贴「输出」正文
 * 正下方、不再另立「附件（user 轮）」分区，卡片结尾只留一行小字归属说明；
 * 推断出来的（FIFO / 未识别）仍按**附件段**呈现。
 */
function attachImages(host: any, line: any, attr: any): any {
  const imgs = line.images || []
  if (!imgs.length) return host
  // 不论怎么归属，看图类调用的行下面都要挂缩略图预览（折叠态可见）。
  if (host.details) addPreview(previewHost(host, attr), line)
  // 宿主可以是调用卡片（node.card）也可以是系统消息块（任务提示）：图片归属
  // 决定挂哪儿——工具图片回执挂那次调用，会话开头投喂的原图挂任务。
  const outSec = attr && attr.how === 'call-id' && host.card
    ? host.card.querySelector('.io-section.out-section') : null
  if (outSec) {
    // 图片 = 这次调用的输出：接在输出正文的下一个块（text-wrap 里、滚动区之外，
    // 所以长输出内滚也不会把图片卷进去）。
    const col = outSec.querySelector('.text-wrap') || outSec
    col.appendChild(imageStrip(line))
    col.appendChild(el('div', 'attach-note',
      '归属：call ' + attr.callId + (attr.name ? '（' + attr.name + '，精确匹配）' : '（精确匹配）')))
    host.attachments = (host.attachments || 0) + imgs.length
    if (host.details) updateCallTail(host)
    return host
  }
  const box = host.card || host
  box.appendChild(el('div', 'io-divider'))
  const section = el('div', 'io-section attach-section')
  section.appendChild(el('div', 'io-label', '附件（user 轮）'))
  const body = el('div', 'attach-body')
  // 任务行**自己**带的图（attr.taskLineN 就是这一行）：正文已经在任务块里了，
  // 附件段只放图，不把任务提示原样再抄一遍。
  if (line.text && line.n !== (attr && attr.taskLineN)) {
    body.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'imgtext.' + line.n, undefined, estOf(line).text))
  }
  body.appendChild(imageStrip(line))
  section.appendChild(body)
  section.appendChild(el('div', 'attach-note',
    '这一轮是 user 轮发出的（' + (attr && attr.kind === 'task' ? '会话开头的原图投喂，作为任务的输入' : '工具的输入/附件') +
    '）· ' + attributionText(attr)))
  box.appendChild(section)
  host.attachments = (host.attachments || 0) + imgs.length
  if (host.details) updateCallTail(host)
  return host
}

function standaloneResult(line: any): HTMLDetailsElement {
  const info = state.calls.get(line.tool_call_id) as any
  const name = info ? info.name : '(未配对的工具回执)'
  const text = String(line.text || '')
  const status = classifyResult(text)
  const d = disclosureLine('disclosure-result status-' + status,
    name, firstLine(text), countText(text.length, estOf(line).text) +
    (status === 'error' ? ' · error' : status === 'ok' ? ' · ok' : ''))
  const card = el('div', 'io-card')
  card.appendChild(ioSection('输出', text, status === 'error', 'result.' + line.n, estOf(line).text))
  const actions = el('div', 'io-actions')
  actions.appendChild(copyButton(text))
  card.appendChild(actions)
  d.appendChild(card)
  if (!info) {
    d.title = line.tool_call_id
      ? '未找到配对的工具调用：id ' + line.tool_call_id
      : '这条回执行没有 tool_call_id，无法与调用配对'
  }
  return d
}

/*
 * 图片轮自己的折叠行（配不上调用时）：与工具行同一套词汇——16px 插槽 +
 * 名称 + 圆点 + 一行摘要 + 右侧张数；**默认折叠**，点开才看具体图片。
 */
function imageTurnRow(line: any, attr: any): HTMLDetailsElement {
  const imgs = line.images || []
  const key = 'image.' + state.current.id + '.' + line.n
  const d = disclosureLine('disclosure-image', '图片（user 轮）', imageTurnSummary(line),
    imgs.length + ' 张', storeGet(key) === '1')
  const body = el('div', 'image-body')
  if (line.text) {
    body.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'imgtext.' + line.n, undefined, estOf(line).text))
  }
  body.appendChild(imageStrip(line))
  if (attr) body.appendChild(el('div', 'attach-note', attributionText(attr)))
  d.appendChild(body)
  d.title = '这一轮是 user 轮发出的（把图片投给模型），不是人打的字\n' +
    IMAGE_WIRE_TITLE + '\n' + attributionText(attr) + '\n' + imageTurnSummary(line)
  d.addEventListener('toggle', () => { storeSet(key, d.open ? '1' : '0') })
  return d
}

// 图片轮做成一条独立消息行（左对齐，永远不靠右）。
function imageTurnSection(line: any, attr: any): HTMLElement {
  const wrap = el('section', 'msg msg-image')
  wrap.appendChild(imageTurnRow(line, attr))
  return wrap
}

/* ---------- 系统消息（不带图的 user 行 = harness 任务提示） ---------- */

function systemTurnSection(line: any): HTMLElement {
  const msg = el('section', 'msg msg-system')
  const head = el('div', 'sys-line')
  head.appendChild(el('span', 'sys-badge', '系统'))
  head.appendChild(el('span', 'sys-meta', 'user 轮'))
  head.appendChild(el('span', 'line-summary', firstLine(line.text)))
  msg.appendChild(head)

  const raw = String(line.text || '')
  if (!raw) {
    msg.appendChild(el('div', 'note', '（无正文）'))
    return msg
  }
  const key = 'sys.' + state.current.id + '.' + line.n
  const lines = raw.split('\n')
  const long = lines.length > SYSTEM_PREVIEW_LINES
  let expanded = storeGet('text.' + key) === '1'
  const scroll = el('div', 'sys-scroll')
  // 正文按 Markdown 开关渲染（与消息正文同一个渲染器），但**折叠只由这一层
  // 负责**：不套 bodyBlock 的第二层折叠，否则会冒出两个「展开全文」按钮。
  if (state.markdown) {
    const md = el('div', 'md-body sys-md')
    md.appendChild(renderMarkdown(raw))
    scroll.appendChild(md)
  } else {
    scroll.appendChild(el('pre', 'body-text sys-text', raw))
  }
  const paint = () => {
    scroll.classList.toggle('folded', long && !expanded)
    scroll.style.maxHeight = (long && !expanded)
      ? (SYSTEM_PREVIEW_LINES * 24) + 'px'
      : 'var(--code-scroll-h)'
  }
  paint()
  msg.appendChild(scroll)
  if (long) {
    const toggle = el('button', 'text-toggle') as HTMLButtonElement
    toggle.type = 'button'
    const label = () => {
      toggle.textContent = foldLabel(expanded, lines.length, raw.length, estOf(line).text)
    }
    label()
    toggle.addEventListener('click', () => {
      expanded = !expanded
      storeSet('text.' + key, expanded ? '1' : '0')
      paint()
      label()
    })
    msg.appendChild(toggle)
  }
  msg.title = '这一轮是 user 角色发出的任务提示（系统性质，不是人打的字）'
  return msg
}

/* ---------- renderLine 与时间线组装 ---------- */

// 会话开头的原图轮排在任务前面时，先记账，等任务行渲染时再挂上去。
let pendingTaskImages: Record<number, { line: any; attr: any }[]> = {}

function anchor(node: HTMLElement, line: any): HTMLElement {
  if (line && line.n !== undefined) state.anchors[line.n] = node
  return node
}

/* Returns the element for one transcript line, or null when it is skipped. */
export function renderLine(line: any, idx: number): HTMLElement | null {
  void idx
  if (line.bad) {
    state.badLines++
    updateBanner()
    return null
  }
  // meta 行不是消息：它由详情栏的元信息块渲染，绝不进消息序列。
  if (line.t && line.t !== 'msg') return null
  if (!line.role) return null

  const isToolCall = line.role === 'assistant' && line.tool_calls && line.tool_calls.length > 0
  // 带图的 user 行是"把图片投给模型"的那一轮（图片投喂 / 工具回执），
  // 不是用户的话，所以「仅看工具调用」里也要看得见它。
  const isImageTurn = line.role === 'user' && !!(line.images && line.images.length)
  if (state.onlyTools && line.role !== 'tool' && !isToolCall && !isImageTurn) return null

  if (line.role === 'user') {
    /*
     * 两条路都不靠右对齐（右侧气泡那套已经去掉）：
     *   · **带图**的 user 行是图片投喂/工具图片回执 → 按归属挂到那一次调用
     *     （任务自己的图则收进任务块）上，配不上才单独一行折叠行；
     *   · **不带图**的 user 行是 harness 自己发的长任务提示 → 按**系统消息**呈现。
     */
    if (isImageTurn) {
      const attr = imageAttributions()[line.n] || { kind: 'none', how: '', callId: '', taskLineN: 0 }
      if (attr.kind === 'call' && state.callNodes[attr.callId]) {
        const callNode = state.callNodes[attr.callId]
        attachImages(callNode, line, attr)
        // 卡片已经在它那条助手消息的 section 里了：**只登记锚点、不返回节点**
        // （返回就会被调用方 appendChild 到 .stream 上，把这一行从消息里拽出来）。
        anchor(callNode.details, line)
        return null
      }
      if (attr.kind === 'task' && attr.taskLineN === line.n) {
        // 任务行**自己**带图：这一行照系统消息/任务块的样式渲染，图片作为
        // **它的附件**收在同一个块里，绝不塞进任何工具调用。
        const own = systemTurnSection(line)
        attachImages(own, line, attr)
        return anchor(own, line)
      }
      if (attr.kind === 'task' && attr.taskLineN) {
        const taskNode = state.anchors[attr.taskLineN]
        if (taskNode) {
          attachImages(taskNode, line, attr)
          return anchor(taskNode, line)
        }
        // 会话开头的原图轮排在任务**前面**：这时任务节点还没建出来，先记账，
        // 等任务行渲染时再挂上去（否则只能退化成一条孤立的图片行）。
        pendingTaskImages[attr.taskLineN] = pendingTaskImages[attr.taskLineN] || []
        pendingTaskImages[attr.taskLineN].push({ line, attr })
        return null
      }
      return anchor(imageTurnSection(line, attr), line)
    }
    const msg = systemTurnSection(line)
    const waiting = pendingTaskImages[line.n]
    if (waiting && waiting.length) {
      waiting.forEach((w) => { attachImages(msg, w.line, w.attr) })
      delete pendingTaskImages[line.n]
    }
    return anchor(msg, line)
  }

  if (line.role === 'tool') {
    // 已配对的调用行已经带了输出，这一行就并入那张卡片（节点已在 DOM 里，
    // 这里返回 null）：**一次调用只占一行**，不另起一行"结果"。
    const node = line.tool_call_id ? state.callNodes[line.tool_call_id] : null
    if (node) {
      attachResult(node, line)
      const lines0 = node.details.getAttribute('data-lines')
      node.details.setAttribute('data-lines', lines0 ? (lines0 + ',' + line.n) : String(line.n))
      anchor(node.details, line)
      return null
    }
    const standalone = el('section', 'msg msg-tool')
    standalone.appendChild(standaloneResult(line))
    return anchor(standalone, line)
  }

  if (line.role === 'assistant') {
    const msg = el('section', 'msg msg-assistant')
    if (line.reasoning) {
      // 思考行不是独立步骤：轨迹里"思考"这一行跳回的就是这条助手消息。
      msg.appendChild(thinkingDisclosure(line))
    }
    if (line.text) {
      msg.appendChild(bodyBlock(line.text, LONG_TEXT_LINES, 'asst.' + line.n, undefined, estOf(line).text))
    }
    ;(line.tool_calls || []).forEach((call: any) => {
      msg.appendChild(toolDisclosure(call).details)
    })
    if (!line.text && !line.reasoning && !(line.tool_calls || []).length) {
      msg.appendChild(el('div', 'note', '（空消息）'))
    }
    return anchor(msg, line)
  }

  const other = el('section', 'msg msg-other')
  other.appendChild(bodyBlock(line.text || '(无正文)', LONG_TEXT_LINES, 'other.' + line.n, undefined, estOf(line).text))
  return anchor(other, line)
}

/*
 * 消息流开头只留一行极简摘要——指标与元信息都在右侧详情栏里，
 * 这里只写"这个会话有多大、花了多少"，点一下就能打开详情。
 */
function streamSummary(): HTMLElement | null {
  if (!state.current) return null
  const bar = el('div', 'stream-summary')
  const bits: string[] = [state.current.messages + ' 条消息']
  const st = aggregate(usageLines(state.lines))
  if (st) {
    bits.push('输入 ' + fmtTokens(st.promptTokens) + ' / 输出 ' + fmtTokens(st.completionTokens))
    if (st.promptTokens) bits.push('缓存 ' + st.cacheHitPct.toFixed(0) + '%')
    if (st.avgTtftMs) bits.push('首字 ' + fmtDur(st.avgTtftMs))
    const money = fmtCost(state.current.cost, false)
    if (money) bits.push(money)
  }
  const m = metaState()
  if (m) bits.push('提示词快照 ' + countText(m.promptChars, m.promptTokenEst))
  bits.forEach((b, i) => {
    if (i) bar.appendChild(el('span', 'dot-sep'))
    bar.appendChild(el('span', null, b))
  })
  const btn = el('button', null, layout.cols.details > 0 ? '收起详情' : '详情 ›') as HTMLButtonElement
  btn.type = 'button'
  btn.addEventListener('click', () => {
    toggleDetails()
    btn.textContent = layout.cols.details > 0 ? '收起详情' : '详情 ›'
  })
  bar.appendChild(btn)
  return bar
}

function timelineEmptyText(): string {
  if (state.onlyTools) return '这个会话没有工具调用记录'
  if (!state.current) return '左侧选择一个会话开始浏览。'
  return '这个会话还没有可显示的消息'
}

function streamNode(container: HTMLElement): Element | null {
  return container.querySelector('.stream')
}

export function renderTimeline(): void {
  const container = document.getElementById('timeline')
  if (!container) return
  clear(container)
  pendingTaskImages = {}
  state.msgSeq = 0
  state.toolSeq = 0
  state.badLines = 0
  state.calls = new Map()
  state.callNodes = {}
  state.anchors = {}

  const stream = el('div', 'stream')
  container.appendChild(stream)
  let rendered = 0
  const summary = streamSummary()
  if (summary) { stream.appendChild(summary); rendered++ }
  state.lines.forEach((line: any, idx: number) => {
    const node = renderLine(line, idx)
    if (node) { stream.appendChild(node); rendered++ }
  })
  if (rendered <= (summary ? 1 : 0)) {
    stream.appendChild(el('div', 'empty', timelineEmptyText()))
  }
  updateBanner()
  renderDetails()
  if (state.view === 'trajectory') renderTrajectory()
}

export function appendLines(lines: any[]): void {
  if (!lines || !lines.length) return
  const container = document.getElementById('timeline')
  if (!container) return
  const stream = streamNode(container)
  let hasMeta = false
  indexCallEstimates(lines)
  lines.forEach((line: any) => {
    state.lines.push(line)
    if (line.t === 'meta') hasMeta = true
  })
  if (!stream) {
    renderTimeline()
    return
  }
  const placeholder = stream.querySelector('.empty')
  // 增量追加时行号要接着已渲染的部分数：图片轮配对要用 state.lines 里的下标。
  const base = state.lines.length - lines.length
  lines.forEach((line: any, idx: number) => {
    const node = renderLine(line, base + idx)
    if (node) stream.appendChild(node)
  })
  if (placeholder && placeholder.parentNode && stream.children.length > 1) {
    stream.removeChild(placeholder)
  }
  updateBanner()
  if (hasMeta) renderDetails()
  else if (lines.some((l: any) => l.t === 'usage')) renderDetails()
  if (state.view === 'trajectory') renderTrajectory()
  if (state.follow) scrollToBottom()
}

export function scrollToBottom(): void {  const t = document.getElementById('timeline')
  if (t) t.scrollTop = t.scrollHeight
}

function updateBanner(): void {
  if (state.badLines > 0) {
    setBannerText('已跳过 ' + state.badLines + ' 行坏数据（无法解析为 JSON，可能是一次写入中途读到的不完整行）')
  } else {
    setBannerText('')
  }
}

/* ---------- 元信息状态（streamSummary 用；详情栏块复用） ---------- */

function metaLines(): any[] {
  const found: any[] = []
  state.lines.forEach((l: any) => { if (l.t === 'meta') found.push(l) })
  return found
}

export function metaState(): any {
  const metas = metaLines()
  if (!metas.length && !metaOf(state.current)) return null
  const line = metas.length ? metas[metas.length - 1] : null
  if (!line) return null
  const scanned = metaOf(state.current)
  return {
    line,
    count: Math.max(metas.length, scanned ? scanned.count : 0),
    promptChars: String(line.text || '').length,
    // 本地估算（Go 侧下发）：显示单位是 token 时用它，单位是字符时用上面的
    // 精确字符数——两个口径都留着，切换开关不用重新拉数据。
    promptTokenEst: estOf(line).text,
  }
}
