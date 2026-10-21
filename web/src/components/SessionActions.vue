<script setup lang="ts">
// 中栏顶栏右侧的动作按钮（P56，T193 挪到顶栏）：重试 / 重新核对 / 继续跑 /
// 重画这张图 / 删除这条记录 / 撤销排队请求。
//
// 状态与动作在 `src/actions.ts`（与顶栏下面的确认条 `SessionActionPanel.vue`
// 共用同一份 pending / queued）。这里只有按钮本身——它现在紧挨着「N 条消息 ·
// 输入…」那段指标，不单独占一行。
import { actions } from '../actions'

const { target, canContinue, canRedraw, canDrop, hasAny, retryPending, continuePending, continueLabel, retryLabel, queued, busy, ask, undoAsk, backToPrev } = actions
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
