<script setup lang="ts">
// 「历史」页：当前会话所在 img2text 调用链（成员数 > 1）的全部成员列表。
// 数据来自纯函数 chainOfSession（legacy/sidebar.ts），computed 按
// state.sessions / state.current 引用缓存，轮询换新数组时才重扫一遍。
// 排序与侧栏链一致（sortChainMembers：新 → 旧）；点行切会话（selectSession）。
import { computed } from 'vue'
import { state, relTime } from '../state'
import { chainOfSession, img2textMemberTag, sessionTitleOf } from '../legacy/sidebar'
import { selectSession } from '../data'
import InlineMD from './InlineMD.vue'

const members = computed(() => chainOfSession(state.sessions, state.current ? (state.current as any).id : null))

function endStateText(s: any): string {
  if (s.live) return '运行中'
  if (s.endState === 'done') return '已完成'
  if (s.endState === 'error') return '错误'
  return '未完成'
}

function endStateCls(s: any): string {
  if (s.live) return 'live'
  if (s.endState === 'done') return 'done'
  if (s.endState === 'error') return 'error'
  return 'pend'
}

function onRowClick(s: any) {
  selectSession(s.id)
}
</script>

<template>
  <div id="history" class="history" :class="{ hidden: state.view !== 'history' }">
    <div v-if="!members" class="history-empty">当前会话不属于任何 img2text 调用链</div>
    <div v-else class="history-list">
      <div class="history-head">这张图的全部会话（{{ members.length }}，新 → 旧）</div>
      <button
        v-for="s in members"
        :key="s.id"
        class="history-row"
        :class="{ active: state.current && state.current.id === s.id }"
        type="button"
        :title="s.id"
        @click="onRowClick(s)"
      >
        <span class="row-chip chain-tag history-tag">{{ img2textMemberTag(s.id) }}</span>
        <InlineMD tag="span" class="history-title" :text="sessionTitleOf(s)"></InlineMD>
        <span class="history-state" :class="endStateCls(s)">{{ endStateText(s) }}</span>
        <span class="history-time">{{ relTime(s.mtime) }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.history {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 12px 28px 20px 20px;
}

.history.hidden {
  display: none;
}

.history-empty {
  padding: 16px 4px;
  color: var(--caption);
  font-size: 13px;
}

.history-head {
  padding: 0 4px 8px;
  color: var(--caption);
  font-size: 12px;
}

.history-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition: background .12s;
}

.history-row:hover {
  background: var(--hover);
}

.history-row.active {
  background: var(--active);
}

.history-tag {
  flex: none;
  color: var(--caption);
  font-size: 11px;
  white-space: nowrap;
  border: .5px solid var(--border);
  border-radius: 999px;
  padding: 0 6px;
  line-height: 16px;
}

.history-title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  line-height: 20px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.history-row.active .history-title {
  font-weight: 500;
}

.history-state {
  flex: none;
  font-size: 12px;
  line-height: 20px;
  color: var(--caption);
}

.history-state.live {
  color: var(--ok);
}

.history-state.done {
  color: var(--ok);
}

.history-state.error {
  color: var(--err);
}

.history-time {
  flex: none;
  color: var(--caption);
  font-size: 12px;
  line-height: 20px;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 1023px) {
  .history { padding: 10px 16px 16px 12px; }
}
</style>
