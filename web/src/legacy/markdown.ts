/*
 * 行内 Markdown 渲染（旧页 mdInline 一族照搬）：`code` / [链](接) / **粗** /
 * __粗__ / ~~删~~ / *斜* / _斜_ / $…$ 与 $$…$$（数学）。一律 DOM API 落树，
 * 从不拼 HTML；名字里的 `<`、`&` 只是文本。
 *
 * 下划线的强调只在词边界成立（T228，CommonMark 的 intraword 规则）：
 * `chapter_012_child_02` 这种名字里的下划线原样保留。
 *
 * 数学转换器（mdMathML）在对话正文块才移植；这里先按旧页的"转换器抛错就
 * 地退回原始 $…$ 文本"路径走（stub 抛错），块3 接上真转换器后此文件不用改。
 */

export const MD_INLINE =
  /(`+)([^`]*?)\1|\[([^\]]*)\]\(([^)\s]*)\)|\*\*([^*]+)\*\*|__([^_]+)__|~~([^~]+)~~|\*([^*\n]+)\*|_([^_\n]+)_|\$\$([\s\S]+?)\$\$|\$([^$\n]+?)\$|\\\(([\s\S]+?)\\\)|\\\[([\s\S]+?)\\\]/

/* 链接协议白名单：javascript: 之类一律拒绝（返回空 = 只显示文字）。 */
export function mdSafeURL(url: unknown): string {
  const s = String(url === undefined || url === null ? '' : url).trim()
  if (!s) return ''
  if (/^(https?:|mailto:|#|\/|\.\/|\.\.\/)/i.test(s)) return s
  if (/^[a-z][a-z0-9+.-]*:/i.test(s)) return ''
  return s
}

function el(tag: string, cls?: string, text?: string): HTMLElement {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  if (text !== undefined && text !== null) node.textContent = text
  return node
}

/*
 * T228：`_` / `__` 只在**词边界**上才是强调（CommonMark 的 intraword 规则）。
 * 转录里到处是 `chapter_012_child_02`、`convert_….jsonl` 这类名字，旧正则把
 * 中间的下划线当成强调标记吃掉了，页面上显示成 `chapter012child_02…`——读起来
 * 就像记录本身被改坏了。`*` 不受这条限制（`a*b*c` 仍是强调）。
 *
 * 不用 lookbehind：老浏览器（Safari < 16.4）会在加载模块时抛 SyntaxError，
 * 整页白屏；前一个字符由 mdInline 跨片带进来。
 */
const WORD_CHAR = /[\p{L}\p{N}_]/u

export function intrawordUnderscore(m: RegExpExecArray, before: string, after: string): boolean {
  if (m[6] === undefined && m[9] === undefined) return false // 6 = __…__、9 = _…_
  return (before !== '' && WORD_CHAR.test(before)) || (after !== '' && WORD_CHAR.test(after))
}

/* 数学：LaTeX → MathML 转换器在 richtext.ts（块3 移植）；转换器抛错就地
 * 退回原始 $…$ 文本——一条坏公式不许打断整页渲染。 */
import { mdMathML } from './richtext'

export function mdInline(parent: Node, text: unknown, depth = 0): void {
  if (depth > 6) {
    parent.appendChild(document.createTextNode(String(text || '')))
    return
  }
  let rest = String(text === undefined || text === null ? '' : text)
  // current[0] 前面那个字符（跨片跟踪）：词内下划线要靠它判断，见
  // intrawordUnderscore（T228）。
  let before = ''
  let guard = 0
  while (rest && guard++ < 800) {
    const m = MD_INLINE.exec(rest)
    if (!m) break
    if (m.index > 0) {
      parent.appendChild(document.createTextNode(rest.slice(0, m.index)))
      before = rest.slice(m.index - 1, m.index)
    }
    const matched = m[0]
    rest = rest.slice(m.index + matched.length)
    // T228：词内的 `_`/`__` 不是强调，原样落成文本（见 intrawordUnderscore）。
    if (intrawordUnderscore(m, before, rest.slice(0, 1))) {
      parent.appendChild(document.createTextNode(matched))
      before = matched.slice(-1)
      continue
    }
    before = matched.slice(-1)
    let node: HTMLElement
    if (m[1] !== undefined) {
      // 行内代码：内容原样进文本节点（含 < > & ，不会被当成标签）
      node = el('code', 'md-inline-code', m[2])
    } else if (m[3] !== undefined) {
      node = el('a', 'md-link')
      const href = mdSafeURL(m[4])
      if (href) {
        node.setAttribute('href', href)
        node.setAttribute('target', '_blank')
        node.setAttribute('rel', 'noopener noreferrer')
      } else {
        node.title = '链接协议不受支持，只显示文字'
      }
      mdInline(node, m[3], depth + 1)
    } else if (m[10] !== undefined || m[11] !== undefined ||
               m[12] !== undefined || m[13] !== undefined) {
      // 数学：$$…$$ 与 \[…\] 独立显示、$…$ 与 \(…\) 行内（T61：LaTeX 括号
      // 形式 \(\Omega\) 曾原样显示）。转换器万一抛错，就地退回原始文本——
      // 一条坏公式不许打断整页渲染。
      const dispSrc = m[10] !== undefined ? m[10] : m[13]
      const inlineSrc = m[11] !== undefined ? m[11] : m[12]
      try {
        node = mdMathML(dispSrc !== undefined ? dispSrc : inlineSrc, dispSrc !== undefined) as HTMLElement
      } catch {
        node = el('span')
        node.appendChild(document.createTextNode(m[0]))
      }
    } else if (m[5] !== undefined || m[6] !== undefined) {
      node = el('strong')
      mdInline(node, m[5] !== undefined ? m[5] : m[6], depth + 1)
    } else if (m[7] !== undefined) {
      node = el('del')
      mdInline(node, m[7], depth + 1)
    } else {
      node = el('em')
      mdInline(node, m[8] !== undefined ? m[8] : m[9], depth + 1)
    }
    parent.appendChild(node)
  }
  if (rest) parent.appendChild(document.createTextNode(rest))
}

/*
 * 行内渲染入口：**单行**文本专用（会话名 / 面包屑 / 详情栏标签）。
 * 名字里没有标记时整串原样落成一个文本节点，可见结果与 textContent 一致。
 */
export function renderInlineMarkdown(text: unknown): DocumentFragment {
  const frag = document.createDocumentFragment()
  mdInline(frag, text, 0)
  return frag
}

/* 块内的软换行按可见换行处理（转录里一行就是一行，不合并成空格）。 */
export function mdInlineLines(parent: Node, text: unknown): void {
  String(text === undefined || text === null ? '' : text)
    .split('\n')
    .forEach((part, i) => {
      if (i) parent.appendChild(el('br', 'md-br'))
      mdInline(parent, part, 0)
    })
}
