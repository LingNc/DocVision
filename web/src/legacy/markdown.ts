/*
 * 行内 Markdown 渲染（旧页 mdInline 一族照搬）：`code` / [链](接) / **粗** /
 * __粗__ / ~~删~~ / *斜* / _斜_ / $…$ 与 $$…$$（数学）。一律 DOM API 落树，
 * 从不拼 HTML；名字里的 `<`、`&` 只是文本。
 *
 * 数学转换器（mdMathML）在对话正文块才移植；这里先按旧页的"转换器抛错就
 * 地退回原始 $…$ 文本"路径走（stub 抛错），块3 接上真转换器后此文件不用改。
 */

export const MD_INLINE =
  /(`+)([^`]*?)\1|\[([^\]]*)\]\(([^)\s]*)\)|\*\*([^*]+)\*\*|__([^_]+)__|~~([^~]+)~~|\*([^*\n]+)\*|_([^_\n]+)_|\$\$([\s\S]+?)\$\$|\$([^$\n]+?)\$/

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

/* 数学：LaTeX → MathML 转换器在 richtext.ts（块3 移植）；转换器抛错就地
 * 退回原始 $…$ 文本——一条坏公式不许打断整页渲染。 */
import { mdMathML } from './richtext'

export function mdInline(parent: Node, text: unknown, depth = 0): void {
  if (depth > 6) {
    parent.appendChild(document.createTextNode(String(text || '')))
    return
  }
  let rest = String(text === undefined || text === null ? '' : text)
  let guard = 0
  while (rest && guard++ < 800) {
    const m = MD_INLINE.exec(rest)
    if (!m) break
    if (m.index > 0) parent.appendChild(document.createTextNode(rest.slice(0, m.index)))
    rest = rest.slice(m.index + m[0].length)
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
    } else if (m[10] !== undefined || m[11] !== undefined) {
      // 数学：$$…$$ 独立显示、$…$ 行内。转换器万一抛错，就地退回原始 $…$ 文本
      // ——一条坏公式不许打断整页渲染。
      try {
        node = mdMathML(m[10] !== undefined ? m[10] : m[11], m[10] !== undefined) as HTMLElement
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
