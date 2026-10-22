/*
 * P34 递归章节树的展开语义：展开状态是每个节点自己的记忆，与"正在看哪个会话"
 * 解耦——修复"点一下别处、刚拉开的树全收回去"与"一次点击既选中又展开"。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state, STORE_PREFIX } from '../src/state'
import { buildChapterTree, chapterActivityLabel, chapterAttemptChain, chapterAttemptTag, chainMemberTag, dedupeChapterAttempts, plainTitle } from '../src/legacy/sidebar'
import { chapterNodeKey, chapterWantOpen, setCollapsed, hasCollapseMemory } from '../src/legacy/sidebar'
import { applyIndex, chapterPendingRetry } from '../src/data'

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
// （中栏「历史 N」页签），并用紧凑的 `#N` 标注尝试号（T173：不再写中文「第 N 次」）。
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

  it('转换与核对混在一条链里（T194：点核对会话不会让列表只剩 #1）', () => {
    const conv1 = mk('a/convert_chapter_042.jsonl', 1, '2026-10-19T10:00:00Z')
    const conv2 = mk('a/convert_chapter_042.try2.jsonl', 2, '2026-10-19T12:00:00Z')
    const chk1 = { ...mk('a/checker_chapter_042.jsonl', 1, '2026-10-19T10:30:00Z'), chapterRole: 'checker' }
    const sessions = [conv1, conv2, chk1]
    // 从任一条进去看到的是同一份列表：两个角色都在，列表不随当前会话的角色变。
    expect(chapterAttemptChain(sessions, conv2.id)!.length).toBe(3)
    const fromChecker = chapterAttemptChain(sessions, chk1.id)!
    expect(fromChecker.map((x: any) => x.id).sort()).toEqual([conv1.id, conv2.id, chk1.id].sort())
    expect(fromChecker.map((x: any) => x.chapterRole).sort()).toEqual(['checker', 'convert', 'convert'])
  })

  it('只尝试过一次 / 非章节会话不出链', () => {
    const sessions = [mk('a/convert_chapter_042.jsonl', 1, '2026-10-19T10:00:00Z')]
    expect(chapterAttemptChain(sessions, 'a/convert_chapter_042.jsonl')).toBeNull()
    expect(chapterAttemptChain([{ id: 'x/style_session.jsonl', project: 'book' }], 'x/style_session.jsonl')).toBeNull()
  })

  it('标签：只用紧凑的 #N（第 1 次也为空/不出中文）', () => {
    expect(chapterAttemptTag({ chapterAttempt: 1 })).toBe('')
    expect(chapterAttemptTag({ chapterAttempt: 3 })).toBe('#3')
    expect(chainMemberTag(mk('a/convert_chapter_042.try2.jsonl', 2, '2026-10-19T12:00:00Z'))).toBe('#2')
    // 历史列表里第 1 次也要有标记，否则同一章几条历史行看起来一样
    expect(chainMemberTag(mk('a/convert_chapter_042.jsonl', 1, '2026-10-19T10:00:00Z'))).toBe('#1')
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

// T175：只钉住"正在看的那一条"——各节点的重试次数互相独立，不能拿一个章节的
// #2 去套全树；点开历史里的 #1，树里只把那一行换成 #1，子会话与其它章节仍按
// 各自最新一次显示。
describe('章节树只钉住正在查看的那一条', () => {
  const conv = (attempt: number, chapterId = 'chapter_042', parent = '') => ({
    id: `book/work/sessions/convert_${chapterId}` + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId, chapterParent: parent, chapterRole: 'convert',
    chapterAttempt: attempt, mtime: `2026-10-19T1${attempt}:00:00Z`,
  })
  const chk = (attempt: number, chapterId = 'chapter_042') => ({
    id: `book/work/sessions/checker_${chapterId}` + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
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

// T194：工作树与 work/attempts 里都会有「同一章 + 同一角色 + 同号」的转录，
// 但两种情形要分开：内容指纹相同（同一份转录被扫了两遍）只留一份；内容不同
// （同一个子章节在两次父尝试里各转过一次，尝试号都从 1 重新数）两条都留，
// 归档那份标「旧#N」——不能让历史把一次真实发生过的对话藏掉。
describe('同号转录：真副本去重，不同对话都留着', () => {
  const conv = (attempt: number, archived = false, sha = 'aaa', mt = '2026-10-19T11:00:00Z') => ({
    id: (archived ? 'book/work/attempts/chapter_007/try1/work/sessions/' : 'book/work/sessions/') +
      `convert_chapter_007` + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: 'chapter_007', chapterRole: 'convert',
    chapterAttempt: attempt, archived, sha, mtime: mt,
  })

  it('内容相同的归档副本被工作树那份顶掉', () => {
    const live = conv(1, false, 'same')
    const arch = conv(1, true, 'same', '2026-10-19T12:00:00Z')
    const out = dedupeChapterAttempts([arch, live])
    expect(out.length).toBe(1)
    expect(out[0].id).toBe(live.id)
    expect(dedupeChapterAttempts([live, arch])[0].id).toBe(live.id)
  })

  it('内容不同的同号转录两条都留（一次真实对话不能被藏掉）', () => {
    const live = conv(1, false, 'new-conv')
    const arch = conv(1, true, 'old-conv')
    const out = dedupeChapterAttempts([live, arch])
    expect(out.length).toBe(2)
    expect(out[1].archived).toBe(true)
    // 标签要分得开：这一次是 #1，早先那次是 旧#1。
    expect(chainMemberTag(out[0])).toBe('#1')
    expect(chainMemberTag(out[1])).toBe('旧#1')
  })

  it('不同尝试号 / 不同角色 / 非章节会话都不受影响', () => {
    const a = conv(1)
    const b = conv(2, true)
    const chk = { ...conv(1), id: 'book/work/sessions/checker_chapter_007.jsonl', chapterRole: 'checker' }
    const other = { id: 'book/work/sessions/style_pilot_trial.jsonl', project: 'book' }
    expect(dedupeChapterAttempts([a, b, chk, other]).length).toBe(4)
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
    id: 'book/work/sessions/convert_chapter_042' + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: 'chapter_042', chapterRole: 'convert',
    chapterAttempt: attempt, mtime: '2026-10-22T10:00:00Z',
  })
  const chk = (attempt: number, anchor: string, mt: string) => ({
    id: 'book/work/sessions/checker_chapter_042' + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
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

  it('历史：每个转换下面跟着它锚定的核对（新 → 旧）', () => {
    const c1 = conv(1)
    const c2 = conv(2)
    const k1 = chk(1, 'convert_chapter_042', '2026-10-22T10:10:00Z')
    const k2 = chk(2, 'convert_chapter_042.try2', '2026-10-22T12:10:00Z')
    const chain = chapterAttemptChain([c1, c2, k1, k2], k2.id)!
    expect(chain.map((s: any) => s.id)).toEqual([c2.id, k2.id, c1.id, k1.id])
  })

  it('旧转录没有锚点：仍按尝试号归组', () => {
    const c = conv()
    const legacy = chk(1, '', '2026-10-22T10:30:00Z')
    const node = buildChapterTree([c, legacy])[0]
    expect(node.checker!.id).toBe(legacy.id)
    expect(chapterAttemptChain([c, legacy], legacy.id)!.map((s: any) => s.id)).toEqual([c.id, legacy.id])
  })
})
