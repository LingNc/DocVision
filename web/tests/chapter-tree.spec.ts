/*
 * P34 递归章节树的展开语义：展开状态是每个节点自己的记忆，与"正在看哪个会话"
 * 解耦——修复"点一下别处、刚拉开的树全收回去"与"一次点击既选中又展开"。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state, STORE_PREFIX } from '../src/state'
import { buildChapterTree, chapterAttemptChain, chapterAttemptTag, chainMemberTag } from '../src/legacy/sidebar'
import { chapterNodeKey, chapterWantOpen, setCollapsed, hasCollapseMemory } from '../src/legacy/sidebar'

beforeEach(() => {
  state.collapsed = {}
})

describe('章节节点展开记忆', () => {
  it('键带 scope：不同项目/阶段的同名节点不串状态', () => {
    const a = chapterNodeKey('书A/convert', 'chapter_001')
    const b = chapterNodeKey('书B/convert', 'chapter_001')
    const c = chapterNodeKey('书A/checker', 'chapter_001')
    expect(new Set([a, b, c]).size).toBe(3)
  })

  it('没记忆时只展开装着当前会话的那一支', () => {
    expect(chapterWantOpen('s/convert', 'chapter_001', false)).toBe(false)
    expect(chapterWantOpen('s/convert', 'chapter_001', true)).toBe(true)
  })

  it('手动展开后一直开着，与当前会话在哪无关', () => {
    setCollapsed(chapterNodeKey('s/convert', 'chapter_002'), false)
    expect(chapterWantOpen('s/convert', 'chapter_002', false)).toBe(true)
    expect(chapterWantOpen('s/convert', 'chapter_002', true)).toBe(true)
  })

  it('手动收起优先于"装着当前会话"，不会被选中自动弹开', () => {
    setCollapsed(chapterNodeKey('s/convert', 'chapter_003'), true)
    expect(chapterWantOpen('s/convert', 'chapter_003', true)).toBe(false)
  })

  it('展开/收起一个节点不影响其它节点（含同一 scope 的兄弟节点）', () => {
    const k1 = chapterNodeKey('s/convert', 'chapter_001')
    const k2 = chapterNodeKey('s/convert', 'chapter_002')
    setCollapsed(k1, false)
    setCollapsed(k2, false)
    setCollapsed(k1, true)
    expect(chapterWantOpen('s/convert', 'chapter_001', false)).toBe(false)
    expect(chapterWantOpen('s/convert', 'chapter_002', false)).toBe(true)
    expect(hasCollapseMemory(k2)).toBe(true)
  })

  it('记忆落盘到 localStorage（前缀 + collapsed 键）', () => {
    /* storeSet 走的是 window.localStorage；node 环境只有 globalThis 上的哑实现，
       这里临时支一个 window，测完还原。 */
    const g = globalThis as any
    const savedWindow = g.window
    g.window = { localStorage: g.localStorage }
    try {
      setCollapsed(chapterNodeKey('s/convert', 'chapter_009'), false)
      const raw = localStorage.getItem(STORE_PREFIX + 'collapsed')
      expect(raw).toBeTruthy()
      expect(JSON.parse(raw as string)[chapterNodeKey('s/convert', 'chapter_009')]).toBe(false)
    } finally {
      g.window = savedWindow
    }
  })
})

// T165：章节转换被 checker 打回后换全新会话重来，几次尝试要能串成一条链
// （中栏「历史 N」页签），并按尝试序号标注「第 N 次」。第一次尝试不带标签。
describe('chapter attempt chain', () => {
  const mk = (id: string, attempt: number, mtime: string, chapterId = 'chapter_042') => ({
    id,
    project: 'book',
    chapterId,
    chapterRole: 'convert',
    chapterAttempt: attempt,
    mtime,
  })

  it('把同一章的各次尝试聚成一条链（新 → 旧）', () => {
    const sessions = [
      mk('a/convert_chapter_042.jsonl', 1, '2026-10-19T10:00:00Z'),
      mk('a/convert_chapter_042.try2.jsonl', 2, '2026-10-19T12:00:00Z'),
      mk('a/convert_chapter_008.jsonl', 1, '2026-10-19T11:00:00Z', 'chapter_008'),
    ]
    const chain = chapterAttemptChain(sessions, 'a/convert_chapter_042.try2.jsonl')
    expect(chain).not.toBeNull()
    expect(chain!.length).toBe(2)
    expect(chain![0].chapterAttempt).toBe(2)
  })

  it('只尝试过一次 / 非章节会话不出链', () => {
    const sessions = [mk('a/convert_chapter_042.jsonl', 1, '2026-10-19T10:00:00Z')]
    expect(chapterAttemptChain(sessions, 'a/convert_chapter_042.jsonl')).toBeNull()
    expect(chapterAttemptChain([{ id: 'x/style_session.jsonl', project: 'book' }], 'x/style_session.jsonl')).toBeNull()
  })

  it('标签：第一次为空，之后是第 N 次', () => {
    expect(chapterAttemptTag({ chapterAttempt: 1 })).toBe('')
    expect(chapterAttemptTag({ chapterAttempt: 3 })).toBe('第 3 次')
    expect(chainMemberTag(mk('a/convert_chapter_042.try2.jsonl', 2, '2026-10-19T12:00:00Z'))).toBe('第 2 次')
  })
})

// T170：章节树的一行代表"这一章现在的样子"，默认打开**最新一次**尝试。
// 旧实现靠"遍历时后者覆盖前者"，而会话列表按时间新→旧，结果停在第一次（被
// 作废的那次）上；这里两个输入顺序都必须选到最新一次。
describe('章节树默认打开最新一次尝试', () => {
  const mk = (attempt: number, mtime: string, role = 'convert', chapterId = 'chapter_042') => ({
    id: `book/work/sessions/${role}_${chapterId}` + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book',
    chapterId,
    chapterRole: role,
    chapterAttempt: attempt,
    mtime,
  })
  const attempts = [
    mk(1, '2026-10-19T10:00:00Z'),
    mk(2, '2026-10-19T12:00:00Z'),
    mk(3, '2026-10-19T14:00:00Z'),
  ]

  it('列表新→旧（接口给的样子）也选最新一次', () => {
    const roots = buildChapterTree([...attempts].reverse())
    expect(roots.length).toBe(1)
    expect(roots[0].convert!.chapterAttempt).toBe(3)
  })

  it('列表旧→新也选最新一次（与入参顺序无关）', () => {
    const roots = buildChapterTree([...attempts])
    expect(roots[0].convert!.chapterAttempt).toBe(3)
  })

  it('核对会话同样取最新一次', () => {
    const roots = buildChapterTree([
      mk(1, '2026-10-19T10:00:00Z', 'checker'),
      mk(2, '2026-10-19T11:00:00Z', 'checker'),
    ])
    expect(roots[0].checker!.chapterAttempt).toBe(2)
    expect(roots[0].convert).toBeNull()
  })

  it('尝试号相同（旧转录没有该字段）时退回修改时间', () => {
    const a = { ...mk(1, '2026-10-19T10:00:00Z'), chapterAttempt: undefined }
    const b = { ...mk(1, '2026-10-19T12:00:00Z'), chapterAttempt: undefined }
    const roots = buildChapterTree([a, b])
    expect(roots[0].convert!.mtime).toBe('2026-10-19T12:00:00Z')
  })

  it('子节点仍挂到父节点上，父子各自选自己的最新一次', () => {
    const parent = mk(2, '2026-10-19T12:00:00Z', 'convert', 'chapter_042')
    const child = { ...mk(1, '2026-10-19T13:00:00Z', 'convert', 'chapter_042_child_01'), chapterParent: 'chapter_042' }
    const roots = buildChapterTree([parent, child])
    expect(roots.length).toBe(1)
    expect(roots[0].children.length).toBe(1)
    expect(roots[0].convert!.chapterAttempt).toBe(2)
    expect(roots[0].children[0].convert!.id).toContain('_child_01')
  })
})
