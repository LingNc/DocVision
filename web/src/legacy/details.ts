/*
 * 右侧详情栏（旧页 renderDetails 一族照搬区，块 6）：会话信息 + 指标瓦片 +
 * 元信息（系统提示词快照与工具清单）。消息流里因此只剩对话本身。
 */
import { state, storeGet, storeSet, fmtClock, fmtSize } from '../state'
import {
  aggregate, countText, estOf, fmtCost, fmtDur, fmtTokens, imageDisplayName, imageTipText,
  projectOf, sessionTitleOf, subPathOf, usageLines,
} from './sidebar'
import { el, clear } from './dom'
import { machineBlock, jsonPretty, copyButton, renderMarkdown } from './richtext'
import { renderInlineMarkdown } from './markdown'
import { metaState } from './timeline'

function currentEstimate(): any {
  const id = state.current ? state.current.id : ''
  const list = state.sessions || []
  for (let i = 0; i < list.length; i++) {
    if (list[i].id === id && list[i].estimate) return list[i].estimate
  }
  return (state.current && state.current.estimate) || null
}

/* 整个会话的图片张数与图片 token 估算（详情栏"图片"瓦片用）。 */
function imageEstimate(lines: any[]): { count: number; tokens: number } {
  const sum = { count: 0, tokens: 0 }
  ;(lines || []).forEach((line) => {
    const est = estOf(line)
    sum.count += est.imageCount || 0
    sum.tokens += est.images || 0
  })
  return sum
}

function detailBlock(title: string): HTMLElement {
  const wrap = el('section', 'detail-block')
  if (title) wrap.appendChild(el('h3', 'detail-block-title', title))
  return wrap
}

/*
 * 详情栏的键值表。值可以是字符串，也可以是一个**已造好的节点**（会话名按
 * 行内 Markdown 渲染，交给这里的就是一个元素）；悬浮说明照旧只放纯文本。
 */
function kvList(pairs: any[]): HTMLElement {
  const dl = el('dl', 'detail-kv')
  pairs.forEach((p) => {
    if (p === null) return
    const dt = el('dt', null, p[0])
    const dd = el('dd', p[3] ? 'mono' : null)
    // 节点值直接接进来（会话名），字符串照旧走 textContent；undefined /
    // null 保持空单元格（不能落成字面量 "undefined"）。
    if (p[1] && p[1].nodeType) {
      dd.appendChild(p[1])
    } else if (p[1] !== undefined && p[1] !== null) {
      dd.textContent = p[1]
    }
    if (p[2]) dd.title = p[2]
    dl.appendChild(dt)
    dl.appendChild(dd)
  })
  return dl
}

function nameNode(tag: string, cls: string, name: string): HTMLElement {
  const node = el(tag, cls)
  node.appendChild(renderInlineMarkdown(name))
  return node
}

function detailsSessionBlock(): HTMLElement | null {
  const cur = state.current
  if (!cur) return null
  const block = detailBlock('会话')
  block.appendChild(kvList([
    ['项目', cur.project || projectOf(cur.id), cur.id],
    ['阶段', cur.stageTitle || cur.stage || '—'],
    ['会话', nameNode('span', 'detail-name', sessionTitleOf(cur)), cur.id, true],
    ['文件', cur.name || '—', cur.path, true],
    ['消息', cur.messages + ' 条 · ' + state.lines.length + ' 行'],
    ['大小', fmtSize(cur.size)],
    ['最后写入', fmtClock(cur.mtime)],
    cur.imageName ? ['图片', imageDisplayName(cur), imageTipText(cur), true] : null,
    cur.imageFile ? ['图片文件', cur.imageFile, cur.imageName, true] : null,
    cur.imagePath ? ['图片路径', cur.imagePath, null, true] : null,
    cur.imageName && cur.page ? ['页码', '第 ' + cur.page + ' 页'] : null,
    cur.imageName && cur.imageOrder ? ['顺序', '书内第 ' + cur.imageOrder + ' 张'] : null,
    cur.imageName && cur.imageType ? ['类型', cur.imageType] : null,
    cur.imageName && cur.imageCaption ? ['图注', cur.imageCaption] : null,
  ]))
  const sub = subPathOf(cur.id)
  if (sub) block.appendChild(el('div', 'stats-sub', '目录：' + sub))
  return block
}

function detailsStatsBlock(): HTMLElement | null {
  const st = aggregate(usageLines(state.lines))
  if (!st) return null
  const block = detailBlock('指标')
  const meta = el('div', 'stats-sub', st.requests + ' 次请求 · ' + (st.streamed ? '流式' : '非流式') +
    (st.spanMs ? ' · 会话跨度 ' + fmtDur(st.spanMs) : ''))
  block.appendChild(meta)

  const tiles = el('div', 'tiles')
  const tile = (label: string, value: string, title?: string) => {
    const t = el('div', 'tile')
    t.appendChild(el('div', 'tile-value', value))
    t.appendChild(el('div', 'tile-label', label))
    if (title) t.title = title
    tiles.appendChild(t)
  }
  tile('输入 tokens', fmtTokens(st.promptTokens),
    st.promptTokens + ' prompt tokens（含缓存命中 ' + st.cachedTokens + '）\n' +
    '厂商实测值：随请求发出的图片 token 已经包含在里面，不单列。')
  const imgs = imageEstimate(state.lines)
  if (imgs.count) {
    // 每张的平均值才是可以跟"每张多少 token"对比的量；总计放悬浮说明里
    // （图片多的会话总计会盖过"每张"这个真正有用的数字）。
    const est = currentEstimate()
    const per = Math.round(imgs.tokens / imgs.count)
    const rule = est ? est.rule : '本地估算'
    const lines = [imgs.count + ' 张图片的本地估算合计 ≈ ' + fmtTokens(imgs.tokens) +
      '（每张 ≈ ' + fmtTokens(per) + '）', '估算口径：' + rule]
    let value = '≈ ' + fmtTokens(per) + '/张'
    if (est && est.measuredPerImage) {
      // 实测值由厂商 prompt_tokens 推出（相邻两次请求的差值），与上面的
      // "输入 tokens"同源、不带 ≈。
      value = '实测 ' + fmtTokens(est.measuredPerImage) + '/张'
      lines.push('实测 ' + fmtTokens(est.measuredPerImage) + '/张：厂商 prompt_tokens 的相邻差值推出的每张均值' +
        (est.measuredSamples ? '（' + est.measuredSamples + ' 步 / ' + est.measuredImages + ' 张）' : ''))
      lines.push('本地估算每张 ≈ ' + fmtTokens(per) + '（口径：' + rule + '，本会话合计 ≈ ' + fmtTokens(imgs.tokens) + '）')
    } else {
      lines.push('没有可用的实测样本：本会话的用量行还不足以推出每张实测值（无用量行、或没有一次请求新增图片）')
    }
    lines.push('对照：上面的「输入 tokens」是厂商实测的 prompt_tokens，其中已经包含图片 token。')
    tile('图片 ' + imgs.count + ' 张', value, lines.join('\n'))
  }
  tile('缓存命中', st.promptTokens ? st.cacheHitPct.toFixed(0) + '%' : '—',
    '前缀缓存命中率 = Σcached_tokens / Σprompt_tokens（供应商未上报时为 —）')
  tile('输出 tokens', fmtTokens(st.completionTokens),
    st.completionTokens + ' completion tokens' + (st.reasoningTokens ? '，其中思考 ' + st.reasoningTokens : ''))
  tile('平均首字', st.avgTtftMs ? fmtDur(st.avgTtftMs) : '—', '每请求"发出→第一个流式增量"的平均耗时')
  tile('输出速度', (st.outputTps || 0).toFixed(1) + ' tok/s',
    '生成速度 = Σ输出 tokens / Σ(请求耗时 − 首字延迟)，不含排队与思考等待')
  tile('平均耗时', fmtDur(st.avgDurationMs), '每请求平均墙钟耗时（含思考与工具执行前后的等待）')
  if (st.reasoningTokens) tile('思考 tokens', fmtTokens(st.reasoningTokens), 'reasoning_tokens（思考链）')
  const sessionCost = state.current && state.current.cost
  if (sessionCost) {
    tile('费用', fmtCost(sessionCost, false),
      '按配置里的 models.*.price 计算：未命中缓存的输入 × input + 命中缓存的输入 × cached + 输出 × output。' +
      '没配价格的模型不显示金额（¥0 会被读成"没花钱"）。')
  }
  block.appendChild(tiles)

  const details = document.createElement('details')
  details.className = 'stats-details'
  details.open = true
  const sum = el('summary', 'schema-head')
  sum.appendChild(el('span', 'schema-name', '每次请求明细'))
  sum.appendChild(el('span', 'schema-meta', st.requests + ' 行'))
  details.appendChild(sum)

  const scroll = el('div', 'stats-scroll')
  const table = el('table', 'stats-table')
  const thead = el('tr')
  // 详情栏很窄（300–520px），所以每次请求只留最能说明问题的五列：首字 /
  // 耗时 / 输入（带缓存命中）/ 输出。种类、速度、结尾、模型进行内 tooltip。
  ;['回合', '首字', '耗时', '输入·缓存', '输出'].forEach((h) => {
    thead.appendChild(el('th', null, h))
  })
  table.appendChild(thead)
  st.perRequest.forEach((l: any) => {
    const one = l.stats
    const tr = el('tr')
    tr.className = 'req-row'
    const kindLabel = one.kind === 'compact' ? '上下文压缩摘要请求'
      : one.kind === 'nudge' ? '空回复后的强制文本请求' : '普通对话回合'
    tr.title = (l.ts ? fmtClock(l.ts) + '\n' : '') + kindLabel +
      (one.kind ? '（kind=' + one.kind + '）' : '') +
      (one.model ? '\n模型 ' + one.model : '') +
      '\n输入 ' + one.promptTokens + ' tokens（缓存命中 ' + one.cachedTokens + '）' +
      '\n输出 ' + one.completionTokens + ' tokens' +
      (one.reasoningTokens ? '（其中思考 ' + one.reasoningTokens + '）' : '') +
      '\n输出速度 ' + (one.outputTps || 0).toFixed(1) + ' tok/s' +
      '\n结束原因 ' + (one.finish || '—')
    const cell = (text: string) => {
      const td = el('td', null, text)
      tr.appendChild(td)
      return td
    }
    cell(one.round ? '#' + one.round : '—')
    cell(one.ttftMs ? fmtDur(one.ttftMs) : '—')
    cell(one.durationMs ? fmtDur(one.durationMs) : '—')
    cell(fmtTokens(one.promptTokens) +
      (one.promptTokens ? ' · ' + ((one.cachedTokens * 100) / one.promptTokens).toFixed(0) + '%' : ''))
    cell(fmtTokens(one.completionTokens))
    table.appendChild(tr)
  })
  scroll.appendChild(table)
  details.appendChild(scroll)
  block.appendChild(details)
  return block
}

/* 工具定义：名字 + 描述；parameters 的 JSON 收在二级折叠里（默认收起）。 */
function toolSchemaBlock(tool: any, index: number): HTMLDetailsElement {
  const d = document.createElement('details')
  d.className = 'tool-schema'
  const head = el('summary', 'schema-head')
  head.appendChild(el('span', 'schema-index', '#' + (index + 1)))
  head.appendChild(el('span', 'schema-name', tool.name || '(未命名工具)'))
  if (tool.description) {
    head.appendChild(el('span', 'schema-meta',
      '描述 ' + countText(String(tool.description).length, tool.descTokens)))
  }
  if (tool.parameters) {
    head.appendChild(el('span', 'schema-meta',
      'schema ' + countText(String(tool.parameters).length, tool.paramTokens)))
  }
  d.appendChild(head)

  const body = el('div', 'schema-body')
  if (tool.description) {
    body.appendChild(el('pre', 'body-text schema-desc', tool.description))
  }
  if (tool.parameters) {
    const pd = document.createElement('details')
    pd.className = 'schema-params'
    const ph = el('summary', 'schema-head')
    ph.appendChild(el('span', 'schema-name', 'parameters'))
    ph.appendChild(el('span', 'schema-meta', 'JSON · 默认收起'))
    pd.appendChild(ph)
    const pbody = el('div', 'schema-body')
    // 注入给模型的工具 parameters 就是 JSON：**高亮**。
    pbody.appendChild(machineBlock(String(tool.parameters), 'code'))
    const actions = el('div', 'row-actions')
    actions.appendChild(copyButton(String(tool.parameters)))
    pbody.appendChild(actions)
    pd.appendChild(pbody)
    body.appendChild(pd)
  } else {
    body.appendChild(el('div', 'note', '（这条工具定义没有记录 parameters）'))
  }
  d.appendChild(body)
  return d
}

function metaCardKey(line: any): string {
  return 'meta.' + (state.current ? state.current.id : '') + '.' + (line.system_sha || line.n)
}

function metaCard(): HTMLDetailsElement | null {
  const m = metaState()
  if (!m) { state.meta = null; return null }
  state.meta = m
  const line = m.line

  const card = document.createElement('details')
  card.className = 'disclosure meta-card'
  // 默认**收起**：系统提示词动辄几万字符，默认展开会把详情栏整个顶下去。
  // 展开状态按会话 + 提示词哈希记忆（用户展开过一次后保持）。
  card.open = storeGet(metaCardKey(line)) === '1'

  const head = el('summary')
  const slot = el('span', 'line-slot')
  slot.appendChild(el('span', 'line-caret'))
  head.appendChild(slot)
  head.appendChild(el('span', 'line-name', '系统提示词（本次运行快照，不参与回放）'))
  head.appendChild(el('span', 'line-sep'))
  const bits = ['模型 ' + (line.model || '—')]
  if (line.session_label) bits.push('会话 ' + line.session_label)
  bits.push('sha ' + shortSHA(line.system_sha))
  bits.push(countText(m.promptChars, m.promptTokenEst))
  head.appendChild(el('span', 'line-summary', bits.join(' · ')))
  card.appendChild(head)

  const body = el('div', 'schema-body')
  const scroll = el('div', 'prompt-scroll')
  const promptText = String(line.text || '（这条 meta 行没有正文）')
  if (jsonPretty(promptText) !== null) {
    // 提示词整段就是 JSON（少数会话会把配置当提示词发）→ 直接 JSON 高亮。
    scroll.appendChild(machineBlock(promptText, 'body-text prompt-text'))
  } else if (state.markdown) {
    // 系统提示词也按 Markdown 预览（与消息正文同一套渲染器与开关）；
    // 里面的 ```json 代码块由 mdCodeBlock 顺带做 JSON 高亮。
    const promptMD = el('div', 'md-body prompt-md')
    promptMD.appendChild(renderMarkdown(promptText))
    scroll.appendChild(promptMD)
  } else {
    scroll.appendChild(machineBlock(promptText, 'body-text prompt-text'))
  }
  body.appendChild(scroll)
  const actions = el('div', 'row-actions')
  actions.appendChild(copyButton(String(line.text || '')))
  body.appendChild(actions)

  const tools = line.tools || []
  const toolsWrap = el('div', 'meta-tools')
  toolsWrap.appendChild(el('div', 'meta-tools-head',
    tools.length ? '工具定义 ' + tools.length + ' 个（parameters 的 JSON 默认收起）' : '工具定义 0 个'))
  if (!tools.length) {
    toolsWrap.appendChild(el('div', 'note', '这条 meta 行没有记录工具定义。'))
  }
  tools.forEach((t: any, i: number) => { toolsWrap.appendChild(toolSchemaBlock(t, i)) })
  body.appendChild(toolsWrap)
  if (m.count > 1) {
    const note = el('div', 'note', '共 ' + m.count + ' 条，显示最新')
    note.title = '同一个转录里有 ' + m.count + ' 条 meta 行（多次运行 / 提示词变化各一条），这里显示最后一条。'
    body.appendChild(note)
  }
  card.appendChild(body)

  card.addEventListener('toggle', () => {
    storeSet(metaCardKey(line), card.open ? '1' : '0')
  })
  return card
}

function shortSHA(value: unknown): string {
  const s = String(value || '')
  if (!s) return '—'
  return s.length > 12 ? s.slice(0, 12) : s
}

/*
 * 详情栏渲染入口：会话信息 + 指标 + 元信息。流摘要里的「详情 ›」按钮、
 * 行内 ⓘ、表头 ⓘ 都走 renderDetails 的宿主（App.vue watch）。
 */
export function renderDetails(): void {
  const host = document.getElementById('details-body')
  if (!host) return
  clear(host)
  const cur = state.current
  if (!cur) {
    host.appendChild(el('div', 'note', '左侧选择一个会话后，这里显示它的指标与元信息。'))
    return
  }
  const blocks = [detailsSessionBlock(), detailsStatsBlock()]
  const meta = metaCard()
  if (meta) {
    const block = detailBlock('元信息')
    block.appendChild(meta)
    blocks.push(block)
  }
  blocks.forEach((b) => { if (b) host.appendChild(b) })
  if (!blocks[0] && !blocks[1] && !blocks[2]) {
    host.appendChild(el('div', 'note', '这个会话没有可显示的详情。'))
  }
}
