/*
 * T266：矢量图阶段同一张图的多次尝试（try1/try2/…）以前平铺成一串**同号**方块
 * ——46 条会话里二十多个序号各出现两次，看不出哪些是重画、也数不清一张图试过
 * 几次。按图片身份把尝试叠成一层：父 = 这张图（最新一次的状态），子 = 各次尝试。
 */
import { describe, it, expect } from 'vitest'
import {
  groupImageAttempts, imageAttemptKey, imageAttemptNo, imageAttemptCurrent, flattenImageGroups,
} from '../src/legacy/sidebar'

function sess(id: string, mtime: string, extra: Record<string, unknown> = {}) {
  return { id, board: 'sessions', mtime, stage: 'vector', ...extra }
}

describe('T266 矢量图尝试分组', () => {
  it('尝试号从转录名的 .tryN 读；没有后缀 = 现役（0），clean_vector 归档出的才是 tryN', () => {
    expect(imageAttemptNo(sess('vector_a.jsonl', '2026-10-01T00:00:00Z'))).toBe(0)
    expect(imageAttemptCurrent(sess('vector_a.jsonl', '2026-10-01T00:00:00Z'))).toBe(true)
    expect(imageAttemptNo(sess('vector_a.try2.jsonl', '2026-10-01T00:00:00Z'))).toBe(2)
    expect(imageAttemptCurrent(sess('vector_a.try2.jsonl', '2026-10-01T00:00:00Z'))).toBe(false)
    expect(imageAttemptNo(sess('vector_a.try10.jsonl', '2026-10-01T00:00:00Z'))).toBe(10)
  })

  it('图片身份优先用 imageName（同一张图换标签也归一组），否则退回去掉 .tryN 的 id', () => {
    expect(imageAttemptKey(sess('vector_a.try2.jsonl', '', { imageName: 'images/x/1.png' }))).toBe('images/x/1.png')
    expect(imageAttemptKey(sess('vector_a.try2.jsonl', ''))).toBe('vector_a')
  })

  it('同一张图的三次尝试叠成一组：现役（无后缀）永远最前，归档的 tryN 大的在前；父行 = 现役', () => {
    const groups = groupImageAttempts([
      sess('vector_a.jsonl', '2026-10-01T00:00:00Z', { imageName: 'p/1.png', imageOrder: 7 }),
      sess('vector_a.try2.jsonl', '2026-10-02T00:00:00Z', { imageName: 'p/1.png', imageOrder: 7 }),
      sess('vector_a.try3.jsonl', '2026-10-03T00:00:00Z', { imageName: 'p/1.png', imageOrder: 7 }),
      sess('vector_b.jsonl', '2026-10-01T00:00:00Z', { imageName: 'p/2.png', imageOrder: 9 }),
    ])
    expect(groups.length).toBe(2)
    const a = groups.find((g) => g.key === 'p/1.png')!
    expect(a.items.map((s: any) => imageAttemptNo(s))).toEqual([0, 3, 2])
    expect(a.main.id).toBe('vector_a.jsonl')
    expect(a.order).toBe(7)
  })

  it('T268：方块视图的全量平铺——同一张图的尝试贴在一起，现役在前；除现役外都算旧次', () => {
    const groups = groupImageAttempts([
      sess('vector_a.jsonl', '2026-10-04T00:00:00Z', { imageName: 'p/1.png', imageOrder: 7 }),
      sess('vector_a.try2.jsonl', '2026-10-02T00:00:00Z', { imageName: 'p/1.png', imageOrder: 7 }),
      sess('vector_b.jsonl', '2026-10-01T00:00:00Z', { imageName: 'p/2.png', imageOrder: 9 }),
    ])
    const flat = flattenImageGroups(groups)
    expect(flat.map((s: any) => s.id)).toEqual(['vector_a.jsonl', 'vector_a.try2.jsonl', 'vector_b.jsonl'])
    expect(flat.map((s: any) => imageAttemptCurrent(s))).toEqual([true, false, true])
  })

  it('按书内序号排，父行带"有失败/运行中"两个一眼可见的信号', () => {
    const groups = groupImageAttempts([
      sess('vector_b.jsonl', '2026-10-01T00:00:00Z', { imageName: 'p/2.png', imageOrder: 9, endState: 'error' }),
      sess('vector_a.jsonl', '2026-10-01T00:00:00Z', { imageName: 'p/1.png', imageOrder: 7, live: true }),
    ])
    expect(groups.map((g) => g.order)).toEqual([7, 9])
    expect(groups[0].live).toBe(true)
    expect(groups[0].hasError).toBe(false)
    expect(groups[1].hasError).toBe(true)
  })

  it('没有 imageName 的老转录按 id 归组：.try2 与无后缀是同一条', () => {
    const groups = groupImageAttempts([
      sess('vector_书__hash__label.jsonl', '2026-10-01T00:00:00Z', { endState: 'done' }),
      sess('vector_书__hash__label.try2.jsonl', '2026-10-02T00:00:00Z', { endState: 'done' }),
    ])
    expect(groups.length).toBe(1)
    expect(groups[0].items.length).toBe(2)
  })
})
