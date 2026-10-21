<script setup lang="ts">
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
import { state } from '../state'
import { cancelChapterRetry, chapterPendingRetry, dropSession, refreshIndex, requestChapterRetry, selectSession } from '../data'
import { canContinueOf, canDropOf, canRedrawOf, confirmTextOf, retryTargetOf } from '../legacy/actions'

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
const continuePending = computed(() => {
  const t = target.value
  return !!t && chapterPendingRetry(t.project, t.chapter, 'continue')
})
const retryLabel = computed(() => {
  const t = target.value
  if (!t) return ''
  if (retryPending.value) return t.role === 'checker' ? '已在核对队列' : '已在重试队列'
  return t.role === 'checker' ? '重新核对这一章' : '重试这一章'
})
const confirmBody = computed(() => (pending.value ? confirmTextOf(pending.value.kind as any, state.current) : ''))

/* 换会话就收起确认条与撤销条：它们的对象是上一章的请求，留着会张冠李戴。 */
watch(
  () => state.current && (state.current as any).id,
  () => {
    pending.value = null
    queued.value = null
    state.retryNote = ''
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
  const t = target.value
  if (!t) { busy.value = false; return }
  const role = p.kind === 'continue' ? 'continue' : t.role
  const what = p.kind === 'continue' ? '继续跑' : (t.role === 'checker' ? '重新核对' : '重试')
  state.retryNote = '正在提交 ' + t.chapter + ' 的' + what + '请求…'
  requestChapterRetry(t.project, t.chapter, role, '中栏' + what)
    .then((res: any) => {
      queued.value = { project: t.project, chapter: t.chapter, role, path: '', prevId: prevId.value }
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
</script>

<template>
  <div v-if="hasAny" id="session-actions" class="session-actions" role="group" aria-label="这条会话的动作">
    <template v-if="!pending">
      <button
        v-if="target"
        class="act-btn act-primary"
        type="button"
        :disabled="retryPending || busy"
        :title="target.role === 'checker' ? '只重跑核对会话：产物不动，旧结论留档' : '重跑这一章：开一个新尝试，旧的整棵留档'"
        @click="ask('retry')"
      >{{ retryLabel }}</button>
      <button
        v-if="canContinue"
        class="act-btn"
        type="button"
        :disabled="continuePending || busy"
        title="接着这一章最近一次尝试继续跑：不新建尝试、不归档、不动产物"
        @click="ask('continue')"
      >{{ continuePending ? '已在继续跑队列' : '继续跑' }}</button>
      <button
        v-if="canRedraw"
        class="act-btn"
        type="button"
        :disabled="busy"
        title="只重做这张图：开一条新作图尝试，类别与分组不变；下游阶段要重跑才能看到新图"
        @click="ask('redraw')"
      >重画这张图</button>
      <button
        v-if="canDrop"
        class="act-btn act-danger"
        type="button"
        :disabled="busy"
        title="删掉这条转录（作废尝试的 work/attempts 归档一并删除）；正在写入的会话不给删"
        @click="ask('drop')"
      >删除这条记录</button>
      <button
        v-if="queued"
        class="act-btn act-undo"
        type="button"
        :disabled="busy"
        title="把刚排进 work/retry/ 的请求删掉；已经被 latex 消费时给出替代动作"
        @click="undoAsk"
      >撤销这个请求</button>
      <button
        v-if="queued && queued.prevId"
        class="act-btn act-quiet"
        type="button"
        title="回到按下重试之前看的那条会话"
        @click="backToPrev(queued.prevId)"
      >回到之前那条</button>
    </template>
    <template v-else>
      <span class="act-confirm-text">{{ confirmBody }}</span>
      <button class="act-btn act-primary" type="button" :disabled="busy" @click="pending.kind === 'cancel' ? doCancel() : run()">确认执行</button>
      <button class="act-btn act-quiet" type="button" :disabled="busy" @click="cancelAsk">取消</button>
    </template>
    <span v-if="state.retryNote" class="act-note">{{ state.retryNote }}</span>
  </div>
</template>

<style scoped>
.session-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 8px 16px 0 20px;
}

.act-btn {
  border: 1px solid var(--line);
  background: var(--btn-bg, transparent);
  color: var(--fg);
  border-radius: 6px;
  padding: 3px 10px;
  font-size: 12px;
  line-height: 20px;
  cursor: pointer;
}

.act-btn:disabled {
  opacity: 0.55;
  cursor: default;
}

.act-primary {
  border-color: color-mix(in srgb, var(--accent, #3b82f6) 55%, var(--line));
  color: var(--accent, #3b82f6);
}

.act-danger {
  border-color: color-mix(in srgb, #dc2626 50%, var(--line));
  color: #dc2626;
}

.act-undo {
  border-color: color-mix(in srgb, #d97706 55%, var(--line));
  color: #b45309;
}

.act-quiet {
  opacity: 0.85;
}

.act-confirm-text {
  font-size: 12px;
  color: var(--caption);
  line-height: 18px;
}

.act-note {
  font-size: 12px;
  color: var(--caption);
}
</style>
