<script setup lang="ts">
// 中栏顶栏右侧的动作按钮（P56，T193 挪到顶栏）：重试 / 重新核对 / 继续跑 /
// 重画这张图 / 删除这条记录 / 撤销排队请求。
//
// 状态与动作在 `src/actions.ts`（与顶栏下面的确认条 `SessionActionPanel.vue`
// 共用同一份 pending / queued）。这里只有按钮本身——它现在紧挨着「N 条消息 ·
// 输入…」那段指标，不单独占一行。
import { computed, ref } from 'vue'
import { actions } from '../actions'
import { state, setRetryNote } from '../state'
import { requestFigureOverride } from '../data'

const { target, canContinue, canRedraw, canDrop, hasAny, retryPending, continuePending, continueLabel, retryLabel, queued, busy, ask, undoAsk, backToPrev } = actions

// 逐图提交门槛（T307/P78）：只在看一条作图会话时出现。值按百分比填（17 = 17%），
// 0 = 回到全局默认，留空不提交。接受"尽力还原"要一句 note（与 submit_figure 的
// accept_as_is 同一义），撤销也是这里。
const threshold = ref('')
const noteOpen = ref(false)
const note = ref('')
const saving = ref(false)
const vectorPath = computed(() => {
  const cur = state.current as any
  if (!cur) return ''
  const name = String(cur.name || '')
  return canRedraw.value && name.indexOf('vector_') === 0 ? name : ''
})

async function saveThreshold() {
  if (!vectorPath.value || saving.value) return
  const raw = threshold.value.trim()
  const pct = raw === '' ? NaN : Number(raw)
  if (!Number.isFinite(pct) || pct < 0 || pct > 100) {
    setRetryNote('阈值要填 0-100 之间的百分比（0 = 用全局默认）')
    return
  }
  saving.value = true
  try {
    const res = await requestFigureOverride(String((state.current as any).project || ''), { path: vectorPath.value }, { minOverlap: pct / 100 })
    setRetryNote(`这张图的阈值已设为 ${pct === 0 ? '全局默认' : pct + '%'}：${(res && res.note) || ''}`)
    threshold.value = ''
  } catch (e: any) {
    setRetryNote('保存阈值失败：' + (e && e.message ? e.message : e))
  } finally {
    saving.value = false
  }
}

async function acceptAsIs() {
  if (!vectorPath.value || saving.value) return
  const text = note.value.trim()
  if (!text) {
    setRetryNote('接受"尽力还原"要写一句差在哪里、为什么到不了')
    return
  }
  saving.value = true
  try {
    await requestFigureOverride(String((state.current as any).project || ''), { path: vectorPath.value }, { acceptAsIs: true, note: text })
    setRetryNote('已按"尽力还原"接受：这张图之后不会再自动重画，只有你点「重画这张图」才动它')
    noteOpen.value = false
    note.value = ''
  } catch (e: any) {
    setRetryNote('接受失败：' + (e && e.message ? e.message : e))
  } finally {
    saving.value = false
  }
}

async function clearAccept() {
  if (!vectorPath.value || saving.value) return
  saving.value = true
  try {
    await requestFigureOverride(String((state.current as any).project || ''), { path: vectorPath.value }, { acceptAsIs: false })
    setRetryNote('已撤销"尽力还原"接受：这张图回到会被自动重画的状态')
  } catch (e: any) {
    setRetryNote('撤销失败：' + (e && e.message ? e.message : e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-if="hasAny" id="session-actions" class="session-actions" role="group" aria-label="这条会话的动作">
    <template v-if="!actions.pending.value">
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
        title="接着这一章最近一次尝试继续跑：不新建尝试、不归档、不动产物（站在递归子章节会话上时，跑的是它所在的父章节）"
        @click="ask('continue')"
      >{{ continueLabel }}</button>
      <button
        v-if="canRedraw"
        class="act-btn"
        type="button"
        :disabled="busy"
        title="只重做这张图：开一条新作图尝试，类别与分组不变；下游阶段要重跑才能看到新图"
        @click="ask('redraw')"
      >重画这张图</button>
      <template v-if="vectorPath">
        <input
          v-model="threshold"
          class="act-num"
          type="number"
          min="0"
          max="100"
          step="1"
          :disabled="saving"
          :placeholder="'阈值%'"
          title="这张图的提交门槛（软墨重叠百分比）：0 = 用 latex.figure_check 的全局值；填了保存后，下次提交/重画按新值判定"
          @keyup.enter="saveThreshold"
        />
        <button class="act-btn" type="button" :disabled="saving" title="保存这张图的提交阈值（只改这一张）" @click="saveThreshold">存阈值</button>
        <template v-if="noteOpen">
          <input
            v-model="note"
            class="act-note"
            type="text"
            :disabled="saving"
            placeholder="差在哪里、为什么到不了（必填）"
            @keyup.enter="acceptAsIs"
          />
          <button class="act-btn act-primary" type="button" :disabled="saving" @click="acceptAsIs">确认接受</button>
          <button class="act-btn act-quiet" type="button" :disabled="saving" @click="noteOpen = false">取消</button>
        </template>
        <template v-else>
          <button class="act-btn" type="button" :disabled="saving" title="达不到阈值但已经尽力：接受后这张图不再自动重画（粘性），只有「重画这张图」才动它" @click="noteOpen = true">尽力还原</button>
          <button class="act-btn act-quiet" type="button" :disabled="saving" title="撤销之前的「尽力还原」接受" @click="clearAccept">撤销接受</button>
        </template>
      </template>
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
  </div>
</template>

<style scoped>
.session-actions {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.act-num {
  width: 5.2em;
  flex: 0 0 auto;
  padding: 2px 4px;
  border: 1px solid var(--line);
  border-radius: 4px;
  background: var(--btn-bg, transparent);
  color: inherit;
  font: inherit;
  font-size: 12px;
}

.act-note {
  width: 12em;
  flex: 0 0 auto;
  padding: 2px 4px;
  border: 1px solid var(--line);
  border-radius: 4px;
  background: var(--btn-bg, transparent);
  color: inherit;
  font: inherit;
  font-size: 12px;
}

.act-btn {
  flex: 0 0 auto;
  white-space: nowrap;
  border: 1px solid var(--line);
  background: var(--btn-bg, transparent);
  color: var(--fg);
  border-radius: 6px;
  padding: 3px 10px;
  font-size: 12px;
  line-height: 20px;
  cursor: pointer;
}

.act-btn:disabled { opacity: 0.55; cursor: default; }
.act-primary { border-color: color-mix(in srgb, var(--accent, #3b82f6) 55%, var(--line)); color: var(--accent, #3b82f6); }
.act-danger { border-color: color-mix(in srgb, #dc2626 50%, var(--line)); color: #dc2626; }
.act-undo { border-color: color-mix(in srgb, #d97706 55%, var(--line)); color: #b45309; }
.act-quiet { opacity: 0.85; }
</style>
