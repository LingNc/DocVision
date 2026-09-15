/*
 * 图片归属算法（legacy/timeline.ts 的 imageAttributions）的单测。
 * 这是组件化后最核心的纯算法：FIFO 一对一配对 + 精确匹配优先 + 任务不许抢，
 * 规则口径见该文件顶部注释；这里把每条规则都钉住，防止后续重构悄悄变形。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { imageAttributions, classifyResult, toolSummary, toolFamily } from '../src/legacy/timeline'
import { type Line } from '../src/legacy/types'

function assistant(n: number, calls: { id: string; name: string }[]): Line {
  return { n, t: 'msg', role: 'assistant', text: '', tool_calls: calls.map((c) => ({ id: c.id, function: { name: c.name, arguments: '{}' } })) }
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

function run(lines: Line[]) {
  state.current = { id: 't/session' }
  state.lines = lines
  state.imgAttr = null
  return imageAttributions()
}

beforeEach(() => { state.imgAttr = null })

describe('imageAttributions：精确匹配优先', () => {
  it('句柄带 (call id) → how=call-id，且 FIFO 让位', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_image' }, { id: 'c2', name: 'view_image' }]),
      toolResult(2, 'c1', 'Image out/a.png (view_image) attached.'),
      toolResult(3, 'c2', 'Image out/b.png (view_image) attached.'),
      imageUser(4, 'Tool image output\nfrom view_image (call c2)'),
    ]
    const map = run(lines)
    expect(map[4]).toMatchObject({ kind: 'call', how: 'call-id', callId: 'c2' })
  })

  it('句柄指向没产图的调用（无回执也在队列外）也认句柄的归属', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_pdf' }]),
      toolResult(2, 'c1', 'TOOL ERROR: bad pdf'),
      imageUser(3, 'Tool image output\nfrom view_pdf (call c1)'),
    ]
    const map = run(lines)
    expect(map[3]).toMatchObject({ kind: 'call', how: 'call-id', callId: 'c1' })
  })

  it('句柄只有 from <tool> → 队列里最早的同名未认领调用', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_image' }, { id: 'c2', name: 'view_image' }]),
      toolResult(2, 'c1', 'Image a.png attached.'),
      toolResult(3, 'c2', 'Image b.png attached.'),
      imageUser(4, 'Tool image output\nfrom view_image'),
    ]
    const map = run(lines)
    expect(map[4]).toMatchObject({ kind: 'call', how: 'tool-name', callId: 'c1' })
  })
})

describe('imageAttributions：FIFO 一对一', () => {
  it('旧转录（句柄无 id 无 tool）按顺序领走最早的未认领调用', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_image' }, { id: 'c2', name: 'view_image' }]),
      toolResult(2, 'c1', 'Image a.png attached.'),
      toolResult(3, 'c2', 'Image b.png attached.'),
      imageUser(4, 'Tool image output'),
      imageUser(5, 'Tool image output'),
    ]
    const map = run(lines)
    expect(map[4]).toMatchObject({ kind: 'call', how: 'order', callId: 'c1' })
    expect(map[5]).toMatchObject({ kind: 'call', how: 'order', callId: 'c2' })
  })

  it('失败的看图调用（回执无 attached 证据）不占队列位置', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_pdf' }, { id: 'c2', name: 'view_image' }]),
      toolResult(2, 'c1', 'TOOL ERROR: no page'),
      toolResult(3, 'c2', 'Image b.png attached.'),
      imageUser(4, 'Tool image output'),
    ]
    const map = run(lines)
    expect(map[4]).toMatchObject({ kind: 'call', how: 'order', callId: 'c2' })
  })

  it('一对一：图片行多于产图调用时，句柄行领完队列后多余的标"未识别"', () => {
    const lines = [
      plainUser(1, '把书里的矢量图都重画一遍'),
      assistant(2, [{ id: 'c1', name: 'view_image' }]),
      toolResult(3, 'c1', 'Image a.png attached.'),
      imageUser(4, 'Tool image output'),
      imageUser(5, 'Tool image output'),
    ]
    const map = run(lines)
    expect(map[4]).toMatchObject({ kind: 'call', how: 'order', callId: 'c1' })
    // 句柄行配不上任何调用 → 未识别（真数据里 537 条全配平，不会落到这）。
    expect(map[5]).toMatchObject({ kind: 'none' })
  })
})

describe('imageAttributions：任务归属不许抢', () => {
  it('带正文的图片轮（任务本身）→ kind=task，FIFO 不认领它', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_image' }]),
      toolResult(2, 'c1', 'Image a.png attached.'),
      imageUser(3, '请按这张原图重画：……（任务正文）'),
      imageUser(4, 'Tool image output'),
    ]
    const map = run(lines)
    expect(map[3]).toMatchObject({ kind: 'task', taskLineN: 3 })
    expect(map[4]).toMatchObject({ kind: 'call', callId: 'c1' })
  })

  it('会话开头投喂的纯图片轮（调用还没发生）归到后面的任务行', () => {
    const lines = [
      imageUser(1, ''),
      plainUser(2, '这是任务'),
      assistant(3, [{ id: 'c1', name: 'view_image' }]),
      toolResult(4, 'c1', 'Image a.png attached.'),
      imageUser(5, 'Tool image output'),
    ]
    const map = run(lines)
    expect(map[1]).toMatchObject({ kind: 'task', taskLineN: 2 })
    expect(map[5]).toMatchObject({ kind: 'call', how: 'order', callId: 'c1' })
  })

  it('什么都配不上 → kind=none', () => {
    const lines = [imageUser(1, 'Tool image output')]
    const map = run(lines)
    expect(map[1]).toMatchObject({ kind: 'none', how: '' })
  })
})

describe('imageAttributions：队列缓存与轮次', () => {
  it('同签名重复调用走缓存；行数变化重算', () => {
    const lines = [
      assistant(1, [{ id: 'c1', name: 'view_image' }]),
      toolResult(2, 'c1', 'Image a.png attached.'),
      imageUser(3, 'Tool image output'),
    ]
    const first = run(lines)
    const cached = imageAttributions()
    expect(cached).toBe(first)
    state.lines = [...lines, imageUser(4, 'Tool image output')]
    state.imgAttr = null
    const second = imageAttributions()
    expect(second).not.toBe(first)
    // candRound/roundLast：预览条挂卡用的轮次索引
    expect(state.imgAttr!.candRound['c1']).toBe(1)
    expect(state.imgAttr!.roundLast[1]).toBe('c1')
  })
})

describe('工具卡文案算法', () => {
  it('classifyResult：错误词优先，ok( 次之，其余 plain', () => {
    expect(classifyResult('TOOL ERROR: 文件不存在')).toBe('error')
    expect(classifyResult('ok(3) rows written')).toBe('ok')
    expect(classifyResult('done')).toBe('plain')
  })

  it('classifyResult：错误标记只在**首行**生效（T23 反馈：回执引用正文里的「失败」不再误报）', () => {
    // 首行干净、后文引到正文关键词 → plain
    expect(
      classifyResult(
        'image 4 of 4 in this document: images/书/be53.jpg\n   259 | 在第一次失败的条件下, 第二次通过的概率分别为 $\\frac{2}{3}$'
      )
    ).toBe('plain')
    expect(classifyResult('image 1 of 1: images/a.jpg\nline with error inside code sample')).toBe('plain')
    // 首行就带标记 → error（COMPILE FAILED / Traceback / REJECTED / not found）
    expect(classifyResult('COMPILE FAILED. Fix figure.tex and call compile again.\nError:\nl.18')).toBe('error')
    expect(classifyResult('Traceback (most recent call last):\n  File "x.py"')).toBe('error')
    expect(classifyResult('REJECTED: 布局不符')).toBe('error')
    expect(classifyResult('bash: foo: command not found')).toBe('error')
  })

  it('toolSummary：按工具族挑关键参数，兜底第一个字符串', () => {
    expect(toolSummary('bash', '{"command":"ls -la","cwd":"/x"}')).toBe('ls -la')
    expect(toolSummary('write', '{"path":"a/b.md","content":"long"}')).toBe('a/b.md')
    expect(toolSummary('unknown_tool', '{"zzz":"first","aaa":"second"}')).toBe('first')
    expect(toolSummary('bash', '{bad json')).toBe('')
  })

  it('toolFamily：家族划分', () => {
    expect(toolFamily('bash')).toBe('shell')
    expect(toolFamily('write_file')).toBe('write')
    expect(toolFamily('grep')).toBe('search')
    expect(toolFamily('view_pdf')).toBe('view')
    expect(toolFamily('submit')).toBe('submit')
  })
})
