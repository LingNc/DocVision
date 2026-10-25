/*
 * P56：中栏动作条给哪条会话哪些按钮，以及"点下去会发生什么"的文案。
 * 这些按钮会真的开新尝试 / 删转录，所以判定只留一处（legacy/actions.ts），
 * 这里把规则钉死：谁有按钮、谁没有、确认文案说的是不是那件事。
 */
import { describe, it, expect } from 'vitest'
import { canContinueOf, canDropOf, canRedrawOf, canRestoreOf, confirmTextOf, continueTargetOf, restoreTargetOf, retryTargetOf, sameActionTarget } from '../src/legacy/actions'

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

/*
 * T208：确认条在轮询自动切换会话时要不要收走。
 * 用户报"点那个重试该章节也没有用"——点了「重试这一章」、确认条出现，紧接着
 * 视图被轮询切到本章新开的那条尝试，条子被静默收掉，看起来什么都没发生。
 * 判据：同项目 + 同章节 + 同角色算同一件事（同一章的另一条尝试也算）。
 */
describe('确认条的归属（T208）', () => {
  const target = (over: Record<string, unknown> = {}) => ({ project: 'book', chapter: 'chapter_017', role: 'convert', ...over })

  it('同一章的另一条尝试算同一件事，确认条留着', () => {
    expect(sameActionTarget(target(), target())).toBe(true)
  })

  it('换了章节 / 换了角色 / 换了项目就收走', () => {
    expect(sameActionTarget(target(), target({ chapter: 'chapter_018' }))).toBe(false)
    expect(sameActionTarget(target(), target({ role: 'checker' }))).toBe(false)
    expect(sameActionTarget(target(), target({ project: 'other' }))).toBe(false)
  })

  it('任一侧没有目标（样式/汇编/静态导出）就不留', () => {
    expect(sameActionTarget(null, target())).toBe(false)
    expect(sameActionTarget(target(), null)).toBe(false)
  })
})

/*
 * P63：「切回第 N 次尝试」只长在**顶层章节的转换会话**上，且号不是当前这一次。
 * 这些边界都不是审美问题——子章节的号是父尝试之内的号，拿它去拼 try<N> 会把
 * 两代子树拼在一起；修复/复盘链的号根本不是"这一章的第几次"。
 */
describe('切回第 N 次尝试的按钮（P63）', () => {
  const convAt = (attempt: number, over: Record<string, unknown> = {}) =>
    conv({ chapterAttempt: attempt, name: `convert_chapter_002.try${attempt}.jsonl`, ...over })

  it('旧尝试给按钮，目标带项目/章节/号', () => {
    expect(restoreTargetOf(convAt(1), 3)).toEqual({ project: 'book', chapter: 'chapter_002', attempt: 1 })
    expect(restoreTargetOf(convAt(2), 3)).toEqual({ project: 'book', chapter: 'chapter_002', attempt: 2 })
    expect(canRestoreOf(convAt(1), 3)).toBe(true)
  })

  it('号就是当前这一次时没有可切回的旧版', () => {
    expect(restoreTargetOf(convAt(3), 3)).toBeNull()
    expect(canRestoreOf(convAt(3), 3)).toBe(false)
  })

  // 切过去还能切回来：切回第 1 次之后，第 3 次就在留档里，号比"现在"大。
  it('切回来也认：号不等于当前这一次就行', () => {
    expect(restoreTargetOf(convAt(3), 1)).toEqual({ project: 'book', chapter: 'chapter_002', attempt: 3 })
  })

  it('子章节 / 核对 / 归档 / 静态导出都不给', () => {
    expect(restoreTargetOf(convAt(1, { chapterId: 'chapter_004_child_01' }), 3)).toBeNull()
    expect(restoreTargetOf(checker({ chapterAttempt: 1 }), 3)).toBeNull()
    expect(restoreTargetOf(convAt(1, { archived: true }), 3)).toBeNull()
    expect(restoreTargetOf(convAt(1, { staticMode: true }), 3)).toBeNull()
    expect(restoreTargetOf(null, 3)).toBeNull()
  })

  it('没有尝试号的老转录按第 1 次算', () => {
    expect(restoreTargetOf(conv(), 2)).toEqual({ project: 'book', chapter: 'chapter_002', attempt: 1 })
  })
})
