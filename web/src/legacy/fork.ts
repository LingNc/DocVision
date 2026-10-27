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

/* 能不能就地截断这一条（⤫）：不能在写入的会话上做（那一行可能还在追加），
 * 静态导出没有服务端。
 *
 * T196：**递归子章节也能截**。它们的转录在父工作区的细分树里
 * （`work/temp/conv_<父>/work/_subdivision` 下的 `node/sessions/`），页面按
 * `SessionInfo.Path` 发绝对路径，服务端按 `<项目>/work/` 之下来解析。 */
export function canCutFrom(s: any): boolean {
  if (!s || s.staticMode || s.live) return false
  if (!s.project) return false
  if (s.stage === 'vector') return String(s.name || '').indexOf('vector_') === 0
  if (!s.chapterId) return false
  return s.chapterRole === 'convert' || s.chapterRole === 'checker'
}

/* 能不能从这一条 fork（⤓ = 截断 + 开一条新尝试接着跑）：比截断窄一档——递归
 * 子章节的"接着跑"要父节点的整合上下文，单给子章节排 continue/checker 到不了
 * 前置条件，所以子章节只给就地截断，跑起来走「继续跑父章节」（T196）。
 *
 * T201：**核对会话例外**——重新核对一个字节的产物都不动，产物在不在细分树里都
 * 找得到（`chapterProductsFor`），源转录也可以在细分树里（服务端按路径解析），
 * 所以子章节的核对会话照样能「从某条记录之后继续核对」。 */
export function canForkFrom(s: any): boolean {
  if (!canCutFrom(s)) return false
  if (s.chapterRole === 'checker') return true
  return String(s.chapterId || '').indexOf('_child_') < 0
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
    // T201：发**完整路径**（会话的 Path 是绝对路径）。子章节的转录在父工作区的
    // 细分树里，只发文件名服务端只能去 work/sessions/ 找，必然报"源转录不存在"。
    extra: { from: Number(line), path: String(s.path || s.name || '') },
  }
}
