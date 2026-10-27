/*
 * T194：排上「重试 / 重新核对」之后，视图要跟着**新尝试**走（用户口径：
 * "这俩是一体的吧，如果切换回去就都切换过去"）。排队只是写一个请求文件，新
 * 转录是 latex 那边建的，所以这里钉住守望条件本身：
 *  · 排队之后出现/更新的同项目同章节同角色转录 → 切过去；
 *  · 比排队更早的老转录不算"新尝试"；
 *  · 排队二十分钟还没被消费 → 守望作废，别事后突然抢视图。
 */
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { applyIndex, selectSession, watchFork } from '../src/data'
import { state } from '../src/state'

const now = Date.now()
const iso = (ms: number) => new Date(ms).toISOString()

const sess = (over: Record<string, unknown> = {}) => ({
  id: 'book/work/sessions/convert_chapter_002.jsonl',
  name: 'convert_chapter_002.jsonl',
  project: 'book',
  chapterId: 'chapter_002',
  chapterRole: 'convert',
  chapterAttempt: 1,
  mtime: iso(now - 3600_000),
  size: 10,
  ...over,
})
const checker2 = sess({
  id: 'book/work/sessions/checker_chapter_002.try2.jsonl',
  name: 'checker_chapter_002.try2.jsonl',
  chapterRole: 'checker',
  chapterAttempt: 2,
  mtime: iso(now),
})

beforeEach(() => {
  state.sessions = []
  state.current = null
  // 选中会话会去拉 /api/session；这里只要"切到哪条"，不让网络参与。
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ lines: [], nextFrom: 0 }) })))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('重新核对之后跟着新尝试走', () => {
  it('排队后出现的核对会话自动成为当前会话', () => {
    applyIndex({ sessions: [sess()] })
    selectSession(sess().id)
    expect(state.current.id).toBe(sess().id)

    watchFork('book', 'chapter_002', 'checker', sess().id)
    applyIndex({ sessions: [sess(), checker2] })

    expect(state.current.id).toBe(checker2.id)
  })

  it('排队之前就存在的老转录不算新尝试，视图不动', () => {
    applyIndex({ sessions: [sess()] })
    selectSession(sess().id)

    watchFork('book', 'chapter_002', 'checker', sess().id)
    // 一条一小时前就写好的核对会话：它跟这次排队无关。
    applyIndex({
      sessions: [sess(), sess({ id: 'x/checker_chapter_002.jsonl', chapterRole: 'checker', chapterAttempt: 2 })],
    })

    expect(state.current.id).toBe(sess().id)
  })

  it('同章节另一条老尝试也不会把视图抢过去', () => {
    applyIndex({ sessions: [sess()] })
    selectSession(sess().id)
    watchFork('book', 'chapter_002', 'convert', sess().id)
    applyIndex({ sessions: [sess(), sess({ id: 'old/convert_chapter_002.jsonl', chapterAttempt: 2 })] })
    expect(state.current.id).toBe(sess().id)
  })

  it('二十分钟内没被消费就作废，之后出现的会话不再抢视图', () => {
    vi.useFakeTimers()
    vi.setSystemTime(now)
    applyIndex({ sessions: [sess()] })
    selectSession(sess().id)

    watchFork('book', 'chapter_002', 'checker', sess().id)
    vi.setSystemTime(now + 30 * 60_000)
    applyIndex({
      sessions: [sess(), { ...checker2, mtime: iso(now + 30 * 60_000) }],
    })
    expect(state.current.id).toBe(sess().id)
    vi.useRealTimers()
  })
})
