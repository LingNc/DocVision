/*
 * P56：中栏动作条给哪条会话哪些按钮，以及"点下去会发生什么"的文案。
 * 这些按钮会真的开新尝试 / 删转录，所以判定只留一处（legacy/actions.ts），
 * 这里把规则钉死：谁有按钮、谁没有、确认文案说的是不是那件事。
 */
import { describe, it, expect } from 'vitest'
import { canContinueOf, canDropOf, canRedrawOf, confirmTextOf, retryTargetOf } from '../src/legacy/actions'

const conv = (over: Record<string, unknown> = {}) => ({
  id: 'book/work/sessions/convert_chapter_002.jsonl',
  project: 'book', chapterId: 'chapter_002', chapterRole: 'convert',
  path: 'book/work/sessions/convert_chapter_002.jsonl', name: 'convert_chapter_002.jsonl',
  ...over,
})
const checker = (over: Record<string, unknown> = {}) => conv({ id: 'x/checker_chapter_002.jsonl', chapterRole: 'checker', name: 'checker_chapter_002.jsonl', ...over })

describe('重试目标', () => {
  it('转换与核对会话都能重试，角色跟着走', () => {
    expect(retryTargetOf(conv())).toEqual({ project: 'book', chapter: 'chapter_002', role: 'convert' })
    expect(retryTargetOf(checker())).toEqual({ project: 'book', chapter: 'chapter_002', role: 'checker' })
  })

  // 子章节由父章节的细分树产出：单独重跑它会让父节点的整合/晋级状态对不上。
  // 不给按钮，而不是点了才报错。
  it('递归子章节不给按钮', () => {
    expect(retryTargetOf(conv({ chapterId: 'chapter_004_child_01' }))).toBe(null)
  })

  it('样式/汇编/整组核验这些屏障阶段没有章节 ID，也就没有按钮', () => {
    expect(retryTargetOf({ project: 'book', stage: 'style' })).toBe(null)
    expect(retryTargetOf({ project: 'book', stage: 'assemble', chapterId: '' })).toBe(null)
  })

  it('静态导出模式一律不给（只读快照，没有服务端）', () => {
    expect(retryTargetOf(conv({ staticMode: true }))).toBe(null)
  })
})

describe('继续跑与重画', () => {
  it('只有转换会话能继续跑', () => {
    expect(canContinueOf(conv())).toBe(true)
    expect(canContinueOf(checker())).toBe(false)
  })

  it('重画只给作图会话，且它不能在写入', () => {
    const vec = { project: 'book', stage: 'vector', name: 'vector_motif_aa.jsonl' }
    expect(canRedrawOf(vec)).toBe(true)
    expect(canRedrawOf({ ...vec, live: true })).toBe(false)
    expect(canRedrawOf({ ...vec, name: 'motif_aa.jsonl' })).toBe(false)
    expect(canRedrawOf({ project: 'book', stage: 'style', name: 'vector_x.jsonl' })).toBe(false)
  })
})

describe('删除记录', () => {
  it('章节会话可删，正在写入的不行', () => {
    expect(canDropOf(conv())).toBe(true)
    expect(canDropOf(conv({ live: true }))).toBe(false)
    expect(canDropOf(conv({ path: '' }))).toBe(false)
    expect(canDropOf({ project: 'book', path: 'x.jsonl' })).toBe(false)
  })
})

describe('二次确认的文案', () => {
  it('重试说清"会开新尝试、旧产物归档"', () => {
    expect(confirmTextOf('retry', conv())).toContain('新尝试')
    expect(confirmTextOf('retry', conv())).toContain('work/attempts/')
  })

  it('重新核对说清"产物不动、旧结论留档"', () => {
    expect(confirmTextOf('retry', checker())).toContain('产物一个字不动')
    expect(confirmTextOf('retry', checker())).toContain('.checker.try')
  })

  it('继续跑说清"不新建尝试"', () => {
    expect(confirmTextOf('continue', conv())).toContain('不新建尝试')
  })

  it('撤销说清"只删队列文件"', () => {
    expect(confirmTextOf('cancel', conv())).toContain('work/retry/')
  })
})
