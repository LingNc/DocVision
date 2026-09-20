/*
 * T56 旧格式归一（legacy/stream.ts 的 parseLegacyImageParts + streamModel 接线）：
 * img2text 旧转录里 user 行 text 是 part 数组 JSON 字符串，拆成文本+图片后
 * 按"任务行自己带图"走附件缩略图链路；不匹配时原样显示。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { streamModel, parseLegacyImageParts } from '../src/legacy/stream'
import { type Line } from '../src/legacy/types'

const DATA_URL = 'data:image/jpeg;base64,/9j/4AAQSkZJRg=='

function legacyUser(n: number, parts: unknown[]): Line {
  return { n, t: 'msg', role: 'user', text: JSON.stringify(parts) }
}

function run(lines: Line[]) {
  state.current = { id: 't/session' }
  state.lines = lines
  state.imgAttr = null
  return streamModel()
}

beforeEach(() => { state.imgAttr = null })

describe('parseLegacyImageParts：纯函数', () => {
  it('text + image 段：文本拼接、data: URL 入图片列表', () => {
    const r = parseLegacyImageParts(JSON.stringify([
      { text: 'The image to describe is at line 1038. Context: ...' },
      { type: 'image_url', image_url: { url: DATA_URL } },
    ]))
    expect(r).toEqual({
      text: 'The image to describe is at line 1038. Context: ...',
      images: [DATA_URL],
    })
  })

  it('多 text 段按顺序拼接', () => {
    const r = parseLegacyImageParts(JSON.stringify([
      { text: '第一段' },
      { type: 'image_url', image_url: { url: DATA_URL } },
      { text: '第二段' },
    ]))
    expect(r).toEqual({ text: '第一段\n第二段', images: [DATA_URL] })
  })

  it('type:"text" 形态同样认（text 字段优先于 type）', () => {
    const r = parseLegacyImageParts(JSON.stringify([
      { type: 'text', text: '任务提示' },
      { type: 'image_url', image_url: { url: DATA_URL } },
    ]))
    expect(r).toEqual({ text: '任务提示', images: [DATA_URL] })
  })

  it('非 JSON 原文 → null（正常消息不动）', () => {
    expect(parseLegacyImageParts('请描述这张图')).toBeNull()
    expect(parseLegacyImageParts('[{这不是 JSON')).toBeNull()
  })

  it('JSON 但不是 part 数组 → null', () => {
    expect(parseLegacyImageParts('["a","b"]')).toBeNull()
    expect(parseLegacyImageParts('[{"foo":1}]')).toBeNull()
    expect(parseLegacyImageParts('[]')).toBeNull()
  })

  it('坏 JSON → null', () => {
    expect(parseLegacyImageParts('[{"text":"x",')).toBeNull()
  })

  it('image_url 不是 data: URL → null（不认识的形态原样）', () => {
    expect(parseLegacyImageParts(JSON.stringify([
      { type: 'image_url', image_url: { url: 'file://media/a.jpg' } },
    ]))).toBeNull()
  })
})

describe('streamModel：旧格式归一接线', () => {
  it('旧格式 user 行 → system 条目，图作为任务自带附件，正文为拼接文本', () => {
    const line = legacyUser(1, [
      { text: 'The image to describe is at line 1038.' },
      { type: 'image_url', image_url: { url: DATA_URL } },
    ])
    const m = run([line])
    expect(m.items).toHaveLength(1)
    const it = m.items[0]
    expect(it.type).toBe('system')
    if (it.type !== 'system') return
    expect(it.line.text).toBe('The image to describe is at line 1038.')
    expect(it.attachments).toHaveLength(1)
    expect(it.attachments[0].line.images).toEqual([DATA_URL])
    expect(it.attachments[0].attr).toMatchObject({ kind: 'task', how: 'task', taskLineN: 1 })
    // 原转录行不被改写（归一只作用在生效副本上）
    expect(line.text.startsWith('[{')).toBe(true)
    expect(line.images).toBeUndefined()
  })

  it('普通 user 行与新格式带图 user 行不受影响', () => {
    const m = run([
      { n: 1, t: 'msg', role: 'user', text: '普通任务提示' },
      { n: 2, t: 'msg', role: 'user', text: '带图任务', images: ['file://media/a.jpg'] },
    ])
    expect(m.items.map((i) => i.type)).toEqual(['system', 'system'])
    const second = m.items[1]
    if (second.type !== 'system') throw new Error('expect system')
    expect(second.line.images).toEqual(['file://media/a.jpg'])
  })

  it('仅看工具调用时旧格式带图任务与带图轮同样保留', () => {
    state.onlyTools = true
    try {
      const m = run([
        legacyUser(1, [{ text: '任务' }, { type: 'image_url', image_url: { url: DATA_URL } }]),
        { n: 2, t: 'msg', role: 'user', text: '无图提示' },
      ])
      expect(m.items).toHaveLength(1)
      expect(m.items[0].type).toBe('system')
    } finally {
      state.onlyTools = false
    }
  })
})
