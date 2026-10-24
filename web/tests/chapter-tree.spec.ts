/*
 * P34 递归章节树的展开语义：展开状态是每个节点自己的记忆，与“正在看哪个会话”
 * 解耦——修复“点一下别处、刚拉开的树全收回去”与“一次点击既选中又展开”。
 *
 * P62：从 chapter-tree.spec.ts 拆出来（行为一字未改）——这里只放章节树本身的
 * 规则：展开记忆、默认取最新一代、只钉住正在看的那一行、标题/状态文案、世代与
 * 归档行。尝试链在 chapter-chain.spec.ts，修复/复盘行在 chapter-fix-rescue.spec.ts。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state, STORE_PREFIX } from '../src/state'
import { buildChapterTree, chapterActivityLabel, chapterAttemptChain, chainMemberTag, plainTitle } from '../src/legacy/sidebar'
import { chapterNodeKey, chapterWantOpen, setCollapsed, hasCollapseMemory } from '../src/legacy/sidebar'
import { applyIndex, chapterPendingRetry } from '../src/data'
import { archivedId, liveId } from './helpers/chapter'

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

// T170：章节树的一行代表"这一章现在的样子"，默认打开**最新一次**尝试。
// 旧实现靠"遍历时后者覆盖前者"，而会话列表按时间新→旧，结果停在第一次（被
// 作废的那次）上；这里两个输入顺序都必须选到最新一次。
describe('章节树默认打开最新一次尝试', () => {
  const mk = (attempt: number, mtime: string, role = 'convert', chapterId = 'chapter_042') => ({
    id: liveId(role, chapterId, attempt),
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

// T175：只钉住"正在看的那一条"——各节点的重试次数互相独立，不能拿一个章节的
// #2 去套全树；点开历史里的 #1，树里只把那一行换成 #1，子会话与其它章节仍按
// 各自最新一次显示。
describe('章节树只钉住正在查看的那一条', () => {
  const conv = (attempt: number, chapterId = 'chapter_042', parent = '') => ({
    id: liveId('convert', chapterId, attempt),
    project: 'book', chapterId, chapterParent: parent, chapterRole: 'convert',
    chapterAttempt: attempt, mtime: `2026-10-19T1${attempt}:00:00Z`,
  })
  const chk = (attempt: number, chapterId = 'chapter_042') => ({
    id: liveId('checker', chapterId, attempt),
    project: 'book', chapterId, chapterRole: 'checker',
    chapterAttempt: attempt, mtime: `2026-10-19T1${attempt}:30:00Z`,
  })
  const child = (attempt: number) => conv(attempt, 'chapter_042_child_01', 'chapter_042')

  it('看 convert #2：只有这一行换成 #2，子会话与别的章节仍是最新', () => {
    const sessions = [
      conv(1), conv(2), conv(3),
      child(1), child(2), child(3),
      conv(1, 'chapter_043'), conv(2, 'chapter_043'),
    ]
    const roots = buildChapterTree(sessions, conv(2).id)
    const a = roots.find((n) => n.id === 'chapter_042')!
    expect(a.convert!.chapterAttempt).toBe(2)
    // 子会话不跟着切（用户口径：子会话默认自己最新的一次）
    expect(a.children[0].convert!.chapterAttempt).toBe(3)
    // 别的章节完全不受影响
    expect(roots.find((n) => n.id === 'chapter_043')!.convert!.chapterAttempt).toBe(2)
  })

  it('同一节点的核对会话只跟同一轮', () => {
    const withSame = [conv(1), conv(2), conv(3), chk(1), chk(2), chk(3)]
    expect(buildChapterTree(withSame, conv(2).id)[0].checker!.chapterAttempt).toBe(2)
  })

  // T186：那一轮还没核对就**不留别的轮次**——旧行为退回最新的 #3，摆在一起会
  // 让人以为"这次已经核对过了"（真实打回理由也就跟着错位）。
  it('那一轮没有核对结论就不显示核对会话', () => {
    const noSame = [conv(1), conv(2), conv(3), chk(1), chk(3)]
    expect(buildChapterTree(noSame, conv(2).id)[0].checker).toBe(null)
  })

  it('默认视图里过期的核对结论也不挂在新的转换旁边', () => {
    const stale = [conv(1), conv(2), conv(3), chk(1), chk(2)]
    expect(buildChapterTree(stale, '')[0].checker).toBe(null)
    // 手工重新核对（核对号更大）仍然显示
    const rechecked = [conv(1), conv(2), chk(1), chk(3)]
    expect(buildChapterTree(rechecked, '')[0].checker!.chapterAttempt).toBe(3)
  })

  it('看核对会话 #1：核对钉住 #1，同一行的转换切到它核对的那一轮 #1', () => {
    const sessions = [conv(1), conv(2), conv(3), chk(1), chk(2), chk(3)]
    const roots = buildChapterTree(sessions, chk(1).id)
    expect(roots[0].checker!.chapterAttempt).toBe(1)
    expect(roots[0].convert!.chapterAttempt).toBe(1)
  })

  // T193：手工重新核对的尝试号比转换大，章节行**仍然是转换会话**——一次重核
  // 不该让"这一章是谁转的"从树上消失（曾经整行变成「核对 · chapter_002」，
  // 转换会话只能去历史里翻）。
  it('重核（核对号更大）不把转换会话顶掉', () => {
    const rechecked = [conv(1), chk(1), chk(2)]
    for (const pin of ['', chk(2).id]) {
      const node = buildChapterTree(rechecked, pin)[0]
      expect(node.convert!.chapterAttempt).toBe(1)
      expect(node.checker!.chapterAttempt).toBe(2)
    }
  })

  it('没有 pin 时每个节点都取最新', () => {
    const sessions = [conv(1), conv(2), child(1), child(2), child(3)]
    const roots = buildChapterTree(sessions, '')
    expect(roots[0].convert!.chapterAttempt).toBe(2)
    expect(roots[0].children[0].convert!.chapterAttempt).toBe(3)
  })
})

// T176：章节会话的标题是流水线标识符（chapter_004_child_01），按纯文本显示，
// 否则行内 markdown 会把下划线当强调吃掉（chapter004child01）。
describe('章节标题按纯文本', () => {
  it('convert / checker 会话走纯文本，其它会话仍可走 markdown', () => {
    expect(plainTitle({ chapterId: 'chapter_004_child_01', chapterRole: 'convert' })).toBe(true)
    expect(plainTitle({ chapterId: 'chapter_004', chapterRole: 'checker' })).toBe(true)
    expect(plainTitle({ title: '**粗体** 与 $x_1$' })).toBe(false)
    expect(plainTitle(null)).toBe(false)
  })
})

// T178：同一个 chapter_activity 在转换会话与核对会话里意思相反，文案不能共用。
// T181：核对已经有结论时，转换行不能还挂着"待核对"。
describe('章节状态文案', () => {
  it('转换跑完是"待核对"，核对跑完才是"待处理"', () => {
    expect(chapterActivityLabel('checking_result', 'convert')).toBe('转换完成待核对')
    expect(chapterActivityLabel('checking_result', 'checker')).toBe('核对完成待处理')
  })

  it('核对说了发现问题，转换行改说"核对发现问题"', () => {
    expect(chapterActivityLabel('checking_result', 'convert', 'issues')).toBe('核对发现问题')
    expect(chapterActivityLabel('checking_result', 'convert', 'pass')).toBe('已通过')
    // 核对还在跑（还没结论）时，转换行老老实实说在等核对
    expect(chapterActivityLabel('checking_result', 'convert', 'running')).toBe('转换完成待核对')
    expect(chapterActivityLabel('checking_result', 'convert', 'queued')).toBe('转换完成待核对')
  })

  it('转换又在排队、且这一章已有核对结论 → 等的是重转换的并发槽', () => {
    expect(chapterActivityLabel('queued', 'convert', 'issues')).toBe('核对完成待并发槽')
    expect(chapterActivityLabel('queued', 'convert', 'pass')).toBe('核对完成待并发槽')
    expect(chapterActivityLabel('queued', 'convert', '')).toBe('等待并发槽')
    expect(chapterActivityLabel('queued', 'checker', 'issues')).toBe('等待并发槽')
  })

  it('其余状态照旧，空状态只在 live 时说运行中', () => {
    expect(chapterActivityLabel('pass')).toBe('已通过')
    expect(chapterActivityLabel('issues', 'checker')).toBe('发现问题')
    expect(chapterActivityLabel('waiting_children')).toBe('等待子会话')
    expect(chapterActivityLabel('interrupted')).toBe('已中断')
    // T231：核对没给结论时写 no_verdict，两个角色都读作"核对无结论"——
    // 转换会话其实交付成功，不能再显示"会话错误"。
    expect(chapterActivityLabel('no_verdict', 'convert')).toBe('核对无结论')
    expect(chapterActivityLabel('no_verdict', 'checker')).toBe('核对无结论')
    expect(chapterActivityLabel(undefined, 'convert', '', false)).toBe('')
    expect(chapterActivityLabel(undefined, 'convert', '', true)).toBe('运行中')
  })
})

// P53：/api/index 的 retries → state.retries，章节行据此显示「待重试」。
describe('重试队列的状态', () => {
  it('按项目区分，消费掉就消失', () => {
    applyIndex({ sessions: [], retries: { 书A: ['chapter_002'] } })
    expect(chapterPendingRetry('书A', 'chapter_002')).toBe(true)
    expect(chapterPendingRetry('书A', 'chapter_003')).toBe(false)
    expect(chapterPendingRetry('书B', 'chapter_002')).toBe(false)

    applyIndex({ sessions: [] })
    expect(chapterPendingRetry('书A', 'chapter_002')).toBe(false)
  })

  // 章节行有两个角色：排的是哪种重试，「待重试」就挂哪一行。
  it('按角色区分', () => {
    applyIndex({ sessions: [], retries: { 书A: ['chapter_002'] }, retryRoles: { 书A: { chapter_002: 'checker' } } })
    expect(chapterPendingRetry('书A', 'chapter_002')).toBe(true)
    expect(chapterPendingRetry('书A', 'chapter_002', 'checker')).toBe(true)
    expect(chapterPendingRetry('书A', 'chapter_002', 'convert')).toBe(false)

    applyIndex({ sessions: [], retries: { 书A: ['chapter_002'] } })
    // 没给角色时按 convert 算（后端的默认值）。
    expect(chapterPendingRetry('书A', 'chapter_002', 'convert')).toBe(true)
    expect(chapterPendingRetry('书A', 'chapter_002', 'checker')).toBe(false)

    applyIndex({ sessions: [], retries: { 书A: ['chapter_002'] }, retryRoles: { 书A: { chapter_002: 'continue' } } })
    expect(chapterPendingRetry('书A', 'chapter_002', 'continue')).toBe(true)
    expect(chapterPendingRetry('书A', 'chapter_002', 'convert')).toBe(false)
  })
})

/* P58 观察（2026-10-22）：根章重来一次后，它的子章节尝试号又从 1 数起，
 * 归档里上一代的 `convert_<子章>.try2` 号比工作树里这一次的 #1 还大。
 * 只比号会把上一代的「已通过」当成这一章的现状：树上显示通过、实际还在排队。
 */
describe('归档世代不抢工作树的行（P58）', () => {
  const mkA = (id: string, att: number, archived: boolean, role = 'convert') => ({
    id, chapterId: 'chapter_004_child_03', chapterRole: role, chapterAttempt: att,
    archived, mtime: '2026-10-22T11:53:00Z', chapterActivity: '', live: false,
  })

  it('工作树里的 #1 优先于归档里的 #2', () => {
    const live = mkA('a/work/temp/conv_chapter_004/work/_subdivision/child_03/node/sessions/convert_chapter_004_child_03.jsonl', 1, false)
    const old = mkA('a/work/attempts/chapter_004/try1/work/_subdivision/child_03/node/sessions/convert_chapter_004_child_03.try2.jsonl', 2, true)
    const nodes = buildChapterTree([live, old])
    const node = nodes.find((n: any) => n.id === 'chapter_004_child_03')!
    expect(node.convert!.id).toBe(live.id)
  })

  it('上一代的核对结论不会挂到这一代的转换上', () => {
    const live = mkA('a/live/convert_chapter_004_child_03.jsonl', 1, false)
    const oldConv = mkA('a/work/attempts/chapter_004/try1/convert_chapter_004_child_03.try2.jsonl', 2, true)
    const oldChk = mkA('a/work/attempts/chapter_004/try1/checker_chapter_004_child_03.try2.jsonl', 2, true, 'checker')
    const node = buildChapterTree([live, oldConv, oldChk]).find((n: any) => n.id === 'chapter_004_child_03')!
    expect(node.convert!.id).toBe(live.id)
    expect(node.checker).toBeNull()
  })

  it('历史标签：核对行带「核对」前缀，归档仍是「旧」', () => {
    expect(chainMemberTag(mkA('a/checker_chapter_004_child_03.jsonl', 2, false, 'checker'))).toBe('核对#2')
    expect(chainMemberTag(mkA('a/checker_chapter_004_child_03.jsonl', 1, true, 'checker'))).toBe('旧核对#1')
    expect(chainMemberTag(mkA('a/convert_chapter_004_child_03.jsonl', 1, false))).toBe('#1')
  })
})

/* P59：转换是唯一锚点。核对会话在转录里记它核对的那份转换（chapterAnchor），
 * 树与历史都按这个锚点配对，不再拿两套各自编号的 .tryN 互相猜——手动重新核对
 * 只让核对号加一，核对的仍是同一份产物。 */
describe('P59 核对按锚点挂在转换下面', () => {
  const conv = (attempt = 1) => ({
    id: liveId('convert', 'chapter_042', attempt),
    project: 'book', chapterId: 'chapter_042', chapterRole: 'convert',
    chapterAttempt: attempt, mtime: '2026-10-22T10:00:00Z',
  })
  const chk = (attempt: number, anchor: string, mt: string) => ({
    id: liveId('checker', 'chapter_042', attempt),
    project: 'book', chapterId: 'chapter_042', chapterRole: 'checker',
    chapterAttempt: attempt, chapterAnchor: anchor, chapterAnchorAttempt: 1, mtime: mt,
  })

  it('同一个转换的两次核对：显示锚定它的最新那条', () => {
    const c = conv()
    const older = chk(1, 'convert_chapter_042', '2026-10-22T10:30:00Z')
    const newer = chk(2, 'convert_chapter_042', '2026-10-22T12:00:00Z')
    const node = buildChapterTree([c, older, newer])[0]
    expect(node.convert!.id).toBe(c.id)
    expect(node.checker!.id).toBe(newer.id)
  })

  it('钉住较老的核对（同锚点）时转换不变', () => {
    const c = conv()
    const older = chk(1, 'convert_chapter_042', '2026-10-22T10:30:00Z')
    const newer = chk(2, 'convert_chapter_042', '2026-10-22T12:00:00Z')
    const node = buildChapterTree([c, older, newer], older.id)[0]
    expect(node.checker!.id).toBe(older.id)
    // 转换由锚点决定，不是"最新那条"也不是按号。
    expect(node.convert!.id).toBe(c.id)
  })

  it('钉住锚定旧转换的核对时，转换跟着锚点切过去', () => {
    const c1 = conv(1)
    const c2 = conv(2)
    const k1 = chk(1, 'convert_chapter_042', '2026-10-22T10:10:00Z')
    const k2 = chk(2, 'convert_chapter_042.try2', '2026-10-22T12:10:00Z')
    expect(buildChapterTree([c1, c2, k1, k2], k1.id)[0].convert!.id).toBe(c1.id)
    expect(buildChapterTree([c1, c2, k1, k2], k2.id)[0].convert!.id).toBe(c2.id)
  })

  it('核对锚定的是另一次转换 → 不挂上来', () => {
    const c = conv()
    const other = chk(1, 'convert_chapter_042.try2', '2026-10-22T11:00:00Z')
    expect(buildChapterTree([c, other])[0].checker).toBeNull()
  })

  it('历史：转换会话只列转换，核对只列同一锚点下的核对', () => {
    const c1 = { ...conv(1), mtime: '2026-10-22T09:00:00Z' }
    const c2 = conv(2)
    const k1 = chk(1, 'convert_chapter_042', '2026-10-22T10:10:00Z')
    const k2 = chk(2, 'convert_chapter_042.try2', '2026-10-22T12:10:00Z')
    expect(chapterAttemptChain([c1, c2, k1, k2], c2.id)!.map((s: any) => s.id)).toEqual([c2.id, c1.id])
    // k2 只核对过 c2，所以它的历史里不该出现 k1（k1 核对的另一次转换）。
    expect(chapterAttemptChain([c1, c2, k1, k2], k2.id)).toBeNull()
  })

  it('旧转录没有锚点：树上退回按号配对，历史里只列同类', () => {
    const c = conv()
    const legacy = chk(1, '', '2026-10-22T10:30:00Z')
    const legacy2 = chk(2, '', '2026-10-22T11:30:00Z')
    const node = buildChapterTree([c, legacy])[0]
    expect(node.checker!.id).toBe(legacy.id)
    expect(chapterAttemptChain([c, legacy, legacy2], legacy.id)!.map((s: any) => s.id)).toEqual([legacy2.id, legacy.id])
  })
})

/* T214：切到某一次转换时，下面子章节的行也要回到**那一代**。根章重来过一次之
 * 后，工作树与 work/attempts/<根章>/tryN 里各有一套子章节转录；子行只按"各自
 * 最新一次"取，钉住上一代的父行时下面挂的仍是这一代的子行（上一代的子行号更大，
 * 纯按号取甚至会反过来）。世代只说工作树 / 归档，与尝试号无关。
 */
describe('T214 子行跟着父行的世代', () => {
  const conv = (chapterId: string, archived: boolean, attempt = 1, parent = '') => ({
    id: archived ? archivedId('convert', chapterId, attempt) : liveId('convert', chapterId, attempt),
    project: 'book', chapterId, chapterParent: parent, chapterRole: 'convert',
    chapterAttempt: attempt, archived, mtime: '2026-10-23T10:00:00Z',
  })
  const liveParent = conv('chapter_042', false)
  const oldParent = conv('chapter_042', true)
  const liveChild = conv('chapter_042_child_01', false, 1, 'chapter_042')
  // 上一代这一支转过两次，号比工作树里的 #1 还大。
  const oldChild = conv('chapter_042_child_01', true, 2, 'chapter_042')
  const sessions = [liveParent, oldParent, liveChild, oldChild]

  it('不钉任何行：父行取工作树，子行也取工作树里的那一条', () => {
    const root = buildChapterTree(sessions)[0]
    expect(root.convert!.id).toBe(liveParent.id)
    expect(root.children[0].convert!.id).toBe(liveChild.id)
  })

  it('钉住工作树里的转换：子行留在工作树', () => {
    const root = buildChapterTree(sessions, liveParent.id)[0]
    expect(root.convert!.id).toBe(liveParent.id)
    expect(root.children[0].convert!.id).toBe(liveChild.id)
  })

  it('钉住归档里的转换：子行回到归档那一代', () => {
    const root = buildChapterTree(sessions, oldParent.id)[0]
    expect(root.convert!.id).toBe(oldParent.id)
    expect(root.children[0].convert!.id).toBe(oldChild.id)
  })

  it('归档那一代的核对挂在归档的子行下，切回工作树就不显示', () => {
    const oldChk = {
      ...oldChild,
      id: oldChild.id.replace('/convert_', '/checker_').replace('.try2.jsonl', '.jsonl'),
      chapterRole: 'checker',
      chapterAnchor: 'convert_chapter_042_child_01.try2',
      mtime: '2026-10-23T10:30:00Z',
    }
    const withCheck = [...sessions, oldChk]
    expect(buildChapterTree(withCheck, oldParent.id)[0].children[0].checker!.id).toBe(oldChk.id)
    expect(buildChapterTree(withCheck, liveParent.id)[0].children[0].checker).toBeNull()
  })
})

/* T235：父行在同一条**工作树**转录上有两次尝试（第 1 次被作废、第 2 次又从头来）
 * 时，下面的子行也得跟着"你看的那一次"走：作废那一次的子会话已经被搬进
 * `work/attempts/<根章>/try1/`，切到它就不能还挂着现在这一代的子行。
 *
 * 父行两次都是活体转录（作废只搬工作树、转录留在原处），所以光看工作树/归档的
 * 布尔分不开——世代要读到"哪一次尝试的归档"。
 */
describe('T235 子行跟着父行选中的那次尝试', () => {
  const root = 'chapter_050'
  const mk = (role: string, chapterId: string, attempt: number, archived: boolean, over: Record<string, unknown> = {}) => {
    const id = archived ? archivedId(role, chapterId, attempt, root) : liveId(role, chapterId, attempt)
    return {
      id, path: '/srv/proj/' + id, project: 'book',
      chapterId, chapterRole: role, chapterAttempt: attempt, archived,
      chapterParent: chapterId === root ? '' : root,
      chapterActivity: 'issues', mtime: '2026-10-25T14:20:00Z', live: !archived,
      ...over,
    }
  }
  // 父章：第 1 次被作废（转录仍在工作树），第 2 次正在等子会话。
  const att1 = mk('convert', root, 1, false, { chapterActivity: 'issues' })
  const att2 = mk('convert', root, 2, false, { chapterActivity: 'waiting_children', mtime: '2026-10-25T21:36:00Z' })
  // 子章：作废那一次的子会话在 try1 归档里；现在这一代在工作树里。
  const oldChild = mk('convert', root + '_child_01', 1, true, { mtime: '2026-10-25T14:19:00Z' })
  const liveChild = mk('convert', root + '_child_01', 1, false, { mtime: '2026-10-25T21:35:00Z' })
  // 同一份锚点、两个世代各一条核对（转录名一样，只有路径里的世代不同）。
  const oldChk = mk('checker', root + '_child_01', 1, true, {
    chapterAnchor: 'convert_' + root + '_child_01', chapterActivity: 'issues', mtime: '2026-10-25T14:19:30Z',
  })
  const liveChk = mk('checker', root + '_child_01', 1, false, {
    chapterAnchor: 'convert_' + root + '_child_01', chapterActivity: 'pass', mtime: '2026-10-25T21:35:30Z',
  })
  // 另一个子章只在作废那一代里转过：现在这一代没有它，退回最新一次（T214 老规矩）。
  const onlyOld = mk('convert', root + '_child_02', 1, true)
  const all = [att1, att2, oldChild, liveChild, oldChk, liveChk, onlyOld]

  it('不钉任何行：父行取最新一次，子行也是工作树这一代', () => {
    const node = buildChapterTree(all)[0]
    expect(node.convert!.id).toBe(att2.id)
    expect(node.children[0].convert!.id).toBe(liveChild.id)
    expect(node.children[0].checker!.id).toBe(liveChk.id)
  })

  it('钉住作废的第 1 次：子行回到 work/attempts/<根章>/try1', () => {
    const node = buildChapterTree(all, att1.id)[0]
    expect(node.convert!.id).toBe(att1.id)
    expect(node.children[0].convert!.id).toBe(oldChild.id)
    expect(node.children[0].checker!.id).toBe(oldChk.id)
  })

  it('钉住正在跑的第 2 次：子行回到工作树', () => {
    const node = buildChapterTree(all, att2.id)[0]
    expect(node.convert!.id).toBe(att2.id)
    expect(node.children[0].convert!.id).toBe(liveChild.id)
    expect(node.children[0].checker!.id).toBe(liveChk.id)
  })

  it('这一代没转过的子章不留空行，退回最新一次', () => {
    const node = buildChapterTree(all, att2.id)[0]
    expect(node.children[1].convert!.id).toBe(onlyOld.id)
  })
})
