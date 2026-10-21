// fork 的提交入口（P57）：对话页与轨迹页共用一处，避免两套确认文案与请求体。
// 排队成功之后登记"等新尝试出现就切过去"，然后刷新一次索引。
import { refreshIndex, requestChapterRetry, watchFork } from './data'
import { canForkFrom, forkPrompt, forkRequestOf } from './legacy/fork'
import { state } from './state'

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
