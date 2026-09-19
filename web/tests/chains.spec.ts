/*
 * T53：img2text 调用链——同一张图的多次会话（逐图分析 / 升级修复 stage0·stage1 /
 * prev 轮转）聚合成一组。覆盖链键归一规则、链内排序、折叠分组。
 */
import { describe, it, expect } from 'vitest'
import {
  parseImg2TextId, img2textMemberTag, sortChainMembers, buildImg2TextChains,
} from '../src/legacy/sidebar'

const HASH = 'aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb'
const HASH2 = 'bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbbccccdddd0000'

function sess(id: string, mtime: string, extra: Record<string, unknown> = {}) {
  return { id, board: 'img2text', mtime, ...extra }
}

describe('T53 parseImg2TextId 链键归一', () => {
  it('sessions 路径：裸图名直接作链键', () => {
    const r = parseImg2TextId(`img2text:sessions/书A.md/${HASH}.jpg.jsonl`)
    expect(r).toEqual({ kind: 'analyze', img: `${HASH}.jpg`, stage: -1, prev: 0 })
  })

  it('sessions 路径：images_ 前缀与 <md>_ 前缀都剥掉（旧/新两种命名归一到同一链键）', () => {
    const oldName = parseImg2TextId(`img2text:sessions/书A.md/images_${HASH}.jpg.jsonl`)
    const newName = parseImg2TextId(`img2text:sessions/书A.md/images_书A_${HASH}.jpg.jsonl`)
    expect(oldName && oldName.img).toBe(`${HASH}.jpg`)
    expect(newName && newName.img).toBe(`${HASH}.jpg`)
  })

  it('sessions 路径：md 名含下划线也能剥前缀', () => {
    const r = parseImg2TextId(`img2text:sessions/我的_书.md/images_我的_书_${HASH}.jpg.jsonl`)
    expect(r && r.img).toBe(`${HASH}.jpg`)
  })

  it('mermaid_fix 路径：按最后一个下划线拆，md 含下划线不影响', () => {
    const r = parseImg2TextId(`img2text:mermaid_fix/我的_书_${HASH}.jpg/session-stage0.jsonl`)
    expect(r).toEqual({ kind: 'fix', img: `${HASH}.jpg`, stage: 0, prev: 0 })
  })

  it('mermaid_fix 路径：stage1 与 prev 轮转解析正确', () => {
    const s1 = parseImg2TextId(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.jsonl`)
    expect(s1).toEqual({ kind: 'fix', img: `${HASH}.jpg`, stage: 1, prev: 0 })
    const p1 = parseImg2TextId(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev1.jsonl`)
    expect(p1).toEqual({ kind: 'fix', img: `${HASH}.jpg`, stage: 0, prev: 1 })
    const p2 = parseImg2TextId(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.prev2.jsonl`)
    expect(p2).toEqual({ kind: 'fix', img: `${HASH}.jpg`, stage: 1, prev: 2 })
  })

  it('两种来源归一到同一链键（<hash>.<ext>）', () => {
    const a = parseImg2TextId(`img2text:sessions/书A.md/images_书A_${HASH}.jpg.jsonl`)
    const f = parseImg2TextId(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl`)
    expect(a && f && a.img === f.img).toBe(true)
  })

  it('拆不出书/图分界的目录段：整段作链键（stage0/stage1 仍归并一致）', () => {
    const a = parseImg2TextId('img2text:mermaid_fix/没有下划线/session-stage0.jsonl')
    const b = parseImg2TextId('img2text:mermaid_fix/没有下划线/session-stage1.jsonl')
    expect(a && a.img).toBe('没有下划线')
    expect(b && b.img).toBe('没有下划线')
  })

  it('非 img2text ID 与不认识的形态返回 null', () => {
    expect(parseImg2TextId('bookA/work/sessions/style_session.jsonl')).toBeNull()
    expect(parseImg2TextId('img2text:other/x/y.jsonl')).toBeNull()
    expect(parseImg2TextId('img2text:mermaid_fix/书A_图/readme.txt')).toBeNull()
    expect(parseImg2TextId('img2text:sessions/书A.md/')).toBeNull()
    expect(parseImg2TextId('')).toBeNull()
  })
})

describe('T53 img2textMemberTag 历史标签', () => {
  it('逐图分析 / stageN / 轮转文案', () => {
    expect(img2textMemberTag(`img2text:sessions/书A.md/${HASH}.jpg.jsonl`)).toBe('逐图分析')
    expect(img2textMemberTag(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl`)).toBe('stage0')
    expect(img2textMemberTag(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.jsonl`)).toBe('stage1')
    expect(img2textMemberTag(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev1.jsonl`)).toBe('stage0 · 上一次')
    expect(img2textMemberTag(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev2.jsonl`)).toBe('stage0 · 上上次')
    expect(img2textMemberTag(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev3.jsonl`)).toBe('stage0 · prev3')
    expect(img2textMemberTag('plain/id.jsonl')).toBe('')
  })
})

describe('T53 sortChainMembers 链内排序', () => {
  it('mtime 最新为主行；其余次新→最旧', () => {
    const items = [
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl`, '2026-01-02T00:00:00Z'),
      sess(`img2text:sessions/书A.md/${HASH}.jpg.jsonl`, '2026-01-01T00:00:00Z'),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.jsonl`, '2026-01-03T00:00:00Z'),
    ]
    const sorted = sortChainMembers(items)
    expect(sorted[0].id).toContain('stage1')
    expect(sorted[1].id).toContain('stage0')
    expect(sorted[2].id).toContain('sessions/')
  })

  it('mtime 并列：stage 编号大的新；再并列 prev 序号小的新（当前 > prev1 > prev2）', () => {
    const t = '2026-01-01T00:00:00Z'
    const items = [
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev2.jsonl`, t),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev1.jsonl`, t),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl`, t),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.jsonl`, t),
    ]
    const sorted = sortChainMembers(items)
    expect(sorted.map((s) => s.id.split('/').pop())).toEqual([
      'session-stage1.jsonl', 'session-stage0.jsonl',
      'session-stage0.prev1.jsonl', 'session-stage0.prev2.jsonl',
    ])
  })
})

describe('T53 buildImg2TextChains 折叠分组', () => {
  it('同图的逐图分析 + stage0 + stage1 + prev 归并成一条链，最新为主行', () => {
    const items = [
      sess(`img2text:sessions/书A.md/images_书A_${HASH}.jpg.jsonl`, '2026-01-01T00:00:00Z', { chapterOrder: 1 }),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.prev1.jsonl`, '2025-12-01T00:00:00Z'),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl`, '2026-01-02T00:00:00Z'),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.jsonl`, '2026-01-03T00:00:00Z'),
    ]
    const { chains, singles } = buildImg2TextChains(items)
    expect(singles).toEqual([])
    expect(chains.length).toBe(1)
    const c = chains[0]
    expect(c.img).toBe(`${HASH}.jpg`)
    expect(c.main.id).toContain('stage1')
    expect(c.rest.length).toBe(3)
    expect(c.items.length).toBe(4)
  })

  it('不同图各成一条链；链按书内序号（成员最小 chapterOrder）排', () => {
    const items = [
      sess(`img2text:sessions/书A.md/${HASH2}.jpg.jsonl`, '2026-01-01T00:00:00Z', { chapterOrder: 2 }),
      sess(`img2text:sessions/书A.md/${HASH}.jpg.jsonl`, '2026-01-01T00:00:00Z', { chapterOrder: 1 }),
      sess(`img2text:mermaid_fix/书A_${HASH2}.jpg/session-stage0.jsonl`, '2026-01-02T00:00:00Z'),
    ]
    const { chains } = buildImg2TextChains(items)
    expect(chains.length).toBe(2)
    expect(chains[0].img).toBe(`${HASH}.jpg`)
    expect(chains[1].img).toBe(`${HASH2}.jpg`)
    // HASH2 的链：修复段比逐图分析新，主行是 stage0
    expect(chains[1].main.id).toContain('stage0')
    expect(chains[1].rest.length).toBe(1)
  })

  it('解析不了 ID 的会话进 singles 原样保留；live 聚到链上', () => {
    const items = [
      sess('img2text:mystery/unknown/deep/x.jsonl', '2026-01-01T00:00:00Z'),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage0.jsonl`, '2026-01-02T00:00:00Z', { live: 1 }),
      sess(`img2text:mermaid_fix/书A_${HASH}.jpg/session-stage1.jsonl`, '2026-01-03T00:00:00Z'),
    ]
    const { chains, singles } = buildImg2TextChains(items)
    expect(singles.length).toBe(1)
    expect(chains.length).toBe(1)
    expect(chains[0].live).toBe(1)
  })

  it('空输入安全', () => {
    const { chains, singles } = buildImg2TextChains([])
    expect(chains).toEqual([])
    expect(singles).toEqual([])
  })
})
