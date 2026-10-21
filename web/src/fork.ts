// fork 的提交入口（P57）：对话页与轨迹页共用一处，避免两套确认文案与请求体。
// 排队成功之后登记"等新尝试出现就切过去"，然后刷新一次索引。
import { refreshIndex, requestChapterRetry, watchFork } from './data'
import { canForkFrom, forkPrompt, forkRequestOf } from './legacy/fork'
import { state } from './state'

/* dropBefore：从这一条**之前** fork——等价于把这条及其之后的（错误）消息删掉，让
 * 它从干净的上下文继续（用户口径："删除某些错误消息让他继续"）。第 1 条之前没有
 * 可截断的点，直接拒绝而不是悄悄退化成重跑整章。 */
export function forkBeforeLine(line: number, source: string): boolean {
  if (!(line > 1)) {
    state.retryNote = '这一条之前没有可截断的记录（要从头重来请用「重试这一章」）'
    return false
  }
  return forkFromLine(line - 1, source)
}

export function forkFromLine(line: number, source: string): boolean {
  const s: any = state.current
  const req = forkRequestOf(s, line, source)
  if (!s || !req) return false
  if (!window.confirm(forkPrompt(line, s))) return false
  const prevId = String(s.id || '')
  state.retryNote = '正在提交从第 ' + line + ' 条记录继续的请求…'
  requestChapterRetry(req.project, req.chapter, req.role, req.reason, req.extra)
    .then((res: any) => {
      state.retryNote = '已排队：从第 ' + line + ' 条记录之后继续（' + ((res && res.note) || '等待并发槽位') + '）；新会话出现后自动切过去'
      watchFork(req.project, req.chapter, req.role, prevId)
      return refreshIndex()
    })
    .catch((err: any) => {
      state.retryNote = '从记录继续失败：' + ((err && err.message) || err)
    })
  return true
}

export { canForkFrom }
