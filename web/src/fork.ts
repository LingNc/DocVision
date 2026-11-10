// fork 的提交入口（P57）：对话页与轨迹页共用一处，避免两套确认文案与请求体。
// 排队成功之后登记"等新尝试出现就切过去"，然后刷新一次索引。
import { refreshIndex, requestChapterRetry, truncateSession, watchFork } from './data'
import { canCutFrom, canForkFrom, forkPrompt, forkRequestOf } from './legacy/fork'
import { state, setRetryNote } from './state'

/* dropBefore：从这一条**之前** fork——等价于把这条及其之后的（错误）消息删掉，让
 * 它从干净的上下文继续（用户口径："删除某些错误消息让他继续"）。第 1 条之前没有
 * 可截断的点，直接拒绝而不是悄悄退化成重跑整章。 */
export function dropFromLine(line: number, source: string): boolean {
  if (!(line > 1)) {
    setRetryNote('这一条之前没有可截断的记录（要从头重来请用「重试这一章」）')
    return false
  }
  const s: any = state.current
  // T311：能不能截断问 canCutFrom（它本来就允许作图会话），**别再看 chapterId**
  // ——作图会话的 chapterId 是 null，以前这里直接静默 return false：点一下既没有
  // 确认框、也没有请求、更没有提示，用户只能得出"这个按钮坏了"。
  if (!s || !canCutFrom(s)) return false
  const keep = line - 1
  const vector = s.stage === 'vector'
  // T196：递归子章节的转录在父工作区的细分树里，跑起来也不能单给这一章排请求
  // ——截断之后要「继续跑父章节」（父节点的细分树重新驱动时，这一支按磁盘上
  // 那条截断后的转录续跑）。
  const child = String(s.chapterId || '').indexOf('_child_') >= 0
  const parent = child ? String(s.chapterId).slice(0, String(s.chapterId).indexOf('_child_')) : ''
  const then = vector
    ? '这张图会从这条截断后的转录接着画：让它重新排进重画（例如把阈值调高让它被退回，或下次运行把它退回）即可；点「重画这张图」是从头开新会话（旧转录留档成 .tryN），不是接着这条'
    : child
    ? '接着点顶栏的「继续跑父章节」（' + parent + '）让它跑起来：子章节不会单独重跑，父节点重新驱动这一支时会从这条截断后的转录续跑'
    : '运行中的 latex 会从这条会话接着跑（也可以点「继续跑」排一次）'
  const ask = '删掉第 ' + line + ' 条及之后（共 ' + keep + ' 条保留）并继续这条会话？\n\n'
    + '就地截断当前转录，不新开尝试、不复制会话；原文会先备份到 work/sessions/.trash/。'
    + '想保留一份对照，请先点 ⤓ 显式 fork 一条同样的会话再在这里操作。'
  if (!window.confirm(ask)) return false
  setRetryNote('正在就地截断到第 ' + keep + ' 条…')
  truncateSession(String(s.project), String(s.path), keep)
    .then((res: any) => {
      setRetryNote('已截断：保留 ' + ((res && res.kept) || keep) + ' 条、丢掉 ' + ((res && res.dropped) || 0)
        + ' 条（备份 ' + ((res && res.backup) || '.trash/') + '）；' + then)
      return refreshIndex()
    })
    .catch((err: any) => {
      setRetryNote('截断失败：' + ((err && err.message) || err))
    })
  return true
}

export function forkFromLine(line: number, source: string): boolean {
  const s: any = state.current
  const req = forkRequestOf(s, line, source)
  if (!s || !req) return false
  if (!window.confirm(forkPrompt(line, s))) return false
  const prevId = String(s.id || '')
  setRetryNote('正在提交从第 ' + line + ' 条记录继续的请求…')
  requestChapterRetry(req.project, req.chapter, req.role, req.reason, req.extra)
    .then((res: any) => {
      setRetryNote('已排队：从第 ' + line + ' 条记录之后继续（' + ((res && res.note) || '等待并发槽位') + '）；新会话出现后自动切过去')
      watchFork(req.project, req.chapter, req.role, prevId)
      return refreshIndex()
    })
    .catch((err: any) => {
      setRetryNote('从记录继续失败：' + ((err && err.message) || err))
    })
  return true
}

export { canForkFrom }
