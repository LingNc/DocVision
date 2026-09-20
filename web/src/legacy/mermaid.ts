/*
 * T56 追加：mermaid 代码块的图表预览。零依赖——**不引 mermaid.js**，渲染走
 * 后端 `POST /api/mermaid`（mmdc 渲染 + 内容哈希缓存），前端只负责：
 *   · 从 md 文本提取 ```mermaid 围栏块（纯函数，可测）；
 *   · 按源码内容缓存结果（Map<source, entry>，轮询重渲不重复请求）；
 *   · 在代码块下方挂一个预览位：渲染中 → SVG（可点进灯箱放大）/ 失败静默
 *     回退成一行小字「预览不可用」。
 * 静态导出模式（state.staticMode）没有 API，调用方直接不挂预览位。
 * SVG 落树用 DOMParser 解析后 importNode，不碰 innerHTML（与 richtext 同一
 * 安全边界）；灯箱只认 <img>，所以点击时把 SVG 编成 data: URL 交给现有灯箱。
 */
import { state, openLightbox } from '../state'

/* 从 md 文本提取所有 ```mermaid 围栏块的源码（收尾围栏同字符、长度不限）。 */
export function extractMermaidBlocks(text: unknown): string[] {
  const out: string[] = []
  const lines = String(text === undefined || text === null ? '' : text).replace(/\r\n?/g, '\n').split('\n')
  const open = /^\s{0,3}(`{3,}|~{3,})\s*mermaid\s*$/i
  let i = 0
  while (i < lines.length) {
    const m = open.exec(lines[i])
    if (!m) { i++; continue }
    const mark = m[1][0]
    const close = new RegExp('^\\s{0,3}' + (mark === '`' ? '`' : '~') + '{3,}\\s*$')
    const code: string[] = []
    i++
    while (i < lines.length && !close.test(lines[i])) { code.push(lines[i]); i++ }
    if (i < lines.length) i++
    out.push(code.join('\n'))
  }
  return out
}

/* ---------- 内容缓存：源码相同只渲染一次（失败也缓存，不反复打 API） ---------- */

export interface MermaidCacheEntry { ok: boolean; svg: string }
export const mermaidCache = new Map<string, MermaidCacheEntry>()

/* 请求体唯一来源（测试可注入假 poster）。 */
export type MermaidPoster = (source: string) => Promise<MermaidCacheEntry>

async function defaultPoster(source: string): Promise<MermaidCacheEntry> {
  const resp = await fetch('/api/mermaid', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source }),
    cache: 'no-store',
  })
  const data = await resp.json().catch(() => null)
  if (data && data.ok && typeof data.svg === 'string' && data.svg) {
    return { ok: true, svg: data.svg }
  }
  return { ok: false, svg: '' }
}

/* 渲染一段 mermaid 源码：命中缓存直接返回；失败（ok:false / 网络错）记为失败。 */
export async function renderMermaid(source: string, poster?: MermaidPoster): Promise<MermaidCacheEntry> {
  const key = String(source || '')
  const hit = mermaidCache.get(key)
  if (hit) return hit
  const post = poster || defaultPoster
  let entry: MermaidCacheEntry
  try {
    entry = await post(key)
  } catch {
    entry = { ok: false, svg: '' }
  }
  mermaidCache.set(key, entry)
  return entry
}

/* SVG 文本 → 可挂树的元素（DOMParser 解析，绝不 innerHTML）。解析失败返回 null。 */
export function svgElement(svg: string): SVGSVGElement | null {
  try {
    const doc = new DOMParser().parseFromString(svg, 'image/svg+xml')
    const node = doc.documentElement
    if (!node || node.tagName.toLowerCase() !== 'svg') return null
    if (node.getElementsByTagName('parsererror').length) return null
    return document.importNode(node, true) as unknown as SVGSVGElement
  } catch {
    return null
  }
}

/*
 * 代码块下方的预览位：初始「渲染中…」，成功后内联 SVG（宽度自适应），
 * 点击进现有灯箱（SVG 编成 data: URL 喂 <img>，滚轮缩放/拖动/复位全套沿用）；
 * 失败静默——只留一行小字「预览不可用」，代码块本体不受影响。
 */
export function mermaidPreviewBlock(source: string): HTMLElement {
  const wrap = document.createElement('div')
  wrap.className = 'md-mermaid'
  const note = document.createElement('div')
  note.className = 'md-mermaid-note'
  note.textContent = '渲染中…'
  wrap.appendChild(note)
  void renderMermaid(source).then((entry) => {
    // 组件可能已卸载/重渲，预览位不在树上就别再动它。
    if (!wrap.isConnected) return
    if (!entry.ok) {
      note.textContent = '预览不可用'
      return
    }
    const svgNode = svgElement(entry.svg)
    if (!svgNode) {
      note.textContent = '预览不可用'
      return
    }
    wrap.removeChild(note)
    const holder = document.createElement('div')
    holder.className = 'md-mermaid-svg'
    holder.title = '点击放大查看'
    holder.appendChild(svgNode)
    holder.addEventListener('click', () => {
      openLightbox('data:image/svg+xml;charset=utf-8,' + encodeURIComponent(entry.svg), 'mermaid 图表')
    })
    wrap.appendChild(holder)
  })
  return wrap
}

/* 静态快照没有 /api/mermaid：调用方据此决定挂不挂预览位。 */
export function mermaidPreviewAvailable(): boolean {
  return !state.staticMode
}
