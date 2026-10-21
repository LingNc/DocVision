// 会话动作的判定（P56）：给哪条会话哪些按钮，全部是纯函数——模板只读结论，
// 单测直接钉规则，不依赖 DOM 与轮询。
//
// 为什么要有这一层：这些按钮会**真的开新尝试 / 删掉转录**，误判的代价是一次
// 白跑或一条记录消失。"能不能点"必须只有一处判断（这里），页面只负责画。

export type RetryRole = 'convert' | 'checker'

export interface SessionActionTarget {
  project: string
  chapter: string
  role: RetryRole
}

/* 重试 / 重新核对的目标：只看章节会话（convert/checker），且不是递归子章节。
 * 子章节由父章节的细分树产出，单独重跑它会与父节点的整合/晋级状态脱节——要
 * 重跑得"重跑子树 + 父节点重新整合"，那是另一件事（P56 第⑤点），这里不给按钮
 * 而不是点了才报错。 */
export function retryTargetOf(s: any): SessionActionTarget | null {
  if (!s || s.staticMode) return null
  if (!s.chapterId || !s.project) return null
  if (s.chapterRole !== 'convert' && s.chapterRole !== 'checker') return null
  // P56⑤：子章节也能重试，但只该在"父节点还在等子会话"时用——那时重跑这一支
  // 不会破坏已定稿的整合；父节点整合完（提交产物已在）服务端会拒，提示去重跑父章节。
  // 所以按钮照给，确认文案负责说清这条边界（见 confirmTextOf）。
  return { project: String(s.project), chapter: String(s.chapterId), role: s.chapterRole }
}

/* 「继续跑」的目标（T196）：顶层转换会话 = 它自己；递归子章节 = 它所在的顶层
 * 父章节。
 *
 * 为什么子章节要换成父章节：子节点的转录与产物都在父工作区的细分树里
 * （`work/temp/conv_<父>/work/_subdivision/**`），单独给子章节排一条 continue
 * 到不了任何前置条件——队列里会一直躺着、按钮看起来毫无反应（这一批之前就是
 * 这样）。父章节的"接着跑"会重新驱动整棵子树，而子树里每一支都按自己磁盘上
 * 那条转录续跑（T174），所以"先就地截断子会话、再继续跑父章节"= 从截断处接着
 * 跑，且不新建尝试、不归档任何东西。 */
export function continueTargetOf(s: any): SessionActionTarget | null {
  const t = retryTargetOf(s)
  if (!t || t.role !== 'convert') return null
  const i = t.chapter.indexOf('_child_')
  if (i <= 0) return t
  return { project: t.project, chapter: t.chapter.slice(0, i), role: 'convert' }
}

/* 继续跑：接着这一章最近一次尝试干（不新建尝试、不归档、不动产物）。转换会话
 * 才有（核对会话没有"继续产出"这回事），子章节按父章节算（见上）。 */
export function canContinueOf(s: any): boolean {
  return !!continueTargetOf(s)
}

/* 重画这张图：只在看某条作图会话、且它不正在写入时给。 */
export function canRedrawOf(s: any): boolean {
  if (!s || s.staticMode || s.live) return false
  if (!s.project || !s.name || s.stage !== 'vector') return false
  return String(s.name).indexOf('vector_') === 0
}

/* 删除这条记录：任何具体的章节会话、不正在写入。 */
export function canDropOf(s: any): boolean {
  if (!s || s.staticMode || !s.project || !s.path) return false
  if (!s.chapterId) return false
  return !s.live
}

/* 二次确认的文案：把"会发生什么"写清楚，避免点下去才发现。 */
export function confirmTextOf(kind: 'retry' | 'continue' | 'redraw' | 'drop' | 'cancel', s: any): string {
  switch (kind) {
    case 'retry':
      if (s && String(s.chapterId || '').indexOf('_child_') >= 0) {
        return '重试这个子章节？只在父节点还在等子会话时有效（那时它还没整合，重跑不会破坏已定稿的东西，父节点会拿新结果重新整合）；父章节一整合完，服务端就会拒并提示去重跑父章节。'
      }
      return s && s.chapterRole === 'checker'
        ? '重新核对这一章？产物一个字不动，只换核对会话；旧结论留档成 .checker.try<N>，旧转录仍在「历史」里。'
        : '重试这一章？会开一个新尝试：旧的整棵（产物 / 分片 / 报告）留档到 work/attempts/，「历史」里两条都在。'
    case 'continue':
      return '继续跑？接着这一章最近一次尝试接着干：不新建尝试、不归档、不动已提交产物。'
    case 'redraw':
      return '重画这张图？会开一条新的作图尝试（旧转录留档成 .tryN），类别与 dedupe 分组不变；要看新图得重跑下游阶段。'
    case 'drop':
      return '删掉这条记录？转录会从「历史」里消失，作废尝试的 work/attempts 归档目录也会一起删。'
    case 'cancel':
      return '撤销这个排队请求？只是把 work/retry/ 里那个文件删掉——真正干活的那一侧还没看到它。'
  }
  return ''
}
