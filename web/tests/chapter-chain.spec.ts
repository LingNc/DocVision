/*
 * 章节尝试链（「历史 N」页签）：chapterAttemptChain 的聚合口径——同一章的各次
 * 尝试串成一条链、按角色分开、同号转录按内容指纹去重、老转录没有锚点时不被丢掉。
 *
 * P62：从 chapter-tree.spec.ts 拆出来（行为一字未改）。树本身的规则在
 * chapter-tree.spec.ts，修复/复盘行在 chapter-fix-rescue.spec.ts。
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { state } from '../src/state'
import { chapterAttemptChain, chapterAttemptTag, chainMemberTag, dedupeChapterAttempts } from '../src/legacy/sidebar'
import { SESSIONS_DIR, archivedId, liveId, session } from './helpers/chapter'

beforeEach(() => {
  state.collapsed = {}
})

// T165：章节转换被 checker 打回后换全新会话重来，几次尝试要能串成一条链
// （中栏「历史 N」页签），并用紧凑的 `#N` 标注尝试号（T173：不再写中文「第 N 次」）。
describe('chapter attempt chain', () => {
  const mk = (id: string, attempt: number, mtime: string, chapterId = 'chapter_042') =>
    session({ id, chapterId, chapterRole: 'convert', chapterAttempt: attempt, mtime })

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

// T194：工作树与 work/attempts 里都会有「同一章 + 同一角色 + 同号」的转录，
// 但两种情形要分开：内容指纹相同（同一份转录被扫了两遍）只留一份；内容不同
// （同一个子章节在两次父尝试里各转过一次，尝试号都从 1 重新数）两条都留，
// 归档那份标「旧#N」——不能让历史把一次真实发生过的对话藏掉。
describe('同号转录：真副本去重，不同对话都留着', () => {
  const conv = (attempt: number, archived = false, sha = 'aaa', mt = '2026-10-19T11:00:00Z') => ({
    id: archived ? archivedId('convert', 'chapter_007', attempt, 'chapter_007') : liveId('convert', 'chapter_007', attempt),
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

/* P59 补充：老核对转录（P58 之前落盘的）**没有锚点**。只按锚点全等过滤时，
 * 只要当前这条写下了锚点，那些老核对就一条都匹配不上——历史里看起来像"最新的
 * 一次核对把旧的盖掉了"。没有锚点的成员没得比，一律留在链里；锚在别处的核对
 * 仍然要排除。 */
describe('P59 老核对转录不被历史丢掉', () => {
  const conv = (attempt = 1, mt = '2026-10-22T09:00:00Z') => ({
    id: liveId('convert', 'chapter_042', attempt),
    project: 'book', chapterId: 'chapter_042', chapterRole: 'convert', chapterAttempt: attempt, mtime: mt,
  })
  const chk = (name: string, attempt: number, anchor: string, mt: string) => ({
    id: SESSIONS_DIR + name,
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
