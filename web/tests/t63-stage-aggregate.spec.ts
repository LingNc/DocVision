// T63：convert/checker 阶段聚合——跑动只显 live 行、全完成收行、错误保留、
// 过滤不聚合；T62：fix/final-review 阶段名进表。
import { describe, expect, test } from 'vitest'
import { stageChapterAggregate, stageRank, stageTitleOf, sessionStageOf, buildGroups } from '../src/legacy/sidebar'
import { state } from '../src/state'

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

describe('T111 去重核验阶段', () => {
  test('从稳定转录名/标签恢复旧索引缺失的核验阶段', () => {
    expect(sessionStageOf({ id: 'book/source/sessions/motif_dedupe_abc.jsonl', stage: '会话' })).toBe('dedupe')
    expect(sessionStageOf({ label: 'dedupe:abc' })).toBe('dedupe')
    expect(sessionStageOf({ label: 'motif-dedupe:abc' })).toBe('dedupe')
    expect(sessionStageOf({ id: 'book/other.jsonl', stage: 'style' })).toBe('style')
    expect(sessionStageOf({ id: 'book/other.jsonl' })).toBe('session')
  })
  test('实际 buildGroups 将旧索引核验会话分到矢量图之前', () => {
    const previous = { sessions: state.sessions, board: state.board, filter: state.filter }
    try {
      state.board = 'latex'
      state.filter = ''
      state.sessions = [
        { id: 'book/source/sessions/vector_book__abc__test.jsonl', project: 'book', stage: 'vector' },
        { id: 'book/source/sessions/motif_dedupe_abc.jsonl', project: 'book', stage: '会话' },
      ]
      const groups = buildGroups()
      const stages = Object.keys(groups[0].stages).sort((a, b) => stageRank(a) - stageRank(b))
      expect(stages).toEqual(['dedupe', 'vector'])
      expect(groups[0].stages.dedupe.title).toBe('相似图核验')
      expect(groups[0].stages.dedupe.items).toHaveLength(1)
    } finally {
      Object.assign(state, previous)
    }
  })
  test('列在矢量作图之前且有中文标题', () => {
    expect(stageTitleOf('dedupe')).toBe('相似图核验')
    expect(stageRank('dedupe')).toBeLessThan(stageRank('vector'))
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
