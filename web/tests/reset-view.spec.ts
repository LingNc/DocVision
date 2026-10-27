/*
 * T215：「重置视图」只清跟"我在看哪里"有关的记忆（手动选中/pin、侧栏展开收起、
 * 搜索过滤），回到当前正在跑的顶层会话；主题 / 显示单位 / Markdown 这些与导航
 * 无关的显示偏好必须原样留着。
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { state, resetViewState, storeSet, STORE_PREFIX } from '../src/state'

const g = globalThis as Record<string, unknown>

beforeEach(() => {
  // storeGet/storeSet 走 window.localStorage；node 环境只有 globalThis 上的哑实现。
  g.window = { localStorage: (globalThis as Record<string, unknown>).localStorage }
  localStorage.clear()
  // 手动摆出来的一屏：选中了一条历史尝试、有过滤、有展开记忆、停在轨迹页。
  state.sessions = [{ id: 'book/work/sessions/style_session.jsonl', project: 'book' }] as never[]
  state.current = { id: 'book/work/sessions/convert_chapter_042.try2.jsonl', chapterId: 'chapter_042' } as never
  state.lines = [{ n: 1 } as never]
  state.nextFrom = 7
  state.meta = { kind: 'system' }
  state.filter = 'chapter_042'
  state.collapsed = { 'chapter:book/convert/chapter_042': true }
  state.overflow = { 'overflow:book/convert': true }
  state.view = 'trajectory'
  state.trajSelected = '3'
  state.trajOpen = { '1-2': true }
  state.trajKinds = { user: false }
  // 与导航无关的显示偏好
  state.theme = 'dark'
  state.unit = 'char'
  state.markdown = false
  state.showThumbs = false
  storeSet('collapsed', JSON.stringify(state.collapsed))
  storeSet('overflow', JSON.stringify(state.overflow))
  storeSet('theme', 'dark')
  storeSet('unit', 'char')
  storeSet('markdown', '0')
})

afterEach(() => {
  delete g.window
  localStorage.clear()
})

describe('T215 重置视图', () => {
  it('清掉导航键（选中/pin、展开收起、过滤）', () => {
    const id = resetViewState()
    expect(state.current).toBeNull()
    expect(state.lines).toEqual([])
    expect(state.nextFrom).toBe(0)
    expect(state.meta).toBeNull()
    expect(state.filter).toBe('')
    expect(state.collapsed).toEqual({})
    expect(state.overflow).toEqual({})
    expect(state.view).toBe('chat')
    expect(state.trajSelected).toBeNull()
    expect(state.trajOpen).toEqual({})
    expect(state.trajKinds).toEqual({})
    // 落盘的展开/更多会话记忆也清掉，否则刷新一次又回来
    expect(localStorage.getItem(STORE_PREFIX + 'collapsed')).toBeNull()
    expect(localStorage.getItem(STORE_PREFIX + 'overflow')).toBeNull()
    // 没有活跃会话时退回列表第一条（与首次打开页面同一个口径）
    expect(id).toBe('book/work/sessions/style_session.jsonl')
  })

  it('主题 / 显示单位 / Markdown / 缩略图原样留着', () => {
    resetViewState()
    expect(state.theme).toBe('dark')
    expect(state.unit).toBe('char')
    expect(state.markdown).toBe(false)
    expect(state.showThumbs).toBe(false)
    expect(localStorage.getItem(STORE_PREFIX + 'theme')).toBe('dark')
    expect(localStorage.getItem(STORE_PREFIX + 'unit')).toBe('char')
    expect(localStorage.getItem(STORE_PREFIX + 'markdown')).toBe('0')
  })

  it('回到当前正在跑的顶层会话：子章节让位给父章节', () => {
    state.sessions = [
      {
        id: 'book/work/temp/conv_chapter_042/work/_subdivision/child_01/node/sessions/convert_chapter_042_child_01.jsonl',
        project: 'book', live: true, chapterId: 'chapter_042_child_01', chapterParent: 'chapter_042',
      },
      { id: 'book/work/sessions/convert_chapter_042.jsonl', project: 'book', live: true, chapterId: 'chapter_042' },
    ] as never[]
    expect(resetViewState()).toBe('book/work/sessions/convert_chapter_042.jsonl')
  })

  it('板块之间不串：img2text 板块只回 img2text 的会话', () => {
    state.board = 'img2text'
    state.sessions = [
      { id: 'book/work/sessions/convert_chapter_042.jsonl', project: 'book', live: true, chapterId: 'chapter_042' },
      { id: 'img2text:sessions/a/1.png.jsonl', project: 'book', board: 'img2text' },
    ] as never[]
    expect(resetViewState()).toBe('img2text:sessions/a/1.png.jsonl')
    state.board = 'latex'
  })
})
