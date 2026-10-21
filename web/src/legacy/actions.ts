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
  if (String(s.chapterId).indexOf('_child_') >= 0) return null
  return { project: String(s.project), chapter: String(s.chapterId), role: s.chapterRole }
}

/* 继续跑：接着这一章最近一次尝试干（不新建尝试、不归档、不动产物）。只给
 * 转换会话——核对会话没有"继续产出"这回事。 */
export function canContinueOf(s: any): boolean {
  const t = retryTargetOf(s)
  return !!t && t.role === 'convert'
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
