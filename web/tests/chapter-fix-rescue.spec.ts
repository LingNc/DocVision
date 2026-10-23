/*
 * 修复行与复盘行：`fix_<章>.jsonl` 与 `rescue_<章>[.tryN].jsonl` 在章节树里怎么挂
 * （锚点 / 章节身份 / 归档世代）、标签与结局怎么写，以及 `rescuing` 这个中间状态
 * 的文案——既有状态一个都不能变。
 *
 * P62：从 chapter-tree.spec.ts 拆出来（行为一字未改）。树本身的规则在
 * chapter-tree.spec.ts，尝试链在 chapter-chain.spec.ts。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { buildChapterTree, chapterActivityLabel, chapterAttemptChain, chapterIdSuffix, chapterNodeHasRows, chainMemberTag, fixOutcomeLabel, fixRoundAt, fixRowLabel, fixRowTitle, fixTargetId, fixTranscriptTag, rescueOrdinal, rescueRowLabel, rescueRowTitle, reviewedTargetLabel, sessionTip } from '../src/legacy/sidebar'
import { archivedId, fixId, liveId, parentOf } from './helpers/chapter'

beforeEach(() => {
  state.collapsed = {}
})

/* P60：修复会话（`fix_<章>.jsonl`）没有章节身份——文件名不是 convert_/checker_
 * 前缀，扫描器给出的 `chapterId`/`chapterAttempt` 都是空的，唯一能挂靠的是
 * `chapterAnchor`（它修的那次转换，与核对同一个口径，P59）。树上它们只是核对行
 * 下面的子行：**绝不参与这一行的转换/核对挑选**。
 */
describe('P60 修复会话作为子行', () => {
  const conv = (attempt = 1, mt = '2026-10-23T10:00:00Z', archived = false) => ({
    id: archived ? archivedId('convert', 'chapter_042', attempt) : liveId('convert', 'chapter_042', attempt),
    project: 'book', chapterId: 'chapter_042', chapterRole: 'convert',
    chapterAttempt: attempt, archived, mtime: mt,
  })
  const fix = (name: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: fixId(name),
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

describe('P60 修复会话带上章节身份后仍只看锚点', () => {
  const conv = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z') => ({
    id: liveId('convert', id, attempt),
    project: 'book', chapterId: id, chapterParent: parentOf(id), chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt,
  })
  // 第四步之后扫描器给出的修复行：有锚点，**也有**章节身份。
  const fix = (name: string, childId: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: fixId(name),
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

/* P60 补充：修复行要说清"这是哪一章的第几轮"——只写「修复 N」时，同一行下面
 * 可能挂着不同子章节的修复，看不出归属。树上写 `修复 child_02 #1`（与核对行
 * 用章节身份的层级口径一致），历史里每行同样带章节后缀与轮次、表头给完整章节 ID。 */
describe('P60 修复行标明章节与轮次', () => {
  const conv = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z') => ({
    id: liveId('convert', id, attempt),
    project: 'book', chapterId: id, chapterParent: parentOf(id), chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt,
  })
  const fix = (name: string, childId: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: fixId(name, 'chapter_042', 'child_02'),
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
    id: liveId('convert', id, attempt),
    project: 'book', chapterId: id, chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  const checker2 = (id: string, attempt: number, mt: string, over: Record<string, unknown> = {}) => ({
    id: liveId('checker', id, attempt),
    project: 'book', chapterId: id, chapterRole: 'checker',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  const fix2 = (id: string, anchor: string, mt: string, over: Record<string, unknown> = {}) => ({
    id: fixId('fix_' + id + '.jsonl', id),
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

/* P61 + P58 补：复盘会话（`rescue_<章>[.tryN].jsonl`）带章节身份后落进章节树，
 * 与核对行**同层**、像修复行那样是一条独立子行。它绝不能变成某一章的
 * convert/checker，也不进**转换/核对**的尝试链。
 *
 * C：它自己有一条链——同一章、**同一目标**（meta 里的 `reviewed_transcript`）的
 * 复盘，新 → 旧；不同目标是两件事，行与号都分开。行号是"这条目标被复盘的第几次"，
 * 由界面按 mtime 读时推断（D：老转录不改名、不重写）。
 */
describe('P61 复盘行挂在章节下面', () => {
  const conv3 = (id: string, attempt = 1, mt = '2026-10-23T10:00:00Z', over: Record<string, unknown> = {}) => ({
    id: liveId('convert', id, attempt),
    project: 'book', chapterId: id, chapterRole: 'convert',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  const checker3 = (id: string, attempt: number, mt: string) => ({
    id: liveId('checker', id, attempt),
    project: 'book', chapterId: id, chapterRole: 'checker',
    chapterAttempt: attempt, mtime: mt,
  })
  const rescue3 = (id: string, attempt: number, mt: string, over: Record<string, unknown> = {}) => ({
    id: liveId('rescue', id, attempt),
    project: 'book', chapterId: id, chapterRole: 'rescue',
    chapterAttempt: attempt, mtime: mt, ...over,
  })
  // 复盘了同章的一次转换；`reviewed` 是被复盘的转录名（不含扩展名）。
  const rescueOf = (chapter: string, n: number, reviewed: string, mt: string, role = 'convert') =>
    rescue3(chapter, n, mt, { reviewedRole: role, reviewedTranscript: reviewed })

  it('复盘会话挂到它复盘的那一章下面，旧 → 新，且不参选 convert/checker', () => {
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

  it('复盘行标签写「复盘 #N」，N 由目标 + mtime 读时推断，归档带「旧」', () => {
    const sessions = [
      rescueOf('chapter_042', 1, 'convert_chapter_042', '2026-10-23T11:00:00Z'),
      rescueOf('chapter_042', 2, 'convert_chapter_042', '2026-10-23T13:00:00Z'),
    ]
    // 名字里就算写着 .try2，界面按目标重新数：第一次是 #1，第二次是 #2。
    expect(rescueRowLabel(sessions[0], rescueOrdinal(sessions, sessions[0]))).toBe('复盘 #1')
    expect(rescueRowLabel(sessions[1], rescueOrdinal(sessions, sessions[1]))).toBe('复盘 #2')
    expect(rescueRowLabel(sessions[1], 2)).toBe('复盘 #2')
    expect(rescueRowLabel({ ...sessions[1], archived: true }, 1)).toBe('旧复盘 #1')
  })

  it('「复盘了 …」写清被复盘的会话与角色：转换#1 / 转换#2 / 核对#1', () => {
    expect(reviewedTargetLabel(rescueOf('chapter_042', 1, 'convert_chapter_042', 'x'))).toBe('转换#1')
    expect(reviewedTargetLabel(rescueOf('chapter_042', 2, 'convert_chapter_042.try2', 'x'))).toBe('转换#2')
    expect(reviewedTargetLabel(rescueOf('chapter_042', 1, 'checker_chapter_042', 'x', 'checker'))).toBe('核对#1')
    // 老转录没有目标备注：不编角色/轮次。
    expect(reviewedTargetLabel(rescue3('chapter_042', 1, 'x'))).toBe('')
    // title = 原有 tip + 复盘对象（新转录带上被复盘的转录名）。
    const r = rescueOf('chapter_042', 1, 'convert_chapter_042.try2', 'x')
    expect(rescueRowTitle(r)).toBe(sessionTip(r) + '\n复盘对象：转换#2（convert_chapter_042.try2）')
    // 没有备注的老转录：原样给转录名去掉 rescue_ 前缀，与之前一致。
    const old = rescue3('chapter_042', 2, 'x')
    expect(rescueRowTitle(old)).toBe(sessionTip(old) + '\n复盘对象：chapter_042.try2')
  })

  it('同一章两个目标的复盘是两行、各算各的号', () => {
    const c = conv3('chapter_042', 1, '2026-10-23T10:00:00Z')
    const k = checker3('chapter_042', 1, '2026-10-23T10:30:00Z')
    const rc = rescueOf('chapter_042', 1, 'convert_chapter_042', '2026-10-23T11:00:00Z')
    const rk = rescueOf('chapter_042', 2, 'checker_chapter_042', '2026-10-23T12:00:00Z', 'checker')
    const list = [c, k, rc, rk]
    const node = buildChapterTree(list)[0]
    // 两行都在、按 mtime 旧 → 新；号各从 1 数（名字里的 .try2 不参与）。
    expect(node.rescues.map((r: any) => r.id)).toEqual([rc.id, rk.id])
    expect(rescueOrdinal(list, rc)).toBe(1)
    expect(rescueOrdinal(list, rk)).toBe(1)
    expect(rescueRowLabel(rc, rescueOrdinal(list, rc))).toBe('复盘 #1')
    expect(rescueRowLabel(rk, rescueOrdinal(list, rk))).toBe('复盘 #1')
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

  it('复盘会话不进转换/核对的历史链（只复盘过一次时它自己也没有页签）', () => {
    const c = conv3('chapter_042', 1, '2026-10-23T12:00:00Z')
    const k = checker3('chapter_042', 1, '2026-10-23T12:30:00Z')
    const r = rescueOf('chapter_042', 1, 'convert_chapter_042', '2026-10-23T13:00:00Z')
    const chain = chapterAttemptChain([c, k, r])
    const ids = (chain || []).map((m: any) => m.id)
    expect(ids).not.toContain(r.id)
    expect(chapterAttemptChain([c, k, r], r.id)).toBeNull()
  })

  it('C：复盘自己的历史只列同一章 + 同一目标的复盘（新 → 旧）', () => {
    const c1 = conv3('chapter_042', 1, '2026-10-23T10:00:00Z')
    const c2 = conv3('chapter_042', 2, '2026-10-23T10:30:00Z')
    const k = checker3('chapter_042', 1, '2026-10-23T10:40:00Z')
    const a1 = rescueOf('chapter_042', 1, 'convert_chapter_042', '2026-10-23T11:00:00Z')
    const a2 = rescueOf('chapter_042', 2, 'convert_chapter_042', '2026-10-23T13:00:00Z')
    const b1 = rescueOf('chapter_042', 3, 'checker_chapter_042', '2026-10-23T12:00:00Z', 'checker')
    const list = [c1, c2, k, a1, a2, b1]
    // 转换 #1 的两条复盘成一条链，新 → 旧。
    expect((chapterAttemptChain(list, a2.id) || []).map((m: any) => m.id)).toEqual([a2.id, a1.id])
    expect((chapterAttemptChain(list, a1.id) || []).map((m: any) => m.id)).toEqual([a2.id, a1.id])
    // 核对那一条目标只有一次复盘：没有页签；另一条目标的复盘绝不混进来。
    expect(chapterAttemptChain(list, b1.id)).toBeNull()
  })

  it('钉住一条复盘会话时，按它 meta 里的目标转录钉住那一次转换', () => {
    const c1 = conv3('chapter_042', 1, '2026-10-23T10:00:00Z')
    const c2 = conv3('chapter_042', 2, '2026-10-23T12:00:00Z')
    const r = rescueOf('chapter_042', 2, 'convert_chapter_042', '2026-10-23T13:00:00Z')
    const node = buildChapterTree([c1, c2, r], r.id)[0]
    expect(node.convert!.id).toBe(c1.id)
    expect(node.rescues.map((x: any) => x.id)).toEqual([r.id])
    // 复盘的那次转换已经不在了：仍然挂在这一行下面，只是不钉（默认最新）。
    const orphan = rescueOf('chapter_042', 1, 'convert_chapter_042.try9', '2026-10-23T13:00:00Z')
    const node2 = buildChapterTree([c2, orphan], orphan.id)[0]
    expect(node2.convert!.id).toBe(c2.id)
    expect(node2.rescues.map((x: any) => x.id)).toEqual([orphan.id])
    // 老转录没有目标备注：不按号猜，默认仍显示最新一次。
    const legacy = rescue3('chapter_042', 9, 'x')
    const node3 = buildChapterTree([c2, legacy], legacy.id)[0]
    expect(node3.convert!.id).toBe(c2.id)
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
