/*
 * 章节树/历史用例共用的夹具构造器（P62 从 chapter-tree.spec.ts 抽出来）。
 *
 * 只放“转录 id 怎么拼、会话对象长什么样”，不放任何断言——期望值留在各自的 spec
 * 里。三个 spec（chapter-tree / chapter-chain / chapter-fix-rescue）都从这里取。
 */

export type Over = Record<string, unknown>

// 章节会话的工作树目录与归档目录（归档 = work/attempts/<根章>/try1/work/sessions/）
export const SESSIONS_DIR = 'book/work/sessions/'
export const archiveDir = (root: string) => `book/work/attempts/${root}/try1/work/sessions/`

// 转录名的尝试号后缀：第 1 次不带后缀，之后 `.tryN`
export const idWithTry = (stem: string, attempt: number) => stem + (attempt > 1 ? `.try${attempt}` : '') + '.jsonl'

// 工作树里的章节会话 id（role ∈ convert / checker / rescue）
export const liveId = (role: string, chapterId: string, attempt: number) =>
  SESSIONS_DIR + idWithTry(`${role}_${chapterId}`, attempt)

// 归档树里的同一份章节会话 id（root 是被重开的那一版根章）
export const archivedId = (role: string, chapterId: string, attempt: number, root = 'chapter_042') =>
  archiveDir(root) + idWithTry(`${role}_${chapterId}`, attempt)

// 修复会话的临时工作区路径：work/temp/conv_<根章>/work/_subdivision/<子章>/node/sessions/
export const fixId = (name: string, root = 'chapter_042', child = 'child_01') =>
  `book/work/temp/conv_${root}/work/_subdivision/${child}/node/sessions/` + name

// 章节会话的公共形状：项目固定 book，其余字段由用例覆盖
export const session = (over: Over = {}) => ({ project: 'book', ...over })

// 子章节 ID → 父章节 ID（顶层章节返回空串）
export const parentOf = (id: string) => (/_child_\d+$/.test(id) ? id.replace(/_child_\d+$/, '') : '')
