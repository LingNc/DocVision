// "从某条消息 fork 一条会话"（P57）的判定与请求体，纯函数。
//
// 语义与 P53 阶段二之一 / P55 一致：把转录截到这一行（含），后面的轮次丢掉，
// 抄成一条**新尝试**接着干；源转录一字不动（原记录仍在「历史」里）。章节给的
// 角色是 continue（带 from），核对是 checker（带 from），作图是 vector。
//
// 对话页与轨迹页共用这一处：以前只有轨迹页每行有 ⤓，用户在对话页找不到入口。

export interface ForkRequest {
  project: string
  chapter: string
  role: 'continue' | 'checker' | 'vector'
  reason: string
  extra: { from: number; path: string }
}

/* 能不能从这一条 fork：不能在写入的会话上做（那一行可能还在追加），静态导出
 * 没有服务端，子章节与屏障阶段没有可重跑的对象。 */
export function canForkFrom(s: any): boolean {
  if (!s || s.staticMode || s.live) return false
  if (!s.project) return false
  if (s.stage === 'vector') return String(s.name || '').indexOf('vector_') === 0
  if (!s.chapterId) return false
  if (s.chapterRole !== 'convert' && s.chapterRole !== 'checker') return false
  return String(s.chapterId).indexOf('_child_') < 0
}

/* 二次确认文案：写清"后面会丢、原记录保留、工作区怎么处理"。 */
export function forkPrompt(line: number, s: any): string {
  if (s && s.stage === 'vector') {
    return '从第 ' + line + ' 条记录之后继续？\n\n历史会截到这一条（完整旧转录留档成 .tryN，仍看得到），只重画这一张；类别与分组不变。'
  }
  if (s && s.chapterRole === 'checker') {
    return '从第 ' + line + ' 条记录之后继续核对？\n\n后面的轮次会从这条核对会话的历史里丢掉（源转录仍保留在「历史」里），并开一条新的核对尝试；已提交的产物一个字不动。'
  }
  return '从第 ' + line + ' 条记录之后继续？\n\n后面的轮次会从"接着干"的历史里丢掉（源转录仍保留在「历史」里），并开一个新尝试；有工作区快照时会连工作区一起退回这一刻（老会话没有快照则只退对话）。'
}

export function forkRequestOf(s: any, line: number, source: string): ForkRequest | null {
  if (!s || !canForkFrom(s) || !(line > 0)) return null
  const vector = s.stage === 'vector'
  return {
    project: String(s.project),
    chapter: vector ? '' : String(s.chapterId),
    role: vector ? 'vector' : s.chapterRole === 'checker' ? 'checker' : 'continue',
    reason: source + '回滚到 #' + line,
    extra: { from: Number(line), path: String(s.name || '') },
  }
}
