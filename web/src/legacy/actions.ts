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

/* 两个动作对象是不是同一件事（T208）：确认条在轮询自动切换会话时收不收走，
 * 只看这一条。同项目 + 同章节 + 同角色就算同一件事——同一章的另一条尝试
 * （重试刚开出来的新转录）也算，用户点开的确认条不该被那一跳静默收掉。
 * 用户报的原话是"点那个重试该章节也没有用"：点了「重试这一章」、确认条出现，
 * 紧接着轮询把视图切走，条子消失，看起来就是什么都没发生。 */
export function sameActionTarget(
  a: SessionActionTarget | null | undefined,
  b: SessionActionTarget | null | undefined,
): boolean {
  if (!a || !b) return false
  return a.project === b.project && a.chapter === b.chapter && a.role === b.role
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

export interface RestoreTarget {
  project: string
  chapter: string
  attempt: number
}

/* 「切回第 N 次尝试」的目标（P63）：把 work/attempts/<章>/try<N>/ 整棵搬回工作区、
 * 把现在这一次整棵留档。判定收在这里，页面只读结论。
 *
 * 为什么只给顶层章节的转换会话：
 *   · 子章节的尝试号是**父尝试之内**的号，单独搬一支会把两代子树拼在一起——它
 *     归父章节（服务端也这么拒）；
 *   · 修复 / 复盘链的号不是"这一章的第几次"（修复按轮次、复盘按这一章的第几次），
 *     拿它去拼 try<N> 会指到不相干的目录；
 *   · 号等于当前这一次时没有旧版可切（那本来就是工作区里这一份）。
 * newestAttempt 是历史链里**还在工作区**的最大尝试号（HistoryPanel 的 newest）。 */
export function restoreTargetOf(s: any, newestAttempt: number): RestoreTarget | null {
  if (!s || s.staticMode || !s.project || !s.chapterId) return null
  if (s.chapterRole !== 'convert') return null
  if (s.archived) return null
  const chapter = String(s.chapterId)
  if (chapter.indexOf('_child_') >= 0) return null
  const attempt = Number(s.chapterAttempt) || 1
  if (attempt === (Number(newestAttempt) || 1)) return null
  return { project: String(s.project), chapter, attempt }
}

export function canRestoreOf(s: any, newestAttempt: number): boolean {
  return !!restoreTargetOf(s, newestAttempt)
}

/* 二次确认的文案：把"会发生什么"写清楚，避免点下去才发现。 */
export function confirmTextOf(kind: 'retry' | 'continue' | 'redraw' | 'drop' | 'cancel', s: any): string {
  switch (kind) {
    case 'retry': {
      // T197：**先分角色，再看层级**。核对会话的按钮是「重新核对这一章」，它一个
      // 字节的产物都不动，所以不受"父节点还在等子会话"那条约束（T192）。以前把
      // `_child_` 判断写在最前面，站到子章节的核对会话上会弹出一段讲"重跑会破坏
      // 已定稿的东西、父章节一整合完就被拒"的转换口径文案——纯属张冠李戴，还把
      // 人劝退。
      if (s && s.chapterRole === 'checker') {
        const child = String(s.chapterId || '').indexOf('_child_') >= 0
        return '重新核对这一章？产物一个字不动，只换核对会话；旧结论留档成 .checker.try<N>，旧转录仍在「历史」里。'
          + (child ? '（递归子章节也能重核：它不改产物，父章节整合完同样可以。）' : '')
      }
      if (s && String(s.chapterId || '').indexOf('_child_') >= 0) {
        return '重试这个子章节？只在父节点还在等子会话时有效（那时它还没整合，重跑不会破坏已定稿的东西，父节点会拿新结果重新整合）；父章节一整合完，服务端就会拒并提示去重跑父章节。'
      }
      return '重试这一章？会开一个新尝试：旧的整棵（产物 / 分片 / 报告）留档到 work/attempts/，「历史」里两条都在。'
    }
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
