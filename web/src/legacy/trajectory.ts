/*
 * 轨迹页（旧页 trajectoryRows/renderTrajectory 一族照搬区，块 5）：把这次
 * 会话的全部步骤摊成一张表——序号 / 类型 / 工具名 / 一行摘要 / 状态 /
 * 字符数 / 耗时。点一行跳到对话里对应的那条消息；左侧箭头展开完整的
 * 输入输出（不离开这个标签页）。**保持真实 wire 结构**：一行一步、类型照
 * 真实角色、工具调用与工具结果各占一行不合并。
 */
import { state, switchView, anchorOf } from '../state'
import { callMsgLine } from './stream'
import { countText, countValue, estOf, firstLine, fmtDur, fmtTokens, unitLabel } from './sidebar'
import { prettyJSON } from './richtext'
import {
  attributionText, classifyResult, imageAttributions, IMAGE_WIRE_TITLE, toolSummary,
} from './timeline'
import { type Line, type ToolCall, type ImageAttr, type ImageCallEntry } from './types'

// 图片占位标记：工具回执正文里如果写了这一笔（调试日志里的形状），说明图片在
// 紧随其后的 user 轮里。我们自己的转录通常只有紧邻关系，所以两种都认。
export const IMAGE_PLACEHOLDER = /\[\s*image\b|\[\s*图片|图片见|image omitted/i

function nextMsgLine(lines: Line[], idx: number): Line | null {
  for (let i = idx + 1; i < lines.length; i++) {
    if (lines[i] && !lines[i].bad && lines[i].t === 'msg') return lines[i]
  }
  return null
}

interface CallInfo { name: string; ts: string; n: number; args: string }

// 工具耗时 = 结果行时间戳 − 调用行时间戳（两边都有 ts 才算，不编数字）。
function callDuration(call: CallInfo | null, resultLine: Line | null): number {
  if (!call || !call.ts || !resultLine || !resultLine.ts) return 0
  const t0 = Date.parse(call.ts)
  const t1 = Date.parse(resultLine.ts)
  if (isNaN(t0) || isNaN(t1) || t1 < t0) return 0
  return t1 - t0
}

/*
 * 一行轨迹。字段随类型（kind）差异大——公共面收紧，专属字段可选；
 * detail 是展开区的内容块（键 = 展开区小标题）。
 */
export interface TrajRow {
  kind: string
  tag: string
  name: string
  summary: string
  chars: number | string
  tokens: number
  status: string
  /** 点行跳回对话的目标行号。 */
  jump?: number
  /** P9 右栏步骤定位键（确定性：kind@jump#序号；append-only 转录下稳定）。
   *  jump 会撞（工具行都挂在 assistant 消息行上、一条消息可多次调用），不能拿它当身份。 */
  rid: string
  /** 工具/请求耗时 ms（有就显示，没有不出列）。 */
  time?: number
  /** 带图 user 轮的图片引用（可展开看图）。 */
  images?: string[]
  /** 展开区内容（悬浮说明单列 title）。 */
  title?: string
  detail: Record<string, string>
}

export function trajectoryRows(): TrajRow[] {
  const rows: TrajRow[] = []
  const callOf: Record<string, CallInfo> = {}
  // 哪些带图 user 轮的图片是**上一行工具回执**投出来的（两行互相提示，但不合并）。
  const imageAfterTool: Record<number, boolean> = {}
  state.lines.forEach((l: Line) => {
    if (!l || l.bad || l.t !== 'msg') return
    if (l.role === 'assistant') {
      ;(l.tool_calls || []).forEach((c: ToolCall) => {
        const fn = c.function || {}
        callOf[c.id || ''] = { name: fn.name || '?', ts: l.ts || '', n: l.n, args: String(fn.arguments || '') }
      })
    }
  })

  state.lines.forEach((line: Line, idx: number) => {
    if (!line || line.bad) return
    if (line.t === 'meta') {
      rows.push({
        kind: 'meta', tag: '元信息', name: line.kind || 'system',
        summary: '模型 ' + (line.model || '—') + ' · 提示词 ' +
          countText(String(line.text || '').length, estOf(line).text) +
          ((line.tools || []).length ? ' · 工具 ' + line.tools!.length : ''),
        chars: String(line.text || '').length,
        tokens: estOf(line).text,
        status: '', detail: { prompt: String(line.text || '') },
      })
      return
    }
    if (line.t === 'usage') {
      const st = line.stats || {}
      rows.push({
        kind: 'usage', tag: '用量', name: '请求' + (st.round ? ' #' + st.round : ''),
        summary: (st.kind || 'chat') + ' · 输入 ' + fmtTokens(st.promptTokens) + '（缓存 ' +
          (st.cachedTokens || 0) + '）· 输出 ' + fmtTokens(st.completionTokens) +
          (st.reasoningTokens ? '（思 ' + fmtTokens(st.reasoningTokens) + '）' : ''),
        chars: '', tokens: 0,
        status: st.finish || '',
        time: Number(st.durationMs) || 0,
        detail: { request: JSON.stringify(st, null, 2) },
      })
      return
    }
    if (line.t !== 'msg' || !line.role) return

    /*
     * 步骤分类与对话页一一对应：
     *   · 带图的 user 轮在类型列照旧是「用户」，摘要里写明 `图片 ×N` 并可展开看图；
     *   · 点行跳回对话里**合并后的那一块**（对话是合并的，轨迹是真实的）。
     */
    if (line.role === 'user' && line.images && line.images.length) {
      const attr: ImageAttr = imageAttributions()[line.n] ||
        { kind: 'none', how: '', callId: '', name: '', lineN: line.n, taskLineN: 0 }
      const fromTool = !!imageAfterTool[line.n]
      // 跳回对话里"合并后的那一块"：归到调用就跳那次调用，归到任务就跳任务行，
      // 都没认出来才跳自己这一行。
      const callLine = attr.kind === 'call' && attr.callId ? callMsgLine()[attr.callId] : undefined
      const jumpTo = callLine !== undefined
        ? callLine
        : (attr.kind === 'task' && attr.taskLineN ? attr.taskLineN : line.n)
      rows.push({
        kind: 'user', tag: '用户', name: '用户',
        summary: (fromTool ? '接上一行工具回执 · ' : '') + '图片 ×' + line.images.length + ' · ' +
          (firstLine(line.text) || '（无正文）') + ' · ' + attributionText(attr),
        title: IMAGE_WIRE_TITLE + '\n' + attributionText(attr) +
          (attr.how === 'call-id' ? '（句柄里写了 call id，属于精确匹配）'
            : attr.how === 'tool-name' ? '（句柄里写了工具名，按名称匹配到本轮的调用）'
            : attr.how === 'order' ? '（旧转录没有 call id，按顺序推断；新转录会写上归属）'
            : attr.how === 'task' ? '（这一轮带的是任务自己的图，不归任何工具调用）' : '') +
          (fromTool ? '\n这一轮的图片就是上一行工具回执投出来的（同一件事的两段 wire 表达，所以两行不合并）' : ''),
        chars: String(line.text || '').length,
        tokens: estOf(line).text + estOf(line).images,
        status: '', jump: jumpTo,
        images: line.images,
        detail: { user: String(line.text || '') || '（这一轮没有正文）' },
      })
    } else if (line.role === 'user') {
      // 会话开头的原图投喂轮归到这条任务上：轨迹里也点明"它带着附件"。
      let fed = 0
      const attrs = imageAttributions()
      Object.keys(attrs).forEach((n) => {
        const a = attrs[Number(n)]
        if (a.kind === 'task' && a.taskLineN === line.n) fed++
      })
      rows.push({
        kind: 'user', tag: '用户', name: '用户',
        summary: firstLine(line.text) + (fed ? ' · 附件 图片 ×' + fed : ''),
        title: fed
          ? '会话开头的原图投喂轮归到了这条任务（对话页里它们收在同一个块里）'
          : '点击跳到对话里对应的那条消息',
        chars: String(line.text || '').length,
        tokens: estOf(line).text,
        status: '', jump: line.n,
        detail: { user: String(line.text || '') },
      })
    } else if (line.role === 'assistant') {
      if (line.reasoning) {
        rows.push({
          kind: 'think', tag: '思考', name: 'reasoning',
          summary: firstLine(line.reasoning), chars: line.reasoning.length,
          tokens: estOf(line).reasoning,
          status: '', jump: line.n, detail: { thinking: line.reasoning },
        })
      }
      if (line.text) {
        rows.push({
          kind: 'msg', tag: '助手', name: 'AI',
          summary: firstLine(line.text), chars: line.text.length,
          tokens: estOf(line).text,
          status: '', jump: line.n, detail: { message: String(line.text) },
        })
      }
      ;(line.tool_calls || []).forEach((c: ToolCall, i: number) => {
        const fn = c.function || {}
        const name = fn.name || '(未命名工具)'
        const args = String(fn.arguments || '')
        rows.push({
          kind: 'tool', tag: '工具', name,
          summary: toolSummary(name, fn.arguments), chars: args.length,
          tokens: estOf(line).calls[i] || 0,
          status: '', jump: line.n,
          detail: { input: prettyJSON(args) || args },
        })
      })
    } else if (line.role === 'tool') {
      // 工具结果**独立成行**（轨迹就是让人看清真实结构的）：配对得上的用调用名，
      // 配不上的注明，不并进调用行。
      const info = line.tool_call_id ? callOf[line.tool_call_id] || null : null
      const text = String(line.text || '')
      const after = nextMsgLine(state.lines, idx)
      const imageNext = !!(after && after.role === 'user' && after.images && after.images.length)
      if (imageNext) imageAfterTool[after.n] = true
      const hint = imageNext
        ? (IMAGE_PLACEHOLDER.test(text) ? ' · 图片见下一行用户轮' : ' · 图片在下一行用户轮里')
        : ''
      rows.push({
        kind: 'result', tag: '结果',
        name: info ? info.name : '(未配对的工具回执)',
        summary: firstLine(text) + hint, chars: text.length,
        tokens: estOf(line).text,
        title: imageNext
          ? '这一行是工具回执：tool 消息的 content 只能是文本，随行的图片被回灌在紧随其后的 user 轮里（两行是同一件事，保持两行不合并）'
          : '点击跳到对话里对应的那条消息',
        status: classifyResult(text), jump: line.n,
        time: callDuration(info, line),
        detail: { output: text },
      })
    }
  })
  rows.forEach((r, i) => { r.rid = r.kind + '@' + (r.jump ?? 'x') + '#' + i })
  return rows
}

/* 轨迹的类型 chip 按**转录里的真实角色**列（对话页是合并后的呈现，两者语义不同）。 */
export const TRAJ_KINDS = [
  { id: 'user', label: '用户' },
  { id: 'msg', label: '助手' },
  { id: 'think', label: '思考' },
  { id: 'tool', label: '工具' },
  { id: 'result', label: '结果' },
  { id: 'meta', label: '元信息' },
  { id: 'usage', label: '用量' },
]

/* 轨迹 → 对话：切回对话标签页并滚到那一条，落点短暂高亮。 */
export function jumpToLine(n: number): void {
  const node = anchorOf(n)
  switchView('chat')
  if (!node) return
  node.scrollIntoView({ block: 'center' })
  node.classList.remove('flash')
  // 强制重排，让动画能从头上重放
  void (node as HTMLElement).offsetWidth
  node.classList.add('flash')
}
