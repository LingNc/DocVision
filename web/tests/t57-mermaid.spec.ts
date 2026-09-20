/*
 * T57 + T56 追加的单测：
 *  A. mediaURL 剥掉 img2text 会话 ID 的板块前缀（/media 404 根因）。
 *  B. 逐图转录的 prevN 轮转归到同一图片链（sessions/<md>/<图>.prev1.jsonl）。
 *  C. mermaid 预览的纯函数：extractMermaidBlocks 提取、内容缓存键、失败缓存。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { mediaURL } from '../src/legacy/timeline'
import { parseImg2TextId, img2textMemberTag, buildImg2TextChains } from '../src/legacy/sidebar'
import { extractMermaidBlocks, renderMermaid, mermaidCache } from '../src/legacy/mermaid'

const HASH = 'aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb'

/* ---------- A：mediaURL 剥前缀 ---------- */

describe('T57 mediaURL 板块前缀', () => {
  beforeEach(() => { state.staticMode = false; state.mediaRoot = '' })

  it('img2text: 前缀被剥掉，/media 路径指向真实目录', () => {
    state.current = { id: `img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl` } as any
    expect(mediaURL('file://img.png')).toBe(
      `/media/mermaid_fix/书A_${HASH}.jpg/img.png`)
  })

  it('img2text:sessions/ 形态同样剥前缀', () => {
    state.current = { id: `img2text:sessions/书A.md/${HASH}.jpg.jsonl` } as any
    expect(mediaURL('file://img.png')).toBe(`/media/sessions/书A.md/img.png`)
  })

  it('latex 会话 ID 无前缀不受影响', () => {
    state.current = { id: 'bookA/work/sessions/style_session.jsonl' } as any
    expect(mediaURL('file://img.png')).toBe('/media/bookA/work/sessions/img.png')
  })

  it('data: / http(s) URL 与静态模式行为不变', () => {
    state.current = { id: 'img2text:sessions/a.md/x.jsonl' } as any
    expect(mediaURL('data:image/png;base64,xx')).toBe('data:image/png;base64,xx')
    expect(mediaURL('https://x/y.png')).toBe('https://x/y.png')
    state.staticMode = true
    state.mediaRoot = 'snap/'
    expect(mediaURL('file://img.png')).toBe('snap/sessions/a.md/img.png')
    state.staticMode = false
  })
})

/* ---------- B：逐图转录 prevN 轮转 ---------- */

describe('T57 逐图转录 prevN 轮转', () => {
  it('sessions 路径的 .prevN 解析：链键不变、prev 序号正确', () => {
    const cur = parseImg2TextId(`img2text:sessions/书A.md/images_${HASH}.jpg.jsonl`)
    const p1 = parseImg2TextId(`img2text:sessions/书A.md/images_${HASH}.jpg.prev1.jsonl`)
    const p2 = parseImg2TextId(`img2text:sessions/书A.md/images_${HASH}.jpg.prev2.jsonl`)
    expect(cur).toEqual({ kind: 'analyze', img: `${HASH}.jpg`, stage: -1, prev: 0 })
    expect(p1).toEqual({ kind: 'analyze', img: `${HASH}.jpg`, stage: -1, prev: 1 })
    expect(p2).toEqual({ kind: 'analyze', img: `${HASH}.jpg`, stage: -1, prev: 2 })
  })

  it('prevN 成员标签复用「·上一次 / ·上上次」规则', () => {
    expect(img2textMemberTag(`img2text:sessions/书A.md/${HASH}.jpg.jsonl`)).toBe('逐图分析')
    expect(img2textMemberTag(`img2text:sessions/书A.md/${HASH}.jpg.prev1.jsonl`)).toBe('逐图分析 · 上一次')
    expect(img2textMemberTag(`img2text:sessions/书A.md/${HASH}.jpg.prev2.jsonl`)).toBe('逐图分析 · 上上次')
    expect(img2textMemberTag(`img2text:sessions/书A.md/${HASH}.jpg.prev3.jsonl`)).toBe('逐图分析 · prev3')
  })

  it('当前 + prev1 + prev2 归并到同一条图片链', () => {
    const items = [
      { id: `img2text:sessions/书A.md/images_${HASH}.jpg.prev2.jsonl`, board: 'img2text', mtime: '2025-12-01T00:00:00Z' },
      { id: `img2text:sessions/书A.md/images_${HASH}.jpg.jsonl`, board: 'img2text', mtime: '2026-01-03T00:00:00Z' },
      { id: `img2text:sessions/书A.md/images_${HASH}.jpg.prev1.jsonl`, board: 'img2text', mtime: '2026-01-02T00:00:00Z' },
    ]
    const { chains, singles } = buildImg2TextChains(items)
    expect(singles).toEqual([])
    expect(chains.length).toBe(1)
    expect(chains[0].img).toBe(`${HASH}.jpg`)
    expect(chains[0].items.length).toBe(3)
    expect(chains[0].main.id).not.toContain('prev')
  })

  it('坏形态仍返回 null', () => {
    expect(parseImg2TextId('img2text:sessions/书A.md/.jsonl')).toBeNull()
    expect(parseImg2TextId('img2text:sessions/书A.md/x.prev1.txt')).toBeNull()
  })
})

/* ---------- C：mermaid 预览纯函数 ---------- */

describe('T56 mermaid 预览', () => {
  it('extractMermaidBlocks 提取 ```mermaid 块（大小写/波浪围栏/多块）', () => {
    const md = [
      '前文', '```mermaid', 'graph TD', 'A-->B', '```',
      '中段 ```js', '```js', 'let a=1', '```',
      '~~~Mermaid', 'sequenceDiagram', 'A->>B: hi', '~~~',
      '未闭合', '```mermaid', 'graph LR', 'X-->Y',
    ].join('\n')
    const blocks = extractMermaidBlocks(md)
    expect(blocks.length).toBe(3)
    expect(blocks[0]).toBe('graph TD\nA-->B')
    expect(blocks[1]).toBe('sequenceDiagram\nA->>B: hi')
    expect(blocks[2]).toBe('graph LR\nX-->Y')
  })

  it('extractMermaidBlocks 非 mermaid 块与空输入安全', () => {
    expect(extractMermaidBlocks('')).toEqual([])
    expect(extractMermaidBlocks(null)).toEqual([])
    expect(extractMermaidBlocks('```mermaidjs\nfoo\n```')).toEqual([])
    expect(extractMermaidBlocks('```json\n{}\n```')).toEqual([])
  })

  it('renderMermaid 按源码缓存：同一源码只请求一次', async () => {
    mermaidCache.clear()
    let calls = 0
    const poster = async (source: string) => { calls++; return { ok: true, svg: '<svg>' + source + '</svg>' } }
    const a = await renderMermaid('graph TD\nA-->B', poster)
    const b = await renderMermaid('graph TD\nA-->B', poster)
    expect(calls).toBe(1)
    expect(a).toBe(b)
    expect(a.ok).toBe(true)
    const c = await renderMermaid('graph TD\nC-->D', poster)
    expect(calls).toBe(2)
    expect(c.svg).toContain('C-->D')
  })

  it('renderMermaid 失败（ok:false / 抛错）记为失败并同样缓存', async () => {
    mermaidCache.clear()
    let calls = 0
    const bad = async () => { calls++; return { ok: false, svg: '' } }
    const r1 = await renderMermaid('bad src', bad)
    const r2 = await renderMermaid('bad src', bad)
    expect(r1.ok).toBe(false)
    expect(r2.ok).toBe(false)
    expect(calls).toBe(1)
    const boom = async () => { throw new Error('network') }
    const r3 = await renderMermaid('boom src', boom)
    expect(r3.ok).toBe(false)
  })
})
