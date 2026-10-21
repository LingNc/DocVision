// 中栏动作条的**状态与动作**（T193 从 SessionActions.vue 抽出来）。
//
// 原来按钮、确认条、回执都在同一个组件里、整块排在面包屑下面。用户要求把按钮
// 挪到顶栏右侧（紧挨着「N 条消息」那段指标），确认条与回执仍留在顶栏下面——
// 两处要共享同一份待确认状态，所以状态放模块级，`SessionActions.vue`（顶栏按钮）
// 与 `SessionActionPanel.vue`（确认条 / 回执）各自引用它。
// 中栏的动作条（P56）：重试 / 重新核对 / 继续跑 / 重画这张图 / 删除这条记录。
//
// 为什么放在这里：上一版这些按钮在右侧详情栏（还有一章一个的侧栏悬浮 ↻），用户
// 的原话是"不要放在侧边栏里面，重试会话这个应该放在中间的界面中"。按钮跟着
// **正在看的这条会话**走，就在视线上方，不用先找。
//
// 三件必须做到的事：
// ① **二次确认**用页面内的确认条，不用 window.confirm——原生弹窗既不好看也没法
//    把"会开新尝试 / 旧产物归档去哪"写清楚；
// ② **误触可恢复**：请求只是排进 work/retry/ 的文件，撤销就是把它删掉（服务端
//    /api/retry/cancel）；同时给「回到刚才那条会话」，把选中项切回去；
// ③ 已经开跑、撤销不到的时候，给「删除这条记录」这条出口（服务端 /api/drop）。
import { computed, ref, watch } from 'vue'
import { state } from './state'
import { cancelChapterRetry, chapterPendingRetry, dropSession, refreshIndex, requestChapterRetry, selectSession, watchFork } from './data'
import { canContinueOf, canDropOf, canRedrawOf, confirmTextOf, continueTargetOf, retryTargetOf } from './legacy/actions'

type Pending =
  | { kind: 'retry' | 'continue' | 'redraw' | 'drop' }
  | { kind: 'cancel'; project: string; chapter: string; role: string; path: string; prevId: string }

const pending = ref<Pending | null>(null)
/* 刚排上的那条请求：用于「撤销这个请求」（只对还没被消费的有效）。 */
const queued = ref<{ project: string; chapter: string; role: string; path: string; prevId: string } | null>(null)
const prevId = ref('')
const busy = ref(false)

const target = computed(() => retryTargetOf(state.current))
const canContinue = computed(() => canContinueOf(state.current))
const canRedraw = computed(() => canRedrawOf(state.current))
const canDrop = computed(() => canDropOf(state.current))
const hasAny = computed(() => !!target.value || canRedraw.value || canDrop.value || !!queued.value)

const retryPending = computed(() => {
  const t = target.value
  return !!t && chapterPendingRetry(t.project, t.chapter, t.role)
})
// T196：「继续跑」的对象可能是父章节（站在递归子章节会话上时），所以这条
// pending 判定不能复用 target。
const continueTarget = computed(() => continueTargetOf(state.current))
const continuePending = computed(() => {
  const t = continueTarget.value
  return !!t && chapterPendingRetry(t.project, t.chapter, 'continue')
})
const continueLabel = computed(() => {
  const t = continueTarget.value
  const child = !!(t && state.current && String((state.current as any).chapterId || '').indexOf('_child_') >= 0)
  if (continuePending.value) return child ? '父章节已在继续跑队列' : '已在继续跑队列'
  return child ? '继续跑父章节' : '继续跑'
})
const retryLabel = computed(() => {
  const t = target.value
  if (!t) return ''
  if (retryPending.value) return t.role === 'checker' ? '已在核对队列' : '已在重试队列'
  return t.role === 'checker' ? '重新核对这一章' : '重试这一章'
})
const confirmBody = computed(() => (pending.value ? confirmTextOf(pending.value.kind as any, state.current) : ''))

/* 换会话就收起确认条与撤销条：它们的对象是上一章的请求，留着会张冠李戴。
 * T194 例外：排队成功后视图会自动切到**这次开出来的新尝试**（同项目同章节同
 * 角色），那一跳不该把「撤销这个请求」一起收走——否则刚排上就没法撤销了。 */
watch(
  () => state.current && (state.current as any).id,
  () => {
    pending.value = null
    const cur: any = state.current
    const q = queued.value
    const same = !!q && !!cur &&
      String(cur.project || '') === q.project &&
      String(cur.chapterId || '') === q.chapter &&
      (q.role === 'vector' || String(cur.chapterRole || '') === q.role)
    if (!same) {
      queued.value = null
      state.retryNote = ''
    }
  },
)

function ask(kind: 'retry' | 'continue' | 'redraw' | 'drop') {
  if (busy.value) return
  prevId.value = (state.current && (state.current as any).id) || ''
  pending.value = { kind }
}

function cancelAsk() {
  pending.value = null
}

function run() {
  const p = pending.value
  const s: any = state.current
  if (!p || busy.value) return
  pending.value = null
  busy.value = true
  const done = (msg: string) => { state.retryNote = msg; busy.value = false; return refreshIndex().catch(() => undefined) }
  const fail = (err: any) => { state.retryNote = '失败：' + ((err && err.message) || err); busy.value = false }
  if (p.kind === 'drop') {
    if (!s) { busy.value = false; return }
    state.retryNote = '正在删除 ' + s.name + '…'
    dropSession(String(s.project), String(s.path))
      .then((res: any) => { state.retryNote = '已删除 ' + s.name + ((res && res.archive) ? '（含 ' + res.archive + '）' : ''); busy.value = false })
      .catch(fail)
    return
  }
  if (p.kind === 'redraw') {
    if (!s) { busy.value = false; return }
    state.retryNote = '正在提交重画请求…'
    requestChapterRetry(String(s.project), '', 'vector', '图不对，重画', { path: String(s.name) })
      .then((res: any) => {
        queued.value = { project: String(s.project), chapter: '', role: 'vector', path: String(s.name), prevId: prevId.value }
        done('已排队重画：' + ((res && res.note) || '等待并发槽位') + '（可以撤销）')
      })
      .catch(fail)
    return
  }
  // 继续跑的目标可能与 target（重试目标）不同：站着递归子章节时是它的父章节。
  const t = p.kind === 'continue' ? continueTarget.value : target.value
  if (!t) { busy.value = false; return }
  const role = p.kind === 'continue' ? 'continue' : t.role
  const child = String((s && s.chapterId) || '').indexOf('_child_') >= 0
  const what = p.kind === 'continue'
    ? (child ? '继续跑父章节' : '继续跑')
    : (t.role === 'checker' ? '重新核对' : '重试')
  state.retryNote = '正在提交 ' + t.chapter + ' 的' + what + '请求…'
  requestChapterRetry(t.project, t.chapter, role, '中栏' + what)
    .then((res: any) => {
      queued.value = { project: t.project, chapter: t.chapter, role, path: '', prevId: prevId.value }
      // T194：新尝试一出现就切过去（用户口径："这俩是一体的吧，如果切换回去就
      // 都切换过去"）。重核对也一样——它开出的核对会话才是有结论的那条。
      if (role !== 'continue') watchFork(t.project, t.chapter, role, prevId.value)
      done(t.chapter + ' ' + what + '请求已排队（' + ((res && res.note) || '等待处理') + '）（可以撤销）')
    })
    .catch(fail)
}

/* 撤销：确认条 → 服务端删队列文件 → 顺手把选中项切回刚才那条会话。 */
function undoAsk() {
  const q = queued.value
  if (!q || busy.value) return
  pending.value = { kind: 'cancel', project: q.project, chapter: q.chapter, role: q.role, path: q.path, prevId: q.prevId }
}

function backToPrev(id: string) {
  if (!id) return
  selectSession(id)
}

function doCancel() {
  const p = pending.value
  if (!p || p.kind !== 'cancel' || busy.value) return
  pending.value = null
  busy.value = true
  cancelChapterRetry(p.project, p.chapter, p.role, p.path)
    .then((res: any) => {
      const n = Number((res && res.removed) || 0)
      if (n > 0) {
        state.retryNote = '已撤销排队请求（' + n + ' 条）'
        queued.value = null
        backToPrev(p.prevId)
      } else {
        // 队列里已经没有了：latex 消费掉了，这一次重试已经（或正在）开跑。
        state.retryNote = '这个请求已经被 latex 消费，撤不回来了——它开出的那条新会话可以在「历史」里删掉。'
        queued.value = null
        return refreshIndex().catch(() => undefined)
      }
      busy.value = false
    })
    .catch((err: any) => { state.retryNote = '撤销失败：' + ((err && err.message) || err); busy.value = false })
}

export const actions = {
  pending, queued, prevId, busy,
  target, canContinue, canRedraw, canDrop, hasAny,
  retryPending, continuePending, continueLabel, retryLabel, confirmBody,
  ask, cancelAsk, run, undoAsk, backToPrev, doCancel,
}
