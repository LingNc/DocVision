// T63：convert/checker 阶段聚合——跑动只显 live 行、全完成收行、错误保留、
// 过滤不聚合；T62：fix/final-review 阶段名进表。
import { describe, expect, test } from 'vitest'
import { stageChapterAggregate, stageRank, stageTitleOf } from '../src/legacy/sidebar'

const mk = (id: string, over: Record<string, unknown> = {}) => ({ id, stage: 'convert', ...over })

describe('stageChapterAggregate（T63）', () => {
  test('全部完成：不显示行，阶段头报总数与完成数', () => {
    const items = [mk('a', { endState: 'done' }), mk('b', { endState: 'done' }), mk('c', { endState: 'done' })]
    const r = stageChapterAggregate('convert', items, false)!
    expect(r.rows).toEqual([])
    expect(r.head).toBe('3 章 · 完成 3')
  })
  test('跑动中：只显 live 行，头带运行数', () => {
    const items = [mk('a', { endState: 'done' }), mk('b', { live: true }), mk('c', { live: true }), mk('d', {})]
    const r = stageChapterAggregate('checker', items, false)!
    expect(r.rows.map((s) => s.id)).toEqual(['b', 'c'])
    expect(r.head).toContain('运行 2')
    expect(r.head).toContain('4 章')
  })
  test('错误收场的会话结束也保留', () => {
    const items = [mk('a', { endState: 'done' }), mk('b', { endState: 'error' })]
    const r = stageChapterAggregate('convert', items, false)!
    expect(r.rows.map((s) => s.id)).toEqual(['b'])
    expect(r.head).toContain('错误 1')
  })
  test('搜索过滤时不聚合', () => {
    expect(stageChapterAggregate('convert', [mk('a')], true)).toBeNull()
  })
  test('其它阶段不聚合', () => {
    expect(stageChapterAggregate('style', [mk('a')], false)).toBeNull()
  })
})

describe('T62 阶段表', () => {
  test('fix / final-review 有排名与标题', () => {
    expect(stageTitleOf('fix')).toBe('全书修复')
    expect(stageTitleOf('final-review')).toBe('终审')
    expect(stageRank('fix')).toBeGreaterThan(stageRank('style-fix'))
    expect(stageRank('final-review')).toBeGreaterThan(stageRank('fix'))
  })
})
