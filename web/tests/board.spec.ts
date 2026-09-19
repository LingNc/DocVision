/*
 * P18：板块（board）过滤的单测——给定混合 board 的会话列表，latex/img2text
 * 各归其位；buildGroups 按 state.board 只产出当前板块的项目组。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { sessionBoard, filterByBoard, buildGroups } from '../src/legacy/sidebar'

interface FakeSession {
  id: string
  board?: string
  project?: string
  live?: number
  [k: string]: unknown
}

function latexSession(id: string, project?: string): FakeSession {
  return { id, project }
}

function imgSession(id: string, book: string): FakeSession {
  return { id, board: 'img2text', project: 'img2text · ' + book }
}

describe('P18 板块过滤', () => {
  const mixed: FakeSession[] = [
    latexSession('bookA/work/sessions/style_session.jsonl'),
    latexSession('bookB/work/sessions/convert_chapter_001.jsonl'),
    imgSession('img2text/bookA/progress_items/mermaid_fix/x/s.jsonl', 'bookA'),
    imgSession('img2text/bookB/progress_items/mermaid_fix/y/s.jsonl', 'bookB'),
  ]

  it('sessionBoard：缺字段/空值归 latex，img2text 原样', () => {
    expect(sessionBoard({})).toBe('latex')
    expect(sessionBoard({ board: '' })).toBe('latex')
    expect(sessionBoard({ board: 'img2text' })).toBe('img2text')
    // 未知取值按 latex 归位
    expect(sessionBoard({ board: 'other' })).toBe('latex')
    expect(sessionBoard(null)).toBe('latex')
  })

  it('filterByBoard：latex 板块只留无 board 会话，img2text 板块只留 img2text 会话', () => {
    const latex = filterByBoard(mixed, 'latex')
    expect(latex.map((s) => s.id)).toEqual([mixed[0].id, mixed[1].id])
    const i2t = filterByBoard(mixed, 'img2text')
    expect(i2t.map((s) => s.id)).toEqual([mixed[2].id, mixed[3].id])
    // 非法 board 值回落 latex；空列表安全
    expect(filterByBoard(mixed, 'nonsense').length).toBe(2)
    expect(filterByBoard([], 'img2text')).toEqual([])
    expect(filterByBoard(null, 'latex')).toEqual([])
  })

  describe('buildGroups 按 state.board 过滤', () => {
    beforeEach(() => {
      state.sessions = mixed as never[]
      state.filter = ''
      state.collapsed = {}
      state.overflow = {}
    })

    it('latex 板块：只含 latex 项目组', () => {
      state.board = 'latex'
      const groups = buildGroups()
      expect(groups.map((g) => g.name).sort()).toEqual(['bookA', 'bookB'])
      expect(groups.reduce((n, g) => n + g.items.length, 0)).toBe(2)
    })

    it('img2text 板块：按「img2text · 书名」分组', () => {
      state.board = 'img2text'
      const groups = buildGroups()
      expect(groups.map((g) => g.name).sort()).toEqual(['img2text · bookA', 'img2text · bookB'])
      expect(groups.reduce((n, g) => n + g.items.length, 0)).toBe(2)
      state.board = 'latex'
    })

    it('搜索仍只在当前板块内过滤', () => {
      state.board = 'img2text'
      state.filter = 'booka'
      const groups = buildGroups()
      expect(groups.length).toBe(1)
      expect(groups[0].name).toBe('img2text · bookA')
      state.filter = ''
      state.board = 'latex'
    })
  })
})
