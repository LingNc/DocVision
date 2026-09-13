/*
 * 消息流**数据模型**（块 C 组件化的核心）：把 state.lines 一遍扫成渲染条目。
 * 命令式版本的跨节点手术（attachResult / attachImages / pendingTaskImages）
 * 在这里全部变成"条目之间的归属记账"——工具回执与图片轮不再操作别人的
 * DOM，而是记进对应调用条目的 results / attachments / previews，由组件
 * 模板按条目渲染。顺序、过滤条件与旧页 renderLine 逐条对应。
 */
import { state, LONG_TEXT_LINES, SYSTEM_PREVIEW_LINES } from '../state'
import { countText, estOf, firstLine } from './sidebar'
import {
  imageAttributions, attributionText, classifyResult, toolFamily, toolSummary, toolPromptLine,
  imageTurnSummary, IMAGE_WIRE_TITLE,
} from './timeline'

export interface ResultItem { line: any; status: string }
export interface AttachItem { line: any; attr: any }

/* 一次工具调用（在对话页里 = 一行卡片）：输入 + 若干回执 + 归属进来的图片。 */
export interface CallItem {
  key: string
  call: any
  id: string
  name: string
  fam: string
  argsText: string
  argsTokens: number
  brief: string
  cmdLine: string
  memKey: string
  results: ResultItem[]
  /** FIFO / 未识别归属的图片轮（「附件（user 轮）」分区）。 */
  attachments: AttachItem[]
  /** 精确匹配（how=call-id）的图片轮：图 = 这次调用的输出，贴输出正文下方。 */
  outImages: AttachItem[]
  /** 看图类调用的缩略图预览行（"一轮多 view"挂在最后一个 view 行下面）。 */
  previews: any[]
  /** 并进这张卡片的行号（回执行 / 图片轮）：锚点与 data-lines 用。 */
  anchorNs: number[]
  lastStatus: string
}

export type StreamItem =
  | { type: 'assistant'; key: string; line: any; calls: CallItem[]; empty: boolean }
  | { type: 'system'; key: string; line: any; raw: string; long: boolean; attachments: AttachItem[] }
  | { type: 'imageTurn'; key: string; line: any; attr: any }
  | { type: 'result'; key: string; line: any; paired: boolean }
  | { type: 'other'; key: string; line: any }

function callItemsOf(line: any, sid: string): CallItem[] {
  return (line.tool_calls || []).map((call: any) => {
    const fn = call.function || {}
    const name = fn.name || '(未命名工具)'
    const argsText = String(fn.arguments || '')
    const argsTokens = callTokensOf(call)
    return {
      key: call.id || 'call' + line.n + '-' + (line.tool_calls || []).indexOf(call),
      call,
      id: call.id || '',
      name,
      fam: toolFamily(name),
      argsText,
      argsTokens,
      brief: toolSummary(name, fn.arguments),
      cmdLine: toolPromptLine(name, fn.arguments),
      memKey: 'call.' + sid + '.' + call.id,
      results: [],
      attachments: [],
      outImages: [],
      previews: [],
      anchorNs: [],
      lastStatus: '',
    }
  })
}

/*
 * 一次调用的参数估算：由 Go 侧下发（line.est.calls，与 calls 一一对应），
 * 前端只取用、不自己算——两套公式必然漂移。
 */
function callTokensOf(call: any): number {
  const est = state.callEst[call.id]
  return est === undefined ? 0 : est
}

/* 卡片尾巴：`输入 → 输出 计数 · ok/error`（再加附件张数）——最后一条回执定状态。 */
export function callTail(item: CallItem): string {
  let tail = countText(item.argsText.length, item.argsTokens)
  const last = item.results[item.results.length - 1]
  if (last) {
    const text = String(last.line.text || '')
    tail = countText(item.argsText.length, item.argsTokens) + ' → ' +
      countText(text.length, estOf(last.line).text) +
      (last.status === 'error' ? ' · error' : last.status === 'ok' ? ' · ok' : '')
  }
  const attachments = item.attachments.reduce((s, a) => s + ((a.line.images || []).length), 0) +
    item.outImages.reduce((s, a) => s + ((a.line.images || []).length), 0)
  if (attachments) tail += ' · 附件 ' + attachments + ' 张（user 轮）'
  return tail
}

/* 附件段脚注（任务自己的图与工具投喂的图说法不同）。 */
export function attachNote(a: AttachItem): string {
  return '这一轮是 user 轮发出的（' +
    (a.attr && a.attr.kind === 'task' ? '会话开头的原图投喂，作为任务的输入' : '工具的输入/附件') +
    '）· ' + attributionText(a.attr)
}

/* 精确匹配时贴在输出正文下方的一行小字。 */
export function outImageNote(a: AttachItem): string {
  const attr = a.attr
  return '归属：call ' + attr.callId + (attr.name ? '（' + attr.name + '，精确匹配）' : '（精确匹配）')
}

/* 图片轮折叠行的悬浮说明（三行：性质 / wire 事实 / 归属 + 摘要）。 */
export function imageTurnTitle(line: any, attr: any): string {
  return '这一轮是 user 轮发出的（把图片投给模型），不是人打的字\n' +
    IMAGE_WIRE_TITLE + '\n' + attributionText(attr) + '\n' + imageTurnSummary(line)
}

/*
 * 缩略图预览挂哪张卡：这一轮里**最后一个**能产图的看图调用（"一轮多 view"）。
 * candRound/roundLast 由归属算法给出，按 callId 查它所在轮的收尾调用。
 */
function previewTarget(attr: any, callById: Record<string, CallItem>): CallItem | null {
  const info = state.imgAttr
  if (!info || !attr || !attr.callId) return null
  const round = info.candRound ? info.candRound[attr.callId] : undefined
  const lastId = (round !== undefined && info.roundLast) ? info.roundLast[round] : ''
  return (lastId && callById[lastId]) || null
}

function systemItem(line: any, sid: string): { type: 'system'; key: string; line: any; raw: string; long: boolean; attachments: AttachItem[] } {
  const raw = String(line.text || '')
  return {
    type: 'system',
    key: 'sys' + line.n,
    line,
    raw,
    long: raw.split('\n').length > SYSTEM_PREVIEW_LINES,
    attachments: [],
  }
}

export interface StreamModel { items: StreamItem[]; bad: number }

export function streamModel(): StreamModel {
  const attrs = imageAttributions()
  const sid = state.current ? state.current.id : ''
  const items: StreamItem[] = []
  const callById: Record<string, CallItem> = {}
  const taskByN: Record<number, Extract<StreamItem, { type: 'system' }>> = {}
  const pendingForTask: Record<number, AttachItem[]> = {}
  let bad = 0

  state.lines.forEach((line: any) => {
    if (!line) return
    if (line.bad) { bad++; return }
    // meta 行不是消息：它由详情栏的元信息块渲染，绝不进消息序列。
    if (line.t && line.t !== 'msg') return
    if (!line.role) return

    const isToolCall = line.role === 'assistant' && line.tool_calls && line.tool_calls.length > 0
    // 带图的 user 行是"把图片投给模型"的那一轮（图片投喂 / 工具回执），
    // 不是用户的话，所以「仅看工具调用」里也要看得见它。
    const isImageTurn = line.role === 'user' && !!(line.images && line.images.length)
    if (state.onlyTools && line.role !== 'tool' && !isToolCall && !isImageTurn) return

    if (line.role === 'user') {
      if (isImageTurn) {
        const attr = attrs[line.n] || { kind: 'none', how: '', callId: '', taskLineN: 0 }
        if (attr.kind === 'call' && callById[attr.callId]) {
          const host = callById[attr.callId]
          // 不论怎么归属，看图类调用的行下面都要挂缩略图预览（折叠态可见）。
          const target = previewTarget(attr, callById) || host
          target.previews.push(line)
          if (attr.how === 'call-id') {
            // 图片 = 这次调用的输出：贴「输出」正文正下方。
            host.outImages.push({ line, attr })
          } else {
            host.attachments.push({ line, attr })
          }
          host.anchorNs.push(line.n)
          return
        }
        if (attr.kind === 'task' && attr.taskLineN === line.n) {
          // 任务行自己带图：图片作为它的附件收在同一个块里。
          const own = systemItem(line, sid)
          own.attachments.push({ line, attr })
          items.push(own)
          taskByN[line.n] = own
          return
        }
        if (attr.kind === 'task' && attr.taskLineN) {
          const task = taskByN[attr.taskLineN]
          if (task) {
            task.attachments.push({ line, attr })
            return
          }
          // 会话开头的原图轮排在任务前面：任务条目还没建，先记账（第二轮补挂）。
          ;(pendingForTask[attr.taskLineN] = pendingForTask[attr.taskLineN] || []).push({ line, attr })
          return
        }
        items.push({ type: 'imageTurn', key: 'img' + line.n, line, attr })
        return
      }
      const own = systemItem(line, sid)
      items.push(own)
      taskByN[line.n] = own
      return
    }

    if (line.role === 'tool') {
      // 已配对的调用卡片带上这段输出（一次调用只占一行）；配不上另起一行。
      const node = line.tool_call_id ? callById[line.tool_call_id] : null
      if (node) {
        const status = classifyResult(String(line.text || ''))
        node.results.push({ line, status })
        node.lastStatus = status
        node.anchorNs.push(line.n)
        return
      }
      items.push({ type: 'result', key: 'res' + line.n, line, paired: false })
      return
    }

    if (line.role === 'assistant') {
      const calls = callItemsOf(line, sid)
      calls.forEach((c) => { if (c.id) callById[c.id] = c })
      items.push({
        type: 'assistant',
        key: 'asst' + line.n,
        line,
        calls,
        empty: !line.text && !line.reasoning && !(line.tool_calls || []).length,
      })
      return
    }

    items.push({ type: 'other', key: 'other' + line.n, line })
  })

  // 兜底：how=call-id 的图片轮挂在「输出」正文下；那次调用还没有回执时
  // （命令式版本查不到 out-section 也走这条路）退回「附件（user 轮）」分区。
  Object.values(callById).forEach((c) => {
    if (c.outImages.length && !c.results.length) {
      c.attachments.push(...c.outImages)
      c.outImages = []
    }
  })

  // 第二轮：排在任务前面、暂存的会话开头投喂原图，挂到（后面才出现的）任务块。
  Object.keys(pendingForTask).forEach((n) => {
    const task = taskByN[Number(n)]
    if (task) task.attachments.push(...pendingForTask[Number(n)])
  })

  return { items, bad }
}

/* 结果/图片行归并后，归属行在展开区的输入输出文本（复用旧页折叠口径）。 */
export const FOLD_PREVIEW_LINES = LONG_TEXT_LINES

/*
 * callId → 所在助手消息行号（轨迹"点行跳回对话里合并后的那一块"用；
 * 旧页记在 state.callNodes 上，组件化后直接从行数据推导）。
 */
export function callMsgLine(): Record<string, number> {
  const map: Record<string, number> = {}
  state.lines.forEach((line: any) => {
    if (!line || line.bad || (line.t && line.t !== 'msg')) return
    if (line.role === 'assistant') {
      ;(line.tool_calls || []).forEach((c: any) => {
        if (c.id && map[c.id] === undefined) map[c.id] = line.n
      })
    }
  })
  return map
}
