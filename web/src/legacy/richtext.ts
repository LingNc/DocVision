/*
 * 富文本渲染层（旧页 viewer.js 的照搬区）：块级 Markdown、机器文本高亮
 * （JSON / 终端 / diff / LaTeX 日志）、LaTeX → MathML 转换器、复制按钮，
 * 以及工具输入输出的「限高内滚 + 展开全文」包装层（machineScroll / foldLabel）。
 * 函数名与旧页一致，方便对照与逐块复验。
 *
 * 安全边界与旧页相同：整棵树只用 DOM API 造（createElement /
 * createElementNS / createTextNode），**从不碰 innerHTML**——转录里的
 * `<script>`、`<img onerror=…>` 落树时就是一个文本节点；链接协议另经
 * markdown.ts 的 mdSafeURL 过滤。零依赖零外链。
 *
 * 行内规则（MD_INLINE / mdInline / mdInlineLines / mdSafeURL）在
 * markdown.ts，这里 import 使用、不重复实现；计数文案 countText 在
 * sidebar.ts，localStorage 读写（storeGet/storeSet）在 ../state。
 */
import { mdInline, mdInlineLines } from './markdown'
import { storeGet, storeSet } from '../state'
import { countText } from './sidebar'
import { mermaidPreviewAvailable, mermaidPreviewBlock } from './mermaid'

function el(tag: string, cls?: string, text?: string): HTMLElement {
  const node = document.createElement(tag)
  if (cls) node.className = cls
  if (text !== undefined && text !== null) node.textContent = text
  return node
}

/* 字符数的短写（给"内容过大"的提示用：200 KB 以上就按 KB 报）。 */
export function fmtChars(n: number): string {
  if (!n) return '0 字符'
  if (n < 1024) return n + ' 字符'
  return (n / 1024).toFixed(1) + ' KB'
}

/* Pretty-print a JSON blob by walking text nodes, never by building HTML. */
export function prettyJSON(raw: unknown): string {
  if (typeof raw !== 'string' || !raw.trim()) return ''
  try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
}

/* ---------- JSON 高亮（词法扫描，不嵌套解析） ---------- */

/*
 * JSON 高亮：把一串 JSON 文本渲染成
 *   span.k（键名）· span.s（字符串）· span.n（数字）· span.b（true/false/null）
 * 类名与配色沿用样式表里既有的 .k/.s/.n/.b 与 --key/--str/--num/--bool，
 * 浅色/深色两套自动生效，不新增配色体系。
 *
 * 只做**一层正则扫描**（不解析、不递归），并且一律用 createTextNode /
 * createElement 落树：值里带 `<script>` 也只是文本。超过阈值就退回纯文本——
 * 高亮是给人看的，为几十万字符的 payload 卡住整页不值。
 */
export const JSON_HL_MAX_CHARS = 200 * 1024
export const JSON_HL_MAX_LINES = 4000
// 机器内容超过这么多行就认为"限高装不下"，给「展开全文」按钮（19px 行高 × 18 行
// ≈ 340px，正好是 --code-scroll-h；这里略保守一点）。
export const IO_FOLD_LINES = 16

/* 文本是 JSON 就返回两空格缩进后的规范化文本，否则 null（非 JSON 不碰）。 */
export function jsonPretty(raw: unknown): string | null {
  const s = String(raw === undefined || raw === null ? '' : raw).trim()
  if (!s || (s.charAt(0) !== '{' && s.charAt(0) !== '[')) return null
  try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return null }
}

export function countLines(text: unknown): number {
  return String(text || '').split('\n').length
}

export function jsonTooBig(text: string): boolean {
  return text.length > JSON_HL_MAX_CHARS || countLines(text) > JSON_HL_MAX_LINES
}

/* 词法着色：键名 / 字符串 / 字面量 / 数字，其余原样进文本节点。 */
export function appendJSONSpans(parent: HTMLElement, text: string): void {
  const re = /("(?:\\.|[^"\\])*")\s*:|("(?:\\.|[^"\\])*")|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?)/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parent.appendChild(document.createTextNode(text.slice(last, m.index)))
    const cls = m[1] !== undefined ? 'k' : m[2] !== undefined ? 's' : m[3] !== undefined ? 'b' : 'n'
    parent.appendChild(el('span', cls, m[0]))
    last = m.index + m[0].length
  }
  if (last < text.length) parent.appendChild(document.createTextNode(text.slice(last)))
}

/*
 * 非 JSON 的机器文本（终端输出 / diff / 编译日志）的着色。全部只产出
 * span + 文本节点（先转义再构树在这里就是 createTextNode），规则尽量少：
 *   · 整行：`$ `/`# ` 命令行、diff 的 `+`/`-`/`@@`/文件头、LaTeX 的 `!` 错误行、
 *     Overfull/Underfull 警告；
 *   · 词级：error/FAIL/warning/OK/PASS… 状态词，URL 与文件路径（弱强调）。
 * 配色只用既有的 --err/--warn/--ok/--accent/--caption，不新增色板。
 */
export function consoleLineClass(line: string): string {
  if (/^\s*(\$|#)\s+\S/.test(line)) return 'cmd'
  if (/^\+\+\+|^---\s/.test(line)) return 'diffhead'
  if (/^@@/.test(line)) return 'hunk'
  if (/^\+/.test(line)) return 'add'
  if (/^-/.test(line)) return 'del'
  if (/^\s*!/.test(line)) return 'bad'
  if (/Overfull|Underfull/.test(line)) return 'warn'
  return ''
}

const CONSOLE_TOKENS = /(https?:\/\/[^\s"'<>]+)|((?:\.{0,2}\/|\/)[\w.\-]+\/[\w.\-/]*[\w.\-])|(\b(?:ERROR|Error|error|FAILED|FAIL|Failure|failed|FATAL|Fatal)\b)|(\b(?:WARNING|Warning|warning|WARN|Warn|OVERFULL|Overfull|UNDERFULL|Underfull)\b)|(\b(?:OK|PASS|PASSED|COMPILE OK|SUCCESS|Success|done)\b)/g

function appendConsoleLine(parent: HTMLElement, line: string): void {
  const whole = consoleLineClass(line)
  if (whole) { parent.appendChild(el('span', whole, line)); return }
  let last = 0
  let m: RegExpExecArray | null
  CONSOLE_TOKENS.lastIndex = 0
  while ((m = CONSOLE_TOKENS.exec(line)) !== null) {
    if (m.index > last) parent.appendChild(document.createTextNode(line.slice(last, m.index)))
    const cls = m[1] || m[2] ? 'path' : m[3] ? 'bad' : m[4] ? 'warn' : 'good'
    parent.appendChild(el('span', cls, m[0]))
    last = m.index + m[0].length
    if (m[0] === '') break
  }
  if (last < line.length) parent.appendChild(document.createTextNode(line.slice(last)))
}

export function appendConsoleSpans(parent: HTMLElement, text: unknown): void {
  String(text === undefined || text === null ? '' : text).split('\n').forEach((line, i) => {
    if (i) parent.appendChild(document.createTextNode('\n'))
    appendConsoleLine(parent, line)
  })
}

/*
 * 把一段机器文本放进 parent（<pre> 之类）：JSON 就做 JSON 高亮，否则按终端
 * 输出着色；两者都过阈值就退回纯文本。返回 { highlighted, note }。
 */
export function highlightMachine(parent: HTMLElement, raw: unknown): { highlighted: boolean; note: string } {
  const text = String(raw === undefined || raw === null ? '' : raw)
  const pretty = jsonPretty(text)
  if (pretty === null) {
    if (jsonTooBig(text)) {
      parent.textContent = text
      return {
        highlighted: false,
        note: '内容过大（' + fmtChars(text.length) + '），已按纯文本显示，不做高亮'
      }
    }
    appendConsoleSpans(parent, text)
    return { highlighted: true, note: '' }
  }
  if (jsonTooBig(pretty)) {
    parent.textContent = pretty
    return {
      highlighted: false,
      note: '内容过大（' + fmtChars(pretty.length) + '），已按纯文本显示，不做 JSON 高亮'
    }
  }
  appendJSONSpans(parent, pretty)
  return { highlighted: true, note: '' }
}

/* 不带折叠的机器文本块（详情栏的 parameters、轨迹的输入输出）。 */
export function machineBlock(text: unknown, cls?: string): DocumentFragment {
  const frag = document.createDocumentFragment()
  const pre = el('pre', cls || 'code')
  const res = highlightMachine(pre, text)
  frag.appendChild(pre)
  if (res.note) frag.appendChild(el('div', 'note', res.note))
  return frag
}

/*
 * 折叠/展开按钮的唯一文案来源：思考块、工具输入输出、系统消息（user 轮任务提示）
 * 三处**逐字一致**，不另造说法。数字随显示单位开关走（token 是本地估算 → 带 ≈）。
 */
export function foldLabel(expanded: boolean, lines: number, chars: number, tokens?: number): string {
  return expanded ? '收起' : '展开全文（' + lines + ' 行 / ' + countText(chars, tokens) + '）'
}

/*
 * 工具输入输出卡片的正文：**与思考块同一套**——固定高度内滚（--code-scroll-h，
 * 与 reasoning-scroll 同值），内容超出限高时给「展开全文（N 行 / M 字符）」，
 * 点掉高度限制直接看全文，再点「收起」（按钮文案与折叠记忆键都沿用既有的那套）。
 * 高亮与 Markdown 开关无关：这里永远高亮，关掉 Markdown 也一样。
 */
export function machineScroll(text: unknown, key: string, extraClass?: string, tokens?: number): HTMLElement {
  const wrap = el('div', 'text-wrap')
  machineScrollInto(wrap, text, key, extraClass, tokens)
  return wrap
}

/*
 * 同 machineScroll，但把内容填进**调用方提供的** .text-wrap 宿主（组件的根
 * 节点自己当 text-wrap，DOM 链条与命令式版本逐层一致，不多出包装层）。
 */
export function machineScrollInto(host: HTMLElement, text: unknown, key: string, extraClass?: string, tokens?: number): void {
  const wrap = host
  const raw = String(text === undefined || text === null ? '' : text)
  const pretty = jsonPretty(raw)
  const body = pretty === null ? raw : pretty
  const big = jsonTooBig(body)
  const lines = body.split('\n')
  const scroll = el('div', 'io-scroll' + (extraClass ? ' ' + extraClass : ''))
  const pre = el('pre', 'io-text')
  let expanded = storeGet('text.' + key) === '1'
  const long = lines.length > IO_FOLD_LINES
  const paint = () => {
    if (big) pre.textContent = body
    else if (pretty !== null) appendJSONSpans(pre, body)
    else appendConsoleSpans(pre, body)
    scroll.classList.toggle('open', expanded)
  }
  paint()
  scroll.appendChild(pre)
  wrap.appendChild(scroll)
  if (big) {
    // 这条说明的依据是"字符数越过了高亮上限"，所以有 token 估算就按当前
    // 显示单位报，没有就退回字符数（KB 形态），两种情况都说清是什么过大。
    const size = tokens ? countText(body.length, tokens) : fmtChars(body.length)
    wrap.appendChild(el('div', 'note', '内容过大（' + size + '），按纯文本显示，不做高亮'))
  }
  if (long) {
    const toggle = el('button', 'text-toggle') as HTMLButtonElement
    toggle.type = 'button'
    const label = () => {
      toggle.textContent = foldLabel(expanded, lines.length, body.length, tokens)
    }
    label()
    toggle.addEventListener('click', () => {
      expanded = !expanded
      storeSet('text.' + key, expanded ? '1' : '0')
      paint()
      label()
    })
    wrap.appendChild(toggle)
  }
}

export function copyButton(text: string): HTMLButtonElement {
  const btn = el('button', 'text-toggle', '复制') as HTMLButtonElement
  btn.type = 'button'
  btn.addEventListener('click', () => {
    const done = () => { btn.textContent = '已复制'; setTimeout(() => { btn.textContent = '复制' }, 1200) }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, () => { if (fallbackCopy(text)) done() })
    } else if (fallbackCopy(text)) {
      done()
    }
  })
  return btn
}

export function fallbackCopy(text: string): boolean {
  try {
    const area = document.createElement('textarea')
    area.value = text
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(area)
    return ok
  } catch {
    return false
  }
}

/* ---------- Markdown 块级解析（自带的小渲染器，无外部依赖） ---------- */

/*
 * 消息正文支持 Markdown 预览：**自己写的小渲染器**，不引任何库、不加任何外链。
 * 行内规则（链接 / 粗斜体 / 行内代码 / 数学）在 markdown.ts 的 mdInline；
 * 这里只做块级：标题 `#`~`######`、围栏代码块（带语言标签与复制按钮）、
 * 有序/无序列表（一级嵌套）、引用、水平线、`|` 表格、段落与软换行。
 */

export function mdFence(line: string): { mark: string; lang: string } | null {
  const m = /^\s{0,3}(`{3,}|~{3,})\s*([^\s`]*)\s*$/.exec(line)
  return m ? { mark: m[1].charAt(0), lang: m[2] || '' } : null
}

export function mdListMarker(line: string): { indent: number; ordered: boolean; text: string } | null {
  const m = /^([ \t]*)([-*+]|\d{1,3}[.)])\s+(.*)$/.exec(line)
  if (!m) return null
  return {
    indent: m[1].replace(/\t/g, '  ').length,
    ordered: /\d/.test(m[2]),
    text: m[3]
  }
}

export const MD_HR = /^(?:\*\s*){3,}$|^(?:-\s*){3,}$|^(?:_\s*){3,}$/
export const MD_HEADING = /^(#{1,6})\s+(.*?)\s*#*\s*$/

// 表格分隔行（`|---|:--:|` 之类）；单独一行 `---` 是水平线，不走这里。
export function mdSeparatorRow(line: unknown): boolean {
  const s = String(line || '').trim()
  if (s.indexOf('-') < 0 || s.indexOf('|') < 0) return false
  return /^\|?[\s:|-]+\|?$/.test(s)
}

export function mdSplitRow(line: string): string[] {
  const s = String(line || '').trim().replace(/^\|/, '').replace(/\|$/, '')
  return s.split('|').map((c) => c.trim())
}

// 这一行会不会开启一个新块？（段落收集时用来判断在哪里停下）
export function mdBlockStart(line: unknown, next: string | undefined): boolean {
  const t = String(line || '').trim()
  if (!t) return true
  if (mdFence(t) || MD_HEADING.test(t) || MD_HR.test(t)) return true
  if (t.startsWith('$$')) return true // 显示数学块
  if (/^\s{0,3}>/.test(String(line)) || mdListMarker(String(line))) return true
  return t.indexOf('|') >= 0 && !!next && mdSeparatorRow(next) && mdSplitRow(t).length > 1
}

/*
 * LaTeX 数学 → MathML（自写、零依赖、零外链；浏览器原生排版 MathML Core）。
 * $…$ 行内、$$…$$ 独立显示。支持的写法覆盖转录里真实出现的那些：
 *   · 结构：{…} 分组、^ _ 上下标（大算符自动转 munderover）、\frac \dfrac \tfrac
 *     \sqrt[n]{}、\left…\right 可伸缩定界符、\overline \underline \hat \vec \bar
 *     \tilde \dot \ddot、\text \mathrm \mathbf \mathbb \mathcal \mathit \mathtt
 *     \operatorname、\quad \qquad \, \; \: \!、矩阵族 \begin{matrix|pmatrix|
 *     bmatrix|vmatrix|cases|aligned|array}（& 分列、\\ 分行）；
 *   · 符号：希腊字母、关系符、二元算符、箭头、\sum \prod \int \oint \bigcup…
 *     \lim \sin \cos \log \ln \exp \det \gcd 等常见函数名。
 * 规则：**认不出来的宏不吞掉、也不留空**——按 \name 原样放进 <mtext> 继续排版，
 * 于是页面上看到的是"大部分排好了 + 那个宏原文"，比整条公式退化成纯文本有用。
 * 一律用 createElementNS 造 MathML（createElement('math') 只会得到 HTMLUnknownElement，
 * 浏览器不会排版），所有文本走 createTextNode，与正文共用"从不拼 HTML"的边界。
 */
const MATHML_NS = 'http://www.w3.org/1998/Math/MathML'
const MATH_CHARS: Record<string, string> = {
  alpha: '\u03b1', beta: '\u03b2', gamma: '\u03b3', delta: '\u03b4',
  epsilon: '\u03b5', varepsilon: '\u03b5', zeta: '\u03b6', eta: '\u03b7',
  theta: '\u03b8', vartheta: '\u03d1', iota: '\u03b9', kappa: '\u03ba',
  lambda: '\u03bb', mu: '\u03bc', nu: '\u03bd', xi: '\u03be', pi: '\u03c0',
  varpi: '\u03d6', rho: '\u03c1', sigma: '\u03c3', varsigma: '\u03c2',
  tau: '\u03c4', upsilon: '\u03c5', phi: '\u03c6', varphi: '\u03d5',
  chi: '\u03c7', psi: '\u03c8', omega: '\u03c9',
  Gamma: '\u0393', Delta: '\u0394', Theta: '\u0398', Lambda: '\u039b',
  Xi: '\u039e', Pi: '\u03a0', Sigma: '\u03a3', Upsilon: '\u03a5',
  Phi: '\u03a6', Psi: '\u03a8', Omega: '\u03a9'
}
const MATH_OPS: Record<string, string> = {
  pm: '\u00b1', mp: '\u2213', times: '\u00d7', div: '\u00f7', cdot: '\u22c5',
  ast: '\u2217', star: '\u22c6', circ: '\u2218', bullet: '\u2219',
  le: '\u2264', leq: '\u2264', ge: '\u2265', geq: '\u2265', ne: '\u2260',
  neq: '\u2260', approx: '\u2248', equiv: '\u2261', sim: '\u223c',
  simeq: '\u2243', cong: '\u2245', propto: '\u221d', ll: '\u226a', gg: '\u226b',
  'in': '\u2208', notin: '\u2209', ni: '\u220b', subset: '\u2282',
  subseteq: '\u2286', supset: '\u2283', supseteq: '\u2287',
  cup: '\u222a', cap: '\u2229', setminus: '\u2216', emptyset: '\u2205',
  varnothing: '\u2205', forall: '\u2200', exists: '\u2203', nexists: '\u2204',
  neg: '\u00ac', land: '\u2227', wedge: '\u2227', lor: '\u2228',
  vee: '\u2228', oplus: '\u2295', otimes: '\u2297', perp: '\u22a5',
  parallel: '\u2225', angle: '\u2220', triangle: '\u25b3', square: '\u25a1',
  to: '\u2192', rightarrow: '\u2192', leftarrow: '\u2190',
  leftrightarrow: '\u2194', Rightarrow: '\u21d2', Leftarrow: '\u21d0',
  Leftrightarrow: '\u21d4', mapsto: '\u21a6', implies: '\u27f9',
  iff: '\u27fa', uparrow: '\u2191', downarrow: '\u2193',
  infty: '\u221e', partial: '\u2202', nabla: '\u2207', ell: '\u2113',
  hbar: '\u210f', imath: '\u0131', jmath: '\u0237', Re: '\u211c',
  Im: '\u2111', aleph: '\u2135', wp: '\u2118', prime: '\u2032',
  dots: '\u2026', ldots: '\u2026', cdots: '\u22ef', vdots: '\u22ee',
  ddots: '\u22f1', cases: '{', lbrace: '{', rbrace: '}',
  langle: '\u27e8', rangle: '\u27e9', lceil: '\u2308', rceil: '\u2309',
  lfloor: '\u230a', rfloor: '\u230b', vert: '|', Vert: '\u2016',
  backslash: '\\', dagger: '\u2020', ddagger: '\u2021', S: '\u00a7',
  therefore: '\u2234', because: '\u2235', checkmark: '\u2713',
  mid: '\u2223', nmid: '\u2224', bmod: 'mod', pmod: 'mod'
}
const MATH_LETTER_OPS: Record<string, string> = {
  sum: '\u2211', prod: '\u220f', coprod: '\u2210', int: '\u222b',
  iint: '\u222c', iiint: '\u222d', oint: '\u222e', bigcup: '\u22c3',
  bigcap: '\u22c2', bigoplus: '\u2a01', bigotimes: '\u2a02',
  bigvee: '\u22c1', bigwedge: '\u22c0', lim: 'lim', limsup: 'lim sup',
  liminf: 'lim inf', sup: 'sup', inf: 'inf', max: 'max', min: 'min',
  det: 'det', gcd: 'gcd', argmax: 'arg max', argmin: 'arg min'
}
const MATH_FUNCS: Record<string, number> = {
  sin: 1, cos: 1, tan: 1, cot: 1, sec: 1, csc: 1, arcsin: 1, arccos: 1,
  arctan: 1, sinh: 1, cosh: 1, tanh: 1, coth: 1, log: 1, ln: 1, lg: 1,
  exp: 1, deg: 1, dim: 1, ker: 1, hom: 1, Pr: 1, sgn: 1, mod: 1
}
const MATH_BB: Record<string, string> = {
  R: '\u211d', N: '\u2115', Z: '\u2124', Q: '\u211a', C: '\u2102',
  P: '\u2119', H: '\u210d', E: '\u1d53c', F: '\u1d53d', A: '\uFFFD\uFFFD',
  B: '\uFFFD\uFFFD', D: '\uFFFD\uFFFD', K: '\u1d542', L: '\u1d53e',
  M: '\u1d544', S: '\uFFFD\uFFFD', U: '\uFFFD\uFFFD', V: '\uFFFD\uFFFD',
  W: '\uFFFD\uFFFD', X: '\uFFFD\uFFFD', Y: '\uFFFD\uFFFD'
}
const MATH_MATRIX_ENVS: Record<string, string> = {
  matrix: '', pmatrix: '()', bmatrix: '[]', vmatrix: '||',
  Bmatrix: '{}', cases: '{', aligned: '', align: '', gathered: '',
  array: '', split: ''
}
const MATH_SPACES: Record<string, string> = {
  ',': '0.167em', ':': '0.222em', ';': '0.278em', '!': '-0.167em',
  quad: '1em', qquad: '2em', thinspace: '0.167em', medspace: '0.222em',
  thickspace: '0.278em', negthinspace: '-0.167em', space: '0.333em'
}
const MATH_ACCENTS: Record<string, string> = {
  hat: '\u02c6', widehat: '\u02c6', bar: '\u00af', overline: '\u00af',
  vec: '\u20d7', tilde: '\u02dc', widetilde: '\u02dc', dot: '\u02d9',
  ddot: '\u00a8', acute: '\u00b4', grave: '`', check: '\u02c7',
  breve: '\u02d8', mathring: '\u02da'
}
const MATH_FONTS: Record<string, string> = {
  mathrm: 'normal', mathbf: 'bold', boldsymbol: 'bold', mathit: 'italic',
  mathsf: 'sans-serif', mathtt: 'monospace', mathcal: 'script',
  mathfrak: 'fraktur', mathbb: 'double-struck', mathnormal: 'italic'
}

export function mathEl(tag: string): Element { return document.createElementNS(MATHML_NS, tag) }

export function mathText(tag: string, str: unknown): Element {
  const n = mathEl(tag)
  n.appendChild(document.createTextNode(String(str)))
  return n
}

export function mathSymbol(ch: string): Element {
  const n = mathEl('mo')
  n.appendChild(document.createTextNode(ch))
  n.setAttribute('stretchy', 'false')
  return n
}

// 把 LaTeX 数学串编成 MathML 子树；display=true 时用块级 <math display="block">。
// 单条公式解析失败就抛错：调用方（markdown.ts 的 mdInline）就地退回原始 $…$
// 文本，一条坏公式不许打断整页渲染。
export function mdMathML(tex: unknown, display: boolean): Element {
  const src = String(tex === undefined || tex === null ? '' : tex)
  let pos = 0
  const root = mathEl('math')
  if (display) root.setAttribute('display', 'block')
  root.setAttribute('class', display ? 'md-math md-math-block' : 'md-math')

  function isSpace(c: string): boolean { return c === ' ' || c === '\t' || c === '\n' }
  function skipSpaces(): void { while (pos < src.length && isSpace(src[pos])) pos += 1 }
  function atEndMarker(): boolean { return src.charAt(pos) === '\\' && /^\\end\b/.test(src.slice(pos)) }
  function isLetter(c: string): boolean { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
  function isDigit(c: string): boolean { return c >= '0' && c <= '9' }

  function readBraceText(): string {
    skipSpaces()
    if (src.charAt(pos) !== '{') return ''
    let depth = 0, out = ''
    const start = pos
    for (; pos < src.length; pos += 1) {
      const c = src[pos]
      if (c === '{') { depth += 1; if (depth === 1) continue }
      else if (c === '}') { depth -= 1; if (depth === 0) { pos += 1; return out } }
      out += c
    }
    pos = start
    return ''
  }

  function readCommandName(): string {
    // pos 停在 '\' 上
    pos += 1
    if (pos >= src.length) return '\\'
    if (isLetter(src[pos])) {
      const s = pos
      while (pos < src.length && isLetter(src[pos])) pos += 1
      return src.slice(s, pos)
    }
    pos += 1
    return src[pos - 1]
  }

  function parseGroup(): Element | null {
    skipSpaces()
    if (src.charAt(pos) === '{') {
      pos += 1
      const row = parseExpr()
      skipSpaces()
      if (src.charAt(pos) === '}') pos += 1
      return row
    }
    return parseAtom()
  }

  // 带参数的宏都吃一个参数组；组缺席（写到串尾）时抛错——调用方就地退回
  // 原始 $…$ 文本，与旧页 appendChild(null) 抛 TypeError 走同一条退路。
  function argGroup(): Element {
    const g = parseGroup()
    if (!g) throw new Error('mdMathML: 参数组缺失')
    return g
  }

  function attachScript(row: Element, sup: boolean): void {
    pos += 1
    const arg = argGroup()
    let base: Node | null = row.lastChild
    if (base) row.removeChild(base)
    else base = mathEl('mrow')
    const prev = base.nodeName
    let isBig = prev === 'mo' && (base as Element).getAttribute('largeop') === 'true'
    // 已经带上一个角标的大算符（munder/mover 且底下是 largeop）也算大算符，
    // 否则 \sum_{i=1}^{n} 的 ^ 会接成 msup，看不到"上下限分居上下"的排版
    if (!isBig && (prev === 'munder' || prev === 'mover') && base.firstChild &&
        base.firstChild.nodeName === 'mo' &&
        (base.firstChild as Element).getAttribute('largeop') === 'true') {
      isBig = true
    }
    let tag: string
    if (isBig) {
      // 大算符的上下限要分居上下：先 _ 再 ^（或反之）累成 munderover
      if (prev === 'munder' && sup || prev === 'mover' && !sup) tag = 'munderover'
      else tag = sup ? 'mover' : 'munder'
    } else if (prev === 'msub' || prev === 'msup') {
      tag = 'msubsup'
    } else {
      tag = sup ? 'msup' : 'msub'
    }
    const n = mathEl(tag)
    if (tag === 'msubsup' || tag === 'munderover') {
      // 上一层的两个子节点恰好就是"基 + 已有的那个角标"
      n.appendChild(base.firstChild!)
      n.appendChild(base.lastChild!)
    } else {
      n.appendChild(base)
    }
    n.appendChild(arg)
    row.appendChild(n)
  }

  function parseMathTable(env: string): Element {
    const table = mathEl('mtable')
    let row = mathEl('mtr')
    let guard = 0
    while (pos < src.length && guard++ < 2000) {
      skipSpaces()
      if (src.charAt(pos) === '\\' && src.substr(pos, 2) === '\\\\') {
        pos += 2
        table.appendChild(row)
        row = mathEl('mtr')
        continue
      }
      if (atEndMarker()) break
      if (src.charAt(pos) === '\\' && /^\\hline\b/.test(src.slice(pos))) {
        readCommandName()
        continue
      }
      if (src.charAt(pos) === '&') {
        pos += 1
        continue
      }
      const cell = parseExpr()
      const td = mathEl('mtd')
      td.appendChild(cell)
      row.appendChild(td)
      if (pos >= src.length) break
    }
    if (row.childNodes.length) table.appendChild(row)
    const delims = MATH_MATRIX_ENVS[env]
    if (!delims) return table
    const wrap = mathEl('mrow')
    const left = mathEl('mo'), right = mathEl('mo')
    left.setAttribute('stretchy', 'true')
    right.setAttribute('stretchy', 'true')
    left.appendChild(document.createTextNode(delims.charAt(0)))
    right.appendChild(document.createTextNode(delims.charAt(1)))
    wrap.appendChild(left)
    wrap.appendChild(table)
    wrap.appendChild(right)
    return wrap
  }

  function parseCommand(): Element | null {
    const name = readCommandName()
    if (name === '\\') {
      const br = mathEl('mspace')
      br.setAttribute('linebreak', 'newline')
      return br
    }
    if (Object.prototype.hasOwnProperty.call(MATH_SPACES, name)) {
      const sp = mathEl('mspace')
      sp.setAttribute('width', MATH_SPACES[name])
      return sp
    }
    if (name === 'frac' || name === 'dfrac' || name === 'tfrac') {
      const f = mathEl('mfrac')
      f.appendChild(argGroup())
      f.appendChild(argGroup())
      return f
    }
    if (name === 'sqrt') {
      skipSpaces()
      let root2: Element
      if (src.charAt(pos) === '[') {
        // 度数直接按字符读（不能用 parseExpr：它会把 ] 和后面的组一起吃掉，
        // 于是随后的 parseGroup 拿到 null、appendChild 抛错，整条公式退化）
        let close = src.indexOf(']', pos + 1)
        if (close < 0) close = src.length
        const degSrc = src.slice(pos + 1, close)
        pos = close + 1
        const deg = mathEl('mrow')
        for (let di = 0; di < degSrc.length; di += 1) {
          const dc = degSrc.charAt(di)
          if (isSpace(dc)) continue
          deg.appendChild(isDigit(dc) ? mathText('mn', dc)
            : isLetter(dc) ? mathText('mi', dc) : mathSymbol(dc))
        }
        root2 = mathEl('mroot')
        root2.appendChild(argGroup())
        root2.appendChild(deg)
        return root2
      }
      root2 = mathEl('msqrt')
      root2.appendChild(argGroup())
      return root2
    }
    if (name === 'left' || name === 'right' || name === 'big' || name === 'Big' ||
        name === 'bigl' || name === 'bigr' || name === 'Bigl' || name === 'Bigr' ||
        name === 'biggl' || name === 'biggr' || name === 'Biggl' || name === 'Biggr') {
      skipSpaces()
      let d = src.charAt(pos)
      if (d === '\\') {
        const dn = readCommandName()
        d = Object.prototype.hasOwnProperty.call(MATH_OPS, dn) ? MATH_OPS[dn] : dn
      } else {
        pos += 1
        if (d === '.') return mathEl('mspace')
      }
      const mo = mathEl('mo')
      mo.setAttribute('stretchy', name === 'left' || name === 'right' ? 'true' : 'false')
      mo.appendChild(document.createTextNode(d || ''))
      return mo
    }
    if (name === 'begin') {
      const env = readBraceText()
      if (!Object.prototype.hasOwnProperty.call(MATH_MATRIX_ENVS, env)) {
        return mathText('mtext', '\\begin{' + env + '}')
      }
      if (env === 'array') readBraceText()
      const built = parseMathTable(env)
      skipSpaces()
      if (atEndMarker()) {
        readCommandName()
        readBraceText()
      }
      return built
    }
    if (name === 'text' || name === 'textrm' || name === 'mbox' || name === 'operatorname') {
      const tx = mathText('mtext', readBraceText())
      if (name === 'operatorname') tx.setAttribute('mathvariant', 'normal')
      return tx
    }
    if (Object.prototype.hasOwnProperty.call(MATH_FONTS, name)) {
      const inner = argGroup()
      inner.setAttribute('mathvariant', MATH_FONTS[name])
      if (name === 'mathbb') {
        (function mapBB(node: Element): void {
          for (let i = 0; i < node.childNodes.length; i += 1) {
            const kid = node.childNodes[i]
            const ch = kid.textContent
            if (kid.nodeType === 3 && ch && ch.length === 1 && MATH_BB[ch]) {
              kid.textContent = MATH_BB[ch]
            } else if (kid.childNodes && kid.childNodes.length) {
              mapBB(kid as Element)
            }
          }
        }(inner))
      }
      return inner
    }
    if (Object.prototype.hasOwnProperty.call(MATH_ACCENTS, name)) {
      const acc = mathEl('mover')
      acc.setAttribute('accent', 'true')
      acc.appendChild(argGroup())
      const am = mathEl('mo')
      am.appendChild(document.createTextNode(MATH_ACCENTS[name]))
      acc.appendChild(am)
      return acc
    }
    if (name === 'underline') {
      const ul = mathEl('munder')
      ul.setAttribute('accentunder', 'true')
      ul.appendChild(argGroup())
      const um = mathEl('mo')
      um.appendChild(document.createTextNode('_'))
      ul.appendChild(um)
      return ul
    }
    if (name === 'overbrace' || name === 'underbrace' ||
        name === 'stackrel' || name === 'overset' || name === 'underset') {
      const ov = mathEl(name === 'underset' || name === 'underbrace' ? 'munder' : 'mover')
      ov.appendChild(argGroup())
      ov.appendChild(argGroup())
      return ov
    }
    if (name === 'displaystyle' || name === 'textstyle' || name === 'limits' ||
        name === 'nolimits' || name === 'nonumber' || name === 'notag' ||
        name === 'label' || name === 'tag') {
      if (name === 'label' || name === 'tag') readBraceText()
      return null
    }
    if (name === 'pmod' || name === 'pod') {
      const pm2 = mathEl('mrow')
      pm2.appendChild(mathEl('mspace'))
      pm2.appendChild(mathText('mtext', '('))
      pm2.appendChild(argGroup())
      pm2.appendChild(mathText('mtext', ')'))
      return pm2
    }
    if (Object.prototype.hasOwnProperty.call(MATH_LETTER_OPS, name)) {
      const big = MATH_LETTER_OPS[name]
      const isWord = !/[\u2200-\u22ff\u2a00-\u2aff]/.test(big)
      const bo = mathEl(isWord ? 'mi' : 'mo')
      bo.appendChild(document.createTextNode(big))
      if (!isWord) { bo.setAttribute('largeop', 'true'); bo.setAttribute('movablelimits', 'true') }
      else bo.setAttribute('mathvariant', 'normal')
      return bo
    }
    if (Object.prototype.hasOwnProperty.call(MATH_FUNCS, name)) {
      const fn = mathText('mi', name)
      fn.setAttribute('mathvariant', 'normal')
      return fn
    }
    if (Object.prototype.hasOwnProperty.call(MATH_CHARS, name)) {
      return mathText('mi', MATH_CHARS[name])
    }
    if (Object.prototype.hasOwnProperty.call(MATH_OPS, name)) {
      return mathSymbol(MATH_OPS[name])
    }
    // 认不出来：原样保留（不吞、不留空），继续排版其余部分
    return mathText('mtext', '\\' + name)
  }

  function parseAtom(): Element | null {
    skipSpaces()
    const c = src.charAt(pos)
    if (!c) return null
    if (c === '{') {
      pos += 1
      const g = parseExpr()
      if (src.charAt(pos) === '}') pos += 1
      return g
    }
    if (c === '\\') return parseCommand()
    if (isDigit(c) || c === '.' && isDigit(src.charAt(pos + 1))) {
      const s = pos
      while (pos < src.length && (isDigit(src[pos]) || src[pos] === '.')) pos += 1
      return mathText('mn', src.slice(s, pos))
    }
    if (isLetter(c)) {
      pos += 1
      return mathText('mi', c)
    }
    pos += 1
    if (c === '~') {
      const nb = mathEl('mspace')
      nb.setAttribute('width', '0.333em')
      return nb
    }
    return mathSymbol(c)
  }

  function parseExpr(): Element {
    const row = mathEl('mrow')
    let guard = 0
    while (pos < src.length && guard++ < 4000) {
      skipSpaces()
      const c = src.charAt(pos)
      if (!c || c === '}' || c === '&') break
      if (c === '\\' && (src.substr(pos, 2) === '\\\\' || atEndMarker())) break
      if (c === '^' || c === '_') { attachScript(row, c === '^'); continue }
      const atom = parseAtom()
      if (atom) row.appendChild(atom)
      else if (pos < src.length && src.charAt(pos) === c) pos += 1
    }
    return row
  }

  let body = parseExpr()
  if (body.childNodes.length === 1 && body.firstChild && body.firstChild.nodeName === 'mrow') {
    body = body.firstChild as Element
  }
  root.appendChild(body)
  return root
}

/*
 * 围栏代码块：顶部 banner（语言名 + 复制按钮）+ 等宽正文，圆角用 --radius，
 * 内容 11px/19px、单块最大高度内滚——与工具 IO 卡片同一套做法与同一套 token。
 */
export function mdCodeBlock(code: string, lang: string): HTMLElement {
  const wrap = el('div', 'md-code-block')
  const head = el('div', 'md-code-head')
  head.appendChild(el('span', 'md-code-lang', lang || 'text'))
  head.appendChild(copyButton(code))
  wrap.appendChild(head)
  const body = el('pre', 'md-code-body')
  const inner = el('code')
  // 代码块内容也走机器文本渲染：```json 做 JSON 高亮，```bash / ```diff / ```log
  // 按终端与 diff 着色，其余纯文本。
  const res = highlightMachine(inner, code)
  body.appendChild(inner)
  wrap.appendChild(body)
  if (res.note) wrap.appendChild(el('div', 'note', res.note))
  // T56 追加：```mermaid 块在下方挂图表预览（后端 /api/mermaid 渲染，零依赖）；
  // 静态快照没有 API，只显示代码块。
  if (String(lang || '').trim().toLowerCase() === 'mermaid' && mermaidPreviewAvailable()) {
    wrap.appendChild(mermaidPreviewBlock(code))
  }
  return wrap
}

export function mdTable(header: string[], rows: string[][]): HTMLElement {
  const wrap = el('div', 'md-table-wrap')
  const table = el('table', 'md-table')
  const thead = el('thead')
  const hrow = el('tr')
  header.forEach((cell) => {
    const th = el('th')
    mdInline(th, cell, 0)
    hrow.appendChild(th)
  })
  thead.appendChild(hrow)
  table.appendChild(thead)
  const tbody = el('tbody')
  rows.forEach((cells) => {
    const tr = el('tr')
    for (let c = 0; c < header.length; c++) {
      const td = el('td')
      mdInline(td, cells[c] === undefined ? '' : cells[c], 0)
      tr.appendChild(td)
    }
    tbody.appendChild(tr)
  })
  table.appendChild(tbody)
  wrap.appendChild(table)
  return wrap
}

export function mdListItem(item: { text: string }): HTMLElement {
  const li = el('li', 'md-item')
  mdInlineLines(li, item.text)
  return li
}

/*
 * 块级解析：围栏代码块 → 标题 → 水平线 → 表格 → 引用 → 列表 → 段落。
 * 返回 DocumentFragment（调用方直接 appendChild 即可）。
 */
/*
 * collectMathBlock 识别 $$ 起手的显示数学块（T58：跨行 $$…$$ 在行内
 * 正则逐行扫描下永远匹配不到，曾原样显示）。命中返回去壳源码与块后
 * 第一行下标；单行 $$…$$（长度 > 3 且同起同收）与多行两种形态都认。
 * 纯函数，便于单测（node 环境无 DOM，renderMarkdown 本体不可测）。
 */
export function collectMathBlock(lines: string[], i: number): { src: string; next: number } | null {
  const first = String(lines[i] || '').trim()
  if (!first.startsWith('$$')) return null
  if (first.length > 3 && first.endsWith('$$')) {
    const src = first.slice(2, -2)
    return src.trim() ? { src, next: i + 1 } : null // 裸 $$$$ 不是数学块
  }
  const buf: string[] = [first.slice(2)]
  let j = i + 1
  while (j < lines.length) {
    const l = lines[j]
    const k = l.indexOf('$$')
    if (k >= 0) {
      buf.push(l.slice(0, k))
      return { src: buf.join('\n'), next: j + 1 }
    }
    buf.push(l)
    j++
  }
  // 未闭合：按到文末收尾（与围栏代码块"没闭合吃到文末"同款宽容）。
  return { src: buf.join('\n'), next: j }
}

export function renderMarkdown(text: unknown): DocumentFragment {
  const frag = document.createDocumentFragment()
  const lines = String(text === undefined || text === null ? '' : text).replace(/\r\n?/g, '\n').split('\n')
  let i = 0

  while (i < lines.length) {
    const line = lines[i]
    const trimmed = line.trim()
    if (!trimmed) { i++; continue }

    // 围栏代码块（收尾围栏用同一个字符、长度不限；没闭合就吃到文末）
    const fence = mdFence(line)
    if (fence) {
      const code: string[] = []
      i++
      const close = new RegExp('^\\s{0,3}' + (fence.mark === '`' ? '`' : '~') + '{3,}\\s*$')
      while (i < lines.length && !close.test(lines[i])) { code.push(lines[i]); i++ }
      if (i < lines.length) i++
      frag.appendChild(mdCodeBlock(code.join('\n'), fence.lang))
      continue
    }

    // 显示数学块：$$ 起手的多行（或单行 $$…$$）整体走 MathML——行内
    // 正则逐行扫描永远匹配不到跨行 $$，曾让双 $ 公式原样显示（T58）。
    const mathBlock = collectMathBlock(lines, i)
    if (mathBlock) {
      const src = mathBlock.src
      i = mathBlock.next
      const div = el('div', 'md-math-display')
      try {
        div.appendChild(mdMathML(src, true))
      } catch {
        div.textContent = '$$' + src + '$$' // 坏公式退回原文
      }
      frag.appendChild(div)
      continue
    }

    const h = MD_HEADING.exec(trimmed)
    if (h) {
      const level = h[1].length
      const heading = el('h' + level, 'md-h md-h' + level)
      mdInlineLines(heading, h[2])
      frag.appendChild(heading)
      i++
      continue
    }

    if (MD_HR.test(trimmed)) {
      frag.appendChild(el('hr', 'md-hr'))
      i++
      continue
    }

    if (trimmed.indexOf('|') >= 0 && i + 1 < lines.length && mdSeparatorRow(lines[i + 1])) {
      const header = mdSplitRow(trimmed)
      if (header.length > 1) {
        const rows: string[][] = []
        i += 2
        while (i < lines.length && lines[i].trim() && lines[i].indexOf('|') >= 0) {
          rows.push(mdSplitRow(lines[i]))
          i++
        }
        frag.appendChild(mdTable(header, rows))
        continue
      }
    }

    if (/^\s{0,3}>/.test(line)) {
      const quote: string[] = []
      while (i < lines.length && /^\s{0,3}>/.test(lines[i])) {
        quote.push(lines[i].replace(/^\s{0,3}>\s?/, ''))
        i++
      }
      const bq = el('blockquote', 'md-quote')
      bq.appendChild(renderMarkdown(quote.join('\n')))
      frag.appendChild(bq)
      continue
    }

    if (mdListMarker(line)) {
      const items: { indent: number; ordered: boolean; text: string }[] = []
      while (i < lines.length) {
        const mk = mdListMarker(lines[i])
        if (mk) { items.push(mk); i++; continue }
        // 列表项的续行（懒续行）：非空、且不是新块开头，就并进上一条。
        if (items.length && lines[i].trim() && i + 1 <= lines.length &&
            !mdBlockStart(lines[i], lines[i + 1])) {
          items[items.length - 1].text += '\n' + lines[i].trim()
          i++
          continue
        }
        break
      }
      const baseIndent = items[0].indent
      let list: HTMLElement | null = null
      let listOrdered = false
      let lastLi: HTMLElement | null = null
      let sub: HTMLElement | null = null
      items.forEach((it) => {
        if (!list || (it.indent <= baseIndent && it.ordered !== listOrdered)) {
          list = el(it.ordered ? 'ol' : 'ul', 'md-list')
          listOrdered = it.ordered
          lastLi = null
          sub = null
          frag.appendChild(list)
        }
        if (it.indent > baseIndent && lastLi) {
          // 一级嵌套：挂在上一顶层项里面
          if (!sub) {
            sub = el(it.ordered ? 'ol' : 'ul', 'md-list md-sub')
            lastLi.appendChild(sub)
          }
          sub.appendChild(mdListItem(it))
          return
        }
        sub = null
        lastLi = mdListItem(it)
        list!.appendChild(lastLi)
      })
      continue
    }

    // 段落：吃到空行或下一个块的开头
    const buf: string[] = []
    while (i < lines.length && !mdBlockStart(lines[i], lines[i + 1])) { buf.push(lines[i]); i++ }
    if (!buf.length) { buf.push(lines[i]); i++ }
    const para = el('p', 'md-p')
    mdInlineLines(para, buf.join('\n'))
    frag.appendChild(para)
  }
  return frag
}
