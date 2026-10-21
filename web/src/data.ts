/*
 * 数据层：/api/index 轮询（2 秒、带列表签名守卫）与 /api/session 增量拉取。
 * 旧页 refreshIndex/applyIndex/selectSession/pullSession 的移植；渲染由
 * Timeline.vue / DetailsPanel.vue 的 computed 响应式接管，这里只管数据流。
 */
import { state, setBannerText } from './state'
import { findSession, normalizeLines, indexCallEstimates, groupKey, setCollapsed } from './legacy/sidebar'
import type { PartialInfo } from './legacy/types'

let pullSeq = 0

/*
 * 面包屑点项目名 → 侧栏里定位：展开装着该项目的组（不写"用户展开过"的记忆，
 * 与旧页一致：这里调 setCollapsed(key,false) 记录的是展开）并滚动到位。
 */
export function revealProject(project: string): void {
  const wraps = document.querySelectorAll('.proj-group')
  let hit: HTMLDetailsElement | null = null
  wraps.forEach((w) => {
    const el = w as HTMLDetailsElement
    const summary = el.querySelector('.proj-row') as HTMLElement | null
    if (!hit && summary && summary.title === project) hit = el
  })
  const target = hit as HTMLDetailsElement | null
  if (!target) return
  if (!target.open) {
    target.open = true
    target.dataset.open = '1'
    setCollapsed(groupKey('proj', project), false)
  }
  target.scrollIntoView({ block: 'nearest' })
}

/* P57：fork 之后自动切到新会话。排队只是写一个文件，新尝试是 latex 那边建的，
 * 所以这里记一个"等它出现"的守望条件：下一轮 /api/index 里出现更晚的、同项目
 * 同章节同角色的一条转录，就把它设为当前（用户口径："自动切换这个为主会话"）。 */
let forkWatch: { project: string; chapter: string; role: string; prevId: string; since: number } | null = null

export function watchFork(project: string, chapter: string, role: string, prevId: string): void {
  forkWatch = { project, chapter, role, prevId, since: Date.now() - 60_000 }
}

function settleForkWatch(): void {
  const w = forkWatch
  if (!w) return
  const want = (s: any) =>
    s && s.project === w.project &&
    (w.chapter ? s.chapterId === w.chapter : s.stage === 'vector') &&
    (w.chapter ? s.chapterRole === w.role : true) &&
    s.id !== w.prevId
  const hit = (state.sessions || []).filter(want).sort((a: any, b: any) => String(b.mtime).localeCompare(String(a.mtime)))[0]
  if (!hit) return
  if (Date.parse(hit.mtime) < w.since && hit.id === w.prevId) return
  forkWatch = null
  selectSession(hit.id)
}

export function applyIndex(payload: any): void {
  state.sessions = payload.sessions || []
  state.dedupeReviews = payload.dedupeReviews || []
  state.retries = payload.retries || {}
  state.retryRoles = payload.retryRoles || {}
  state.root = payload.root || state.root
  state.generated = payload.generated || state.generated
  // Vue 的响应式列表会做 keyed 更新：签名没变时算出的 groups 不变，DOM 不动；
  // 这里仍保留签名（旧页字段），给需要显式判重的调用方用。
  state.listSig = listSigOf()
  settleForkWatch()
}

function listSigOf(): string {
  const parts: (string | number)[] = [state.filter, state.sessions.length]
  state.sessions.forEach((s: any) => {
    parts.push([s.id, s.messages,
      s.cost ? s.cost.total + (s.cost.currency || '') : '',
      s.stats ? s.stats.requests : 0,
      s.projectStages ? JSON.stringify(s.projectStages) : ''].join('~'))
  })
  return parts.join('|')
}

export function refreshIndex(): Promise<void> {
  return fetch('/api/index', { cache: 'no-store' })
    .then((res) => {
      if (!res.ok) throw new Error('HTTP ' + res.status)
      return res.json()
    })
    .then((payload) => {
      applyIndex(payload)
      state.polling = true
      const current = findSession(state.current ? state.current.id : '')
      if (!current) return
      state.current = current
      const mtime = new Date(current.mtime).getTime()
      const changed = current.size !== state.curSize || mtime !== state.curMtime
      if (changed && state.curSize >= 0) {
        state.curMtime = mtime
        return pullSession(false)
      }
      state.curSize = current.size
      state.curMtime = mtime
    })
    .catch(() => {
      state.polling = false
    })
}

/* 增量拉取：reset=true 从头拉（选中新会话）；false 从 nextFrom 续。 */
// T107：增量拉取在飞锁——响应慢于轮询节拍（网络挂载上 ReadSession 全量
// 读一遍，>2s 很常见）时两个 pull 以同一 from 先后发出，先发的先落地、
// 后发的 seq 仍是最大值不被丢弃，同一段行追加两次（真实缺陷：工具输出
// 整段翻倍）。reset（切会话）不受锁约束，靠 pullSeq 丢弃旧结果。
let pullInFlight = false

export function pullSession(reset: boolean): Promise<void> {
  const s = state.current
  if (!s) return Promise.resolve()
  if (!reset && pullInFlight) return Promise.resolve()
  pullInFlight = true
  const from = reset ? 0 : state.nextFrom
  const url = '/api/session?id=' + encodeURIComponent(s.id) + '&from=' + from
  const seq = ++pullSeq
  return fetch(url, { cache: 'no-store' })
    .then((res) => {
      if (!res.ok) throw new Error('HTTP ' + res.status)
      return res.json()
    })
    .then((payload) => {
      if (seq !== pullSeq) return // 已切到别的会话，丢弃这次结果
      if (!reset && payload.nextFrom < state.nextFrom) {
        // 转录被替换（章节重跑过），手里的行已不属于盘上这份文件。
        state.lines = []
        state.nextFrom = 0
        return pullSession(true)
      }
      state.nextFrom = payload.nextFrom
      state.curSize = payload.size
      // P7：流式快照跟随当前会话——消息完整落盘后服务端不再返回 partial，
      // 这里同步置空，快照卡片自然消失。
      state.partial = (payload.partial as PartialInfo | undefined) || null
      if (reset) {
        state.lines = normalizeLines(payload.lines)
        indexCallEstimates(state.lines)
        // 滚动与重渲由 Timeline.vue 的 watch（会话 id / 行数）接管
      } else {
        const fresh = normalizeLines(payload.lines)
        indexCallEstimates(fresh)
        fresh.forEach((line) => { state.lines.push(line) })
      }
    })
    .catch((err: Error) => {
      setBannerText('拉取会话失败：' + err.message)
    })
    .finally(() => {
      pullInFlight = false
    })
}

export function selectSession(id: string): void {
  const s = findSession(id)
  state.current = s
  if (staticSessions && staticSessions[id]) {
    // 静态快照：行已在手，不走 /api。
    const fresh = normalizeLines(staticSessions[id].lines as never[])
    indexCallEstimates(fresh)
    state.lines = fresh
    state.nextFrom = 0
    return
  }
  state.lines = []
  state.partial = null
  state.callEst = {}
  state.nextFrom = 0
  state.curSize = -1
  state.curMtime = 0
  state.meta = null
  state.trajOpen = {}
  // 高亮与消息流重建都走响应式（Timeline computed）；不重建侧栏
  // （展开状态与滚动位置不被打断）。滚动由 Timeline 的 watch 接管。
  if (!s) return
  void pullSession(true)
}

/*
 * 静态快照：页面带 #dsh-data 块时整个数据层离线——会话行已在 JSON 里，
 * mediaRoot 前缀拼图，不拉 /api、不轮询。live 分支与它互斥。
 */
let staticSessions: Record<string, { lines: unknown[] }> | null = null

function bootStatic(): boolean {
  const el = document.getElementById('dsh-data')
  if (!el || !el.textContent) return false
  let payload: any
  try {
    payload = JSON.parse(el.textContent)
  } catch {
    return false
  }
  state.staticMode = true
  state.mediaRoot = String(payload.mediaRoot || '')
  state.root = payload.root || ''
  state.generated = payload.generated || ''
  state.sessions = payload.sessions || []
  // 静态快照没有轮询：badge 语义保持「不在线」，快照身份用 banner 声明。
  state.polling = false
  setBannerText('静态快照 · 生成于 ' + (payload.generated || '未知时间') + '（不更新；重跑 sessions 重新导出）')
  staticSessions = {}
  state.sessions.forEach((s: any) => {
    staticSessions![s.id] = { lines: s.lines || [] }
  })
  let live: any = null
  state.sessions.forEach((s: any) => {
    if (!live && s.live) live = s
  })
  if (state.sessions.length) selectSession((live || state.sessions[0]).id)
  return true
}

/*
 * P18：img2text 进度概览（/api/img2text-progress）。preview.img2text 未开时
 * books 为空/ null；失败静默（概览区只是附属信息，不打横幅）。轮询节奏跟随
 * App.vue 的独立 ~5s 定时器，静态快照模式不拉。
 */
export function refreshImg2TextProgress(): Promise<void> {
  return fetch('/api/img2text-progress', { cache: 'no-store' })
    .then((res) => {
      if (!res.ok) throw new Error('HTTP ' + res.status)
      return res.json()
    })
    .then((payload) => {
      state.img2textProgress = (payload && payload.books) || []
    })
    .catch(() => { /* 静默：保持上一次数据 */ })
}

/* boot：静态快照直接装数据；live 先拉一次索引并选中第一个活跃会话。
 * 轮询定时器只在 App.vue 注册一份（pollTimer，页面隐藏时跳过、可随卸载清理）——
 * 这里不再自建 interval：曾经两份并存，live 页每秒打 4 次 /api/index 全树扫描，
 * 运行刚启动（samba 冷缓存）时请求堆积，页面迟迟拿不到首份数据。 */
export function bootData(): void {
  if (bootStatic()) return
  void refreshImg2TextProgress()
  void refreshIndex().then(() => {
    if (state.sessions.length && !state.current) {
      let live: any = null
      state.sessions.forEach((s: any) => {
        if (!live && s.live) live = s
      })
      selectSession((live || state.sessions[0]).id)
    }
  })
}

// requestChapterRetry 是页面上「重试这一章」的落点（P53）：只往项目的
// work/retry/ 排一个请求文件，不执行任何命令。正在跑的 docvision latex 会在
// 转换阶段的安全点消费它；没在跑就留到下次启动。
/* role：convert（重转，开新尝试）/ checker（只重新核对）/ continue（接着干）。
   from > 0 = 从这条记录之后继续：历史截到这一行，后面丢掉，开一个新尝试。 */
export function requestChapterRetry(
  project: string,
  chapter: string,
  role = 'convert',
  reason = '',
  extra: { from?: number; path?: string } = {},
): Promise<any> {
  return fetch('/api/retry', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project, chapter, role, reason, ...extra }),
  }).then(async (res) => {
    const text = await res.text()
    if (!res.ok) throw new Error(text.trim() || 'HTTP ' + res.status)
    return text ? JSON.parse(text) : {}
  })
}

// chapterPendingRetry 报告某项目的某章是否已经在重试队列里（侧栏/详情栏据此
// 改文案，避免用户连点）。
/* 删掉一条"转错的记录"（P2）：只删转录，作废尝试的证据目录由服务端一起清。 */
// truncateSession 就地截断一条转录到第 line 行（含），返回 null 表示成功；
// 服务端会先把整份原文件备份到 work/sessions/.trash/ 下。
export function truncateSession(project: string, path: string, line: number): Promise<any> {
  return fetch('/api/truncate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project, path, line }),
  }).then(async (res) => {
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error(data.error || 'HTTP ' + res.status)
    return data
  })
}

export function dropSession(project: string, path: string): Promise<any> {
  return fetch('/api/drop', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project, path }),
  }).then(async (res) => {
    const data = await res.json().catch(() => ({}))
    if (!res.ok) throw new Error((data && data.error) || 'HTTP ' + res.status)
    void refreshIndex()
    return data
  })
}

/* cancelChapterRetry 撤销一条还没被消费的排队请求（P56）：误触的出口。返回
   撤掉了几条——0 表示 latex 已经把它消费了，这时只能去「历史」里删那条新会话。 */
export function cancelChapterRetry(project: string, chapter: string, role = 'convert', path = ''): Promise<any> {
  return fetch('/api/retry/cancel', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project, chapter, role, path }),
  }).then(async (res) => {
    const text = await res.text()
    if (!res.ok) throw new Error(text.trim() || 'HTTP ' + res.status)
    const data = text ? JSON.parse(text) : {}
    void refreshIndex()
    return data
  })
}

export function chapterPendingRetry(project: string, chapter: string, role = ''): boolean {
  const list = (state.retries || {})[project] || []
  if (list.indexOf(chapter) < 0) return false
  if (!role) return true
  const roles = (state.retryRoles || {})[project] || {}
  const queued = roles[chapter] || 'convert'
  return queued === role
}
