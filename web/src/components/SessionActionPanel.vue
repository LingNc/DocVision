<script setup lang="ts">
// 顶栏下面的确认条与回执（T193）。按钮在顶栏右侧（`SessionActions.vue`），
// 点下去之后"会开新尝试 / 产物归档去哪 / 撤销排队请求"这些必须写清楚的话
// 放在这里——顶栏放不下，也不该把它撑成两行。状态共享 `src/actions.ts`。
import { state } from '../state'
import { actions } from '../actions'

const { pending, busy, confirmBody, cancelAsk, run, doCancel } = actions
</script>

<template>
  <div v-if="pending || state.retryNote" class="session-action-panel">
    <template v-if="pending">
      <span class="act-confirm-text">{{ confirmBody }}</span>
      <button class="act-btn act-primary" type="button" :disabled="busy" @click="pending.kind === 'cancel' ? doCancel() : run()">确认执行</button>
      <button class="act-btn act-quiet" type="button" :disabled="busy" @click="cancelAsk">取消</button>
    </template>
    <span v-if="state.retryNote" class="act-note">{{ state.retryNote }}</span>
  </div>
</template>

<style scoped>
.session-action-panel {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 6px 16px 6px 20px;
  border-bottom: 1px solid var(--line);
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
.act-quiet { opacity: 0.85; }
.act-confirm-text { font-size: 12px; color: var(--caption); line-height: 18px; }
.act-note { font-size: 12px; color: var(--caption); }
</style>
