/*
 * 数据层：/api/index 轮询（2 秒、带列表签名守卫）与 /api/session 增量拉取。
 * 旧页 refreshIndex/applyIndex/selectSession/pullSession 的移植；渲染由
 * Timeline.vue / DetailsPanel.vue 的 computed 响应式接管，这里只管数据流。
 */
import { state, POLL_MS, setBannerText } from './state'
import { findSession, normalizeLines, indexCallEstimates, groupKey, setCollapsed } from './legacy/sidebar'

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

export function applyIndex(payload: any): void {
  state.sessions = payload.sessions || []
  state.root = payload.root || state.root
  state.generated = payload.generated || state.generated
  // Vue 的响应式列表会做 keyed 更新：签名没变时算出的 groups 不变，DOM 不动；
  // 这里仍保留签名（旧页字段），给需要显式判重的调用方用。
  state.listSig = listSigOf()
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
export function pullSession(reset: boolean): Promise<void> {
  const s = state.current
  if (!s) return Promise.resolve()
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
}

export function selectSession(id: string): void {
  const s = findSession(id)
  state.current = s
  state.lines = []
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

/* boot：先拉一次索引并选中第一个活跃会话；然后 2 秒轮询（页面隐藏时跳过）。 */
export function bootData(): void {
  void refreshIndex().then(() => {
    if (state.sessions.length && !state.current) {
      let live: any = null
      state.sessions.forEach((s: any) => {
        if (!live && s.live) live = s
      })
      selectSession((live || state.sessions[0]).id)
    }
  })
  window.setInterval(() => {
    if (!document.hidden) void refreshIndex()
  }, POLL_MS)
}
