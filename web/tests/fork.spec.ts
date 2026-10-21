/* P57：从某条消息 fork 的判定与请求体。对话页与轨迹页共用这一处，
 * 规则不对就会在写入中的会话上截断，或者把子章节当成可重跑的对象。 */
import { describe, it, expect } from 'vitest'
import { canForkFrom, forkPrompt, forkRequestOf } from '../src/legacy/fork'

const conv = (over: Record<string, unknown> = {}) => ({
  id: 'book/work/sessions/convert_chapter_002.jsonl',
  project: 'book', chapterId: 'chapter_002', chapterRole: 'convert',
  name: 'convert_chapter_002.jsonl', ...over,
})

describe('能不能 fork', () => {
  it('转换与核对会话都能，角色映射成 continue / checker', () => {
    expect(canForkFrom(conv())).toBe(true)
    expect(forkRequestOf(conv(), 12, '对话')!.role).toBe('continue')
    expect(forkRequestOf(conv({ chapterRole: 'checker' }), 12, '轨迹')!.role).toBe('checker')
  })

  it('作图会话走 vector，且要带作图转录名', () => {
    const vec = { project: 'book', stage: 'vector', name: 'vector_motif_aa.jsonl' }
    expect(canForkFrom(vec)).toBe(true)
    const req = forkRequestOf(vec, 7, '轨迹')!
    expect(req.role).toBe('vector')
    expect(req.chapter).toBe('')
    expect(req.extra).toEqual({ from: 7, path: 'vector_motif_aa.jsonl' })
  })

  it('写入中的会话不给 fork（那一行可能还在追加）', () => {
    expect(canForkFrom(conv({ live: true }))).toBe(false)
  })

  it('子章节、屏障阶段、静态导出都不给', () => {
    expect(canForkFrom(conv({ chapterId: 'chapter_004_child_01' }))).toBe(false)
    expect(canForkFrom({ project: 'book', stage: 'assemble' })).toBe(false)
    expect(canForkFrom(conv({ staticMode: true }))).toBe(false)
  })

  it('行号必须是正数（没有可截断的点就不发请求）', () => {
    expect(forkRequestOf(conv(), 0, '对话')).toBe(null)
    expect(forkRequestOf(conv(), -3, '对话')).toBe(null)
  })
})

describe('确认文案', () => {
  it('转换说清"后面会丢、原记录保留"', () => {
    expect(forkPrompt(9, conv())).toContain('丢掉')
    expect(forkPrompt(9, conv())).toContain('保留')
  })

  it('核对说清"产物不动"', () => {
    expect(forkPrompt(9, conv({ chapterRole: 'checker' }))).toContain('产物一个字不动')
  })
})
