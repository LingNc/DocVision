// 跨行 $$…$$ 显示数学块识别（renderMarkdown 本体要 DOM，纯函数在此钉住）。
import { describe, expect, test } from 'vitest'
import { collectMathBlock } from '../src/legacy/richtext'
import { MD_INLINE, intrawordUnderscore, renderInlineMarkdown } from '../src/legacy/markdown'

describe('collectMathBlock', () => {
  test('多行 $$ 块：去壳源码 + 块后下标', () => {
    const r = collectMathBlock(['前文', '$$', 'x+1=2', '$$', '后文'], 1)
    expect(r).toEqual({ src: '\nx+1=2\n', next: 4 })
  })
  test('单行 $$…$$', () => {
    expect(collectMathBlock(['$$a^2+b^2=c^2$$'], 0)).toEqual({ src: 'a^2+b^2=c^2', next: 1 })
  })
  test('起手行带内容、收尾行带 $$', () => {
    const r = collectMathBlock(['$$x+1', '=2$$'], 0)
    expect(r).toEqual({ src: 'x+1\n=2', next: 2 })
  })
  test('未闭合吃到文末', () => {
    const r = collectMathBlock(['$$', 'x+1'], 0)
    expect(r).toEqual({ src: '\nx+1', next: 2 })
  })
  test('非 $$ 起手返回 null；裸 $$ 不算单行', () => {
    expect(collectMathBlock(['普通文本'], 0)).toBeNull()
    expect(collectMathBlock(['$$$$'], 0)).toBeNull()
  })
})

// T61：LaTeX 括号形式 \(...\) 行内与 \[…\] 显示块（此前原样显示）。
describe('T61 括号数学形式', () => {
  test('MD_INLINE 认 \\(…\\) 行内（第 12 组）', () => {
    const m = MD_INLINE.exec('寸并把 \\(Omega\\) 放回')
    expect(m).toBeTruthy()
    expect(m![12]).toBe('Omega')
  })
  test('MD_INLINE 认 \\[…\\] 显示（第 13 组）', () => {
    const m = MD_INLINE.exec('\\[E = mc^2\\]')
    expect(m).toBeTruthy()
    expect(m![13]).toBe('E = mc^2')
  })
  test('既有形式不受影响：**粗** 与 $…$', () => {
    const m = MD_INLINE.exec('普通 **加粗** 与 $x^2$')
    expect(m![5]).toBe('加粗')
    const m2 = MD_INLINE.exec('$x^2$')
    expect(m2![11]).toBe('x^2')
  })
  test('collectMathBlock：单行 \\[…\\]', () => {
    expect(collectMathBlock(['\\[a^2+b^2=c^2\\]'], 0)).toEqual({ src: 'a^2+b^2=c^2', next: 1 })
  })
  test('collectMathBlock：多行 \\[ 块', () => {
    const r = collectMathBlock(['\\[', 'x+1', '\\]', '后文'], 0)
    expect(r).toEqual({ src: '\nx+1\n', next: 3 })
  })
  test('collectMathBlock：$$ 行为不变', () => {
    expect(collectMathBlock(['$$a$$'], 0)).toEqual({ src: 'a', next: 1 })
  })
})
/*
 * T228：`_` / `__` 只在**词边界**上是强调（CommonMark 的 intraword 规则）。
 * 转录里的章节名、转录文件名全是下划线，旧正则把它们当成强调标记吃掉，页面上
 * 显示成 `chapter012child_02…`——读起来就像记录本身被改坏了。
 *
 * node 环境没有 DOM：给 mdInline 一个最小实现，只为读回可见文本与标签名。
 */
type FakeNode = {
  nodeType: number
  tag: string
  text: string
  className: string
  attrs: Record<string, string>
  children: FakeNode[]
  textContent: string
}

function shimDom() {
  const make = (tag: string, text = ''): FakeNode => {
    const n = {
      nodeType: tag === '#text' ? 3 : 1,
      tag,
      text,
      className: '',
      attrs: {} as Record<string, string>,
      children: [] as FakeNode[],
      get textContent(): string {
        if (n.nodeType === 3 || n.children.length === 0) return n.text
        return n.children.map((c) => c.textContent).join('')
      },
      set textContent(v: string) {
        n.children = []
        n.text = v
      },
      appendChild(child: FakeNode) {
        n.children.push(child)
        return child
      },
      setAttribute(k: string, v: string) {
        n.attrs[k] = v
      },
    }
    return n as FakeNode
  }
  ;(globalThis as Record<string, unknown>).document = {
    createTextNode: (t: string) => make('#text', t),
    createElement: (tag: string) => make(tag),
    createDocumentFragment: () => make('#frag'),
  }
}

/** 渲染结果里出现过的标签名（含 class），用来确认强调真的落了。 */
function tagsOf(node: FakeNode): string[] {
  const out: string[] = []
  const walk = (n: FakeNode) => {
    if (n.nodeType === 1) out.push(n.className ? n.tag + '.' + n.className : n.tag)
    n.children.forEach(walk)
  }
  walk(node)
  return out
}

describe('T228 词内下划线不当强调', () => {
  test('章节名与转录文件名原样显示（不吞下划线）', () => {
    shimDom()
    for (const s of [
      'chapter_012_child_02_child_03',
      'convert_chapter_012_child_02_child_03.jsonl',
      '回溯到 chapter_001_child_02 的 node/sessions',
      'a_b_c',
    ]) {
      expect(renderInlineMarkdown(s).textContent).toBe(s)
    }
  })
  test('词边界的 _斜_ / __粗__ 仍是强调', () => {
    shimDom()
    const frag = renderInlineMarkdown('这是 _斜体_ 与 __加粗__ 和 ~~删~~ 以及 `code`')
    expect(frag.textContent).toBe('这是 斜体 与 加粗 和 删 以及 code')
    const tags = tagsOf(frag as unknown as FakeNode)
    expect(tags).toContain('em')
    expect(tags).toContain('strong')
    expect(tags).toContain('del')
    expect(tags).toContain('code.md-inline-code')
  })
  test('星号词内仍是强调（CommonMark 允许 *a*b）', () => {
    shimDom()
    expect(renderInlineMarkdown('a*b*c').textContent).toBe('abc')
  })
  test('词内下划线后面的正常内容不受影响', () => {
    shimDom()
    const frag = renderInlineMarkdown('会话 convert_chapter_002_child_01 未提交 **要重跑**')
    expect(frag.textContent).toBe('会话 convert_chapter_002_child_01 未提交 要重跑')
    expect(tagsOf(frag as unknown as FakeNode)).toContain('strong')
  })
  test('intrawordUnderscore 只拦下划线两支，不拦 **粗** 与 *斜*', () => {
    const m = MD_INLINE.exec('**粗**')!
    expect(intrawordUnderscore(m, 'x', 'y')).toBe(false)
    const u = MD_INLINE.exec('a_b_')!
    expect(intrawordUnderscore(u, 'a', '')).toBe(true)
  })
})
