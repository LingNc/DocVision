/*
 * 右侧详情栏（旧页 renderDetails 一族的**数据模型**，块 B 组件化后只剩纯函数）：
 * 会话信息 + 指标瓦片 + 元信息（系统提示词快照与工具清单）。DOM 一概由
 * DetailsPanel.vue 用模板渲染；这里的函数只产出可序列化的行/瓦片/段落数据。
 */
import { state, storeGet, fmtClock, fmtSize } from '../state'
import {
  aggregate, countText, estOf, fmtCost, fmtDur, fmtTokens, imageDisplayName, imageTipText,
  projectOf, sessionTitleOf, shortSHA, subPathOf, usageLines,
} from './sidebar'
import { jsonPretty } from './richtext'
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

/* 「会话」块：键值行。md 非空时该格按行内 Markdown 渲染（会话名，单行）。 */
export interface KVRow { k: string; v?: string; title?: string; mono?: boolean; md?: string }

export function sessionKVRows(): KVRow[] {
  const cur = state.current
  if (!cur) return []
  const rows: KVRow[] = [
    { k: '项目', v: cur.project || projectOf(cur.id), title: cur.id },
    { k: '阶段', v: cur.stageTitle || cur.stage || '—' },
    { k: '会话', md: sessionTitleOf(cur), title: cur.id, mono: true },
    { k: '文件', v: cur.name || '—', title: cur.path, mono: true },
    { k: '消息', v: cur.messages + ' 条 · ' + state.lines.length + ' 行' },
    { k: '大小', v: fmtSize(cur.size) },
    { k: '最后写入', v: fmtClock(cur.mtime) },
    ...(cur.sha ? [{ k: '会话哈希', v: cur.sha, title: '转录文件内容哈希（P10）——报障时引用它可精确定位当时的状态', mono: true }] : []),
  ]
  if (cur.imageName) rows.push({ k: '图片', v: imageDisplayName(cur), title: imageTipText(cur), mono: true })
  if (cur.imageFile) rows.push({ k: '图片文件', v: cur.imageFile, title: cur.imageName, mono: true })
  if (cur.imagePath) rows.push({ k: '图片路径', v: cur.imagePath, mono: true })
  if (cur.imageName && cur.page) rows.push({ k: '页码', v: '第 ' + cur.page + ' 页' })
  if (cur.imageName && cur.imageOrder) rows.push({ k: '顺序', v: '书内第 ' + cur.imageOrder + ' 张' })
  if (cur.imageName && cur.imageType) rows.push({ k: '类型', v: cur.imageType })
  if (cur.imageName && cur.imageCaption) rows.push({ k: '图注', v: cur.imageCaption })
  return rows
}

export function sessionSubPath(): string {
  const cur = state.current
  return cur ? subPathOf(cur.id) : ''
}

export interface Tile { label: string; value: string; title: string }

/* 「指标」块整包：null = 这个会话没有用量行、无可显示指标。 */
export function statsModel(): { sub: string; tiles: Tile[]; cols: string[]; rows: { cells: string[]; title: string }[]; rowCount: number } | null {
  const st = aggregate(usageLines(state.lines))
  if (!st) return null
  const sub = st.requests + ' 次请求 · ' + (st.streamed ? '流式' : '非流式') +
    (st.spanMs ? ' · 会话跨度 ' + fmtDur(st.spanMs) : '')

  const tiles: Tile[] = []
  tiles.push({
    label: '输入 tokens', value: fmtTokens(st.promptTokens),
    title: st.promptTokens + ' prompt tokens（含缓存命中 ' + st.cachedTokens + '）\n' +
      '厂商实测值：随请求发出的图片 token 已经包含在里面，不单列。',
  })
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
    tiles.push({ label: '图片 ' + imgs.count + ' 张', value, title: lines.join('\n') })
  }
  tiles.push({
    label: '缓存命中', value: st.promptTokens ? st.cacheHitPct.toFixed(0) + '%' : '—',
    title: '前缀缓存命中率 = Σcached_tokens / Σprompt_tokens（供应商未上报时为 —）',
  })
  tiles.push({
    label: '输出 tokens', value: fmtTokens(st.completionTokens),
    title: st.completionTokens + ' completion tokens' + (st.reasoningTokens ? '，其中思考 ' + st.reasoningTokens : ''),
  })
  tiles.push({ label: '平均首字', value: st.avgTtftMs ? fmtDur(st.avgTtftMs) : '—', title: '每请求"发出→第一个流式增量"的平均耗时' })
  tiles.push({
    label: '输出速度', value: (st.outputTps || 0).toFixed(1) + ' tok/s',
    title: '生成速度 = Σ输出 tokens / Σ(请求耗时 − 首字延迟)，不含排队与思考等待',
  })
  tiles.push({ label: '平均耗时', value: fmtDur(st.avgDurationMs), title: '每请求平均墙钟耗时（含思考与工具执行前后的等待）' })
  if (st.reasoningTokens) tiles.push({ label: '思考 tokens', value: fmtTokens(st.reasoningTokens), title: 'reasoning_tokens（思考链）' })
  const sessionCost = state.current && state.current.cost
  if (sessionCost) {
    tiles.push({
      label: '费用', value: fmtCost(sessionCost, false),
      title: '按配置里的 models.*.price 计算：未命中缓存的输入 × input + 命中缓存的输入 × cached + 输出 × output。' +
        '没配价格的模型不显示金额（¥0 会被读成"没花钱"）。',
    })
  }

  // 每次请求明细：五列 + 行悬浮说明（种类/模型/速度/结束原因收在 title）。
  const rows = st.perRequest.map((l: any) => {
    const one = l.stats
    const kindLabel = one.kind === 'compact' ? '上下文压缩摘要请求'
      : one.kind === 'nudge' ? '空回复后的推动请求（同轮续发，未重开会话）' : '普通对话回合'
    return {
      cells: [
        // T35：nudge 与同轮同号，请求明细里标成「#N·续」，输入 tokens 因
        // 工具表不计入而变小——不是上下文丢失/重开，悬浮说明里讲清。
        one.round ? '#' + one.round + (one.kind === 'nudge' ? '·续' : '') : '—',
        one.ttftMs ? fmtDur(one.ttftMs) : '—',
        one.durationMs ? fmtDur(one.durationMs) : '—',
        fmtTokens(one.promptTokens) +
          (one.promptTokens ? ' · ' + ((one.cachedTokens * 100) / one.promptTokens).toFixed(0) + '%' : ''),
        fmtTokens(one.completionTokens),
      ],
      title: (l.ts ? fmtClock(l.ts) + '\n' : '') + kindLabel +
        (one.kind ? '（kind=' + one.kind + '）' : '') +
        (one.model ? '\n模型 ' + one.model : '') +
        '\n输入 ' + one.promptTokens + ' tokens（缓存命中 ' + one.cachedTokens + '）' +
        (one.kind === 'nudge' ? '\n注：推动请求禁用了工具调用，部分厂商此时不把工具表计入输入 tokens，故数字比同轮前一次小——历史上下文是完整带上的。' : '') +
        '\n输出 ' + one.completionTokens + ' tokens' +
        (one.reasoningTokens ? '（其中思考 ' + one.reasoningTokens + '）' : '') +
        '\n输出速度 ' + (one.outputTps || 0).toFixed(1) + ' tok/s' +
        '\n结束原因 ' + (one.finish || '—'),
    }
  })
  return { sub, tiles, cols: ['回合', '首字', '耗时', '输入·缓存', '输出'], rows, rowCount: st.requests }
}

/* 工具定义（元信息卡内）：名字 + 描述/参数计数；parameters 的 JSON 默认收起。 */
export interface ToolSchema { name: string; desc: string; params: string; descCount: string; paramCount: string }

export function toolSchemas(line: any): ToolSchema[] {
  return (line.tools || []).map((t: any) => ({
    name: t.name || '(未命名工具)',
    desc: t.description ? String(t.description) : '',
    params: t.parameters ? String(t.parameters) : '',
    descCount: t.description ? '描述 ' + countText(String(t.description).length, t.descTokens) : '',
    paramCount: t.parameters ? 'schema ' + countText(String(t.parameters).length, t.paramTokens) : '',
  }))
}

export interface MetaModel {
  key: string
  open: boolean
  bits: string
  promptText: string
  copyText: string
  mode: 'json' | 'md' | 'plain'
  tools: ToolSchema[]
  toolsHead: string
  countNote: string
}

export function metaCardKey(line: any): string {
  return 'meta.' + (state.current ? state.current.id : '') + '.' + (line.system_sha || line.n)
}

/* 元信息卡模型：null = 这个会话没有 meta 行（不渲染「元信息」块）。 */
export function metaModel(): MetaModel | null {
  const m = metaState()
  if (!m) return null
  const line = m.line
  const bits = ['模型 ' + (line.model || '—')]
  if (line.session_label) bits.push('会话 ' + line.session_label)
  bits.push('sha ' + shortSHA(line.system_sha))
  bits.push(countText(m.promptChars, m.promptTokenEst))
  const promptText = String(line.text || '（这条 meta 行没有正文）')
  let mode: MetaModel['mode'] = 'plain'
  if (jsonPretty(promptText) !== null) mode = 'json'
  else if (state.markdown) mode = 'md'
  const tools = toolSchemas(line)
  return {
    key: metaCardKey(line),
    open: storeGet(metaCardKey(line)) === '1',
    bits: bits.join(' · '),
    promptText,
    copyText: String(line.text || ''),
    mode,
    tools,
    toolsHead: tools.length ? '工具定义 ' + tools.length + ' 个（parameters 的 JSON 默认收起）' : '工具定义 0 个',
    countNote: m.count > 1 ? '共 ' + m.count + ' 条，显示最新' : '',
  }
}

/* 元信息卡悬浮说明（多次运行 / 提示词变化各一条时显示最后一条）。 */
export const META_COUNT_TIP = '同一个转录里有 {n} 条 meta 行（多次运行 / 提示词变化各一条），这里显示最后一条。'
