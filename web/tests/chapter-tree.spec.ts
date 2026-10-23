/*
 * P34 递归章节树的展开语义：展开状态是每个节点自己的记忆，与"正在看哪个会话"
 * 解耦——修复"点一下别处、刚拉开的树全收回去"与"一次点击既选中又展开"。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state, STORE_PREFIX } from '../src/state'
import { buildChapterTree, chapterActivityLabel, chapterAttemptChain, chapterAttemptTag, chapterIdSuffix, chapterNodeHasRows, chainMemberTag, dedupeChapterAttempts, fixOutcomeLabel, fixRoundAt, fixRowLabel, fixRowTitle, fixTargetId, fixTranscriptTag, plainTitle, rescueAttemptOf, rescueRowLabel, rescueRowTitle, sessionTip } from '../src/legacy/sidebar'
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

  it('历史按角色分开：转换会话下面只有转换，核对会话下面只有它核对的那次转换的核对', () => {
    const conv1 = mk('a/convert_chapter_042.jsonl', 1, '2026-10-19T10:00:00Z')
    const conv2 = mk('a/convert_chapter_042.try2.jsonl', 2, '2026-10-19T12:00:00Z')
    const chk1 = { ...mk('a/checker_chapter_042.jsonl', 1, '2026-10-19T10:30:00Z'), chapterRole: 'checker', chapterAnchor: 'convert_chapter_042' }
    const chk2 = { ...mk('a/checker_chapter_042.try2.jsonl', 2, '2026-10-19T11:30:00Z'), chapterRole: 'checker', chapterAnchor: 'convert_chapter_042' }
    const sessions = [conv1, conv2, chk1, chk2]
    // 转换会话的历史 = 各次转换（新 → 旧），核对不混进来。
    expect(chapterAttemptChain(sessions, conv2.id)!.map((x: any) => x.id)).toEqual([conv2.id, conv1.id])
    // 核对会话的历史 = 挂在同一个转换名下的各次核对（新 → 旧），转换不混进来。
    expect(chapterAttemptChain(sessions, chk1.id)!.map((x: any) => x.id)).toEqual([chk2.id, chk1.id])
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
    id: (archived ? 'book/work/attempts/chapter_042/try1/work/sessions/' : 'book/work/sessions/') +
      'convert_' + chapterId + (attempt > 1 ? '.try' + attempt : '') + '.jsonl',
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

/* P60：修复会话（`fix_<章>.jsonl`）没有章节身份——文件名不是 convert_/checker_
 * 前缀，扫描器给出的 `chapterId`/`chapterAttempt` 都是空的，唯一能挂靠的是
 * `chapterAnchor`（它修的那次转换，与核对同一个口径，P59）。树上它们只是核对行
 * 下面的子行：**绝不参与这一行的转换/核对挑选**。
 */
describe('P60 修复会话作为子行', () => {
  const conv = (attempt = 1, mt = '2026-10-23T10:00:00Z', archived = false) => ({
    id: (archived ? 'book/work/attempts/chapter_042/try1/work/sessions/' : 'book/work/sessions/') +
      'convert_chapter_042' + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: 'chapter_042', chapterRole: 'convert',
    chapterAttempt: attempt, archived, mtime: mt,
  })
  const fix = (name: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: 'book/work/temp/conv_chapter_042/work/_subdivision/child_01/node/sessions/' + name,
    project: 'book', chapterRole: 'fix', chapterAnchor: anchor, mtime: mt, ...over,
  })

  it('锚点相同的两轮修复都列在行下（旧 → 新），行头仍是转换会话', () => {
    const c = conv()
    const first = fix('fix_chapter_042.jsonl', 'convert_chapter_042', '2026-10-23T11:00:00Z')
    const second = fix('fix_chapter_042.try2.jsonl', 'convert_chapter_042', '2026-10-23T11:30:00Z')
    // 入参顺序反着给，输出仍按时间旧 → 新。
    const node = buildChapterTree([second, c, first])[0]
    expect(node.convert!.id).toBe(c.id)
    expect(node.checker).toBeNull()
    expect(node.fixes.map((f: any) => f.id)).toEqual([first.id, second.id])
    // 修复绝不变成这一行的转换（它就是子行）。
    expect(node.convert!.chapterRole).toBe('convert')
  })

  it('锚定另一次转换的修复不挂上来', () => {
    const c1 = conv(1)
    const c2 = conv(2, '2026-10-23T12:00:00Z')
    const other = fix('fix_chapter_042.jsonl', 'convert_chapter_042', '2026-10-23T11:00:00Z')
    const node = buildChapterTree([c1, c2, other])[0]
    expect(node.convert!.id).toBe(c2.id)
    expect(node.fixes).toEqual([])
  })

  it('钉住一条修复会话时，那一行就显示它锚定的那次转换', () => {
    const c1 = conv(1)
    const c2 = conv(2, '2026-10-23T12:00:00Z')
    const onOld = fix('fix_chapter_042.jsonl', 'convert_chapter_042', '2026-10-23T11:00:00Z')
    const defaultRow = buildChapterTree([c1, c2, onOld])[0]
    expect(defaultRow.convert!.id).toBe(c2.id)
    expect(defaultRow.fixes).toEqual([])
    const pinnedRow = buildChapterTree([c1, c2, onOld], onOld.id)[0]
    expect(pinnedRow.convert!.id).toBe(c1.id)
    expect(pinnedRow.fixes.map((f: any) => f.id)).toEqual([onOld.id])
  })

  it('归档那一代的修复不挂到工作树这一行上', () => {
    const live = conv(1, '2026-10-23T10:00:00Z')
    const archived = conv(1, '2026-10-22T10:00:00Z', true)
    const oldFix = fix('fix_chapter_042.jsonl', 'convert_chapter_042', '2026-10-22T11:00:00Z', { archived: true })
    expect(buildChapterTree([live, archived, oldFix])[0].fixes).toEqual([])
    expect(buildChapterTree([live, archived, oldFix], archived.id)[0].fixes.map((f: any) => f.id)).toEqual([oldFix.id])
  })

  it('没有修复时 fixes 是空数组（树上不多出任何行）', () => {
    expect(buildChapterTree([conv()])[0].fixes).toEqual([])
    expect(buildChapterTree([])).toEqual([])
  })

  it('只有修复会话时不凭空造出一个节点', () => {
    const lone = fix('fix_chapter_042.jsonl', 'convert_chapter_042', '2026-10-23T11:00:00Z')
    expect(buildChapterTree([lone])).toEqual([])
  })

  it('修复会话自己的历史 = 同一锚点的其他修复轮次（新 → 旧）', () => {
    const c1 = conv(1)
    const c2 = conv(2, '2026-10-23T12:00:00Z')
    const first = fix('fix_chapter_042.jsonl', 'convert_chapter_042.try2', '2026-10-23T13:00:00Z')
    const second = fix('fix_chapter_042.try2.jsonl', 'convert_chapter_042.try2', '2026-10-23T13:30:00Z')
    // 另一处（另一次的产物）的修复不能混进这条链。
    const elsewhere = fix('fix_chapter_042.jsonl', 'convert_chapter_042', '2026-10-23T13:10:00Z')
    const sessions = [c1, c2, first, second, elsewhere]
    expect(chapterAttemptChain(sessions, second.id)!.map((s: any) => s.id)).toEqual([second.id, first.id])
    // 只修过一次就不出「历史」页签。
    expect(chapterAttemptChain([c1, first], first.id)).toBeNull()
  })

  it('转换会话的历史仍只有转换（修复只在树上显示）', () => {
    const c1 = conv(1, '2026-10-23T09:00:00Z')
    const c2 = conv(2, '2026-10-23T12:00:00Z')
    const f = fix('fix_chapter_042.jsonl', 'convert_chapter_042.try2', '2026-10-23T13:00:00Z')
    expect(chapterAttemptChain([c1, c2, f], c2.id)!.map((s: any) => s.id)).toEqual([c2.id, c1.id])
  })

  it('文件名里的 .tryN 原样标出，标签不编尝试号', () => {
    expect(fixTranscriptTag(fix('fix_chapter_042.try2.jsonl', 'x', '2026-10-23T11:00:00Z'))).toBe('.try2')
    expect(fixTranscriptTag(fix('fix_chapter_042.jsonl', 'x', '2026-10-23T11:00:00Z'))).toBe('')
    // 修复转录没有 chapterAttempt，标签不能退回一个假的 #1。
    expect(chainMemberTag(fix('fix_chapter_042.try2.jsonl', 'x', '2026-10-23T11:00:00Z'))).toBe('修复.try2')
    expect(chainMemberTag(fix('fix_chapter_042.jsonl', 'x', '2026-10-23T11:00:00Z', { archived: true }))).toBe('旧修复')
  })
})

/* P60 第四步：扫描器给修复转录补上了章节身份（`fix_` 前缀与 convert_/checker_ 同一
 * 套规则，`chapterId`/`chapterAttempt` 都有值）。身份只用于**阶段归组与标题**；
 * 挂到哪一行**仍然只由锚点决定**——同一章的转换/核对挑选规则一行没动。
 */
describe('P60 修复会话带上章节身份后仍只看锚点', () => {
  const parentOf = (id: string) => (/_child_\d+$/.test(id) ? id.replace(/_child_\d+$/, '') : '')
  const conv = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z') => ({
    id: 'book/work/sessions/convert_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterParent: parentOf(id), chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt,
  })
  // 第四步之后扫描器给出的修复行：有锚点，**也有**章节身份。
  const fix = (name: string, childId: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: 'book/work/temp/conv_chapter_042/work/_subdivision/child_01/node/sessions/' + name,
    project: 'book', chapterRole: 'fix',
    chapterId: childId, chapterParent: parentOf(childId), chapterDepth: 1,
    chapterAttempt: 1, chapterAnchor: anchor, mtime: mt, ...over,
  })

  it('锚点仍决定挂靠：锚在父章节那次转换上就挂父行，子章节那一行不多出子行', () => {
    const parent = conv('chapter_042')
    const child = conv('chapter_042_child_01', 1, '2026-10-23T10:05:00Z')
    // 章节身份是 child_01，锚点却是父章节那次转换——按 id 挂会挂错行。
    const f = fix('fix_chapter_042_child_01.jsonl', 'chapter_042_child_01', 'convert_chapter_042', '2026-10-23T11:00:00Z')
    const tree = buildChapterTree([parent, child, f])
    expect(tree[0].fixes.map((x: any) => x.id)).toEqual([f.id])
    expect(tree[0].children[0].fixes).toEqual([])
  })

  it('带 chapterId 的修复也不会自己造出一行', () => {
    const f = fix('fix_chapter_042_child_01.jsonl', 'chapter_042_child_01', 'convert_chapter_042', '2026-10-23T11:00:00Z')
    expect(buildChapterTree([f])).toEqual([])
    // 同一个 id 已经有节点时也不重复建行：节点数仍由转换/核对决定。
    const parent = conv('chapter_042')
    expect(buildChapterTree([parent, f]).map((n: any) => n.id)).toEqual(['chapter_042'])
  })

  it('没有锚点的旧转录退回按章节 ID 挂（身份就是唯一线索）', () => {
    const child = conv('chapter_042_child_01')
    const noAnchor = fix('fix_chapter_042_child_01.jsonl', 'chapter_042_child_01', '', '2026-10-23T11:00:00Z')
    const tree = buildChapterTree([child, noAnchor])
    expect(tree[0].fixes.map((x: any) => x.id)).toEqual([noAnchor.id])
  })

  it('标签仍是「修复」+ 文件名里的 .tryN，不因为有了 chapterId 退回 #N', () => {
    const f = fix('fix_chapter_042_child_01.try2.jsonl', 'chapter_042_child_01', 'convert_chapter_042', '2026-10-23T11:00:00Z', { chapterAttempt: 2 })
    expect(chainMemberTag(f)).toBe('修复.try2')
    expect(chainMemberTag({ ...f, archived: true })).toBe('旧修复.try2')
  })

  it('转换会话的历史仍然不放修复（哪怕它也有 chapterId）', () => {
    const c1 = conv('chapter_042_child_01', 1, '2026-10-23T09:00:00Z')
    const c2 = conv('chapter_042_child_01', 2, '2026-10-23T12:00:00Z')
    const f = fix('fix_chapter_042_child_01.jsonl', 'chapter_042_child_01', 'convert_chapter_042_child_01.try2', '2026-10-23T13:00:00Z')
    expect(chapterAttemptChain([c1, c2, f], c2.id)!.map((s: any) => s.id)).toEqual([c2.id, c1.id])
    // 修复自己的历史仍是同一锚点的轮次。
    const f2 = fix('fix_chapter_042_child_01.try2.jsonl', 'chapter_042_child_01', 'convert_chapter_042_child_01.try2', '2026-10-23T13:30:00Z')
    expect(chapterAttemptChain([c1, c2, f, f2], f2.id)!.map((s: any) => s.id)).toEqual([f2.id, f.id])
  })
})

/* P59 补充：老核对转录（P58 之前落盘的）**没有锚点**。只按锚点全等过滤时，
 * 只要当前这条写下了锚点，那些老核对就一条都匹配不上——历史里看起来像"最新的
 * 一次核对把旧的盖掉了"。没有锚点的成员没得比，一律留在链里；锚在别处的核对
 * 仍然要排除。 */
describe('P59 老核对转录不被历史丢掉', () => {
  const conv = (attempt = 1, mt = '2026-10-22T09:00:00Z') => ({
    id: 'book/work/sessions/convert_chapter_042' + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: 'chapter_042', chapterRole: 'convert', chapterAttempt: attempt, mtime: mt,
  })
  const chk = (name: string, attempt: number, anchor: string, mt: string) => ({
    id: 'book/work/sessions/' + name,
    project: 'book', chapterId: 'chapter_042', chapterRole: 'checker', chapterAttempt: attempt,
    chapterAnchor: anchor, mtime: mt,
  })

  it('两次有锚点的核对 + 一条没有锚点的老核对：三条都在（新 → 旧）', () => {
    const c = conv()
    const first = chk('checker_chapter_042.jsonl', 1, 'convert_chapter_042', '2026-10-22T10:10:00Z')
    // 老转录：扫描不出锚点（chapterAnchor 为空）。
    const legacy = chk('checker_chapter_042.try2.jsonl', 2, '', '2026-10-22T10:40:00Z')
    const newest = chk('checker_chapter_042.try3.jsonl', 3, 'convert_chapter_042', '2026-10-22T12:10:00Z')
    const sessions = [c, first, legacy, newest]
    const chain = chapterAttemptChain(sessions, newest.id)
    expect(chain).not.toBeNull()
    expect(chain!.map((s: any) => s.id)).toEqual([newest.id, legacy.id, first.id])
    // 从老转录那边点开历史，同样三条都在（不是反向把有锚点的过滤掉）。
    expect(chapterAttemptChain(sessions, legacy.id)!.map((s: any) => s.id)).toEqual([newest.id, legacy.id, first.id])
  })

  it('锚在另一次转换上的核对仍然排除（别处的结论不混进来）', () => {
    const c1 = conv(1, '2026-10-22T09:00:00Z')
    const c2 = conv(2, '2026-10-22T11:00:00Z')
    const onFirst = chk('checker_chapter_042.jsonl', 1, 'convert_chapter_042', '2026-10-22T10:00:00Z')
    const legacy = chk('checker_chapter_042.try2.jsonl', 2, '', '2026-10-22T10:30:00Z')
    const onSecond = chk('checker_chapter_042.try3.jsonl', 3, 'convert_chapter_042.try2', '2026-10-22T11:30:00Z')
    const sessions = [c1, c2, onFirst, legacy, onSecond]
    expect(chapterAttemptChain(sessions, onSecond.id)!.map((s: any) => s.id)).toEqual([onSecond.id, legacy.id])
    expect(chapterAttemptChain(sessions, onFirst.id)!.map((s: any) => s.id)).toEqual([legacy.id, onFirst.id])
  })
})

/* P60 补充：修复行要说清"这是哪一章的第几轮"——只写「修复 N」时，同一行下面
 * 可能挂着不同子章节的修复，看不出归属。树上写 `修复 child_02 #1`（与核对行
 * 用章节身份的层级口径一致），历史里每行同样带章节后缀与轮次、表头给完整章节 ID。 */
describe('P60 修复行标明章节与轮次', () => {
  const parentOf = (id: string) => (/_child_\d+$/.test(id) ? id.replace(/_child_\d+$/, '') : '')
  const conv = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z') => ({
    id: 'book/work/sessions/convert_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterParent: parentOf(id), chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt,
  })
  const fix = (name: string, childId: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: 'book/work/temp/conv_chapter_042/work/_subdivision/child_02/node/sessions/' + name,
    project: 'book', chapterRole: 'fix',
    chapterId: childId, chapterParent: parentOf(childId), chapterDepth: 1,
    chapterAttempt: 1, chapterAnchor: anchor, mtime: mt, ...over,
  })

  it('章节 ID 在本层只留后缀；顶层照写自己的 ID', () => {
    expect(chapterIdSuffix('chapter_042_child_02')).toBe('child_02')
    // 嵌套子章节保留整段后缀，层级不会被抹成同一个名字。
    expect(chapterIdSuffix('chapter_042_child_01_child_02')).toBe('child_01_child_02')
    expect(chapterIdSuffix('chapter_042')).toBe('chapter_042')
    expect(chapterIdSuffix('')).toBe('')
  })

  it('标签 = 修复 <章节后缀> #<轮次>，归档前缀照旧', () => {
    const f = fix('fix_chapter_042_child_02.jsonl', 'chapter_042_child_02', 'convert_chapter_042_child_02', '2026-10-23T11:00:00Z')
    expect(fixRowLabel(f, 'chapter_042', 1)).toBe('修复 child_02 #1')
    // 老转录没有章节身份：退回它挂靠的那一行（节点）的章节 ID。
    const legacy = { ...f, chapterId: '' }
    expect(fixTargetId(legacy, 'chapter_042_child_02')).toBe('chapter_042_child_02')
    expect(fixRowLabel(legacy, 'chapter_042_child_02', 2)).toBe('修复 child_02 #2')
    expect(fixRowLabel({ ...f, archived: true }, 'chapter_042', 3)).toBe('旧修复 child_02 #3')
  })

  it('修复链的轮次：新 → 旧，队尾是最早一轮', () => {
    expect(fixRoundAt(0, 3)).toBe(3)
    expect(fixRoundAt(1, 3)).toBe(2)
    expect(fixRoundAt(2, 3)).toBe(1)
    expect(fixRoundAt(0, 1)).toBe(1)
  })

  it('子章节那一行下挂两轮修复：标签分别是 #1 / #2，且都点名子章节', () => {
    const parent = conv('chapter_042')
    const child = conv('chapter_042_child_02', 1, '2026-10-23T10:05:00Z')
    const first = fix('fix_chapter_042_child_02.jsonl', 'chapter_042_child_02', 'convert_chapter_042_child_02', '2026-10-23T11:00:00Z')
    const second = fix('fix_chapter_042_child_02.try2.jsonl', 'chapter_042_child_02', 'convert_chapter_042_child_02', '2026-10-23T11:30:00Z', { chapterAttempt: 2 })
    const node = buildChapterTree([parent, child, first, second])[0].children[0]
    expect(node.fixes.map((f: any) => f.id)).toEqual([first.id, second.id])
    // 与 ChapterSessionTree 的渲染同一口径：树上旧 → 新，轮次从 1 数。
    expect(node.fixes.map((f: any, i: number) => fixRowLabel(f, node.id, i + 1)))
      .toEqual(['修复 child_02 #1', '修复 child_02 #2'])
  })

  it('修复历史表头能拿到目标章节，行按新 → 旧标 #N', () => {
    const first = fix('fix_chapter_042_child_02.jsonl', 'chapter_042_child_02', 'convert_chapter_042_child_02', '2026-10-23T11:00:00Z')
    const second = fix('fix_chapter_042_child_02.try2.jsonl', 'chapter_042_child_02', 'convert_chapter_042_child_02', '2026-10-23T11:30:00Z')
    const chain = chapterAttemptChain([conv('chapter_042_child_02'), first, second], second.id)!
    expect(chain.map((s: any) => s.id)).toEqual([second.id, first.id])
    // 表头取同链里第一份能读到的章节身份（老转录没有 chapterId 时靠它兜底）。
    const target = chain.map((s: any) => fixTargetId(s)).find((id: string) => !!id)
    expect(target).toBe('chapter_042_child_02')
    // 历史是新 → 旧：最新一轮在最上面，轮次从队尾数起。
    expect(chain.map((s: any, i: number) => fixRoundAt(i, chain.length))).toEqual([2, 1])
  })
})

/* P60 第五步：修复行独立于核对行。
 *
 * 真实跑批里出现过「修复转录明明在磁盘上、章节树里却一条修复行都看不见」：
 * 渲染逻辑把修复行当成核对块的附属物，这一行没有核对（还没核对，或核对是上
 * 一次尝试、按 T187 不显示）时它跟着一起没。现在渲染条件只认 `node.fixes`
 * 自己，`chapterNodeHasRows` 也把「核对 / 修复 / 子章节」当三种互不依赖的来源。
 */
describe('P60 修复行独立于核对行', () => {
  const conv2 = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z', over: Record<string, unknown> = {}) => ({
    id: 'book/work/sessions/convert_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  const checker2 = (id: string, attempt: number, mt: string, over: Record<string, unknown> = {}) => ({
    id: 'book/work/sessions/checker_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterRole: 'checker',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  const fix2 = (id: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: 'book/work/temp/conv_' + id + '/work/_subdivision/child_01/node/sessions/fix_' + id + '.jsonl',
    project: 'book', chapterRole: 'fix', chapterAnchor: anchor, mtime: mt, ...over,
  })

  it('核对因尝试号不显示时，修复行仍然挂在这一行上', () => {
    // 转换是第 2 次（旧结论作废重来），核对还是第 1 次且没有锚点 → 按 T187
    // 不显示；修复锚定的是第 2 次转换，必须照旧显示。
    const c = conv2('chapter_042', 2, '2026-10-23T12:00:00Z')
    const oldCheck = checker2('chapter_042', 1, '2026-10-23T10:30:00Z')
    const fx = fix2('chapter_042', 'convert_chapter_042.try2', '2026-10-23T13:00:00Z')
    const node = buildChapterTree([c, oldCheck, fx])[0]
    expect(node.convert!.id).toBe(c.id)
    expect(node.checker).toBeNull()
    expect(node.fixes.map((f: any) => f.id)).toEqual([fx.id])
    // 渲染判据：有修复行 → 有子行可展开，与核对在不在无关。
    expect(chapterNodeHasRows(node)).toBe(true)
  })

  it('chapterNodeHasRows：三种子行来源互不依赖，都没有时不撑高一行', () => {
    expect(chapterNodeHasRows({ convert: {}, checker: {}, fixes: [], children: [] })).toBe(true)
    expect(chapterNodeHasRows({ convert: {}, checker: null, fixes: [{ id: 'f' }], children: [] })).toBe(true)
    expect(chapterNodeHasRows({ convert: {}, checker: null, fixes: [], children: [{ id: 'c' }] })).toBe(true)
    // 没有修复的节点与引入修复行之前完全一致：不多出展开箭头。
    expect(chapterNodeHasRows({ convert: {}, checker: null, fixes: [], children: [] })).toBe(false)
    expect(chapterNodeHasRows({ convert: null, checker: null, fixes: [], children: [] })).toBe(false)
  })

  it('修复行的结局写进 title：通过 / 未通过 / 失败 / 运行中', () => {
    const base = { id: 'f', label: '修复 · chapter_042', status: 'done' }
    expect(fixOutcomeLabel({ ...base, chapterActivity: 'pass' })).toBe('通过')
    expect(fixOutcomeLabel({ ...base, chapterActivity: 'issues' })).toBe('未通过')
    expect(fixOutcomeLabel({ ...base, chapterActivity: 'error' })).toBe('失败')
    // 还活着就只说"运行中"：上一次的结论不能冒充这一次。
    expect(fixOutcomeLabel({ ...base, chapterActivity: 'pass', live: true })).toBe('运行中')
    expect(fixOutcomeLabel({ ...base })).toBe('')
    expect(fixOutcomeLabel(null)).toBe('')
    // title = 原有 tip + 结局；没有结局时与之前一字不差。
    expect(fixRowTitle({ ...base })).toBe(sessionTip({ ...base }))
    expect(fixRowTitle({ ...base, chapterActivity: 'issues' })).toBe(sessionTip({ ...base }) + '\n修复结果：未通过')
  })
})


/* P61：复盘会话（`rescue_<章>[.tryN].jsonl`）带章节身份后落进章节树，与核对行
 * **同层**、像修复行那样是一条独立子行。它绝不能变成某一章的 convert/checker；
 * 它也不进「历史」尝试链（那是"这一章转过几次"，复盘是附注）。
 */
describe('P61 复盘行挂在章节下面', () => {
  const conv3 = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z', over: Record<string, unknown> = {}) => ({
    id: 'book/work/sessions/convert_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  const checker3 = (id: string, attempt: number, mt: string) => ({
    id: 'book/work/sessions/checker_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterRole: 'checker',
    chapterAttempt: attempt, mtime: mt,
  })
  const rescue3 = (id: string, attempt: number, mt: string, over: Record<string, unknown> = {}) => ({
    id: 'book/work/sessions/rescue_' + id + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl',
    project: 'book', chapterId: id, chapterRole: 'rescue',
    chapterAttempt: attempt, mtime: mt, ...over,
  })

  it('复原话挂到它复盘的那一章下面，旧 → 新，且不参选 convert/checker', () => {
    const c = conv3('chapter_042', 2, '2026-10-23T12:00:00Z')
    const k = checker3('chapter_042', 2, '2026-10-23T12:30:00Z')
    const r1 = rescue3('chapter_042', 1, '2026-10-23T11:00:00Z')
    const r2 = rescue3('chapter_042', 2, '2026-10-23T13:00:00Z')
    const node = buildChapterTree([c, k, r1, r2])[0]
    // 只建了一棵节点：复盘会话没有把自己变成节点，也没有抢走 convert/checker。
    expect(node.convert!.id).toBe(c.id)
    expect(node.checker!.id).toBe(k.id)
    expect(node.rescues.map((r: any) => r.id)).toEqual([r1.id, r2.id])
  })

  it('复盘行标签写「复盘 #N」，N 是它复盘的那次转换的尝试号，归档带「旧」', () => {
    expect(rescueAttemptOf(rescue3('chapter_042', 3, 'x'))).toBe(3)
    expect(rescueRowLabel(rescue3('chapter_042', 2, 'x'))).toBe('复盘 #2')
    expect(rescueRowLabel(rescue3('chapter_042', 1, 'x', { archived: true }))).toBe('旧复盘 #1')
    // title = 原有 tip + 复盘对象（转录名去掉 rescue_ 前缀）。
    const r = rescue3('chapter_042', 2, 'x')
    expect(rescueRowTitle(r)).toBe(sessionTip(r) + '\n复盘对象：chapter_042.try2')
  })

  it('只有复盘的节点也算有子行（展开箭头与在引入复盘之前一致）', () => {
    const c = conv3('chapter_042', 1, '2026-10-23T12:00:00Z')
    const node = buildChapterTree([c, rescue3('chapter_042', 1, '2026-10-23T13:00:00Z')])[0]
    expect(node.checker).toBeNull()
    expect(chapterNodeHasRows(node)).toBe(true)
    // 没有复盘的节点不多出展开箭头。
    expect(chapterNodeHasRows({ convert: {}, checker: null, fixes: [], rescues: [], children: [] })).toBe(false)
    // 只有复盘没别的：也要能展开。
    expect(chapterNodeHasRows({ convert: {}, checker: null, rescues: [{ id: 'r' }], children: [] })).toBe(true)
  })

  it('复盘会话不进「历史」尝试链，点它没有历史页签', () => {
    const c = conv3('chapter_042', 1, '2026-10-23T12:00:00Z')
    const k = checker3('chapter_042', 1, '2026-10-23T12:30:00Z')
    const r = rescue3('chapter_042', 1, '2026-10-23T13:00:00Z')
    const chain = chapterAttemptChain([c, k, r])
    const ids = (chain || []).map((m: any) => m.id)
    expect(ids).not.toContain(r.id)
    expect(chapterAttemptChain([c, k, r], r.id)).toBeNull()
  })

  it('钉住一条复盘会话时，那一行显示到它复盘的那次转换（不拿别的尝试凑）', () => {
    const c1 = conv3('chapter_042', 1, '2026-10-23T10:00:00Z')
    const c2 = conv3('chapter_042', 2, '2026-10-23T12:00:00Z')
    const r = rescue3('chapter_042', 1, '2026-10-23T13:00:00Z')
    const node = buildChapterTree([c1, c2, r], r.id)[0]
    expect(node.convert!.id).toBe(c1.id)
    expect(node.rescues.map((x: any) => x.id)).toEqual([r.id])
    // 复盘的那次转换已经不在了：仍然挂在这一行下面，只是不钉（默认最新）。
    const node2 = buildChapterTree([c2, rescue3('chapter_042', 9, 'x')], rescue3('chapter_042', 9, 'x').id)[0]
    expect(node2.convert!.id).toBe(c2.id)
    expect(node2.rescues.map((x: any) => x.id)).toEqual([rescue3('chapter_042', 9, 'x').id])
  })
})

/* P61：`rescuing` 是复盘在目标转录里写的中间状态——文案「复盘处理中」，
 * 既有状态一个都不能变。
 */
describe('P61 复盘处理中的文案', () => {
  it('rescuing → 复盘处理中', () => {
    expect(chapterActivityLabel('rescuing', 'convert')).toBe('复盘处理中')
    expect(chapterActivityLabel('rescuing', 'checker')).toBe('复盘处理中')
  })

  it('既有状态文案不变', () => {
    expect(chapterActivityLabel('queued', 'convert')).toBe('等待并发槽')
    expect(chapterActivityLabel('running', 'convert')).toBe('运行中')
    expect(chapterActivityLabel('error', 'convert')).toBe('会话错误')
    expect(chapterActivityLabel('interrupted', 'convert')).toBe('已中断')
    expect(chapterActivityLabel('pass', 'checker')).toBe('已通过')
  })
})
