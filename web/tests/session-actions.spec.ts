/*
 * P56：中栏动作条给哪条会话哪些按钮，以及"点下去会发生什么"的文案。
 * 这些按钮会真的开新尝试 / 删转录，所以判定只留一处（legacy/actions.ts），
 * 这里把规则钉死：谁有按钮、谁没有、确认文案说的是不是那件事。
 */
import { describe, it, expect } from 'vitest'
import { canContinueOf, canDropOf, canRedrawOf, confirmTextOf, continueTargetOf, retryTargetOf } from '../src/legacy/actions'

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

  // P56⑤：子章节给按钮，但只在"父节点还在等子会话"时服务端才接受；整合完就拒。
  it('递归子章节给按钮，边界交给服务端与确认文案', () => {
    const t = retryTargetOf(conv({ chapterId: 'chapter_004_child_01' }))
    expect(t).toEqual({ project: 'book', chapter: 'chapter_004_child_01', role: 'convert' })
    expect(confirmTextOf('retry', conv({ chapterId: 'chapter_004_child_01' }))).toContain('父节点还在等子会话')
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

  /* T196：站在递归子章节的转换会话上点「继续跑」，跑的是它所在的顶层父章节。
   * 子节点的转录与产物都在父工作区的细分树里，单给子章节排一条 continue 到不了
   * 任何前置条件（会一直躺在 work/retry/ 里），按钮看起来毫无反应。 */
  it('继续跑：子章节换成它的父章节，顶层就是自己', () => {
    expect(continueTargetOf({ project: 'book', chapterId: 'chapter_007', chapterRole: 'convert' }))
      .toEqual({ project: 'book', chapter: 'chapter_007', role: 'convert' })
    expect(continueTargetOf({ project: 'book', chapterId: 'chapter_007_child_01_child_01', chapterRole: 'convert' }))
      .toEqual({ project: 'book', chapter: 'chapter_007', role: 'convert' })
    // 核对会话没有"继续产出"这回事。
    expect(continueTargetOf({ project: 'book', chapterId: 'chapter_007_child_02_child_02', chapterRole: 'checker' })).toBe(null)
    expect(canContinueOf({ project: 'book', chapterId: 'chapter_007_child_01_child_01', chapterRole: 'convert' })).toBe(true)
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

  /* T197：子章节的核对会话曾经弹的是转换口径的文案（"重跑会破坏已定稿的东西、
   * 父章节一整合完就被拒"），把"重核不改产物"这条铁律说反了。角色要先于层级。 */
  it('子章节的核对会话说的是"重新核对"，不是"重试这个子章节"', () => {
    const child = checker({ chapterId: 'chapter_014_child_04' })
    expect(confirmTextOf('retry', child)).toContain('产物一个字不动')
    expect(confirmTextOf('retry', child)).not.toContain('父节点还在等子会话')
    expect(confirmTextOf('retry', child)).not.toContain('重试这个子章节')
    // 转换子章节仍然走"只在父节点等子会话时有效"那套说法。
    expect(confirmTextOf('retry', conv({ chapterId: 'chapter_014_child_04' }))).toContain('父节点还在等子会话')
  })

  it('继续跑说清"不新建尝试"', () => {
    expect(confirmTextOf('continue', conv())).toContain('不新建尝试')
  })

  it('撤销说清"只删队列文件"', () => {
    expect(confirmTextOf('cancel', conv())).toContain('work/retry/')
  })
})
