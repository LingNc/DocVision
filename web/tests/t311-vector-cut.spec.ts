// T311：作图会话的 chapterId 是 null，而"就地截断"（⤫）以前拿它当门槛，
// 于是点下去直接静默 return——没有确认框、没有请求、没有提示。能不能截断
// 只该问 canCutFrom（它本来就允许 vector）。
import { describe, expect, it } from 'vitest'
import { canCutFrom, canForkFrom } from '../src/legacy/fork'

const vectorSession = {
  stage: 'vector',
  name: 'vector_书A__aabb__fig.jsonl',
  chapterId: null,
  chapterRole: null,
  project: '书A',
  path: '/tmp/proj/source/sessions/vector_书A__aabb__fig.jsonl',
  live: false,
}

describe('T311 作图会话能不能就地截断', () => {
  it('有转录就能截，且不因为 chapterId 为空被挡', () => {
    expect(vectorSession.chapterId).toBe(null)
    expect(canCutFrom(vectorSession)).toBe(true)
  })

  it('作图会话也能 fork（⤓ 走 role=vector 排重画）', () => {
    expect(canForkFrom(vectorSession)).toBe(true)
  })

  it('正在写 / 静态导出 / 没有项目 一律不给截', () => {
    expect(canCutFrom({ ...vectorSession, live: true })).toBe(false)
    expect(canCutFrom({ ...vectorSession, staticMode: true })).toBe(false)
    expect(canCutFrom({ ...vectorSession, project: '' })).toBe(false)
  })

  it('不是 vector_ 开头的作图转录不给截', () => {
    expect(canCutFrom({ ...vectorSession, name: 'other.jsonl' })).toBe(false)
  })
})
