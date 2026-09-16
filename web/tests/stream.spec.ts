/*
 * streamModel()（legacy/stream.ts）的单测：消息流条目化 + 归属记账。
 * 组件渲染读的就是这些条目——顺序、归并、兜底规则都在这里钉住。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { streamModel, callTail } from '../src/legacy/stream'
import { type Line } from '../src/legacy/types'

function assistant(n: number, opts: { calls?: { id: string; name: string }[]; text?: string; reasoning?: string } = {}): Line {
  return {
    n, t: 'msg', role: 'assistant',
    text: opts.text ?? '', reasoning: opts.reasoning,
    tool_calls: (opts.calls || []).map((c) => ({ id: c.id, function: { name: c.name, arguments: '{"command":"ls"}' } })),
  }
}
function toolResult(n: number, callId: string, text: string): Line {
  return { n, t: 'msg', role: 'tool', tool_call_id: callId, text }
}
function imageUser(n: number, text: string): Line {
  return { n, t: 'msg', role: 'user', text, images: ['file://media/a.jpg'] }
}
function plainUser(n: number, text: string): Line {
  return { n, t: 'msg', role: 'user', text }
}
function usage(n: number): Line {
  return { n, t: 'usage', stats: {} }
}
function meta(n: number): Line {
  return { n, t: 'meta', kind: 'system', model: 'm', text: 'prompt' }
}

function run(lines: Line[]) {
  state.current = { id: 't/session' }
  state.lines = lines
  state.imgAttr = null
  return streamModel()
}

beforeEach(() => { state.imgAttr = null })

describe('streamModel：基础条目化', () => {
  it('meta/usage 行绝不进消息序列；坏行只计数', () => {
    const m = run([meta(1), usage(2), plainUser(3, 'hi'), { n: 4, bad: true } as unknown as Line, assistant(5, { text: 'yo' })])
    expect(m.bad).toBe(1)
    expect(m.items.map((i) => i.type)).toEqual(['system', 'assistant'])
  })

  it('无正文无思考无调用的助手行 → empty 标记（组件渲染"空消息"小字）', () => {
    const m = run([assistant(1), assistant(2, { text: 'ok' })])
    expect(m.items[0]).toMatchObject({ type: 'assistant', empty: true })
    expect(m.items[1]).toMatchObject({ type: 'assistant', empty: false })
  })

  it('仅看工具调用：普通 user/助手正文被滤掉，图片轮与调用保留', () => {
    const lines = [
      plainUser(1, '任务'),
      assistant(2, { calls: [{ id: 'c1', name: 'bash' }] }),
      toolResult(3, 'c1', 'ok(1)'),
      imageUser(4, 'Tool image output'),
    ]
    state.onlyTools = true
    const m = run(lines)
    expect(m.items.map((i) => i.type)).toEqual(['assistant', 'imageTurn'])
  })
})

describe('streamModel：调用与回执归并', () => {
  it('回执按 tool_call_id 并进同一张卡片，lastStatus 取最后一条', () => {
    const m = run([
      assistant(1, { calls: [{ id: 'c1', name: 'bash' }, { id: 'c2', name: 'grep' }] }),
      toolResult(2, 'c1', 'ok(1)'),
      toolResult(3, 'c1', 'TOOL ERROR: x'),
      toolResult(4, 'c2', 'done'),
    ])
    expect(m.items).toHaveLength(1)
    const item = m.items[0]
    if (item.type !== 'assistant') throw new Error('expect assistant')
    expect(item.calls).toHaveLength(2)
    expect(item.calls[0].results.map((r) => r.status)).toEqual(['ok', 'error'])
    expect(item.calls[0].lastStatus).toBe('error')
    expect(item.calls[0].anchorNs).toEqual([2, 3])
    expect(item.calls[1].results).toHaveLength(1)
  })

  it('配不上调用的回执 → 独立 result 条目（paired: false）', () => {
    const m = run([toolResult(1, 'ghost', 'orphan')])
    expect(m.items[0]).toMatchObject({ type: 'result', paired: false })
  })

  it('callTail：精确匹配图挂输出下（outImages），尾巴带附件张数', () => {
    const lines = [
      assistant(1, { calls: [{ id: 'c1', name: 'view_image' }] }),
      toolResult(2, 'c1', 'Image a.png attached.'),
      imageUser(3, 'Tool image output\n(call c1)'),
    ]
    const m = run(lines)
    const item = m.items[0]
    if (item.type !== 'assistant') throw new Error('expect assistant')
    const card = item.calls[0]
    expect(card.outImages).toHaveLength(1) // how=call-id 精确匹配 → 输出正文下方
    expect(card.attachments).toHaveLength(0)
    expect(callTail(card)).toContain('附件 1 张（user 轮）')
    expect(callTail(card)).toContain('→')
  })

  it('兜底：how=call-id 的图片轮挂输出下，但那次调用无回执时退回附件分区', () => {
    const lines = [
      assistant(1, { calls: [{ id: 'c1', name: 'view_image' }] }),
      imageUser(2, 'Tool image output\n(call c1)'),
    ]
    const m = run(lines)
    const item = m.items[0]
    if (item.type !== 'assistant') throw new Error('expect assistant')
    expect(item.calls[0].results).toHaveLength(0)
    expect(item.calls[0].outImages).toHaveLength(0)
    expect(item.calls[0].attachments).toHaveLength(1)
  })
})

describe('streamModel：图片轮归属', () => {
  it('精确匹配 → outImages；FIFO → attachments；任务行附件挂任务块', () => {
    const lines = [
      imageUser(1, '原始投喂'),
      plainUser(2, '任务正文'),
      assistant(3, { calls: [{ id: 'c1', name: 'view_image' }] }),
      toolResult(4, 'c1', 'Image a.png attached.'),
      imageUser(5, 'Tool image output'),
    ]
    const m = run(lines)
    // 行 1：会话开头投喂 → 挂到任务行 2 的附件（第二轮补挂）
    const sys = m.items.find((i) => i.type === 'system')
    if (!sys || sys.type !== 'system') throw new Error('expect system')
    expect(sys.attachments).toHaveLength(1)
    // 行 5：FIFO → 附件分区
    const card = m.items.find((i) => i.type === 'assistant')
    if (!card || card.type !== 'assistant') throw new Error('expect assistant')
    expect(card.calls[0].attachments).toHaveLength(1)
    expect(card.calls[0].outImages).toHaveLength(0)
  })

  it('预览条挂"这一轮最后一个 view 调用"的卡片', () => {
    const lines = [
      assistant(1, { calls: [{ id: 'c1', name: 'view_pdf' }, { id: 'c2', name: 'view_image' }] }),
      toolResult(2, 'c1', 'Image p1.png attached.'),
      toolResult(3, 'c2', 'Image p2.png attached.'),
      imageUser(4, 'Tool image output\nfrom view_image'),
    ]
    const m = run(lines)
    const item = m.items[0]
    if (item.type !== 'assistant') throw new Error('expect assistant')
    // c1 是 view_pdf（也在队列里，回执有 attached 证据）；from view_image 按
    // 工具名匹配 → c2；预览目标 = 该轮 roundLast = c2（同一行号最后一笔）
    expect(item.calls[1].previews.map((l) => l.n)).toContain(4)
    expect(item.calls[0].previews).toHaveLength(0)
  })
})

describe('streamModel：callMsgLine', () => {
  it('callId → 所在助手行号（轨迹跳回目标）', async () => {
    const { callMsgLine } = await import('../src/legacy/stream')
    run([
      assistant(1, { calls: [{ id: 'c1', name: 'bash' }] }),
      assistant(4, { calls: [{ id: 'c1', name: 'bash' }] }),
    ])
    expect(callMsgLine()).toEqual({ c1: 1 })
  })
})

describe('splitProtocolLeak（T32）', () => {
  it('纯残片整体剥出（含标签体内的值行）', async () => {
    const { splitProtocolLeak } = await import('../src/legacy/stream')
    const r = splitProtocolLeak('<parameter=path>\ncheck:parts/\n</parameter>\n</function>\n</tool_call>')
    expect(r.main).toBe('')
    expect(r.leak).toContain('<parameter=path>')
    expect(r.leak).toContain('check:parts/')
  })
  it('完整 XML 块混在正文里：块删除、正文保留', async () => {
    const { splitProtocolLeak } = await import('../src/legacy/stream')
    const r = splitProtocolLeak('Let me check:\n<tool_call>\n<function=read_file>\n<parameter=path>\nwork/x.md\n</parameter>\n</function>\n</tool_call>\ndone')
    expect(r.main).toBe('Let me check:\n\ndone')
    expect(r.leak).toBe('')
  })
  it('正常正文不受影响', async () => {
    const { splitProtocolLeak } = await import('../src/legacy/stream')
    const r = splitProtocolLeak('Reading the chapter file first.')
    expect(r).toEqual({ main: 'Reading the chapter file first.', leak: '' })
  })
})
